"""
Slippage calculator for realistic trade simulation.

Walks the orderbook to calculate actual execution prices and slippage.
"""

from dataclasses import dataclass
from decimal import Decimal
from typing import Optional, List, Tuple
from enum import Enum

from .orderbook import OrderbookSnapshot, OrderbookLevel, OrderbookSide


class TradeSide(Enum):
    BUY = "buy"
    SELL = "sell"


@dataclass
class SlippageResult:
    """
    Result of slippage calculation.
    
    Attributes:
        expected_price: Price if fully filled at best level
        actual_price: Volume-weighted average execution price
        slippage_abs: Absolute slippage (actual - expected)
        slippage_bps: Slippage in basis points
        total_cost: Total cost including fees
        filled_quantity: Quantity that could be filled
        unfilled_quantity: Quantity that couldn't be filled due to depth
        fills: List of (price, quantity) fills across levels
        insufficient_liquidity: True if order couldn't be fully filled
    """
    expected_price: Decimal
    actual_price: Decimal
    slippage_abs: Decimal
    slippage_bps: Decimal
    total_cost: Decimal
    filled_quantity: Decimal
    unfilled_quantity: Decimal
    fills: List[Tuple[Decimal, Decimal]]
    insufficient_liquidity: bool
    
    @property
    def fill_rate(self) -> Decimal:
        """Percentage of order filled."""
        total = self.filled_quantity + self.unfilled_quantity
        if total == 0:
            return Decimal("0")
        return (self.filled_quantity / total) * Decimal("100")


@dataclass
class FeeStructure:
    """Exchange fee structure."""
    maker_fee_bps: Decimal = Decimal("2")   # 0.02% = 2 bps
    taker_fee_bps: Decimal = Decimal("5")   # 0.05% = 5 bps
    
    def get_fee(self, is_maker: bool) -> Decimal:
        """Get fee rate as decimal (e.g., 0.0002 for 2 bps)."""
        bps = self.maker_fee_bps if is_maker else self.taker_fee_bps
        return bps / Decimal("10000")


# Default fee structures per exchange
EXCHANGE_FEES = {
    "binance": FeeStructure(maker_fee_bps=Decimal("2"), taker_fee_bps=Decimal("4")),
    "bybit": FeeStructure(maker_fee_bps=Decimal("1"), taker_fee_bps=Decimal("6")),
    "okx": FeeStructure(maker_fee_bps=Decimal("2"), taker_fee_bps=Decimal("5")),
    "kucoin": FeeStructure(maker_fee_bps=Decimal("2"), taker_fee_bps=Decimal("6")),
    "gate": FeeStructure(maker_fee_bps=Decimal("2"), taker_fee_bps=Decimal("5")),
    "mexc": FeeStructure(maker_fee_bps=Decimal("0"), taker_fee_bps=Decimal("2")),
    "bitget": FeeStructure(maker_fee_bps=Decimal("2"), taker_fee_bps=Decimal("6")),
    "bingx": FeeStructure(maker_fee_bps=Decimal("2"), taker_fee_bps=Decimal("5")),
    "coinex": FeeStructure(maker_fee_bps=Decimal("3"), taker_fee_bps=Decimal("5")),
    "lbank": FeeStructure(maker_fee_bps=Decimal("2"), taker_fee_bps=Decimal("6")),
    "htx": FeeStructure(maker_fee_bps=Decimal("2"), taker_fee_bps=Decimal("5")),
}


