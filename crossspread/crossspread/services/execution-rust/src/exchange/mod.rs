//! Exchange adapter traits and implementations

use async_trait::async_trait;
use anyhow::Result;
use rust_decimal::Decimal;
use serde::{Deserialize, Serialize};
use uuid::Uuid;

use crate::config::ExchangeConfig;

pub mod binance;
pub mod bybit;
pub mod okx;
pub mod mexc;
pub mod bitget;
pub mod kucoin;
pub mod gateio;
pub mod bingx;
pub mod coinex;
pub mod lbank;
pub mod htx;

/// Order side
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum Side {
    Buy,
    Sell,
}

/// Order type
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum OrderType {
    Limit,
    Market,
}

/// Order status
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum OrderStatus {
    Pending,
    Open,
    Partial,
    Filled,
    Cancelled,
    Rejected,
    Expired,
}

/// Order request to place on exchange
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct OrderRequest {
    pub client_order_id: String,
    pub symbol: String,
    pub side: Side,
    pub order_type: OrderType,
    pub price: Option<Decimal>,
    pub quantity: Decimal,
    pub reduce_only: bool,
}

/// Order response from exchange
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct OrderResponse {
    pub exchange_order_id: String,
    pub client_order_id: String,
    pub symbol: String,
    pub side: Side,
    pub order_type: OrderType,
    pub price: Option<Decimal>,
    pub quantity: Decimal,
    pub filled_quantity: Decimal,
    pub avg_fill_price: Option<Decimal>,
    pub status: OrderStatus,
    pub timestamp: i64,
}

/// Credentials for exchange API
#[derive(Debug, Clone)]
pub struct Credentials {
    pub api_key: String,
    pub api_secret: String,
    pub passphrase: Option<String>, // For OKX
}

/// Exchange adapter trait
#[async_trait]
pub trait ExchangeAdapter: Send + Sync {
    /// Get exchange ID
    fn id(&self) -> &str;

    /// Place a limit order
    async fn place_order(
        &self,
        credentials: &Credentials,
        request: &OrderRequest,
    ) -> Result<OrderResponse>;

    /// Cancel an order
    async fn cancel_order(
        &self,
        credentials: &Credentials,
        symbol: &str,
        order_id: &str,
    ) -> Result<OrderResponse>;

    /// Get order status
    async fn get_order(
        &self,
        credentials: &Credentials,
        symbol: &str,
        order_id: &str,
    ) -> Result<OrderResponse>;

    /// Get current best bid/ask for a symbol
    async fn get_best_price(&self, symbol: &str) -> Result<(Decimal, Decimal)>;

    /// Check if connected
    fn is_connected(&self) -> bool;
}

/// Create an exchange adapter from config
pub async fn create_adapter(config: &ExchangeConfig) -> Result<Box<dyn ExchangeAdapter>> {
    match config.id.as_str() {
        "binance" => Ok(Box::new(binance::BinanceAdapter::new(config.clone()).await?)),
        "bybit" => Ok(Box::new(bybit::BybitAdapter::new(config.clone()).await?)),
        "okx" => Ok(Box::new(okx::OkxAdapter::new(config.clone()).await?)),
        "mexc" => Ok(Box::new(mexc::MexcAdapter::new(config.clone()).await?)),
        "bitget" => Ok(Box::new(bitget::BitgetAdapter::new(config.clone()).await?)),
        "kucoin" => Ok(Box::new(kucoin::KucoinAdapter::new(config.clone()).await?)),
        "gateio" => Ok(Box::new(gateio::GateioAdapter::new(config.clone()).await?)),
        "bingx" => Ok(Box::new(bingx::BingxAdapter::new(config.clone()).await?)),
        "coinex" => Ok(Box::new(coinex::CoinexAdapter::new(config.clone()).await?)),
        "lbank" => Ok(Box::new(lbank::LbankAdapter::new(config.clone()).await?)),
        "htx" => Ok(Box::new(htx::HtxAdapter::new(config.clone()).await?)),
        _ => anyhow::bail!("Unknown exchange: {}", config.id),
    }
}

/// Generate a unique client order ID
pub fn generate_client_order_id() -> String {
    format!("cs_{}", Uuid::new_v4().to_string().replace("-", "")[..16].to_string())
}
