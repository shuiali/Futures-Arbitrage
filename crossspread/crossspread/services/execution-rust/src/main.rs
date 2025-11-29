//! CrossSpread Execution Service
//! 
//! Low-latency order execution microservice for crypto futures arbitrage.
//! Handles sliced limit order placement across multiple exchanges.

use anyhow::Result;
use tracing::{info, Level};
use tracing_subscriber::FmtSubscriber;

mod config;
mod crypto;
mod exchange;
mod order;
mod slicer;

#[tokio::main]
async fn main() -> Result<()> {
    // Initialize tracing
    let subscriber = FmtSubscriber::builder()
        .with_max_level(Level::INFO)
        .with_target(false)
        .init();

    info!("Starting CrossSpread Execution Service");

    // Load configuration
    let config = config::Config::from_env()?;
    info!("Loaded configuration for {} exchanges", config.exchanges.len());

    // Initialize exchange adapters
    let mut adapters = Vec::new();
    for exchange_config in &config.exchanges {
        let adapter = exchange::create_adapter(exchange_config).await?;
        adapters.push(adapter);
        info!("Initialized {} adapter", exchange_config.id);
    }

    // Start the order execution server
    let server = order::ExecutionServer::new(adapters, config.clone());
    server.run().await?;

    Ok(())
}
