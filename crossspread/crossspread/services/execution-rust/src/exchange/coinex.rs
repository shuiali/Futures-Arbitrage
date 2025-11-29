//! CoinEx Futures adapter

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

pub struct CoinexAdapter {
    config: ExchangeConfig,
    client: Client,
}

impl CoinexAdapter {
    pub async fn new(config: ExchangeConfig) -> Result<Self> {
        let client = Client::builder()
            .timeout(std::time::Duration::from_secs(10))
            .build()?;

        Ok(Self { config, client })
    }

    fn timestamp() -> i64 {
        SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap()
            .as_millis() as i64
    }

    fn sign(&self, secret: &str, method: &str, path: &str, timestamp: i64, body: &str) -> String {
        let prepared = format!("{}{}{}{}",
            method.to_uppercase(),
            path,
            body,
            timestamp
        );
        let mut mac = HmacSha256::new_from_slice(secret.as_bytes())
            .expect("HMAC can take key of any size");
        mac.update(prepared.as_bytes());
        hex::encode(mac.finalize().into_bytes()).to_lowercase()
    }
}

#[derive(Debug, Deserialize)]
struct CoinexResponse<T> {
    code: i32,
    message: String,
    data: Option<T>,
}

#[derive(Debug, Deserialize)]
struct CoinexOrder {
    order_id: i64,
    market: String,
    side: i32,
    #[serde(rename = "type")]
    order_type: i32,
    amount: String,
    price: String,
    deal_amount: Option<String>,
    avg_price: Option<String>,
    status: String,
    created_at: i64,
    client_id: Option<String>,
}

#[async_trait]
impl ExchangeAdapter for CoinexAdapter {
    fn id(&self) -> &str {
        "coinex"
    }

    async fn place_order(
        &self,
        credentials: &Credentials,
        request: &OrderRequest,
    ) -> Result<OrderResponse> {
        let timestamp = Self::timestamp();
        let path = "/v2/futures/order";
        
        let body = serde_json::json!({
            "market": request.symbol,
            "side": match request.side {
                Side::Buy => 1,
                Side::Sell => 2,
            },
            "type": match request.order_type {
                OrderType::Limit => 1,
                OrderType::Market => 2,
            },
            "amount": request.quantity.to_string(),
            "price": request.price.map(|p| p.to_string()),
            "client_id": request.client_order_id,
        }).to_string();

        let signature = self.sign(&credentials.api_secret, "POST", path, timestamp, &body);

        debug!("Placing CoinEx order: {}", request.symbol);

        let url = format!("{}{}", self.config.rest_url, path);
        let response = self.client
            .post(&url)
            .header("X-COINEX-KEY", &credentials.api_key)
            .header("X-COINEX-SIGN", &signature)
            .header("X-COINEX-TIMESTAMP", timestamp.to_string())
            .header("Content-Type", "application/json")
            .body(body)
            .send()
            .await
            .context("Failed to send order request")?;

        let status = response.status();
        let body = response.text().await?;

        if !status.is_success() {
            anyhow::bail!("CoinEx order failed: {} - {}", status, body);
        }

        let resp: CoinexResponse<CoinexOrder> = serde_json::from_str(&body)
            .context("Failed to parse order response")?;

        if resp.code != 0 {
            anyhow::bail!("CoinEx order error: {} - {}", resp.code, resp.message);
        }

        let order = resp.data.ok_or_else(|| anyhow::anyhow!("No order data"))?;

        info!("CoinEx order placed: {} status={}", order.order_id, order.status);

        Ok(OrderResponse {
            exchange_order_id: order.order_id.to_string(),
            client_order_id: order.client_id.unwrap_or_default(),
            symbol: order.market,
            side: match order.side {
                1 => Side::Buy,
                _ => Side::Sell,
            },
            order_type: match order.order_type {
                1 => OrderType::Limit,
                _ => OrderType::Market,
            },
            price: order.price.parse().ok(),
            quantity: order.amount.parse().unwrap_or_default(),
            filled_quantity: order.deal_amount.and_then(|s| s.parse().ok()).unwrap_or_default(),
            avg_fill_price: order.avg_price.and_then(|s| s.parse().ok()),
            status: parse_coinex_status(&order.status),
            timestamp: order.created_at,
        })
    }

