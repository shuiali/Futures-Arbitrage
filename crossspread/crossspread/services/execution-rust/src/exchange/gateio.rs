//! Gate.io Futures adapter

use anyhow::{Context, Result};
use async_trait::async_trait;
use hmac::{Hmac, Mac};
use reqwest::Client;
use rust_decimal::Decimal;
use serde::Deserialize;
use sha2::Sha512;
use std::time::{SystemTime, UNIX_EPOCH};
use tracing::{debug, info};

use super::{Credentials, ExchangeAdapter, OrderRequest, OrderResponse, OrderStatus, OrderType, Side};
use crate::config::ExchangeConfig;

type HmacSha512 = Hmac<Sha512>;

pub struct GateioAdapter {
    config: ExchangeConfig,
    client: Client,
}

impl GateioAdapter {
    pub async fn new(config: ExchangeConfig) -> Result<Self> {
        let client = Client::builder()
            .timeout(std::time::Duration::from_secs(10))
            .build()?;

        Ok(Self { config, client })
    }

    fn timestamp() -> String {
        SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap()
            .as_secs()
            .to_string()
    }

    fn sign(&self, secret: &str, method: &str, path: &str, query: &str, body: &str, timestamp: &str) -> String {
        // Gate.io uses: sha512 of body + sha512 of (method + path + query + body_hash + timestamp)
        use sha2::{Digest, Sha512};
        
        let body_hash = hex::encode(Sha512::digest(body.as_bytes()));
        let str_to_sign = format!("{}\n{}\n{}\n{}\n{}", method.to_uppercase(), path, query, body_hash, timestamp);
        
        let mut mac = HmacSha512::new_from_slice(secret.as_bytes())
            .expect("HMAC can take key of any size");
        mac.update(str_to_sign.as_bytes());
        hex::encode(mac.finalize().into_bytes())
    }
}

#[derive(Debug, Deserialize)]
struct GateioOrder {
    id: i64,
    contract: String,
    size: i64,
    price: String,
    close: bool,
    #[serde(rename = "tif")]
    time_in_force: String,
    #[serde(rename = "fill_price")]
    fill_price: Option<String>,
    left: i64,
    status: String,
    #[serde(rename = "create_time")]
    create_time: f64,
    text: Option<String>,
}

#[async_trait]
impl ExchangeAdapter for GateioAdapter {
    fn id(&self) -> &str {
        "gateio"
    }

    async fn place_order(
        &self,
        credentials: &Credentials,
        request: &OrderRequest,
    ) -> Result<OrderResponse> {
        let timestamp = Self::timestamp();
        let path = "/api/v4/futures/usdt/orders";
        
        let size = if request.side == Side::Sell {
            -request.quantity.to_string().parse::<i64>().unwrap_or(1)
        } else {
            request.quantity.to_string().parse::<i64>().unwrap_or(1)
        };

        let body = serde_json::json!({
            "contract": request.symbol,
            "size": size,
            "price": request.price.map(|p| p.to_string()).unwrap_or_else(|| "0".to_string()),
            "tif": if request.order_type == OrderType::Market { "ioc" } else { "gtc" },
            "reduce_only": request.reduce_only,
            "text": request.client_order_id,
        }).to_string();

        let signature = self.sign(&credentials.api_secret, "POST", path, "", &body, &timestamp);

        debug!("Placing Gate.io order: {}", request.symbol);

        let url = format!("{}{}", self.config.rest_url, path);
        let response = self.client
            .post(&url)
            .header("KEY", &credentials.api_key)
            .header("SIGN", &signature)
            .header("Timestamp", &timestamp)
            .header("Content-Type", "application/json")
            .body(body)
            .send()
            .await
            .context("Failed to send order request")?;

        let status = response.status();
        let body = response.text().await?;

        if !status.is_success() {
            anyhow::bail!("Gate.io order failed: {} - {}", status, body);
        }

        let order: GateioOrder = serde_json::from_str(&body)
            .context("Failed to parse order response")?;

        info!("Gate.io order placed: {} status={}", order.id, order.status);

        Ok(OrderResponse {
            exchange_order_id: order.id.to_string(),
            client_order_id: order.text.unwrap_or_default(),
            symbol: order.contract,
            side: if order.size > 0 { Side::Buy } else { Side::Sell },
            order_type: match order.time_in_force.as_str() {
                "ioc" => OrderType::Market,
                _ => OrderType::Limit,
            },
            price: order.price.parse().ok(),
            quantity: Decimal::from(order.size.abs()),
            filled_quantity: Decimal::from((order.size.abs() - order.left).abs()),
            avg_fill_price: order.fill_price.and_then(|p| p.parse().ok()),
            status: parse_gateio_status(&order.status),
            timestamp: (order.create_time * 1000.0) as i64,
        })
    }

