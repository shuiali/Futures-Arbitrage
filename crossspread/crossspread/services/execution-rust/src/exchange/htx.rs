//! HTX (Huobi) Futures adapter

use anyhow::{Context, Result};
use async_trait::async_trait;
use base64::{engine::general_purpose::STANDARD, Engine};
use chrono::Utc;
use hmac::{Hmac, Mac};
use reqwest::Client;
use rust_decimal::Decimal;
use serde::Deserialize;
use sha2::Sha256;
use tracing::{debug, info};

use super::{Credentials, ExchangeAdapter, OrderRequest, OrderResponse, OrderStatus, OrderType, Side};
use crate::config::ExchangeConfig;

type HmacSha256 = Hmac<Sha256>;

pub struct HtxAdapter {
    config: ExchangeConfig,
    client: Client,
}

impl HtxAdapter {
    pub async fn new(config: ExchangeConfig) -> Result<Self> {
        let client = Client::builder()
            .timeout(std::time::Duration::from_secs(10))
            .build()?;

        Ok(Self { config, client })
    }

    fn timestamp() -> String {
        Utc::now().format("%Y-%m-%dT%H:%M:%S").to_string()
    }

    fn sign(&self, api_key: &str, secret: &str, method: &str, host: &str, path: &str, timestamp: &str) -> String {
        let params = format!(
            "AccessKeyId={}&SignatureMethod=HmacSHA256&SignatureVersion=2&Timestamp={}",
            api_key,
            urlencoding::encode(timestamp)
        );
        
        let payload = format!("{}\n{}\n{}\n{}", method.to_uppercase(), host, path, params);
        
        let mut mac = HmacSha256::new_from_slice(secret.as_bytes())
            .expect("HMAC can take key of any size");
        mac.update(payload.as_bytes());
        STANDARD.encode(mac.finalize().into_bytes())
    }

    fn get_host(&self) -> &str {
        // Extract host from rest_url
        if self.config.rest_url.contains("huobi") {
            "api.huobi.pro"
        } else {
            "api.htx.com"
        }
    }
}

#[derive(Debug, Deserialize)]
struct HtxResponse<T> {
    status: String,
    data: Option<T>,
    #[serde(rename = "err-code")]
    err_code: Option<String>,
    #[serde(rename = "err-msg")]
    err_msg: Option<String>,
}

#[derive(Debug, Deserialize)]
struct HtxOrderId {
    order_id: i64,
    order_id_str: String,
}

#[derive(Debug, Deserialize)]
struct HtxOrderDetail {
    order_id: i64,
    order_id_str: String,
    symbol: String,
    contract_code: String,
    direction: String,
    offset: String,
    price: f64,
    volume: i64,
    trade_volume: i64,
    trade_avg_price: Option<f64>,
    status: i32,
    created_at: i64,
    client_order_id: Option<i64>,
}

#[async_trait]
impl ExchangeAdapter for HtxAdapter {
    fn id(&self) -> &str {
        "htx"
    }

    async fn place_order(
        &self,
        credentials: &Credentials,
        request: &OrderRequest,
    ) -> Result<OrderResponse> {
        let timestamp = Self::timestamp();
        let path = "/linear-swap-api/v1/swap_cross_order";
        let host = self.get_host();
        
        let signature = self.sign(
            &credentials.api_key,
            &credentials.api_secret,
            "POST",
            host,
            path,
            &timestamp
        );

        let body = serde_json::json!({
            "contract_code": request.symbol,
            "direction": match request.side {
                Side::Buy => "buy",
                Side::Sell => "sell",
            },
            "offset": "open",
            "order_price_type": match request.order_type {
                OrderType::Limit => "limit",
                OrderType::Market => "optimal_20",
            },
            "volume": request.quantity.to_string().parse::<i64>().unwrap_or(1),
            "price": request.price,
            "lever_rate": 5,
            "reduce_only": if request.reduce_only { 1 } else { 0 },
        }).to_string();

        let url = format!(
            "{}{}?AccessKeyId={}&SignatureMethod=HmacSHA256&SignatureVersion=2&Timestamp={}&Signature={}",
            self.config.rest_url,
            path,
            &credentials.api_key,
            urlencoding::encode(&timestamp),
            urlencoding::encode(&signature)
        );

        debug!("Placing HTX order: {}", request.symbol);

        let response = self.client
            .post(&url)
            .header("Content-Type", "application/json")
            .body(body)
            .send()
            .await
            .context("Failed to send order request")?;

        let status = response.status();
        let body = response.text().await?;

        if !status.is_success() {
            anyhow::bail!("HTX order failed: {} - {}", status, body);
        }

        let resp: HtxResponse<HtxOrderId> = serde_json::from_str(&body)
            .context("Failed to parse order response")?;

        if resp.status != "ok" {
            anyhow::bail!("HTX order error: {:?} - {:?}", resp.err_code, resp.err_msg);
        }

        let order = resp.data.ok_or_else(|| anyhow::anyhow!("No order data"))?;

        info!("HTX order placed: {}", order.order_id_str);

        Ok(OrderResponse {
            exchange_order_id: order.order_id_str,
            client_order_id: request.client_order_id.clone(),
            symbol: request.symbol.clone(),
            side: request.side.clone(),
            order_type: request.order_type.clone(),
            price: request.price,
            quantity: request.quantity,
            filled_quantity: Decimal::ZERO,
            avg_fill_price: None,
            status: OrderStatus::Pending,
            timestamp: chrono::Utc::now().timestamp_millis(),
        })
    }

