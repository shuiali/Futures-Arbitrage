//! LBank Futures adapter

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

pub struct LbankAdapter {
    config: ExchangeConfig,
    client: Client,
}

impl LbankAdapter {
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
            .as_millis()
            .to_string()
    }

    fn sign(&self, secret: &str, params: &str) -> String {
        let mut mac = HmacSha256::new_from_slice(secret.as_bytes())
            .expect("HMAC can take key of any size");
        mac.update(params.as_bytes());
        hex::encode(mac.finalize().into_bytes())
    }
}

#[derive(Debug, Deserialize)]
struct LbankResponse<T> {
    result: bool,
    error_code: Option<i32>,
    data: Option<T>,
}

#[derive(Debug, Deserialize)]
struct LbankOrder {
    order_id: String,
    symbol: String,
    direction: String,
    offset: String,
    price: String,
    volume: String,
    traded_volume: Option<String>,
    avg_price: Option<String>,
    status: i32,
    create_time: i64,
    client_order_id: Option<String>,
}

#[async_trait]
impl ExchangeAdapter for LbankAdapter {
    fn id(&self) -> &str {
        "lbank"
    }

    async fn place_order(
        &self,
        credentials: &Credentials,
        request: &OrderRequest,
    ) -> Result<OrderResponse> {
        let timestamp = Self::timestamp();
        
        let mut params = vec![
            ("api_key", credentials.api_key.clone()),
            ("symbol", request.symbol.clone()),
            ("direction", match request.side {
                Side::Buy => "buy".to_string(),
                Side::Sell => "sell".to_string(),
            }),
            ("offset", "open".to_string()),
            ("type", match request.order_type {
                OrderType::Limit => "1".to_string(),
                OrderType::Market => "2".to_string(),
            }),
            ("volume", request.quantity.to_string()),
            ("timestamp", timestamp.clone()),
        ];

        if let Some(price) = request.price {
            params.push(("price", price.to_string()));
        }
        if !request.client_order_id.is_empty() {
            params.push(("client_order_id", request.client_order_id.clone()));
        }

        params.sort_by(|a, b| a.0.cmp(b.0));
        let params_str = params.iter()
            .map(|(k, v)| format!("{}={}", k, v))
            .collect::<Vec<_>>()
            .join("&");

        let signature = self.sign(&credentials.api_secret, &params_str);

        debug!("Placing LBank order: {}", request.symbol);

        let url = format!("{}/cfd/openApi/v1/order/create", self.config.rest_url);
        let response = self.client
            .post(&url)
            .header("Content-Type", "application/x-www-form-urlencoded")
            .body(format!("{}&sign={}", params_str, signature))
            .send()
            .await
            .context("Failed to send order request")?;

        let status = response.status();
        let body = response.text().await?;

        if !status.is_success() {
            anyhow::bail!("LBank order failed: {} - {}", status, body);
        }

        let resp: LbankResponse<LbankOrder> = serde_json::from_str(&body)
            .context("Failed to parse order response")?;

        if !resp.result {
            anyhow::bail!("LBank order error: {:?}", resp.error_code);
        }

        let order = resp.data.ok_or_else(|| anyhow::anyhow!("No order data"))?;

        info!("LBank order placed: {} status={}", order.order_id, order.status);

        Ok(OrderResponse {
            exchange_order_id: order.order_id,
            client_order_id: order.client_order_id.unwrap_or_default(),
            symbol: order.symbol,
            side: match order.direction.as_str() {
                "buy" => Side::Buy,
                _ => Side::Sell,
            },
            order_type: match request.order_type {
                OrderType::Limit => OrderType::Limit,
                OrderType::Market => OrderType::Market,
            },
            price: order.price.parse().ok(),
            quantity: order.volume.parse().unwrap_or_default(),
            filled_quantity: order.traded_volume.and_then(|s| s.parse().ok()).unwrap_or_default(),
            avg_fill_price: order.avg_price.and_then(|s| s.parse().ok()),
            status: parse_lbank_status(order.status),
            timestamp: order.create_time,
        })
    }

