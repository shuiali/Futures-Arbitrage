//! Order execution server
//!
//! Handles order requests from the backend API via Redis

use anyhow::Result;
use redis::aio::ConnectionManager;
use redis::AsyncCommands;
use rust_decimal::Decimal;
use serde::{Deserialize, Serialize};
use std::collections::HashMap;
use std::sync::Arc;
use tokio::sync::RwLock;
use tracing::{error, info, warn};
use uuid::Uuid;

use crate::config::Config;
use crate::crypto::decrypt_credentials;
use crate::exchange::{Credentials, ExchangeAdapter, Side};
use crate::slicer::{OrderSlicer, SlicingConfig};

/// Trade entry request from backend
#[derive(Debug, Clone, Deserialize)]
pub struct TradeEntryRequest {
    pub trade_id: Uuid,
    pub user_id: Uuid,
    pub spread_id: Uuid,
    pub size_in_coins: Decimal,
    pub slicing: SlicingParams,
    pub mode: ExecutionMode,
    
    // Long leg
    pub long_exchange_id: String,
    pub long_symbol: String,
    pub long_api_key_id: Uuid,
    
    // Short leg
    pub short_exchange_id: String,
    pub short_symbol: String,
    pub short_api_key_id: Uuid,
}

#[derive(Debug, Clone, Deserialize)]
pub struct SlicingParams {
    pub slice_size_coins: Option<Decimal>,
    pub slice_interval_ms: Option<u64>,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum ExecutionMode {
    Live,
    Sim,
}

/// Trade exit request
#[derive(Debug, Clone, Deserialize)]
pub struct TradeExitRequest {
    pub trade_id: Uuid,
    pub position_id: Uuid,
    pub is_emergency: bool,
    
    // Long leg (need to sell)
    pub long_exchange_id: String,
    pub long_symbol: String,
    pub long_quantity: Decimal,
    pub long_api_key_id: Uuid,
    
    // Short leg (need to buy)
    pub short_exchange_id: String,
    pub short_symbol: String,
    pub short_quantity: Decimal,
    pub short_api_key_id: Uuid,
}

/// Execution result to send back
#[derive(Debug, Clone, Serialize)]
pub struct ExecutionResult {
    pub trade_id: Uuid,
    pub success: bool,
    pub long_filled: Decimal,
    pub long_avg_price: Decimal,
    pub short_filled: Decimal,
    pub short_avg_price: Decimal,
    pub error: Option<String>,
}

/// Execution server
pub struct ExecutionServer {
    adapters: HashMap<String, Arc<dyn ExchangeAdapter>>,
    config: Config,
    redis: Option<ConnectionManager>,
    api_key_cache: Arc<RwLock<HashMap<Uuid, CachedCredentials>>>,
}

struct CachedCredentials {
    credentials: Credentials,
    expires_at: std::time::Instant,
}

impl ExecutionServer {
    pub fn new(adapters: Vec<Box<dyn ExchangeAdapter>>, config: Config) -> Self {
        let mut adapter_map = HashMap::new();
        for adapter in adapters {
            let id = adapter.id().to_string();
            adapter_map.insert(id, Arc::from(adapter));
        }

        Self {
            adapters: adapter_map,
            config,
            redis: None,
            api_key_cache: Arc::new(RwLock::new(HashMap::new())),
        }
    }

    pub async fn run(&self) -> Result<()> {
        info!("Starting execution server on port {}", self.config.port);

        // Connect to Redis
        let redis_client = redis::Client::open(self.config.redis_url.as_str())?;
        let mut conn = redis_client.get_connection_manager().await?;

        info!("Connected to Redis, listening for execution requests");

        // Listen on execution request stream
        loop {
            let result: redis::streams::StreamReadReply = conn
                .xread_options(
                    &["execution:requests"],
                    &["$"],
                    &redis::streams::StreamReadOptions::default()
                        .block(5000)
                        .count(10),
                )
                .await?;

            for stream in result.keys {
                for id_and_data in stream.ids {
                    self.handle_request(&mut conn, &id_and_data).await;
                }
            }
        }
    }

