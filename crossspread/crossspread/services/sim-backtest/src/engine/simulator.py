"""
Simulated exchange and order execution.

Provides realistic order simulation including:
- Limit order matching against orderbook
- Partial fills
- Order slicing simulation
"""

from dataclasses import dataclass, field
from datetime import datetime
from decimal import Decimal
from typing import Optional, List, Dict, Callable
from enum import Enum
from uuid import uuid4
import asyncio

from structlog import get_logger

from .orderbook import OrderbookSnapshot, OrderbookLevel
from .slippage import SlippageCalculator, TradeSide, FeeStructure, EXCHANGE_FEES

logger = get_logger(__name__)


class OrderSide(Enum):
    BUY = "buy"
    SELL = "sell"


class OrderType(Enum):
    LIMIT = "limit"
    MARKET = "market"


class OrderStatus(Enum):
    PENDING = "pending"
    OPEN = "open"
    PARTIALLY_FILLED = "partially_filled"
    FILLED = "filled"
    CANCELLED = "cancelled"
    REJECTED = "rejected"


@dataclass
class SimulatedFill:
    """A single fill of an order."""
    fill_id: str
    order_id: str
    timestamp: datetime
    price: Decimal
    quantity: Decimal
    fee: Decimal
    is_maker: bool
    
    @property
    def value(self) -> Decimal:
        """Total fill value."""
        return self.price * self.quantity
    
    @property
    def total_cost(self) -> Decimal:
        """Fill value including fee."""
        return self.value + self.fee


@dataclass
class SimulatedOrder:
    """A simulated order."""
    order_id: str
    exchange: str
    symbol: str
    side: OrderSide
    order_type: OrderType
    price: Optional[Decimal]  # None for market orders
    quantity: Decimal
    filled_quantity: Decimal = Decimal("0")
    status: OrderStatus = OrderStatus.PENDING
    created_at: datetime = field(default_factory=datetime.utcnow)
    updated_at: datetime = field(default_factory=datetime.utcnow)
    fills: List[SimulatedFill] = field(default_factory=list)
    
    @property
    def remaining_quantity(self) -> Decimal:
        """Unfilled quantity."""
        return self.quantity - self.filled_quantity
    
    @property
    def average_fill_price(self) -> Optional[Decimal]:
        """Volume-weighted average fill price."""
        if not self.fills:
            return None
        total_value = sum(f.value for f in self.fills)
        total_qty = sum(f.quantity for f in self.fills)
        return total_value / total_qty if total_qty > 0 else None
    
    @property
    def total_fees(self) -> Decimal:
        """Total fees paid."""
        return sum(f.fee for f in self.fills)
    
    def add_fill(self, fill: SimulatedFill):
        """Add a fill to the order."""
        self.fills.append(fill)
        self.filled_quantity += fill.quantity
        self.updated_at = fill.timestamp
        
        if self.filled_quantity >= self.quantity:
            self.status = OrderStatus.FILLED
        elif self.filled_quantity > 0:
            self.status = OrderStatus.PARTIALLY_FILLED


