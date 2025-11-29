//! OKX Futures adapter

use anyhow::{Context, Result};
use async_trait::async_trait;
use base64::{engine::general_purpose::STANDARD, Engine};
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

pub struct OkxAdapter {
    config: ExchangeConfig,
    client: Client,
}

impl OkxAdapter {
    pub async fn new(config: ExchangeConfig) -> Result<Self> {
        let client = Client::builder()
            .timeout(std::time::Duration::from_secs(10))
            .build()?;

        Ok(Self { config, client })
    }

    fn timestamp_iso() -> String {
        chrono::Utc::now().format("%Y-%m-%dT%H:%M:%S%.3fZ").to_string()
    }

    fn sign(&self, secret: &str, timestamp: &str, method: &str, path: &str, body: &str) -> String {
        let prehash = format!("{}{}{}{}", timestamp, method, path, body);
        let mut mac = HmacSha256::new_from_slice(secret.as_bytes())
            .expect("HMAC can take key of any size");
        mac.update(prehash.as_bytes());
        STANDARD.encode(mac.finalize().into_bytes())
    }
}

#[derive(Debug, Deserialize)]
struct OkxResponse<T> {
    code: String,
    msg: String,
    data: Vec<T>,
}

#[derive(Debug, Deserialize)]
struct OkxOrderData {
    #[serde(rename = "ordId")]
    ord_id: String,
    #[serde(rename = "clOrdId")]
    cl_ord_id: String,
    #[serde(rename = "instId")]
    inst_id: String,
    side: String,
    #[serde(rename = "ordType")]
    ord_type: String,
    px: String,
    sz: String,
    #[serde(rename = "fillSz")]
    fill_sz: Option<String>,
    #[serde(rename = "avgPx")]
    avg_px: Option<String>,
    state: String,
    #[serde(rename = "uTime")]
    u_time: String,
}

#[async_trait]
impl ExchangeAdapter for OkxAdapter {
    fn id(&self) -> &str {
        "okx"
    }

    async fn place_order(
        &self,
        credentials: &Credentials,
        request: &OrderRequest,
    ) -> Result<OrderResponse> {
        let timestamp = Self::timestamp_iso();
        let path = "/api/v5/trade/order";
        
        let body = serde_json::json!({
            "instId": request.symbol,
            "tdMode": "cross",
            "side": match request.side {
                Side::Buy => "buy",
                Side::Sell => "sell",
            },
            "ordType": match request.order_type {
                OrderType::Limit => "limit",
                OrderType::Market => "market",
            },
            "sz": request.quantity.to_string(),
            "px": request.price.map(|p| p.to_string()),
            "clOrdId": request.client_order_id,
            "reduceOnly": request.reduce_only,
        }).to_string();

        let signature = self.sign(&credentials.api_secret, &timestamp, "POST", path, &body);

        let passphrase = credentials.passphrase.as_deref().unwrap_or("");
        
        debug!("Placing OKX order: {}", request.symbol);

        let url = format!("{}{}", self.config.rest_url, path);
        let response = self.client
            .post(&url)
            .header("OK-ACCESS-KEY", &credentials.api_key)
            .header("OK-ACCESS-SIGN", &signature)
            .header("OK-ACCESS-TIMESTAMP", &timestamp)
            .header("OK-ACCESS-PASSPHRASE", passphrase)
            .header("Content-Type", "application/json")
            .body(body)
            .send()
            .await
            .context("Failed to send order request")?;

        let status = response.status();
        let body = response.text().await?;

        if !status.is_success() {
            anyhow::bail!("OKX order failed: {} - {}", status, body);
        }

        let resp: OkxResponse<OkxOrderData> = serde_json::from_str(&body)
            .context("Failed to parse order response")?;

        if resp.code != "0" {
            anyhow::bail!("OKX order error: {} - {}", resp.code, resp.msg);
        }

        let order = resp.data.into_iter().next()
            .ok_or_else(|| anyhow::anyhow!("No order data in response"))?;

        info!("OKX order placed: {} state={}", order.ord_id, order.state);

        Ok(OrderResponse {
            exchange_order_id: order.ord_id,
            client_order_id: order.cl_ord_id,
            symbol: order.inst_id,
            side: match order.side.as_str() {
                "buy" => Side::Buy,
                _ => Side::Sell,
            },
            order_type: match order.ord_type.as_str() {
                "limit" => OrderType::Limit,
                _ => OrderType::Market,
            },
            price: order.px.parse().ok(),
            quantity: order.sz.parse().unwrap_or_default(),
            filled_quantity: order.fill_sz.and_then(|s| s.parse().ok()).unwrap_or_default(),
            avg_fill_price: order.avg_px.and_then(|s| s.parse().ok()),
            status: parse_okx_status(&order.state),
            timestamp: order.u_time.parse().unwrap_or(0),
        })
    }

