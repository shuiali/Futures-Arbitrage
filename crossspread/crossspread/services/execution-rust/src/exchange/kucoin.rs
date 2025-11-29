//! KuCoin Futures adapter

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

pub struct KucoinAdapter {
    config: ExchangeConfig,
    client: Client,
}

impl KucoinAdapter {
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
        let str_to_sign = format!("{}{}{}{}", timestamp, method.to_uppercase(), path, body);
        let mut mac = HmacSha256::new_from_slice(secret.as_bytes())
            .expect("HMAC can take key of any size");
        mac.update(str_to_sign.as_bytes());
        STANDARD.encode(mac.finalize().into_bytes())
    }

    fn sign_passphrase(&self, secret: &str, passphrase: &str) -> String {
        let mut mac = HmacSha256::new_from_slice(secret.as_bytes())
            .expect("HMAC can take key of any size");
        mac.update(passphrase.as_bytes());
        STANDARD.encode(mac.finalize().into_bytes())
    }
}

#[derive(Debug, Deserialize)]
struct KucoinResponse<T> {
    code: String,
    data: Option<T>,
    msg: Option<String>,
}

#[derive(Debug, Deserialize)]
struct KucoinOrderId {
    #[serde(rename = "orderId")]
    order_id: String,
}

#[derive(Debug, Deserialize)]
struct KucoinOrderDetail {
    id: String,
    symbol: String,
    #[serde(rename = "clientOid")]
    client_oid: Option<String>,
    side: String,
    #[serde(rename = "type")]
    order_type: String,
    price: Option<String>,
    size: String,
    #[serde(rename = "filledSize")]
    filled_size: String,
    #[serde(rename = "dealFunds")]
    deal_funds: Option<String>,
    status: String,
    #[serde(rename = "createdAt")]
    created_at: i64,
}

#[async_trait]
impl ExchangeAdapter for KucoinAdapter {
    fn id(&self) -> &str {
        "kucoin"
    }

    async fn place_order(
        &self,
        credentials: &Credentials,
        request: &OrderRequest,
    ) -> Result<OrderResponse> {
        let timestamp = Self::timestamp();
        let path = "/api/v1/orders";
        
        let body = serde_json::json!({
            "symbol": request.symbol,
            "side": match request.side {
                Side::Buy => "buy",
                Side::Sell => "sell",
            },
            "type": match request.order_type {
                OrderType::Limit => "limit",
                OrderType::Market => "market",
            },
            "leverage": "5",
            "size": request.quantity.to_string(),
            "price": request.price.map(|p| p.to_string()),
            "clientOid": request.client_order_id,
            "reduceOnly": request.reduce_only,
        }).to_string();

        let signature = self.sign(&credentials.api_secret, &timestamp, "POST", path, &body);
        let passphrase = credentials.passphrase.as_deref().unwrap_or("");
        let signed_passphrase = self.sign_passphrase(&credentials.api_secret, passphrase);

        debug!("Placing KuCoin order: {}", request.symbol);

        let url = format!("{}{}", self.config.rest_url, path);
        let response = self.client
            .post(&url)
            .header("KC-API-KEY", &credentials.api_key)
            .header("KC-API-SIGN", &signature)
            .header("KC-API-TIMESTAMP", &timestamp)
            .header("KC-API-PASSPHRASE", &signed_passphrase)
            .header("KC-API-KEY-VERSION", "2")
            .header("Content-Type", "application/json")
            .body(body)
            .send()
            .await
            .context("Failed to send order request")?;

        let status = response.status();
        let body = response.text().await?;

        if !status.is_success() {
            anyhow::bail!("KuCoin order failed: {} - {}", status, body);
        }

        let resp: KucoinResponse<KucoinOrderId> = serde_json::from_str(&body)
            .context("Failed to parse order response")?;

        if resp.code != "200000" {
            anyhow::bail!("KuCoin order error: {} - {:?}", resp.code, resp.msg);
        }

        let order_id = resp.data.ok_or_else(|| anyhow::anyhow!("No order data"))?.order_id;

        info!("KuCoin order placed: {}", order_id);

        Ok(OrderResponse {
            exchange_order_id: order_id,
            client_order_id: request.client_order_id.clone(),
            symbol: request.symbol.clone(),
            side: request.side.clone(),
            order_type: request.order_type.clone(),
            price: request.price,
            quantity: request.quantity,
            filled_quantity: Decimal::ZERO,
            avg_fill_price: None,
            status: OrderStatus::Pending,
            timestamp: timestamp.parse().unwrap_or(0),
        })
    }