class SimulatedExchange:
    """
    Simulates an exchange for backtesting.
    
    Maintains order state and matches orders against orderbook snapshots.
    """
    
    def __init__(
        self,
        exchange_name: str,
        fee_structure: Optional[FeeStructure] = None,
        latency_ms: int = 10,  # Simulated order latency
    ):
        """
        Initialize simulated exchange.
        
        Args:
            exchange_name: Exchange identifier
            fee_structure: Fee structure for the exchange
            latency_ms: Simulated order placement latency
        """
        self.exchange_name = exchange_name
        self.fee_structure = fee_structure or EXCHANGE_FEES.get(
            exchange_name.lower(), FeeStructure()
        )
        self.latency_ms = latency_ms
        
        self._orders: Dict[str, SimulatedOrder] = {}
        self._open_orders: Dict[str, SimulatedOrder] = {}
        self._current_book: Optional[OrderbookSnapshot] = None
        
        # Event callbacks
        self._on_fill: Optional[Callable[[SimulatedFill], None]] = None
        self._on_order_update: Optional[Callable[[SimulatedOrder], None]] = None
    
    def on_fill(self, callback: Callable[[SimulatedFill], None]):
        """Register callback for fills."""
        self._on_fill = callback
    
    def on_order_update(self, callback: Callable[[SimulatedOrder], None]):
        """Register callback for order updates."""
        self._on_order_update = callback
    
    def update_orderbook(self, book: OrderbookSnapshot):
        """
        Update the current orderbook and match pending orders.
        
        Args:
            book: New orderbook snapshot
        """
        if book.exchange != self.exchange_name:
            return
        
        self._current_book = book
        
        # Try to fill open orders
        for order_id, order in list(self._open_orders.items()):
            if order.symbol != book.symbol:
                continue
            
            self._try_fill_order(order, book)
            
            # Remove from open orders if complete
            if order.status in (OrderStatus.FILLED, OrderStatus.CANCELLED):
                del self._open_orders[order_id]
    
    def place_order(
        self,
        symbol: str,
        side: OrderSide,
        order_type: OrderType,
        quantity: Decimal,
        price: Optional[Decimal] = None,
        timestamp: Optional[datetime] = None
    ) -> SimulatedOrder:
        """
        Place a simulated order.
        
        Args:
            symbol: Trading symbol
            side: Buy or sell
            order_type: Limit or market
            quantity: Order quantity
            price: Limit price (required for limit orders)
            timestamp: Order timestamp
            
        Returns:
            The created order
        """
        if order_type == OrderType.LIMIT and price is None:
            raise ValueError("Limit orders require a price")
        
        order = SimulatedOrder(
            order_id=str(uuid4()),
            exchange=self.exchange_name,
            symbol=symbol,
            side=side,
            order_type=order_type,
            price=price,
            quantity=quantity,
            status=OrderStatus.OPEN,
            created_at=timestamp or datetime.utcnow(),
        )
        
        self._orders[order.order_id] = order
        self._open_orders[order.order_id] = order
        
        logger.debug(
            "simulated_order_placed",
            order_id=order.order_id,
            symbol=symbol,
            side=side.value,
            quantity=str(quantity),
            price=str(price) if price else None
        )
        
        # Try immediate fill if we have an orderbook
        if self._current_book and self._current_book.symbol == symbol:
            self._try_fill_order(order, self._current_book)
        
        if self._on_order_update:
            self._on_order_update(order)
        
        return order
    
    def cancel_order(self, order_id: str) -> bool:
        """
        Cancel an open order.
        
        Args:
            order_id: Order to cancel
            
        Returns:
            True if cancelled, False if not found or already complete
        """
        if order_id not in self._orders:
            return False
        
        order = self._orders[order_id]
        
        if order.status in (OrderStatus.FILLED, OrderStatus.CANCELLED):
            return False
        
        order.status = OrderStatus.CANCELLED
        order.updated_at = datetime.utcnow()
        
        if order_id in self._open_orders:
            del self._open_orders[order_id]
        
        logger.debug("simulated_order_cancelled", order_id=order_id)
        
        if self._on_order_update:
            self._on_order_update(order)
        
        return True
    
    def get_order(self, order_id: str) -> Optional[SimulatedOrder]:
        """Get order by ID."""
        return self._orders.get(order_id)
    
    def get_open_orders(self, symbol: Optional[str] = None) -> List[SimulatedOrder]:
        """Get all open orders, optionally filtered by symbol."""
        orders = list(self._open_orders.values())
        if symbol:
            orders = [o for o in orders if o.symbol == symbol]
        return orders
    
    def _try_fill_order(self, order: SimulatedOrder, book: OrderbookSnapshot):
        """
        Try to fill an order against the orderbook.
        
        Args:
            order: Order to fill
            book: Current orderbook
        """
        if order.status in (OrderStatus.FILLED, OrderStatus.CANCELLED):
            return
        
        remaining = order.remaining_quantity
        if remaining <= 0:
            return
        
        # Get the relevant side of the book
        if order.side == OrderSide.BUY:
            levels = book.asks
        else:
            levels = book.bids
        
        if not levels:
            return
        
        fills = []
        remaining_qty = remaining
        
        for level in levels:
            if remaining_qty <= 0:
                break
            
            # Check price for limit orders
            if order.order_type == OrderType.LIMIT:
                if order.side == OrderSide.BUY:
                    # Can only fill if ask <= limit price
                    if level.price > order.price:
                        break
                else:
                    # Can only fill if bid >= limit price
                    if level.price < order.price:
                        break
            
            # Calculate fill quantity
            fill_qty = min(remaining_qty, level.quantity)
            
            # Check if this is a maker fill (limit order that provides liquidity)
            is_maker = (
                order.order_type == OrderType.LIMIT and
                ((order.side == OrderSide.BUY and order.price < book.best_ask.price) or
                 (order.side == OrderSide.SELL and order.price > book.best_bid.price))
            ) if book.best_ask and book.best_bid else False
            
            # Calculate fee
            fee_rate = self.fee_structure.get_fee(is_maker)
            fee = fill_qty * level.price * fee_rate
            
            fill = SimulatedFill(
                fill_id=str(uuid4()),
                order_id=order.order_id,
                timestamp=book.timestamp,
                price=level.price,
                quantity=fill_qty,
                fee=fee,
                is_maker=is_maker
            )
            
            fills.append(fill)
            remaining_qty -= fill_qty
        
        # Apply fills
        for fill in fills:
            order.add_fill(fill)
            
            logger.debug(
                "simulated_fill",
                order_id=order.order_id,
                price=str(fill.price),
                quantity=str(fill.quantity),
                fee=str(fill.fee)
            )
            
            if self._on_fill:
                self._on_fill(fill)
        
        if fills and self._on_order_update:
            self._on_order_update(order)
    
    def reset(self):
        """Reset all orders and state."""
        self._orders.clear()
        self._open_orders.clear()
        self._current_book = None