    async fn cancel_order(
        &self,
        credentials: &Credentials,
        symbol: &str,
        order_id: &str,
    ) -> Result<OrderResponse> {
        let timestamp = Self::timestamp_iso();
        let path = "/api/v5/trade/cancel-order";
        
        let body = serde_json::json!({
            "instId": symbol,
            "ordId": order_id,
        }).to_string();

        let signature = self.sign(&credentials.api_secret, &timestamp, "POST", path, &body);
        let passphrase = credentials.passphrase.as_deref().unwrap_or("");

        let url = format!("{}{}", self.config.rest_url, path);
        let response = self.client
            .post(&url)
            .header("OK-ACCESS-KEY", &credentials.api_key)
            .header("OK-ACCESS-SIGN", &signature)
            .header("OK-ACCESS-TIMESTAMP", &timestamp)
            .header("OK-ACCESS-PASSPHRASE", passphrase)
            .header("Content-Type", "application/json")
            .body(body)
            .send()
            .await?;

        let body = response.text().await?;
        let resp: OkxResponse<OkxOrderData> = serde_json::from_str(&body)?;

        let order = resp.data.into_iter().next()
            .ok_or_else(|| anyhow::anyhow!("No order data"))?;

        Ok(OrderResponse {
            exchange_order_id: order.ord_id,
            client_order_id: order.cl_ord_id,
            symbol: order.inst_id,
            side: match order.side.as_str() {
                "buy" => Side::Buy,
                _ => Side::Sell,
            },
            order_type: OrderType::Limit,
            price: order.px.parse().ok(),
            quantity: order.sz.parse().unwrap_or_default(),
            filled_quantity: order.fill_sz.and_then(|s| s.parse().ok()).unwrap_or_default(),
            avg_fill_price: order.avg_px.and_then(|s| s.parse().ok()),
            status: OrderStatus::Cancelled,
            timestamp: order.u_time.parse().unwrap_or(0),
        })
    }

    async fn get_order(
        &self,
        credentials: &Credentials,
        symbol: &str,
        order_id: &str,
    ) -> Result<OrderResponse> {
        let timestamp = Self::timestamp_iso();
        let path = format!("/api/v5/trade/order?instId={}&ordId={}", symbol, order_id);
        
        let signature = self.sign(&credentials.api_secret, &timestamp, "GET", &path, "");
        let passphrase = credentials.passphrase.as_deref().unwrap_or("");

        let url = format!("{}{}", self.config.rest_url, path);
        let response = self.client
            .get(&url)
            .header("OK-ACCESS-KEY", &credentials.api_key)
            .header("OK-ACCESS-SIGN", &signature)
            .header("OK-ACCESS-TIMESTAMP", &timestamp)
            .header("OK-ACCESS-PASSPHRASE", passphrase)
            .send()
            .await?;

        let body = response.text().await?;
        let resp: OkxResponse<OkxOrderData> = serde_json::from_str(&body)?;

        let order = resp.data.into_iter().next()
            .ok_or_else(|| anyhow::anyhow!("No order data"))?;

        Ok(OrderResponse {
            exchange_order_id: order.ord_id,
            client_order_id: order.cl_ord_id,
            symbol: order.inst_id,
            side: match order.side.as_str() {
                "buy" => Side::Buy,
                _ => Side::Sell,
            },
            order_type: match order.ord_type.as_str() {
                "limit" => OrderType::Limit,
                _ => OrderType::Market,
            },
            price: order.px.parse().ok(),
            quantity: order.sz.parse().unwrap_or_default(),
            filled_quantity: order.fill_sz.and_then(|s| s.parse().ok()).unwrap_or_default(),
            avg_fill_price: order.avg_px.and_then(|s| s.parse().ok()),
            status: parse_okx_status(&order.state),
            timestamp: order.u_time.parse().unwrap_or(0),
        })
    }

    async fn get_best_price(&self, symbol: &str) -> Result<(Decimal, Decimal)> {
        let url = format!("{}/api/v5/market/ticker?instId={}", self.config.rest_url, symbol);
        
        let response = self.client.get(&url).send().await?;
        let body = response.text().await?;
        
        #[derive(Deserialize)]
        struct Ticker {
            #[serde(rename = "bidPx")]
            bid_px: String,
            #[serde(rename = "askPx")]
            ask_px: String,
        }
        
        let resp: OkxResponse<Ticker> = serde_json::from_str(&body)?;
        let ticker = resp.data.into_iter().next()
            .ok_or_else(|| anyhow::anyhow!("No ticker data"))?;

        Ok((
            ticker.bid_px.parse()?,
            ticker.ask_px.parse()?,
        ))
    }

    fn is_connected(&self) -> bool {
        true
    }
}

fn parse_okx_status(status: &str) -> OrderStatus {
    match status {
        "live" => OrderStatus::Open,
        "partially_filled" => OrderStatus::Partial,
        "filled" => OrderStatus::Filled,
        "canceled" | "cancelled" => OrderStatus::Cancelled,
        _ => OrderStatus::Pending,
    }
}