    async fn cancel_order(
        &self,
        credentials: &Credentials,
        symbol: &str,
        order_id: &str,
    ) -> Result<OrderResponse> {
        let timestamp = Self::timestamp();
        let path = "/v2/futures/order";
        
        let body = serde_json::json!({
            "market": symbol,
            "order_id": order_id.parse::<i64>().unwrap_or(0),
        }).to_string();

        let signature = self.sign(&credentials.api_secret, "DELETE", path, timestamp, &body);

        let url = format!("{}{}", self.config.rest_url, path);
        let response = self.client
            .delete(&url)
            .header("X-COINEX-KEY", &credentials.api_key)
            .header("X-COINEX-SIGN", &signature)
            .header("X-COINEX-TIMESTAMP", timestamp.to_string())
            .header("Content-Type", "application/json")
            .body(body)
            .send()
            .await?;

        let body = response.text().await?;
        let resp: CoinexResponse<CoinexOrder> = serde_json::from_str(&body)?;

        let order = resp.data.ok_or_else(|| anyhow::anyhow!("No order data"))?;

        Ok(OrderResponse {
            exchange_order_id: order.order_id.to_string(),
            client_order_id: order.client_id.unwrap_or_default(),
            symbol: order.market,
            side: match order.side {
                1 => Side::Buy,
                _ => Side::Sell,
            },
            order_type: OrderType::Limit,
            price: order.price.parse().ok(),
            quantity: order.amount.parse().unwrap_or_default(),
            filled_quantity: order.deal_amount.and_then(|s| s.parse().ok()).unwrap_or_default(),
            avg_fill_price: order.avg_price.and_then(|s| s.parse().ok()),
            status: OrderStatus::Cancelled,
            timestamp: order.created_at,
        })
    }

    async fn get_order(
        &self,
        credentials: &Credentials,
        symbol: &str,
        order_id: &str,
    ) -> Result<OrderResponse> {
        let timestamp = Self::timestamp();
        let path = format!("/v2/futures/order?market={}&order_id={}", symbol, order_id);
        
        let signature = self.sign(&credentials.api_secret, "GET", &path, timestamp, "");

        let url = format!("{}{}", self.config.rest_url, path);
        let response = self.client
            .get(&url)
            .header("X-COINEX-KEY", &credentials.api_key)
            .header("X-COINEX-SIGN", &signature)
            .header("X-COINEX-TIMESTAMP", timestamp.to_string())
            .send()
            .await?;

        let body = response.text().await?;
        let resp: CoinexResponse<CoinexOrder> = serde_json::from_str(&body)?;

        let order = resp.data.ok_or_else(|| anyhow::anyhow!("No order data"))?;

        Ok(OrderResponse {
            exchange_order_id: order.order_id.to_string(),
            client_order_id: order.client_id.unwrap_or_default(),
            symbol: order.market,
            side: match order.side {
                1 => Side::Buy,
                _ => Side::Sell,
            },
            order_type: match order.order_type {
                1 => OrderType::Limit,
                _ => OrderType::Market,
            },
            price: order.price.parse().ok(),
            quantity: order.amount.parse().unwrap_or_default(),
            filled_quantity: order.deal_amount.and_then(|s| s.parse().ok()).unwrap_or_default(),
            avg_fill_price: order.avg_price.and_then(|s| s.parse().ok()),
            status: parse_coinex_status(&order.status),
            timestamp: order.created_at,
        })
    }

    async fn get_best_price(&self, symbol: &str) -> Result<(Decimal, Decimal)> {
        let url = format!("{}/v2/futures/ticker?market={}", self.config.rest_url, symbol);
        
        let response = self.client.get(&url).send().await?;
        let body = response.text().await?;
        
        #[derive(Deserialize)]
        struct TickerData {
            best_bid_price: String,
            best_ask_price: String,
        }
        
        let resp: CoinexResponse<TickerData> = serde_json::from_str(&body)?;
        let ticker = resp.data.ok_or_else(|| anyhow::anyhow!("No ticker data"))?;

        Ok((
            ticker.best_bid_price.parse()?,
            ticker.best_ask_price.parse()?,
        ))
    }

    fn is_connected(&self) -> bool {
        true
    }
}

fn parse_coinex_status(status: &str) -> OrderStatus {
    match status {
        "open" | "not_deal" => OrderStatus::Open,
        "part_deal" => OrderStatus::Partial,
        "done" | "filled" => OrderStatus::Filled,
        "cancel" | "canceled" => OrderStatus::Cancelled,
        _ => OrderStatus::Pending,
    }
}
