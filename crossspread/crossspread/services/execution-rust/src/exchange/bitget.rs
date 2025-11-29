//! Bitget Futures adapter

use anyhow::{Context, Result};
use async_trait::async_trait;
use base64::{engine::general_purpose::STANDARD, Engine};
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

pub struct BitgetAdapter {
    config: ExchangeConfig,
    client: Client,
}

impl BitgetAdapter {
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

    fn sign(&self, secret: &str, timestamp: &str, method: &str, path: &str, body: &str) -> String {
        let prehash = format!("{}{}{}{}", timestamp, method.to_uppercase(), path, body);
        let mut mac = HmacSha256::new_from_slice(secret.as_bytes())
            .expect("HMAC can take key of any size");
        mac.update(prehash.as_bytes());
        STANDARD.encode(mac.finalize().into_bytes())
    }
}

#[derive(Debug, Deserialize)]
struct BitgetResponse<T> {
    code: String,
    msg: String,
    data: Option<T>,
}

#[derive(Debug, Deserialize)]
struct BitgetOrderData {
    #[serde(rename = "orderId")]
    order_id: String,
    #[serde(rename = "clientOid")]
    client_oid: Option<String>,
    symbol: String,
    side: String,
    #[serde(rename = "orderType")]
    order_type: String,
    price: String,
    size: String,
    #[serde(rename = "filledQty")]
    filled_qty: Option<String>,
    #[serde(rename = "priceAvg")]
    price_avg: Option<String>,
    state: String,
    #[serde(rename = "cTime")]
    c_time: String,
}

#[async_trait]
impl ExchangeAdapter for BitgetAdapter {
    fn id(&self) -> &str {
        "bitget"
    }

    async fn place_order(
        &self,
        credentials: &Credentials,
        request: &OrderRequest,
    ) -> Result<OrderResponse> {
        let timestamp = Self::timestamp();
        let path = "/api/v2/mix/order/place-order";
        
        let body = serde_json::json!({
            "symbol": request.symbol,
            "productType": "USDT-FUTURES",
            "marginMode": "crossed",
            "marginCoin": "USDT",
            "side": match request.side {
                Side::Buy => "buy",
                Side::Sell => "sell",
            },
            "tradeSide": "open",
            "orderType": match request.order_type {
                OrderType::Limit => "limit",
                OrderType::Market => "market",
            },
            "size": request.quantity.to_string(),
            "price": request.price.map(|p| p.to_string()),
            "clientOid": request.client_order_id,
            "reduceOnly": request.reduce_only,
        }).to_string();

        let signature = self.sign(&credentials.api_secret, &timestamp, "POST", path, &body);
        let passphrase = credentials.passphrase.as_deref().unwrap_or("");

        debug!("Placing Bitget order: {}", request.symbol);

        let url = format!("{}{}", self.config.rest_url, path);
        let response = self.client
            .post(&url)
            .header("ACCESS-KEY", &credentials.api_key)
            .header("ACCESS-SIGN", &signature)
            .header("ACCESS-TIMESTAMP", &timestamp)
            .header("ACCESS-PASSPHRASE", passphrase)
            .header("Content-Type", "application/json")
            .body(body)
            .send()
            .await
            .context("Failed to send order request")?;

        let status = response.status();
        let body = response.text().await?;

        if !status.is_success() {
            anyhow::bail!("Bitget order failed: {} - {}", status, body);
        }

        let resp: BitgetResponse<BitgetOrderData> = serde_json::from_str(&body)
            .context("Failed to parse order response")?;

        if resp.code != "00000" {
            anyhow::bail!("Bitget order error: {} - {}", resp.code, resp.msg);
        }

        let order = resp.data.ok_or_else(|| anyhow::anyhow!("No order data"))?;

        info!("Bitget order placed: {} state={}", order.order_id, order.state);

        Ok(OrderResponse {
            exchange_order_id: order.order_id,
            client_order_id: order.client_oid.unwrap_or_default(),
            symbol: order.symbol,
            side: match order.side.as_str() {
                "buy" => Side::Buy,
                _ => Side::Sell,
            },
            order_type: match order.order_type.as_str() {
                "limit" => OrderType::Limit,
                _ => OrderType::Market,
            },
            price: order.price.parse().ok(),
            quantity: order.size.parse().unwrap_or_default(),
            filled_quantity: order.filled_qty.and_then(|s| s.parse().ok()).unwrap_or_default(),
            avg_fill_price: order.price_avg.and_then(|s| s.parse().ok()),
            status: parse_bitget_status(&order.state),
            timestamp: order.c_time.parse().unwrap_or(0),
        })
    }

