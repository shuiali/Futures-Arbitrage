//! Bybit Futures adapter

use anyhow::{Context, Result};
use async_trait::async_trait;
use hmac::{Hmac, Mac};
use reqwest::Client;
use rust_decimal::Decimal;
use serde::Deserialize;
use sha2::Sha256;
use std::time::{SystemTime, UNIX_EPOCH};
use tracing::{debug, info};

use super::{Credentials, ExchangeAdapter, OrderRequest, OrderResponse, OrderStatus, OrderType, Side};
use crate::config::ExchangeConfig;

type HmacSha256 = Hmac<Sha256>;

pub struct BybitAdapter {
    config: ExchangeConfig,
    client: Client,
}

impl BybitAdapter {
    pub async fn new(config: ExchangeConfig) -> Result<Self> {
        let client = Client::builder()
            .timeout(std::time::Duration::from_secs(10))
            .build()?;

        Ok(Self { config, client })
    }

    fn sign(&self, secret: &str, timestamp: u64, api_key: &str, recv_window: u64, query: &str) -> String {
        let sign_str = format!("{}{}{}{}", timestamp, api_key, recv_window, query);
        let mut mac = HmacSha256::new_from_slice(secret.as_bytes())
            .expect("HMAC can take key of any size");
        mac.update(sign_str.as_bytes());
        hex::encode(mac.finalize().into_bytes())
    }

    fn timestamp() -> u64 {
        SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap()
            .as_millis() as u64
    }
}

#[async_trait]
impl ExchangeAdapter for BybitAdapter {
    fn id(&self) -> &str {
        "bybit"
    }

    async fn place_order(
        &self,
        credentials: &Credentials,
        request: &OrderRequest,
    ) -> Result<OrderResponse> {
        let timestamp = Self::timestamp();
        let recv_window = 5000u64;

        let body = serde_json::json!({
            "category": "linear",
            "symbol": request.symbol,
            "side": match request.side {
                Side::Buy => "Buy",
                Side::Sell => "Sell",
            },
            "orderType": match request.order_type {
                OrderType::Limit => "Limit",
                OrderType::Market => "Market",
            },
            "qty": request.quantity.to_string(),
            "price": request.price.map(|p| p.to_string()),
            "timeInForce": "GTC",
            "orderLinkId": request.client_order_id,
            "reduceOnly": request.reduce_only,
        });

        let body_str = serde_json::to_string(&body)?;
        let signature = self.sign(
            &credentials.api_secret,
            timestamp,
            &credentials.api_key,
            recv_window,
            &body_str,
        );

        let url = format!("{}/v5/order/create", self.config.rest_url);
        
        debug!("Placing Bybit order: {}", request.symbol);

        let response = self.client
            .post(&url)
            .header("X-BAPI-API-KEY", &credentials.api_key)
            .header("X-BAPI-SIGN", &signature)
            .header("X-BAPI-TIMESTAMP", timestamp.to_string())
            .header("X-BAPI-RECV-WINDOW", recv_window.to_string())
            .header("Content-Type", "application/json")
            .body(body_str)
            .send()
            .await
            .context("Failed to send order request")?;

        let status = response.status();
        let body = response.text().await?;

        if !status.is_success() {
            anyhow::bail!("Bybit order failed: {} - {}", status, body);
        }

        let resp: BybitResponse<BybitOrderResult> = serde_json::from_str(&body)
            .context("Failed to parse order response")?;

        if resp.ret_code != 0 {
            anyhow::bail!("Bybit error: {} - {}", resp.ret_code, resp.ret_msg);
        }

        let result = resp.result.ok_or_else(|| anyhow::anyhow!("No result in response"))?;

        info!("Bybit order placed: {}", result.order_id);

        Ok(OrderResponse {
            exchange_order_id: result.order_id,
            client_order_id: result.order_link_id,
            symbol: request.symbol.clone(),
            side: request.side,
            order_type: request.order_type,
            price: request.price,
            quantity: request.quantity,
            filled_quantity: Decimal::ZERO,
            avg_fill_price: None,
            status: OrderStatus::Open,
            timestamp: timestamp as i64,
        })
    }

