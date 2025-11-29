//! MEXC Futures adapter

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

pub struct MexcAdapter {
    config: ExchangeConfig,
    client: Client,
}

impl MexcAdapter {
    pub async fn new(config: ExchangeConfig) -> Result<Self> {
        let client = Client::builder()
            .timeout(std::time::Duration::from_secs(10))
            .build()?;

        Ok(Self { config, client })
    }

    fn timestamp() -> u64 {
        SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap()
            .as_millis() as u64
    }

    fn sign(&self, secret: &str, query: &str) -> String {
        let mut mac = HmacSha256::new_from_slice(secret.as_bytes())
            .expect("HMAC can take key of any size");
        mac.update(query.as_bytes());
        hex::encode(mac.finalize().into_bytes())
    }
}

#[derive(Debug, Deserialize)]
struct MexcResponse<T> {
    code: i32,
    data: Option<T>,
    msg: Option<String>,
}

#[derive(Debug, Deserialize)]
struct MexcOrderData {
    #[serde(rename = "orderId")]
    order_id: String,
    #[serde(rename = "clientOrderId")]
    client_order_id: Option<String>,
    symbol: String,
    side: i32,  // 1=open long, 2=close short, 3=open short, 4=close long
    #[serde(rename = "orderType")]
    order_type: i32,
    price: String,
    vol: String,
    #[serde(rename = "dealVol")]
    deal_vol: String,
    #[serde(rename = "dealAvgPrice")]
    deal_avg_price: String,
    state: i32,  // 1=pending, 2=filled, 3=partial, 4=cancelled
    #[serde(rename = "createTime")]
    create_time: i64,
}

#[async_trait]
impl ExchangeAdapter for MexcAdapter {
    fn id(&self) -> &str {
        "mexc"
    }