    async fn cancel_order(
        &self,
        credentials: &Credentials,
        symbol: &str,
        order_id: &str,
    ) -> Result<OrderResponse> {
        let timestamp = Self::timestamp();
        let path = "/api/v2/mix/order/cancel-order";
        
        let body = serde_json::json!({
            "symbol": symbol,
            "productType": "USDT-FUTURES",
            "orderId": order_id,
        }).to_string();

        let signature = self.sign(&credentials.api_secret, &timestamp, "POST", path, &body);
        let passphrase = credentials.passphrase.as_deref().unwrap_or("");

        let url = format!("{}{}", self.config.rest_url, path);
        let response = self.client
            .post(&url)
            .header("ACCESS-KEY", &credentials.api_key)
            .header("ACCESS-SIGN", &signature)
            .header("ACCESS-TIMESTAMP", &timestamp)
            .header("ACCESS-PASSPHRASE", passphrase)
            .header("Content-Type", "application/json")
            .body(body)
            .send()
            .await?;

        let body = response.text().await?;
        let resp: BitgetResponse<BitgetOrderData> = serde_json::from_str(&body)?;

        let order = resp.data.ok_or_else(|| anyhow::anyhow!("No order data"))?;

        Ok(OrderResponse {
            exchange_order_id: order.order_id,
            client_order_id: order.client_oid.unwrap_or_default(),
            symbol: order.symbol,
            side: match order.side.as_str() {
                "buy" => Side::Buy,
                _ => Side::Sell,
            },
            order_type: OrderType::Limit,
            price: order.price.parse().ok(),
            quantity: order.size.parse().unwrap_or_default(),
            filled_quantity: order.filled_qty.and_then(|s| s.parse().ok()).unwrap_or_default(),
            avg_fill_price: order.price_avg.and_then(|s| s.parse().ok()),
            status: OrderStatus::Cancelled,
            timestamp: order.c_time.parse().unwrap_or(0),
        })
    }

    async fn get_order(
        &self,
        credentials: &Credentials,
        symbol: &str,
        order_id: &str,
    ) -> Result<OrderResponse> {
        let timestamp = Self::timestamp();
        let path = format!("/api/v2/mix/order/detail?symbol={}&productType=USDT-FUTURES&orderId={}", symbol, order_id);
        
        let signature = self.sign(&credentials.api_secret, &timestamp, "GET", &path, "");
        let passphrase = credentials.passphrase.as_deref().unwrap_or("");

        let url = format!("{}{}", self.config.rest_url, path);
        let response = self.client
            .get(&url)
            .header("ACCESS-KEY", &credentials.api_key)
            .header("ACCESS-SIGN", &signature)
            .header("ACCESS-TIMESTAMP", &timestamp)
            .header("ACCESS-PASSPHRASE", passphrase)
            .send()
            .await?;

        let body = response.text().await?;
        let resp: BitgetResponse<BitgetOrderData> = serde_json::from_str(&body)?;

        let order = resp.data.ok_or_else(|| anyhow::anyhow!("No order data"))?;

        Ok(OrderResponse {
            exchange_order_id: order.order_id,
            client_order_id: order.client_oid.unwrap_or_default(),
            symbol: order.symbol,
            side: match order.side.as_str() {
                "buy" => Side::Buy,
                _ => Side::Sell,
            },
            order_type: match order.order_type.as_str() {
                "limit" => OrderType::Limit,
                _ => OrderType::Market,
            },
            price: order.price.parse().ok(),
            quantity: order.size.parse().unwrap_or_default(),
            filled_quantity: order.filled_qty.and_then(|s| s.parse().ok()).unwrap_or_default(),
            avg_fill_price: order.price_avg.and_then(|s| s.parse().ok()),
            status: parse_bitget_status(&order.state),
            timestamp: order.c_time.parse().unwrap_or(0),
        })
    }

    async fn get_best_price(&self, symbol: &str) -> Result<(Decimal, Decimal)> {
        let url = format!("{}/api/v2/mix/market/ticker?symbol={}&productType=USDT-FUTURES", 
            self.config.rest_url, symbol);
        
        let response = self.client.get(&url).send().await?;
        let body = response.text().await?;
        
        #[derive(Deserialize)]
        struct Ticker {
            #[serde(rename = "bestBid")]
            best_bid: String,
            #[serde(rename = "bestAsk")]
            best_ask: String,
        }
        
        let resp: BitgetResponse<Vec<Ticker>> = serde_json::from_str(&body)?;
        let tickers = resp.data.ok_or_else(|| anyhow::anyhow!("No ticker data"))?;
        let ticker = tickers.into_iter().next()
            .ok_or_else(|| anyhow::anyhow!("No ticker"))?;

        Ok((
            ticker.best_bid.parse()?,
            ticker.best_ask.parse()?,
        ))
    }

    fn is_connected(&self) -> bool {
        true
    }
}

fn parse_bitget_status(state: &str) -> OrderStatus {
    match state {
        "new" | "init" => OrderStatus::Open,
        "partial-fill" | "partially_filled" => OrderStatus::Partial,
        "full-fill" | "filled" => OrderStatus::Filled,
        "cancelled" | "canceled" => OrderStatus::Cancelled,
        _ => OrderStatus::Pending,
    }
}
