//! Binance Futures adapter

use anyhow::{Context, Result};
use async_trait::async_trait;
use hmac::{Hmac, Mac};
use reqwest::Client;
use rust_decimal::Decimal;
use serde::{Deserialize, Serialize};
use sha2::Sha256;
use std::time::{SystemTime, UNIX_EPOCH};
use tracing::{debug, info};

use super::{Credentials, ExchangeAdapter, OrderRequest, OrderResponse, OrderStatus, OrderType, Side};
use crate::config::ExchangeConfig;

type HmacSha256 = Hmac<Sha256>;

pub struct BinanceAdapter {
    config: ExchangeConfig,
    client: Client,
}

impl BinanceAdapter {
    pub async fn new(config: ExchangeConfig) -> Result<Self> {
        let client = Client::builder()
            .timeout(std::time::Duration::from_secs(10))
            .build()?;

        Ok(Self { config, client })
    }

    fn sign(&self, secret: &str, query: &str) -> String {
        let mut mac = HmacSha256::new_from_slice(secret.as_bytes())
            .expect("HMAC can take key of any size");
        mac.update(query.as_bytes());
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
impl ExchangeAdapter for BinanceAdapter {
    fn id(&self) -> &str {
        "binance"
    }

    async fn place_order(
        &self,
        credentials: &Credentials,
        request: &OrderRequest,
    ) -> Result<OrderResponse> {
        let timestamp = Self::timestamp();
        
        let mut params = vec![
            format!("symbol={}", request.symbol),
            format!("side={}", match request.side {
                Side::Buy => "BUY",
                Side::Sell => "SELL",
            }),
            format!("type={}", match request.order_type {
                OrderType::Limit => "LIMIT",
                OrderType::Market => "MARKET",
            }),
            format!("quantity={}", request.quantity),
            format!("newClientOrderId={}", request.client_order_id),
            format!("timestamp={}", timestamp),
        ];

        if request.order_type == OrderType::Limit {
            if let Some(price) = &request.price {
                params.push(format!("price={}", price));
                params.push("timeInForce=GTC".to_string());
            }
        }

        if request.reduce_only {
            params.push("reduceOnly=true".to_string());
        }

        let query = params.join("&");
        let signature = self.sign(&credentials.api_secret, &query);
        let full_query = format!("{}&signature={}", query, signature);

        let url = format!("{}/fapi/v1/order?{}", self.config.rest_url, full_query);
        
        debug!("Placing Binance order: {}", request.symbol);

        let response = self.client
            .post(&url)
            .header("X-MBX-APIKEY", &credentials.api_key)
            .send()
            .await
            .context("Failed to send order request")?;

        let status = response.status();
        let body = response.text().await?;

        if !status.is_success() {
            anyhow::bail!("Binance order failed: {} - {}", status, body);
        }

        let order: BinanceOrderResponse = serde_json::from_str(&body)
            .context("Failed to parse order response")?;

        info!("Binance order placed: {} status={}", order.order_id, order.status);

        Ok(OrderResponse {
            exchange_order_id: order.order_id.to_string(),
            client_order_id: order.client_order_id,
            symbol: order.symbol,
            side: match order.side.as_str() {
                "BUY" => Side::Buy,
                _ => Side::Sell,
            },
            order_type: match order.order_type.as_str() {
                "LIMIT" => OrderType::Limit,
                _ => OrderType::Market,
            },
            price: order.price.parse().ok(),
            quantity: order.orig_qty.parse().unwrap_or_default(),
            filled_quantity: order.executed_qty.parse().unwrap_or_default(),
            avg_fill_price: order.avg_price.parse().ok(),
            status: parse_binance_status(&order.status),
            timestamp: order.update_time,
        })
    }

    async fn cancel_order(
        &self,
        credentials: &Credentials,
        symbol: &str,
        order_id: &str,
    ) -> Result<OrderResponse> {
        let timestamp = Self::timestamp();
        
        let query = format!(
            "symbol={}&orderId={}&timestamp={}",
            symbol, order_id, timestamp
        );
        let signature = self.sign(&credentials.api_secret, &query);
        let full_query = format!("{}&signature={}", query, signature);

        let url = format!("{}/fapi/v1/order?{}", self.config.rest_url, full_query);

        let response = self.client
            .delete(&url)
            .header("X-MBX-APIKEY", &credentials.api_key)
            .send()
            .await?;

        let body = response.text().await?;
        let order: BinanceOrderResponse = serde_json::from_str(&body)?;

        Ok(OrderResponse {
            exchange_order_id: order.order_id.to_string(),
            client_order_id: order.client_order_id,
            symbol: order.symbol,
            side: match order.side.as_str() {
                "BUY" => Side::Buy,
                _ => Side::Sell,
            },
            order_type: OrderType::Limit,
            price: order.price.parse().ok(),
            quantity: order.orig_qty.parse().unwrap_or_default(),
            filled_quantity: order.executed_qty.parse().unwrap_or_default(),
            avg_fill_price: order.avg_price.parse().ok(),
            status: parse_binance_status(&order.status),
            timestamp: order.update_time,
        })
    }

    async fn get_order(
        &self,
        credentials: &Credentials,
        symbol: &str,
        order_id: &str,
    ) -> Result<OrderResponse> {
        let timestamp = Self::timestamp();
        
        let query = format!(
            "symbol={}&orderId={}&timestamp={}",
            symbol, order_id, timestamp
        );
        let signature = self.sign(&credentials.api_secret, &query);
        let full_query = format!("{}&signature={}", query, signature);

        let url = format!("{}/fapi/v1/order?{}", self.config.rest_url, full_query);

        let response = self.client
            .get(&url)
            .header("X-MBX-APIKEY", &credentials.api_key)
            .send()
            .await?;

        let body = response.text().await?;
        let order: BinanceOrderResponse = serde_json::from_str(&body)?;

        Ok(OrderResponse {
            exchange_order_id: order.order_id.to_string(),
            client_order_id: order.client_order_id,
            symbol: order.symbol,
            side: match order.side.as_str() {
                "BUY" => Side::Buy,
                _ => Side::Sell,
            },
            order_type: OrderType::Limit,
            price: order.price.parse().ok(),
            quantity: order.orig_qty.parse().unwrap_or_default(),
            filled_quantity: order.executed_qty.parse().unwrap_or_default(),
            avg_fill_price: order.avg_price.parse().ok(),
            status: parse_binance_status(&order.status),
            timestamp: order.update_time,
        })
    }

    async fn get_best_price(&self, symbol: &str) -> Result<(Decimal, Decimal)> {
        let url = format!(
            "{}/fapi/v1/ticker/bookTicker?symbol={}",
            self.config.rest_url, symbol
        );

        let response = self.client.get(&url).send().await?;
        let body = response.text().await?;

        #[derive(Deserialize)]
        struct BookTicker {
            #[serde(rename = "bidPrice")]
            bid_price: String,
            #[serde(rename = "askPrice")]
            ask_price: String,
        }

        let ticker: BookTicker = serde_json::from_str(&body)?;
        
        Ok((
            ticker.bid_price.parse()?,
            ticker.ask_price.parse()?,
        ))
    }

    fn is_connected(&self) -> bool {
        true // REST adapter is always "connected"
    }
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct BinanceOrderResponse {
    order_id: i64,
    symbol: String,
    status: String,
    client_order_id: String,
    price: String,
    orig_qty: String,
    executed_qty: String,
    avg_price: String,
    side: String,
    #[serde(rename = "type")]
    order_type: String,
    update_time: i64,
}

fn parse_binance_status(status: &str) -> OrderStatus {
    match status {
        "NEW" => OrderStatus::Open,
        "PARTIALLY_FILLED" => OrderStatus::Partial,
        "FILLED" => OrderStatus::Filled,
        "CANCELED" => OrderStatus::Cancelled,
        "REJECTED" => OrderStatus::Rejected,
        "EXPIRED" => OrderStatus::Expired,
        _ => OrderStatus::Pending,
    }
}
