//! Order slicing engine
//! 
//! Splits large orders into smaller slices to reduce market impact and slippage.

use anyhow::Result;
use rust_decimal::Decimal;
use rust_decimal_macros::dec;
use std::time::Duration;
use tokio::time::sleep;
use tracing::{debug, info, warn};

use crate::exchange::{
    Credentials, ExchangeAdapter, OrderRequest, OrderResponse, OrderStatus, OrderType, Side,
    generate_client_order_id,
};

/// Configuration for order slicing
#[derive(Debug, Clone)]
pub struct SlicingConfig {
    /// Size of each slice as a fraction of total (e.g., 0.05 = 5%)
    pub slice_percent: f64,
    /// Time between slices in milliseconds
    pub interval_ms: u64,
    /// Maximum number of parallel slices
    pub max_parallel: usize,
    /// Price tolerance in basis points for limit orders
    pub price_tolerance_bps: f64,
    /// Timeout for each slice in seconds
    pub slice_timeout_secs: u64,
}

impl Default for SlicingConfig {
    fn default() -> Self {
        Self {
            slice_percent: 0.05,      // 5%
            interval_ms: 100,
            max_parallel: 1,          // Sequential by default
            price_tolerance_bps: 5.0, // 5 bps
            slice_timeout_secs: 30,
        }
    }
}

/// Result of sliced order execution
#[derive(Debug)]
pub struct SlicedOrderResult {
    pub total_quantity: Decimal,
    pub filled_quantity: Decimal,
    pub avg_fill_price: Decimal,
    pub slices: Vec<SliceResult>,
    pub total_fees: Decimal,
    pub is_complete: bool,
}

/// Result of a single slice
#[derive(Debug)]
pub struct SliceResult {
    pub index: usize,
    pub client_order_id: String,
    pub exchange_order_id: Option<String>,
    pub quantity: Decimal,
    pub price: Decimal,
    pub filled_quantity: Decimal,
    pub avg_fill_price: Option<Decimal>,
    pub status: OrderStatus,
}

/// Order slicer for splitting and executing orders
pub struct OrderSlicer {
    config: SlicingConfig,
}

impl OrderSlicer {
    pub fn new(config: SlicingConfig) -> Self {
        Self { config }
    }

    /// Calculate slice sizes for a given total quantity
    pub fn calculate_slices(&self, total_quantity: Decimal) -> Vec<Decimal> {
        let slice_size = total_quantity * Decimal::try_from(self.config.slice_percent).unwrap();
        let min_slice = dec!(0.001); // Minimum slice size

        if slice_size < min_slice {
            return vec![total_quantity];
        }

        let mut slices = Vec::new();
        let mut remaining = total_quantity;

        while remaining > Decimal::ZERO {
            let slice = if remaining < slice_size {
                remaining
            } else {
                slice_size
            };
            slices.push(slice);
            remaining -= slice;
        }

        slices
    }

    /// Execute a sliced order on an exchange
    pub async fn execute_sliced_order(
        &self,
        adapter: &dyn ExchangeAdapter,
        credentials: &Credentials,
        symbol: &str,
        side: Side,
        total_quantity: Decimal,
        reference_price: Decimal,
    ) -> Result<SlicedOrderResult> {
        let slices = self.calculate_slices(total_quantity);
        let num_slices = slices.len();

        info!(
            "Executing sliced order: {} {} {} in {} slices",
            side_str(side),
            total_quantity,
            symbol,
            num_slices
        );

        let mut results = Vec::new();
        let mut total_filled = Decimal::ZERO;
        let mut weighted_price_sum = Decimal::ZERO;

        for (index, slice_qty) in slices.iter().enumerate() {
            // Calculate limit price with tolerance
            let (best_bid, best_ask) = adapter.get_best_price(symbol).await?;
            let limit_price = calculate_limit_price(
                side,
                best_bid,
                best_ask,
                self.config.price_tolerance_bps,
            );

            let client_order_id = generate_client_order_id();

            let request = OrderRequest {
                client_order_id: client_order_id.clone(),
                symbol: symbol.to_string(),
                side,
                order_type: OrderType::Limit,
                price: Some(limit_price),
                quantity: *slice_qty,
                reduce_only: false,
            };

            debug!(
                "Placing slice {}/{}: {} @ {}",
                index + 1,
                num_slices,
                slice_qty,
                limit_price
            );

            match adapter.place_order(credentials, &request).await {
                Ok(response) => {
                    let slice_result = SliceResult {
                        index,
                        client_order_id,
                        exchange_order_id: Some(response.exchange_order_id),
                        quantity: *slice_qty,
                        price: limit_price,
                        filled_quantity: response.filled_quantity,
                        avg_fill_price: response.avg_fill_price,
                        status: response.status,
                    };

                    total_filled += response.filled_quantity;
                    if let Some(avg_price) = response.avg_fill_price {
                        weighted_price_sum += avg_price * response.filled_quantity;
                    }

                    results.push(slice_result);
                }
                Err(e) => {
                    warn!("Slice {} failed: {}", index + 1, e);
                    results.push(SliceResult {
                        index,
                        client_order_id,
                        exchange_order_id: None,
                        quantity: *slice_qty,
                        price: limit_price,
                        filled_quantity: Decimal::ZERO,
                        avg_fill_price: None,
                        status: OrderStatus::Rejected,
                    });
                }
            }

            // Wait between slices
            if index < num_slices - 1 {
                sleep(Duration::from_millis(self.config.interval_ms)).await;
            }
        }

        let avg_fill_price = if total_filled > Decimal::ZERO {
            weighted_price_sum / total_filled
        } else {
            Decimal::ZERO
        };

        let is_complete = total_filled >= total_quantity * dec!(0.99); // 99% fill threshold

        info!(
            "Sliced order complete: filled {} / {} @ avg {}",
            total_filled, total_quantity, avg_fill_price
        );

        Ok(SlicedOrderResult {
            total_quantity,
            filled_quantity: total_filled,
            avg_fill_price,
            slices: results,
            total_fees: Decimal::ZERO, // TODO: Calculate actual fees
            is_complete,
        })
    }

