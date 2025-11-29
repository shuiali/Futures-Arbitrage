//! BingX Futures adapter

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

pub struct BingxAdapter {
    config: ExchangeConfig,
    client: Client,
}

impl BingxAdapter {
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

    fn sign(&self, secret: &str, query: &str) -> String {
        let mut mac = HmacSha256::new_from_slice(secret.as_bytes())
            .expect("HMAC can take key of any size");
        mac.update(query.as_bytes());
        hex::encode(mac.finalize().into_bytes())
    }
}

#[derive(Debug, Deserialize)]
struct BingxResponse<T> {
    code: i32,
    msg: Option<String>,
    data: Option<T>,
}

#[derive(Debug, Deserialize)]
struct BingxOrderResponse {
    order: BingxOrder,
}

#[derive(Debug, Deserialize)]
struct BingxOrder {
    #[serde(rename = "orderId")]
    order_id: String,
    symbol: String,
    #[serde(rename = "clientOrderId")]
    client_order_id: Option<String>,
    side: String,
    #[serde(rename = "type")]
    order_type: String,
    price: Option<String>,
    #[serde(rename = "origQty")]
    orig_qty: String,
    #[serde(rename = "executedQty")]
    executed_qty: String,
    #[serde(rename = "avgPrice")]
    avg_price: Option<String>,
    status: String,
    time: i64,
}

#[async_trait]
impl ExchangeAdapter for BingxAdapter {
    fn id(&self) -> &str {
        "bingx"
    }

    async fn place_order(
        &self,
        credentials: &Credentials,
        request: &OrderRequest,
    ) -> Result<OrderResponse> {
        let timestamp = Self::timestamp();
        
        let mut params = vec![
            ("symbol", request.symbol.clone()),
            ("side", match request.side {
                Side::Buy => "BUY".to_string(),
                Side::Sell => "SELL".to_string(),
            }),
            ("type", match request.order_type {
                OrderType::Limit => "LIMIT".to_string(),
                OrderType::Market => "MARKET".to_string(),
            }),
            ("quantity", request.quantity.to_string()),
            ("timestamp", timestamp.to_string()),
        ];

        if let Some(price) = request.price {
            params.push(("price", price.to_string()));
        }
        if !request.client_order_id.is_empty() {
            params.push(("clientOrderId", request.client_order_id.clone()));
        }

        params.sort_by(|a, b| a.0.cmp(b.0));
        let query_string = params.iter()
            .map(|(k, v)| format!("{}={}", k, v))
            .collect::<Vec<_>>()
            .join("&");

        let signature = self.sign(&credentials.api_secret, &query_string);
        let final_query = format!("{}&signature={}", query_string, signature);

        debug!("Placing BingX order: {}", request.symbol);

        let url = format!("{}/openApi/swap/v2/trade/order?{}", self.config.rest_url, final_query);
        let response = self.client
            .post(&url)
            .header("X-BX-APIKEY", &credentials.api_key)
            .header("Content-Type", "application/json")
            .send()
            .await
            .context("Failed to send order request")?;

        let status = response.status();
        let body = response.text().await?;

        if !status.is_success() {
            anyhow::bail!("BingX order failed: {} - {}", status, body);
        }

        let resp: BingxResponse<BingxOrderResponse> = serde_json::from_str(&body)
            .context("Failed to parse order response")?;

        if resp.code != 0 {
            anyhow::bail!("BingX order error: {} - {:?}", resp.code, resp.msg);
        }

        let order = resp.data.ok_or_else(|| anyhow::anyhow!("No order data"))?.order;

        info!("BingX order placed: {} status={}", order.order_id, order.status);

        Ok(OrderResponse {
            exchange_order_id: order.order_id,
            client_order_id: order.client_order_id.unwrap_or_default(),
            symbol: order.symbol,
            side: match order.side.as_str() {
                "BUY" => Side::Buy,
                _ => Side::Sell,
            },
            order_type: match order.order_type.as_str() {
                "LIMIT" => OrderType::Limit,
                _ => OrderType::Market,
            },
            price: order.price.and_then(|p| p.parse().ok()),
            quantity: order.orig_qty.parse().unwrap_or_default(),
            filled_quantity: order.executed_qty.parse().unwrap_or_default(),
            avg_fill_price: order.avg_price.and_then(|p| p.parse().ok()),
            status: parse_bingx_status(&order.status),
            timestamp: order.time,
        })
    }