    async fn cancel_order(
        &self,
        credentials: &Credentials,
        symbol: &str,
        order_id: &str,
    ) -> Result<OrderResponse> {
        let timestamp = Self::timestamp();
        let recv_window = 5000u64;

        let body = serde_json::json!({
            "category": "linear",
            "symbol": symbol,
            "orderId": order_id,
        });

        let body_str = serde_json::to_string(&body)?;
        let signature = self.sign(
            &credentials.api_secret,
            timestamp,
            &credentials.api_key,
            recv_window,
            &body_str,
        );

        let url = format!("{}/v5/order/cancel", self.config.rest_url);

        let response = self.client
            .post(&url)
            .header("X-BAPI-API-KEY", &credentials.api_key)
            .header("X-BAPI-SIGN", &signature)
            .header("X-BAPI-TIMESTAMP", timestamp.to_string())
            .header("X-BAPI-RECV-WINDOW", recv_window.to_string())
            .header("Content-Type", "application/json")
            .body(body_str)
            .send()
            .await?;

        let body = response.text().await?;
        let resp: BybitResponse<BybitOrderResult> = serde_json::from_str(&body)?;

        let result = resp.result.ok_or_else(|| anyhow::anyhow!("No result"))?;

        Ok(OrderResponse {
            exchange_order_id: result.order_id,
            client_order_id: result.order_link_id,
            symbol: symbol.to_string(),
            side: Side::Buy,
            order_type: OrderType::Limit,
            price: None,
            quantity: Decimal::ZERO,
            filled_quantity: Decimal::ZERO,
            avg_fill_price: None,
            status: OrderStatus::Cancelled,
            timestamp: timestamp as i64,
        })
    }

    async fn get_order(
        &self,
        credentials: &Credentials,
        symbol: &str,
        order_id: &str,
    ) -> Result<OrderResponse> {
        let timestamp = Self::timestamp();
        let recv_window = 5000u64;

        let query = format!("category=linear&symbol={}&orderId={}", symbol, order_id);
        let signature = self.sign(
            &credentials.api_secret,
            timestamp,
            &credentials.api_key,
            recv_window,
            &query,
        );

        let url = format!("{}/v5/order/realtime?{}", self.config.rest_url, query);

        let response = self.client
            .get(&url)
            .header("X-BAPI-API-KEY", &credentials.api_key)
            .header("X-BAPI-SIGN", &signature)
            .header("X-BAPI-TIMESTAMP", timestamp.to_string())
            .header("X-BAPI-RECV-WINDOW", recv_window.to_string())
            .send()
            .await?;

        let body = response.text().await?;
        let resp: BybitResponse<BybitOrderListResult> = serde_json::from_str(&body)?;

        let result = resp.result.ok_or_else(|| anyhow::anyhow!("No result"))?;
        let order = result.list.first().ok_or_else(|| anyhow::anyhow!("Order not found"))?;

        Ok(OrderResponse {
            exchange_order_id: order.order_id.clone(),
            client_order_id: order.order_link_id.clone(),
            symbol: order.symbol.clone(),
            side: match order.side.as_str() {
                "Buy" => Side::Buy,
                _ => Side::Sell,
            },
            order_type: match order.order_type.as_str() {
                "Limit" => OrderType::Limit,
                _ => OrderType::Market,
            },
            price: order.price.parse().ok(),
            quantity: order.qty.parse().unwrap_or_default(),
            filled_quantity: order.cum_exec_qty.parse().unwrap_or_default(),
            avg_fill_price: order.avg_price.parse().ok(),
            status: parse_bybit_status(&order.order_status),
            timestamp: order.updated_time.parse().unwrap_or(0),
        })
    }

    async fn get_best_price(&self, symbol: &str) -> Result<(Decimal, Decimal)> {
        let url = format!(
            "{}/v5/market/tickers?category=linear&symbol={}",
            self.config.rest_url, symbol
        );

        let response = self.client.get(&url).send().await?;
        let body = response.text().await?;

        #[derive(Deserialize)]
        struct TickerResult {
            list: Vec<Ticker>,
        }

        #[derive(Deserialize)]
        struct Ticker {
            bid1Price: String,
            ask1Price: String,
        }

        let resp: BybitResponse<TickerResult> = serde_json::from_str(&body)?;
        let result = resp.result.ok_or_else(|| anyhow::anyhow!("No result"))?;
        let ticker = result.list.first().ok_or_else(|| anyhow::anyhow!("No ticker"))?;

        Ok((
            ticker.bid1Price.parse()?,
            ticker.ask1Price.parse()?,
        ))
    }

    fn is_connected(&self) -> bool {
        true
    }
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct BybitResponse<T> {
    ret_code: i32,
    ret_msg: String,
    result: Option<T>,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct BybitOrderResult {
    order_id: String,
    order_link_id: String,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct BybitOrderListResult {
    list: Vec<BybitOrder>,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct BybitOrder {
    order_id: String,
    order_link_id: String,
    symbol: String,
    side: String,
    order_type: String,
    price: String,
    qty: String,
    cum_exec_qty: String,
    avg_price: String,
    order_status: String,
    updated_time: String,
}

fn parse_bybit_status(status: &str) -> OrderStatus {
    match status {
        "New" => OrderStatus::Open,
        "PartiallyFilled" => OrderStatus::Partial,
        "Filled" => OrderStatus::Filled,
        "Cancelled" => OrderStatus::Cancelled,
        "Rejected" => OrderStatus::Rejected,
        _ => OrderStatus::Pending,
    }
}