    async fn cancel_order(
        &self,
        credentials: &Credentials,
        symbol: &str,
        order_id: &str,
    ) -> Result<OrderResponse> {
        let timestamp = Self::timestamp();
        
        let mut params = vec![
            ("api_key", credentials.api_key.clone()),
            ("symbol", symbol.to_string()),
            ("order_id", order_id.to_string()),
            ("timestamp", timestamp),
        ];

        params.sort_by(|a, b| a.0.cmp(b.0));
        let params_str = params.iter()
            .map(|(k, v)| format!("{}={}", k, v))
            .collect::<Vec<_>>()
            .join("&");

        let signature = self.sign(&credentials.api_secret, &params_str);

        let url = format!("{}/cfd/openApi/v1/order/cancel", self.config.rest_url);
        let response = self.client
            .post(&url)
            .header("Content-Type", "application/x-www-form-urlencoded")
            .body(format!("{}&sign={}", params_str, signature))
            .send()
            .await?;

        let body = response.text().await?;
        let resp: LbankResponse<LbankOrder> = serde_json::from_str(&body)?;

        let order = resp.data.ok_or_else(|| anyhow::anyhow!("No order data"))?;

        Ok(OrderResponse {
            exchange_order_id: order.order_id,
            client_order_id: order.client_order_id.unwrap_or_default(),
            symbol: order.symbol,
            side: match order.direction.as_str() {
                "buy" => Side::Buy,
                _ => Side::Sell,
            },
            order_type: OrderType::Limit,
            price: order.price.parse().ok(),
            quantity: order.volume.parse().unwrap_or_default(),
            filled_quantity: order.traded_volume.and_then(|s| s.parse().ok()).unwrap_or_default(),
            avg_fill_price: order.avg_price.and_then(|s| s.parse().ok()),
            status: OrderStatus::Cancelled,
            timestamp: order.create_time,
        })
    }

    async fn get_order(
        &self,
        credentials: &Credentials,
        symbol: &str,
        order_id: &str,
    ) -> Result<OrderResponse> {
        let timestamp = Self::timestamp();
        
        let mut params = vec![
            ("api_key", credentials.api_key.clone()),
            ("symbol", symbol.to_string()),
            ("order_id", order_id.to_string()),
            ("timestamp", timestamp),
        ];

        params.sort_by(|a, b| a.0.cmp(b.0));
        let params_str = params.iter()
            .map(|(k, v)| format!("{}={}", k, v))
            .collect::<Vec<_>>()
            .join("&");

        let signature = self.sign(&credentials.api_secret, &params_str);

        let url = format!("{}/cfd/openApi/v1/order/detail?{}&sign={}", 
            self.config.rest_url, params_str, signature);
        
        let response = self.client.get(&url).send().await?;
        let body = response.text().await?;
        let resp: LbankResponse<LbankOrder> = serde_json::from_str(&body)?;

        let order = resp.data.ok_or_else(|| anyhow::anyhow!("No order data"))?;

        Ok(OrderResponse {
            exchange_order_id: order.order_id,
            client_order_id: order.client_order_id.unwrap_or_default(),
            symbol: order.symbol,
            side: match order.direction.as_str() {
                "buy" => Side::Buy,
                _ => Side::Sell,
            },
            order_type: OrderType::Limit,
            price: order.price.parse().ok(),
            quantity: order.volume.parse().unwrap_or_default(),
            filled_quantity: order.traded_volume.and_then(|s| s.parse().ok()).unwrap_or_default(),
            avg_fill_price: order.avg_price.and_then(|s| s.parse().ok()),
            status: parse_lbank_status(order.status),
            timestamp: order.create_time,
        })
    }

    async fn get_best_price(&self, symbol: &str) -> Result<(Decimal, Decimal)> {
        let url = format!("{}/cfd/openApi/v1/pub/depth?symbol={}&size=1", 
            self.config.rest_url, symbol);
        
        let response = self.client.get(&url).send().await?;
        let body = response.text().await?;
        
        #[derive(Deserialize)]
        struct DepthData {
            bids: Vec<Vec<String>>,
            asks: Vec<Vec<String>>,
        }
        
        let resp: LbankResponse<DepthData> = serde_json::from_str(&body)?;
        let depth = resp.data.ok_or_else(|| anyhow::anyhow!("No depth data"))?;

        let bid = depth.bids.first()
            .and_then(|b| b.first())
            .ok_or_else(|| anyhow::anyhow!("No bid"))?;
        let ask = depth.asks.first()
            .and_then(|a| a.first())
            .ok_or_else(|| anyhow::anyhow!("No ask"))?;

        Ok((
            bid.parse()?,
            ask.parse()?,
        ))
    }

    fn is_connected(&self) -> bool {
        true
    }
}

fn parse_lbank_status(status: i32) -> OrderStatus {
    match status {
        0 => OrderStatus::Pending,
        1 => OrderStatus::Open,
        2 => OrderStatus::Partial,
        3 => OrderStatus::Filled,
        4 | 5 => OrderStatus::Cancelled,
        _ => OrderStatus::Pending,
    }
}