    async fn handle_request(
        &self,
        conn: &mut ConnectionManager,
        entry: &redis::streams::StreamId,
    ) {
        // Extract data from the stream entry - handle various redis Value types
        let data: Vec<u8> = match entry.map.get("data") {
            Some(value) => {
                match redis::from_redis_value::<Vec<u8>>(value) {
                    Ok(d) => d,
                    Err(_) => {
                        // Try as string
                        match redis::from_redis_value::<String>(value) {
                            Ok(s) => s.into_bytes(),
                            Err(_) => {
                                warn!("Invalid message format");
                                return;
                            }
                        }
                    }
                }
            }
            None => {
                warn!("No data field in message");
                return;
            }
        };

        let data_str = match std::str::from_utf8(&data) {
            Ok(s) => s,
            Err(_) => {
                warn!("Invalid UTF-8 in message");
                return;
            }
        };

        // Try to parse as entry request
        if let Ok(request) = serde_json::from_str::<TradeEntryRequest>(data_str) {
            let result = self.execute_entry(request).await;
            self.publish_result(conn, &result).await;
            return;
        }

        // Try to parse as exit request
        if let Ok(request) = serde_json::from_str::<TradeExitRequest>(data_str) {
            let result = self.execute_exit(request).await;
            self.publish_result(conn, &result).await;
            return;
        }

        warn!("Unknown request format");
    }

    async fn execute_entry(&self, request: TradeEntryRequest) -> ExecutionResult {
        info!("Executing trade entry: {}", request.trade_id);

        if request.mode == ExecutionMode::Sim {
            return self.simulate_entry(&request);
        }

        // Get adapters
        let long_adapter = match self.adapters.get(&request.long_exchange_id) {
            Some(a) => a.clone(),
            None => {
                return ExecutionResult {
                    trade_id: request.trade_id,
                    success: false,
                    long_filled: Decimal::ZERO,
                    long_avg_price: Decimal::ZERO,
                    short_filled: Decimal::ZERO,
                    short_avg_price: Decimal::ZERO,
                    error: Some(format!("Unknown exchange: {}", request.long_exchange_id)),
                };
            }
        };

        let short_adapter = match self.adapters.get(&request.short_exchange_id) {
            Some(a) => a.clone(),
            None => {
                return ExecutionResult {
                    trade_id: request.trade_id,
                    success: false,
                    long_filled: Decimal::ZERO,
                    long_avg_price: Decimal::ZERO,
                    short_filled: Decimal::ZERO,
                    short_avg_price: Decimal::ZERO,
                    error: Some(format!("Unknown exchange: {}", request.short_exchange_id)),
                };
            }
        };

        // TODO: Fetch credentials from database
        // For now, return error indicating credentials needed
        ExecutionResult {
            trade_id: request.trade_id,
            success: false,
            long_filled: Decimal::ZERO,
            long_avg_price: Decimal::ZERO,
            short_filled: Decimal::ZERO,
            short_avg_price: Decimal::ZERO,
            error: Some("Credential loading not yet implemented".to_string()),
        }
    }

    async fn execute_exit(&self, request: TradeExitRequest) -> ExecutionResult {
        info!(
            "Executing trade exit: {} (emergency: {})",
            request.trade_id, request.is_emergency
        );

        // Similar to entry but with reverse sides
        ExecutionResult {
            trade_id: request.trade_id,
            success: false,
            long_filled: Decimal::ZERO,
            long_avg_price: Decimal::ZERO,
            short_filled: Decimal::ZERO,
            short_avg_price: Decimal::ZERO,
            error: Some("Exit execution not yet implemented".to_string()),
        }
    }

    fn simulate_entry(&self, request: &TradeEntryRequest) -> ExecutionResult {
        info!("Simulating trade entry: {}", request.trade_id);

        // In simulation mode, assume perfect fills at market price
        // Real implementation would walk the orderbook
        ExecutionResult {
            trade_id: request.trade_id,
            success: true,
            long_filled: request.size_in_coins,
            long_avg_price: Decimal::ZERO, // Would be calculated from orderbook
            short_filled: request.size_in_coins,
            short_avg_price: Decimal::ZERO,
            error: None,
        }
    }

    async fn publish_result(&self, conn: &mut ConnectionManager, result: &ExecutionResult) {
        let data = match serde_json::to_string(result) {
            Ok(d) => d,
            Err(e) => {
                error!("Failed to serialize result: {}", e);
                return;
            }
        };

        let _: Result<(), _> = conn
            .xadd(
                "execution:results",
                "*",
                &[("data", data.as_str())],
            )
            .await;
    }
}