    async fn cancel_order(
        &self,
        credentials: &Credentials,
        symbol: &str,
        order_id: &str,
    ) -> Result<OrderResponse> {
        let timestamp = Self::timestamp();
        let path = "/linear-swap-api/v1/swap_cross_cancel";
        let host = self.get_host();
        
        let signature = self.sign(
            &credentials.api_key,
            &credentials.api_secret,
            "POST",
            host,
            path,
            &timestamp
        );

        let body = serde_json::json!({
            "contract_code": symbol,
            "order_id": order_id,
        }).to_string();

        let url = format!(
            "{}{}?AccessKeyId={}&SignatureMethod=HmacSHA256&SignatureVersion=2&Timestamp={}&Signature={}",
            self.config.rest_url,
            path,
            &credentials.api_key,
            urlencoding::encode(&timestamp),
            urlencoding::encode(&signature)
        );

        let response = self.client
            .post(&url)
            .header("Content-Type", "application/json")
            .body(body)
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
            timestamp: chrono::Utc::now().timestamp_millis(),
        })
    }

    async fn get_order(
        &self,
        credentials: &Credentials,
        symbol: &str,
        order_id: &str,
    ) -> Result<OrderResponse> {
        let timestamp = Self::timestamp();
        let path = "/linear-swap-api/v1/swap_cross_order_info";
        let host = self.get_host();
        
        let signature = self.sign(
            &credentials.api_key,
            &credentials.api_secret,
            "POST",
            host,
            path,
            &timestamp
        );

        let body = serde_json::json!({
            "contract_code": symbol,
            "order_id": order_id,
        }).to_string();

        let url = format!(
            "{}{}?AccessKeyId={}&SignatureMethod=HmacSHA256&SignatureVersion=2&Timestamp={}&Signature={}",
            self.config.rest_url,
            path,
            &credentials.api_key,
            urlencoding::encode(&timestamp),
            urlencoding::encode(&signature)
        );

        let response = self.client
            .post(&url)
            .header("Content-Type", "application/json")
            .body(body)
            .send()
            .await?;

        let body = response.text().await?;
        let resp: HtxResponse<Vec<HtxOrderDetail>> = serde_json::from_str(&body)?;

        let orders = resp.data.ok_or_else(|| anyhow::anyhow!("No order data"))?;
        let order = orders.into_iter().next()
            .ok_or_else(|| anyhow::anyhow!("Order not found"))?;

        Ok(OrderResponse {
            exchange_order_id: order.order_id_str,
            client_order_id: order.client_order_id.map(|c| c.to_string()).unwrap_or_default(),
            symbol: order.contract_code,
            side: match order.direction.as_str() {
                "buy" => Side::Buy,
                _ => Side::Sell,
            },
            order_type: OrderType::Limit,
            price: Some(Decimal::from_f64_retain(order.price).unwrap_or_default()),
            quantity: Decimal::from(order.volume),
            filled_quantity: Decimal::from(order.trade_volume),
            avg_fill_price: order.trade_avg_price.and_then(Decimal::from_f64_retain),
            status: parse_htx_status(order.status),
            timestamp: order.created_at,
        })
    }

    async fn get_best_price(&self, symbol: &str) -> Result<(Decimal, Decimal)> {
        let url = format!("{}/linear-swap-ex/market/depth?contract_code={}&type=step0", 
            self.config.rest_url, symbol);
        
        let response = self.client.get(&url).send().await?;
        let body = response.text().await?;
        
        #[derive(Deserialize)]
        struct DepthData {
            bids: Vec<Vec<f64>>,
            asks: Vec<Vec<f64>>,
        }
        
        #[derive(Deserialize)]
        struct DepthResp {
            tick: DepthData,
        }
        
        let resp: DepthResp = serde_json::from_str(&body)?;
        
        let bid = resp.tick.bids.first()
            .ok_or_else(|| anyhow::anyhow!("No bid"))?[0];
        let ask = resp.tick.asks.first()
            .ok_or_else(|| anyhow::anyhow!("No ask"))?[0];

        Ok((
            Decimal::from_f64_retain(bid).unwrap_or_default(),
            Decimal::from_f64_retain(ask).unwrap_or_default(),
        ))
    }

    fn is_connected(&self) -> bool {
        true
    }
}

fn parse_htx_status(status: i32) -> OrderStatus {
    match status {
        1 | 2 => OrderStatus::Pending,  // Preparing / Submitted
        3 => OrderStatus::Open,         // Pending order
        4 => OrderStatus::Partial,      // Partially matched
        5 | 6 => OrderStatus::Cancelled, // Partially cancelled / Cancelled
        7 => OrderStatus::Filled,       // Completed
        _ => OrderStatus::Pending,
    }
}