class SlippageCalculator:
    """
    Calculates slippage by walking the orderbook.
    
    Simulates executing a market order of a given size against
    the current orderbook depth to determine the average fill price.
    """
    
    def __init__(self, default_fees: Optional[FeeStructure] = None):
        """
        Initialize calculator.
        
        Args:
            default_fees: Default fee structure if exchange not found
        """
        self.default_fees = default_fees or FeeStructure()
    
    def get_fees(self, exchange: str) -> FeeStructure:
        """Get fee structure for an exchange."""
        return EXCHANGE_FEES.get(exchange.lower(), self.default_fees)
    
    def calculate(
        self,
        orderbook: OrderbookSnapshot,
        side: TradeSide,
        size_in_coins: Decimal,
        include_fees: bool = True,
        is_aggressive: bool = True  # Taker order
    ) -> SlippageResult:
        """
        Calculate slippage for an order.
        
        Args:
            orderbook: Current orderbook snapshot
            side: BUY or SELL
            size_in_coins: Size of order in base currency
            include_fees: Whether to include exchange fees
            is_aggressive: True for taker orders (market-crossing)
            
        Returns:
            SlippageResult with execution details
        """
        # Buying = consume asks, Selling = consume bids
        levels = orderbook.asks if side == TradeSide.BUY else orderbook.bids
        
        if not levels:
            return SlippageResult(
                expected_price=Decimal("0"),
                actual_price=Decimal("0"),
                slippage_abs=Decimal("0"),
                slippage_bps=Decimal("0"),
                total_cost=Decimal("0"),
                filled_quantity=Decimal("0"),
                unfilled_quantity=size_in_coins,
                fills=[],
                insufficient_liquidity=True
            )
        
        # Best price (what we'd expect if infinite liquidity at top)
        expected_price = levels[0].price
        
        # Walk the book
        remaining = size_in_coins
        fills: List[Tuple[Decimal, Decimal]] = []
        total_value = Decimal("0")
        
        for level in levels:
            if remaining <= 0:
                break
            
            fill_qty = min(remaining, level.quantity)
            fills.append((level.price, fill_qty))
            total_value += fill_qty * level.price
            remaining -= fill_qty
        
        filled_quantity = size_in_coins - remaining
        
        if filled_quantity == 0:
            return SlippageResult(
                expected_price=expected_price,
                actual_price=Decimal("0"),
                slippage_abs=Decimal("0"),
                slippage_bps=Decimal("0"),
                total_cost=Decimal("0"),
                filled_quantity=Decimal("0"),
                unfilled_quantity=size_in_coins,
                fills=[],
                insufficient_liquidity=True
            )
        
        # Volume-weighted average price
        actual_price = total_value / filled_quantity
        
        # Slippage calculation
        if side == TradeSide.BUY:
            slippage_abs = actual_price - expected_price
        else:
            slippage_abs = expected_price - actual_price  # For sells, we want higher price
        
        slippage_bps = (slippage_abs / expected_price) * Decimal("10000") if expected_price > 0 else Decimal("0")
        
        # Calculate total cost including fees
        total_cost = total_value
        if include_fees:
            fees = self.get_fees(orderbook.exchange)
            fee_rate = fees.get_fee(is_maker=not is_aggressive)
            fee_amount = total_value * fee_rate
            total_cost += fee_amount
        
        return SlippageResult(
            expected_price=expected_price,
            actual_price=actual_price,
            slippage_abs=abs(slippage_abs),
            slippage_bps=abs(slippage_bps),
            total_cost=total_cost,
            filled_quantity=filled_quantity,
            unfilled_quantity=remaining,
            fills=fills,
            insufficient_liquidity=remaining > 0
        )
    
    def calculate_round_trip(
        self,
        entry_book: OrderbookSnapshot,
        exit_book: OrderbookSnapshot,
        side: TradeSide,
        size_in_coins: Decimal,
        include_fees: bool = True
    ) -> Tuple[SlippageResult, SlippageResult, Decimal]:
        """
        Calculate slippage for a round-trip trade (entry + exit).
        
        Args:
            entry_book: Orderbook for entry
            exit_book: Orderbook for exit  
            side: Entry side (BUY or SELL)
            size_in_coins: Position size
            include_fees: Whether to include fees
            
        Returns:
            Tuple of (entry_result, exit_result, total_pnl)
        """
        # Entry: BUY consumes asks, SELL consumes bids
        entry_result = self.calculate(
            entry_book, side, size_in_coins, include_fees
        )
        
        # Exit: opposite side
        exit_side = TradeSide.SELL if side == TradeSide.BUY else TradeSide.BUY
        exit_result = self.calculate(
            exit_book, exit_side, entry_result.filled_quantity, include_fees
        )
        
        # Calculate PnL
        if side == TradeSide.BUY:
            # Long: profit if exit > entry
            raw_pnl = (exit_result.actual_price - entry_result.actual_price) * entry_result.filled_quantity
        else:
            # Short: profit if exit < entry
            raw_pnl = (entry_result.actual_price - exit_result.actual_price) * entry_result.filled_quantity
        
        # Subtract fees from both sides
        if include_fees:
            entry_fees = entry_result.total_cost - (entry_result.actual_price * entry_result.filled_quantity)
            exit_fees = exit_result.total_cost - (exit_result.actual_price * exit_result.filled_quantity)
            total_pnl = raw_pnl - entry_fees - exit_fees
        else:
            total_pnl = raw_pnl
        
        return entry_result, exit_result, total_pnl


def calculate_spread_slippage(
    long_book: OrderbookSnapshot,
    short_book: OrderbookSnapshot,
    size_in_coins: Decimal,
    include_fees: bool = True
) -> dict:
    """
    Calculate total slippage for a spread trade (long one exchange, short another).
    
    Args:
        long_book: Orderbook for long leg (buying)
        short_book: Orderbook for short leg (selling)
        size_in_coins: Position size per leg
        include_fees: Whether to include fees
        
    Returns:
        Dictionary with execution details for both legs
    """
    calc = SlippageCalculator()
    
    # Long leg: buying
    long_result = calc.calculate(
        long_book, TradeSide.BUY, size_in_coins, include_fees
    )
    
    # Short leg: selling
    short_result = calc.calculate(
        short_book, TradeSide.SELL, size_in_coins, include_fees
    )
    
    # Spread at execution prices
    if long_result.actual_price > 0 and short_result.actual_price > 0:
        spread_bps = ((short_result.actual_price - long_result.actual_price) / long_result.actual_price) * Decimal("10000")
    else:
        spread_bps = Decimal("0")
    
    return {
        "long_leg": {
            "exchange": long_book.exchange,
            "expected_price": str(long_result.expected_price),
            "actual_price": str(long_result.actual_price),
            "slippage_bps": str(long_result.slippage_bps),
            "total_cost": str(long_result.total_cost),
            "filled": str(long_result.filled_quantity),
            "unfilled": str(long_result.unfilled_quantity),
            "insufficient_liquidity": long_result.insufficient_liquidity,
        },
        "short_leg": {
            "exchange": short_book.exchange,
            "expected_price": str(short_result.expected_price),
            "actual_price": str(short_result.actual_price),
            "slippage_bps": str(short_result.slippage_bps),
            "total_cost": str(short_result.total_cost),
            "filled": str(short_result.filled_quantity),
            "unfilled": str(short_result.unfilled_quantity),
            "insufficient_liquidity": short_result.insufficient_liquidity,
        },
        "spread_at_execution_bps": str(spread_bps),
        "total_slippage_bps": str(long_result.slippage_bps + short_result.slippage_bps),
        "can_execute": not (long_result.insufficient_liquidity or short_result.insufficient_liquidity),
    }