    async fn cancel_order(
        &self,
        credentials: &Credentials,
        symbol: &str,
        order_id: &str,
    ) -> Result<OrderResponse> {
        let timestamp = Self::timestamp();
        
        let query_string = format!("orderId={}&symbol={}&timestamp={}", order_id, symbol, timestamp);
        let signature = self.sign(&credentials.api_secret, &query_string);
        let final_query = format!("{}&signature={}", query_string, signature);

        let url = format!("{}/openApi/swap/v2/trade/order?{}", self.config.rest_url, final_query);
        let response = self.client
            .delete(&url)
            .header("X-BX-APIKEY", &credentials.api_key)
            .send()
            .await?;

        let body = response.text().await?;
        let resp: BingxResponse<BingxOrderResponse> = serde_json::from_str(&body)?;

        let order = resp.data.ok_or_else(|| anyhow::anyhow!("No order data"))?.order;

        Ok(OrderResponse {
            exchange_order_id: order.order_id,
            client_order_id: order.client_order_id.unwrap_or_default(),
            symbol: order.symbol,
            side: match order.side.as_str() {
                "BUY" => Side::Buy,
                _ => Side::Sell,
            },
            order_type: OrderType::Limit,
            price: order.price.and_then(|p| p.parse().ok()),
            quantity: order.orig_qty.parse().unwrap_or_default(),
            filled_quantity: order.executed_qty.parse().unwrap_or_default(),
            avg_fill_price: order.avg_price.and_then(|p| p.parse().ok()),
            status: OrderStatus::Cancelled,
            timestamp: order.time,
        })
    }

    async fn get_order(
        &self,
        credentials: &Credentials,
        symbol: &str,
        order_id: &str,
    ) -> Result<OrderResponse> {
        let timestamp = Self::timestamp();
        
        let query_string = format!("orderId={}&symbol={}&timestamp={}", order_id, symbol, timestamp);
        let signature = self.sign(&credentials.api_secret, &query_string);
        let final_query = format!("{}&signature={}", query_string, signature);

        let url = format!("{}/openApi/swap/v2/trade/order?{}", self.config.rest_url, final_query);
        let response = self.client
            .get(&url)
            .header("X-BX-APIKEY", &credentials.api_key)
            .send()
            .await?;

        let body = response.text().await?;
        let resp: BingxResponse<BingxOrderResponse> = serde_json::from_str(&body)?;

        let order = resp.data.ok_or_else(|| anyhow::anyhow!("No order data"))?.order;

        Ok(OrderResponse {
            exchange_order_id: order.order_id,
            client_order_id: order.client_order_id.unwrap_or_default(),
            symbol: order.symbol,
            side: match order.side.as_str() {
                "BUY" => Side::Buy,
                _ => Side::Sell,
            },
            order_type: match order.order_type.as_str() {
                "LIMIT" => OrderType::Limit,
                _ => OrderType::Market,
            },
            price: order.price.and_then(|p| p.parse().ok()),
            quantity: order.orig_qty.parse().unwrap_or_default(),
            filled_quantity: order.executed_qty.parse().unwrap_or_default(),
            avg_fill_price: order.avg_price.and_then(|p| p.parse().ok()),
            status: parse_bingx_status(&order.status),
            timestamp: order.time,
        })
    }

    async fn get_best_price(&self, symbol: &str) -> Result<(Decimal, Decimal)> {
        let url = format!("{}/openApi/swap/v2/quote/ticker?symbol={}", self.config.rest_url, symbol);
        
        let response = self.client.get(&url).send().await?;
        let body = response.text().await?;
        
        #[derive(Deserialize)]
        struct TickerData {
            #[serde(rename = "bidPrice")]
            bid_price: String,
            #[serde(rename = "askPrice")]
            ask_price: String,
        }
        
        let resp: BingxResponse<TickerData> = serde_json::from_str(&body)?;
        let ticker = resp.data.ok_or_else(|| anyhow::anyhow!("No ticker data"))?;

        Ok((
            ticker.bid_price.parse()?,
            ticker.ask_price.parse()?,
        ))
    }

    fn is_connected(&self) -> bool {
        true
    }
}

fn parse_bingx_status(status: &str) -> OrderStatus {
    match status {
        "NEW" | "PENDING" => OrderStatus::Open,
        "PARTIALLY_FILLED" => OrderStatus::Partial,
        "FILLED" => OrderStatus::Filled,
        "CANCELED" | "CANCELLED" => OrderStatus::Cancelled,
        _ => OrderStatus::Pending,
    }
}