class OrderSlicingSimulator:
    """
    Simulates order slicing as the real execution engine does.
    
    Breaks large orders into smaller slices and executes them
    over time to reduce market impact.
    """
    
    def __init__(
        self,
        exchange: SimulatedExchange,
        slice_size_pct: Decimal = Decimal("5"),  # 5% of total
        slice_interval_ms: int = 100,  # 100ms between slices
    ):
        """
        Initialize slicer.
        
        Args:
            exchange: Simulated exchange to place orders on
            slice_size_pct: Percentage of order per slice
            slice_interval_ms: Milliseconds between slices
        """
        self.exchange = exchange
        self.slice_size_pct = slice_size_pct
        self.slice_interval_ms = slice_interval_ms
    
    def calculate_slices(
        self,
        total_quantity: Decimal,
        min_slice_qty: Decimal = Decimal("0.001")
    ) -> List[Decimal]:
        """
        Calculate slice quantities.
        
        Args:
            total_quantity: Total order quantity
            min_slice_qty: Minimum slice size
            
        Returns:
            List of slice quantities
        """
        slice_qty = total_quantity * (self.slice_size_pct / Decimal("100"))
        slice_qty = max(slice_qty, min_slice_qty)
        
        slices = []
        remaining = total_quantity
        
        while remaining > 0:
            qty = min(slice_qty, remaining)
            slices.append(qty)
            remaining -= qty
        
        return slices
    
    async def execute_sliced_order(
        self,
        symbol: str,
        side: OrderSide,
        total_quantity: Decimal,
        limit_price: Decimal,
        price_tolerance_bps: Decimal = Decimal("5"),  # Adjust limit by this
    ) -> List[SimulatedOrder]:
        """
        Execute a sliced order.
        
        Args:
            symbol: Trading symbol
            side: Buy or sell
            total_quantity: Total order quantity
            limit_price: Base limit price
            price_tolerance_bps: Price adjustment per slice
            
        Returns:
            List of slice orders
        """
        slices = self.calculate_slices(total_quantity)
        orders = []
        
        for i, slice_qty in enumerate(slices):
            # Adjust price for each slice
            tolerance = limit_price * (price_tolerance_bps * i / Decimal("10000"))
            
            if side == OrderSide.BUY:
                adjusted_price = limit_price + tolerance  # More aggressive
            else:
                adjusted_price = limit_price - tolerance  # More aggressive
            
            order = self.exchange.place_order(
                symbol=symbol,
                side=side,
                order_type=OrderType.LIMIT,
                quantity=slice_qty,
                price=adjusted_price
            )
            
            orders.append(order)
            
            # Wait between slices (simulated)
            if i < len(slices) - 1:
                await asyncio.sleep(self.slice_interval_ms / 1000)
        
        return orders