    async fn cancel_order(
        &self,
        credentials: &Credentials,
        symbol: &str,
        order_id: &str,
    ) -> Result<OrderResponse> {
        let timestamp = Self::timestamp();
        let path = format!("/api/v4/futures/usdt/orders/{}", order_id);
        
        let signature = self.sign(&credentials.api_secret, "DELETE", &path, "", "", &timestamp);

        let url = format!("{}{}", self.config.rest_url, path);
        let response = self.client
            .delete(&url)
            .header("KEY", &credentials.api_key)
            .header("SIGN", &signature)
            .header("Timestamp", &timestamp)
            .send()
            .await?;

        let body = response.text().await?;
        let order: GateioOrder = serde_json::from_str(&body)?;

        Ok(OrderResponse {
            exchange_order_id: order.id.to_string(),
            client_order_id: order.text.unwrap_or_default(),
            symbol: order.contract,
            side: if order.size > 0 { Side::Buy } else { Side::Sell },
            order_type: OrderType::Limit,
            price: order.price.parse().ok(),
            quantity: Decimal::from(order.size.abs()),
            filled_quantity: Decimal::from((order.size.abs() - order.left).abs()),
            avg_fill_price: order.fill_price.and_then(|p| p.parse().ok()),
            status: OrderStatus::Cancelled,
            timestamp: (order.create_time * 1000.0) as i64,
        })
    }

    async fn get_order(
        &self,
        credentials: &Credentials,
        symbol: &str,
        order_id: &str,
    ) -> Result<OrderResponse> {
        let timestamp = Self::timestamp();
        let path = format!("/api/v4/futures/usdt/orders/{}", order_id);
        
        let signature = self.sign(&credentials.api_secret, "GET", &path, "", "", &timestamp);

        let url = format!("{}{}", self.config.rest_url, path);
        let response = self.client
            .get(&url)
            .header("KEY", &credentials.api_key)
            .header("SIGN", &signature)
            .header("Timestamp", &timestamp)
            .send()
            .await?;

        let body = response.text().await?;
        let order: GateioOrder = serde_json::from_str(&body)?;

        Ok(OrderResponse {
            exchange_order_id: order.id.to_string(),
            client_order_id: order.text.unwrap_or_default(),
            symbol: order.contract,
            side: if order.size > 0 { Side::Buy } else { Side::Sell },
            order_type: match order.time_in_force.as_str() {
                "ioc" => OrderType::Market,
                _ => OrderType::Limit,
            },
            price: order.price.parse().ok(),
            quantity: Decimal::from(order.size.abs()),
            filled_quantity: Decimal::from((order.size.abs() - order.left).abs()),
            avg_fill_price: order.fill_price.and_then(|p| p.parse().ok()),
            status: parse_gateio_status(&order.status),
            timestamp: (order.create_time * 1000.0) as i64,
        })
    }

    async fn get_best_price(&self, symbol: &str) -> Result<(Decimal, Decimal)> {
        let url = format!("{}/api/v4/futures/usdt/tickers?contract={}", self.config.rest_url, symbol);
        
        let response = self.client.get(&url).send().await?;
        let body = response.text().await?;
        
        #[derive(Deserialize)]
        struct Ticker {
            highest_bid: String,
            lowest_ask: String,
        }
        
        let tickers: Vec<Ticker> = serde_json::from_str(&body)?;
        let ticker = tickers.into_iter().next()
            .ok_or_else(|| anyhow::anyhow!("No ticker data"))?;

        Ok((
            ticker.highest_bid.parse()?,
            ticker.lowest_ask.parse()?,
        ))
    }

    fn is_connected(&self) -> bool {
        true
    }
}

fn parse_gateio_status(status: &str) -> OrderStatus {
    match status {
        "open" => OrderStatus::Open,
        "finished" => OrderStatus::Filled,
        "cancelled" => OrderStatus::Cancelled,
        _ => OrderStatus::Pending,
    }
}