    async fn place_order(
        &self,
        credentials: &Credentials,
        request: &OrderRequest,
    ) -> Result<OrderResponse> {
        let timestamp = Self::timestamp();
        
        // MEXC uses different side codes for futures
        let side = match request.side {
            Side::Buy => 1,  // Open long
            Side::Sell => 3, // Open short
        };

        let order_type = match request.order_type {
            OrderType::Limit => 1,
            OrderType::Market => 5,
        };

        let mut params = vec![
            format!("symbol={}", request.symbol),
            format!("side={}", side),
            format!("openType=2"),  // Cross margin
            format!("type={}", order_type),
            format!("vol={}", request.quantity),
            format!("timestamp={}", timestamp),
        ];

        if let Some(price) = &request.price {
            params.push(format!("price={}", price));
        }

        if !request.client_order_id.is_empty() {
            params.push(format!("externalOid={}", request.client_order_id));
        }

        let query = params.join("&");
        let signature = self.sign(&credentials.api_secret, &query);

        debug!("Placing MEXC order: {}", request.symbol);

        let url = format!("{}/api/v1/private/order/submit", self.config.rest_url);
        let response = self.client
            .post(&url)
            .header("ApiKey", &credentials.api_key)
            .header("Request-Time", timestamp.to_string())
            .header("Signature", &signature)
            .header("Content-Type", "application/json")
            .query(&[("signature", &signature)])
            .body(query)
            .send()
            .await
            .context("Failed to send order request")?;

        let status = response.status();
        let body = response.text().await?;

        if !status.is_success() {
            anyhow::bail!("MEXC order failed: {} - {}", status, body);
        }

        let resp: MexcResponse<MexcOrderData> = serde_json::from_str(&body)
            .context("Failed to parse order response")?;

        if resp.code != 0 {
            anyhow::bail!("MEXC order error: {} - {:?}", resp.code, resp.msg);
        }

        let order = resp.data.ok_or_else(|| anyhow::anyhow!("No order data"))?;

        info!("MEXC order placed: {} state={}", order.order_id, order.state);

        Ok(OrderResponse {
            exchange_order_id: order.order_id,
            client_order_id: order.client_order_id.unwrap_or_default(),
            symbol: order.symbol,
            side: if order.side == 1 || order.side == 2 { Side::Buy } else { Side::Sell },
            order_type: if order.order_type == 1 { OrderType::Limit } else { OrderType::Market },
            price: order.price.parse().ok(),
            quantity: order.vol.parse().unwrap_or_default(),
            filled_quantity: order.deal_vol.parse().unwrap_or_default(),
            avg_fill_price: order.deal_avg_price.parse().ok(),
            status: parse_mexc_status(order.state),
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
        
        let query = format!("symbol={}&orderId={}&timestamp={}", symbol, order_id, timestamp);
        let signature = self.sign(&credentials.api_secret, &query);

        let url = format!("{}/api/v1/private/order/cancel", self.config.rest_url);
        let response = self.client
            .post(&url)
            .header("ApiKey", &credentials.api_key)
            .header("Request-Time", timestamp.to_string())
            .header("Signature", &signature)
            .query(&[("signature", &signature)])
            .body(query)
            .send()
            .await?;

        let body = response.text().await?;
        let resp: MexcResponse<MexcOrderData> = serde_json::from_str(&body)?;

        let order = resp.data.ok_or_else(|| anyhow::anyhow!("No order data"))?;

        Ok(OrderResponse {
            exchange_order_id: order.order_id,
            client_order_id: order.client_order_id.unwrap_or_default(),
            symbol: order.symbol,
            side: if order.side == 1 || order.side == 2 { Side::Buy } else { Side::Sell },
            order_type: OrderType::Limit,
            price: order.price.parse().ok(),
            quantity: order.vol.parse().unwrap_or_default(),
            filled_quantity: order.deal_vol.parse().unwrap_or_default(),
            avg_fill_price: order.deal_avg_price.parse().ok(),
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
        
        let query = format!("symbol={}&order_id={}&timestamp={}", symbol, order_id, timestamp);
        let signature = self.sign(&credentials.api_secret, &query);

        let url = format!("{}/api/v1/private/order/get/{}", self.config.rest_url, order_id);
        let response = self.client
            .get(&url)
            .header("ApiKey", &credentials.api_key)
            .header("Request-Time", timestamp.to_string())
            .header("Signature", &signature)
            .send()
            .await?;

        let body = response.text().await?;
        let resp: MexcResponse<MexcOrderData> = serde_json::from_str(&body)?;

        let order = resp.data.ok_or_else(|| anyhow::anyhow!("No order data"))?;

        Ok(OrderResponse {
            exchange_order_id: order.order_id,
            client_order_id: order.client_order_id.unwrap_or_default(),
            symbol: order.symbol,
            side: if order.side == 1 || order.side == 2 { Side::Buy } else { Side::Sell },
            order_type: if order.order_type == 1 { OrderType::Limit } else { OrderType::Market },
            price: order.price.parse().ok(),
            quantity: order.vol.parse().unwrap_or_default(),
            filled_quantity: order.deal_vol.parse().unwrap_or_default(),
            avg_fill_price: order.deal_avg_price.parse().ok(),
            status: parse_mexc_status(order.state),
            timestamp: order.create_time,
        })
    }

    async fn get_best_price(&self, symbol: &str) -> Result<(Decimal, Decimal)> {
        let url = format!("{}/api/v1/contract/ticker?symbol={}", self.config.rest_url, symbol);
        
        let response = self.client.get(&url).send().await?;
        let body = response.text().await?;
        
        #[derive(Deserialize)]
        struct Ticker {
            #[serde(rename = "bid1")]
            bid: String,
            #[serde(rename = "ask1")]
            ask: String,
        }
        
        let resp: MexcResponse<Ticker> = serde_json::from_str(&body)?;
        let ticker = resp.data.ok_or_else(|| anyhow::anyhow!("No ticker data"))?;

        Ok((
            ticker.bid.parse()?,
            ticker.ask.parse()?,
        ))
    }

    fn is_connected(&self) -> bool {
        true
    }
}

fn parse_mexc_status(state: i32) -> OrderStatus {
    match state {
        1 => OrderStatus::Pending,
        2 => OrderStatus::Filled,
        3 => OrderStatus::Partial,
        4 => OrderStatus::Cancelled,
        _ => OrderStatus::Pending,
    }
}
