//! Configuration module

use anyhow::{Context, Result};
use std::env;

#[derive(Clone, Debug)]
pub struct Config {
    pub port: u16,
    pub redis_url: String,
    pub database_url: String,
    pub encryption_key: Vec<u8>,
    pub exchanges: Vec<ExchangeConfig>,
    pub default_slice_percent: f64,
    pub default_slice_interval_ms: u64,
    pub max_parallel_slices: usize,
}

#[derive(Clone, Debug)]
pub struct ExchangeConfig {
    pub id: String,
    pub rest_url: String,
    pub ws_url: String,
    pub testnet: bool,
}

impl Config {
    pub fn from_env() -> Result<Self> {
        let port = env::var("EXEC_SERVICE_PORT")
            .unwrap_or_else(|_| "9000".to_string())
            .parse()
            .context("Invalid EXEC_SERVICE_PORT")?;

        let redis_host = env::var("REDIS_HOST").unwrap_or_else(|_| "localhost".to_string());
        let redis_port = env::var("REDIS_PORT").unwrap_or_else(|_| "6379".to_string());
        let redis_url = format!("redis://{}:{}", redis_host, redis_port);

        let db_host = env::var("DB_HOST").unwrap_or_else(|_| "localhost".to_string());
        let db_port = env::var("DB_PORT").unwrap_or_else(|_| "5432".to_string());
        let db_user = env::var("DB_USER").unwrap_or_else(|_| "crossspread".to_string());
        let db_pass = env::var("DB_PASS").unwrap_or_else(|_| "changeme".to_string());
        let db_name = env::var("DB_NAME").unwrap_or_else(|_| "crossspread".to_string());
        let database_url = format!(
            "postgres://{}:{}@{}:{}/{}",
            db_user, db_pass, db_host, db_port, db_name
        );

        let encryption_key_b64 = env::var("ENCRYPTION_KEY_BASE64")
            .context("ENCRYPTION_KEY_BASE64 must be set")?;
        let encryption_key = base64::decode(&encryption_key_b64)
            .context("Invalid base64 in ENCRYPTION_KEY_BASE64")?;

        // Configure supported exchanges
        let exchanges = vec![
            ExchangeConfig {
                id: "binance".to_string(),
                rest_url: "https://fapi.binance.com".to_string(),
                ws_url: "wss://fstream.binance.com".to_string(),
                testnet: false,
            },
            ExchangeConfig {
                id: "bybit".to_string(),
                rest_url: "https://api.bybit.com".to_string(),
                ws_url: "wss://stream.bybit.com".to_string(),
                testnet: false,
            },
            ExchangeConfig {
                id: "okx".to_string(),
                rest_url: "https://www.okx.com".to_string(),
                ws_url: "wss://ws.okx.com:8443".to_string(),
                testnet: false,
            },
            ExchangeConfig {
                id: "kucoin".to_string(),
                rest_url: "https://api-futures.kucoin.com".to_string(),
                ws_url: "wss://ws-api-futures.kucoin.com".to_string(),
                testnet: false,
            },
        ];

        Ok(Config {
            port,
            redis_url,
            database_url,
            encryption_key,
            exchanges,
            default_slice_percent: 0.05, // 5%
            default_slice_interval_ms: 100,
            max_parallel_slices: 5,
        })
    }
}

use base64::Engine;
use base64::engine::general_purpose::STANDARD as base64;