    /// Execute emergency exit with aggressive pricing
    pub async fn execute_emergency_exit(
        &self,
        adapter: &dyn ExchangeAdapter,
        credentials: &Credentials,
        symbol: &str,
        side: Side,
        quantity: Decimal,
    ) -> Result<SlicedOrderResult> {
        info!(
            "Executing EMERGENCY EXIT: {} {} {}",
            side_str(side),
            quantity,
            symbol
        );

        // Get current price
        let (best_bid, best_ask) = adapter.get_best_price(symbol).await?;

        // Use aggressive pricing (cross the spread)
        let aggressive_price = match side {
            Side::Buy => best_ask * dec!(1.005),  // 0.5% above ask
            Side::Sell => best_bid * dec!(0.995), // 0.5% below bid
        };

        let client_order_id = generate_client_order_id();

        let request = OrderRequest {
            client_order_id: client_order_id.clone(),
            symbol: symbol.to_string(),
            side,
            order_type: OrderType::Limit,
            price: Some(aggressive_price),
            quantity,
            reduce_only: true,
        };

        let response = adapter.place_order(credentials, &request).await?;

        let slice_result = SliceResult {
            index: 0,
            client_order_id,
            exchange_order_id: Some(response.exchange_order_id),
            quantity,
            price: aggressive_price,
            filled_quantity: response.filled_quantity,
            avg_fill_price: response.avg_fill_price,
            status: response.status,
        };

        Ok(SlicedOrderResult {
            total_quantity: quantity,
            filled_quantity: response.filled_quantity,
            avg_fill_price: response.avg_fill_price.unwrap_or(aggressive_price),
            slices: vec![slice_result],
            total_fees: Decimal::ZERO,
            is_complete: response.status == OrderStatus::Filled,
        })
    }
}

/// Calculate limit price with tolerance
fn calculate_limit_price(
    side: Side,
    best_bid: Decimal,
    best_ask: Decimal,
    tolerance_bps: f64,
) -> Decimal {
    let tolerance = Decimal::try_from(tolerance_bps / 10000.0).unwrap();

    match side {
        Side::Buy => {
            // For buys, place slightly above best bid to increase fill probability
            best_bid * (Decimal::ONE + tolerance)
        }
        Side::Sell => {
            // For sells, place slightly below best ask
            best_ask * (Decimal::ONE - tolerance)
        }
    }
}

fn side_str(side: Side) -> &'static str {
    match side {
        Side::Buy => "BUY",
        Side::Sell => "SELL",
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_calculate_slices() {
        let slicer = OrderSlicer::new(SlicingConfig {
            slice_percent: 0.1, // 10%
            ..Default::default()
        });

        let slices = slicer.calculate_slices(dec!(1.0));
        assert_eq!(slices.len(), 10);
        assert!(slices.iter().all(|s| *s == dec!(0.1)));
    }

    #[test]
    fn test_calculate_slices_remainder() {
        let slicer = OrderSlicer::new(SlicingConfig {
            slice_percent: 0.3, // 30%
            ..Default::default()
        });

        let slices = slicer.calculate_slices(dec!(1.0));
        assert_eq!(slices.len(), 4);
        // 0.3 + 0.3 + 0.3 + 0.1 = 1.0
    }
}