    async fn cancel_order(
        &self,
        credentials: &Credentials,
        symbol: &str,
        order_id: &str,
    ) -> Result<OrderResponse> {
        let timestamp = Self::timestamp();
        let path = format!("/api/v1/orders/{}", order_id);
        
        let signature = self.sign(&credentials.api_secret, &timestamp, "DELETE", &path, "");
        let passphrase = credentials.passphrase.as_deref().unwrap_or("");
        let signed_passphrase = self.sign_passphrase(&credentials.api_secret, passphrase);

        let url = format!("{}{}", self.config.rest_url, path);
        let response = self.client
            .delete(&url)
            .header("KC-API-KEY", &credentials.api_key)
            .header("KC-API-SIGN", &signature)
            .header("KC-API-TIMESTAMP", &timestamp)
            .header("KC-API-PASSPHRASE", &signed_passphrase)
            .header("KC-API-KEY-VERSION", "2")
            .send()
            .await?;

        let _body = response.text().await?;

        Ok(OrderResponse {
            exchange_order_id: order_id.to_string(),
            client_order_id: String::new(),
            symbol: symbol.to_string(),
            side: Side::Buy,
            order_type: OrderType::Limit,
            price: None,
            quantity: Decimal::ZERO,
            filled_quantity: Decimal::ZERO,
            avg_fill_price: None,
            status: OrderStatus::Cancelled,
            timestamp: timestamp.parse().unwrap_or(0),
        })
    }

    async fn get_order(
        &self,
        credentials: &Credentials,
        symbol: &str,
        order_id: &str,
    ) -> Result<OrderResponse> {
        let timestamp = Self::timestamp();
        let path = format!("/api/v1/orders/{}", order_id);
        
        let signature = self.sign(&credentials.api_secret, &timestamp, "GET", &path, "");
        let passphrase = credentials.passphrase.as_deref().unwrap_or("");
        let signed_passphrase = self.sign_passphrase(&credentials.api_secret, passphrase);

        let url = format!("{}{}", self.config.rest_url, path);
        let response = self.client
            .get(&url)
            .header("KC-API-KEY", &credentials.api_key)
            .header("KC-API-SIGN", &signature)
            .header("KC-API-TIMESTAMP", &timestamp)
            .header("KC-API-PASSPHRASE", &signed_passphrase)
            .header("KC-API-KEY-VERSION", "2")
            .send()
            .await?;

        let body = response.text().await?;
        let resp: KucoinResponse<KucoinOrderDetail> = serde_json::from_str(&body)?;

        let order = resp.data.ok_or_else(|| anyhow::anyhow!("No order data"))?;

        Ok(OrderResponse {
            exchange_order_id: order.id,
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
            price: order.price.and_then(|p| p.parse().ok()),
            quantity: order.size.parse().unwrap_or_default(),
            filled_quantity: order.filled_size.parse().unwrap_or_default(),
            avg_fill_price: order.deal_funds.and_then(|f| f.parse().ok()),
            status: parse_kucoin_status(&order.status),
            timestamp: order.created_at,
        })
    }

    async fn get_best_price(&self, symbol: &str) -> Result<(Decimal, Decimal)> {
        let url = format!("{}/api/v1/ticker?symbol={}", self.config.rest_url, symbol);
        
        let response = self.client.get(&url).send().await?;
        let body = response.text().await?;
        
        #[derive(Deserialize)]
        struct Ticker {
            #[serde(rename = "bestBidPrice")]
            best_bid_price: String,
            #[serde(rename = "bestAskPrice")]
            best_ask_price: String,
        }
        
        let resp: KucoinResponse<Ticker> = serde_json::from_str(&body)?;
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

fn parse_kucoin_status(status: &str) -> OrderStatus {
    match status {
        "open" | "new" => OrderStatus::Open,
        "match" | "partial" => OrderStatus::Partial,
        "done" | "filled" => OrderStatus::Filled,
        "canceled" | "cancelled" => OrderStatus::Cancelled,
        _ => OrderStatus::Pending,
    }
}
