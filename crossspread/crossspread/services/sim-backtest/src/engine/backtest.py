"""
Backtest engine for spread trading strategies.

Provides full backtesting capability including:
- Historical L2 playback
- Spread trade simulation
- Performance metrics calculation
"""

from dataclasses import dataclass, field
from datetime import datetime, timedelta
from decimal import Decimal
from typing import Optional, List, Dict, Callable, Any
from enum import Enum
import asyncio

from structlog import get_logger

from .orderbook import OrderbookSnapshot, OrderbookPlayback, InMemoryOrderbookStore
from .simulator import SimulatedExchange, SimulatedOrder, SimulatedFill, OrderSide, OrderType
from .slippage import SlippageCalculator, TradeSide

logger = get_logger(__name__)


class TradeDirection(Enum):
    LONG_SHORT = "long_short"  # Long exchange A, short exchange B
    SHORT_LONG = "short_long"  # Short exchange A, long exchange B


@dataclass
class SpreadTrade:
    """A spread trade with two legs."""
    trade_id: str
    canonical_symbol: str
    long_exchange: str
    short_exchange: str
    entry_time: datetime
    size_in_coins: Decimal
    
    # Entry details
    long_entry_price: Optional[Decimal] = None
    short_entry_price: Optional[Decimal] = None
    entry_spread_bps: Optional[Decimal] = None
    
    # Exit details
    exit_time: Optional[datetime] = None
    long_exit_price: Optional[Decimal] = None
    short_exit_price: Optional[Decimal] = None
    exit_spread_bps: Optional[Decimal] = None
    
    # P&L
    gross_pnl: Decimal = Decimal("0")
    fees: Decimal = Decimal("0")
    net_pnl: Decimal = Decimal("0")
    
    # Status
    is_open: bool = True
    
    @property
    def duration(self) -> Optional[timedelta]:
        """Trade duration."""
        if self.exit_time:
            return self.exit_time - self.entry_time
        return None
    
    @property
    def pnl_bps(self) -> Decimal:
        """P&L in basis points relative to notional."""
        if self.long_entry_price and self.size_in_coins:
            notional = self.size_in_coins * self.long_entry_price
            if notional > 0:
                return (self.net_pnl / notional) * Decimal("10000")
        return Decimal("0")


@dataclass
class BacktestConfig:
    """Configuration for a backtest run."""
    # Data range
    start_time: datetime
    end_time: datetime
    
    # Instruments
    exchanges: List[str]
    symbols: List[str]  # Canonical symbols
    
    # Trade parameters
    size_in_coins: Decimal
    entry_spread_threshold_bps: Decimal = Decimal("10")  # Min spread to enter
    exit_spread_threshold_bps: Decimal = Decimal("2")    # Target spread to exit
    max_position_hold_time: timedelta = timedelta(hours=24)
    max_concurrent_positions: int = 5
    
    # Risk
    max_slippage_bps: Decimal = Decimal("5")  # Skip if slippage too high
    min_liquidity_ratio: Decimal = Decimal("3")  # Book depth must be 3x size
    
    # Execution simulation
    slice_count: int = 5
    slice_interval_ms: int = 100
    
    # Database
    db_url: str = ""


@dataclass
class BacktestResult:
    """Results from a backtest run."""
    config: BacktestConfig
    
    # Timing
    run_start: datetime = field(default_factory=datetime.utcnow)
    run_end: Optional[datetime] = None
    
    # Trades
    trades: List[SpreadTrade] = field(default_factory=list)
    
    # Summary metrics
    total_trades: int = 0
    winning_trades: int = 0
    losing_trades: int = 0
    
    # P&L
    gross_pnl: Decimal = Decimal("0")
    total_fees: Decimal = Decimal("0")
    net_pnl: Decimal = Decimal("0")
    
    # Risk metrics
    max_drawdown: Decimal = Decimal("0")
    max_drawdown_pct: Decimal = Decimal("0")
    sharpe_ratio: Optional[Decimal] = None
    sortino_ratio: Optional[Decimal] = None
    
    # Execution stats
    total_snapshots_processed: int = 0
    avg_spread_bps: Decimal = Decimal("0")
    avg_slippage_bps: Decimal = Decimal("0")
    
    @property
    def win_rate(self) -> Decimal:
        """Win rate percentage."""
        if self.total_trades == 0:
            return Decimal("0")
        return (Decimal(self.winning_trades) / Decimal(self.total_trades)) * Decimal("100")
    
    @property
    def profit_factor(self) -> Optional[Decimal]:
        """Gross profit / gross loss."""
        gross_profit = sum(t.net_pnl for t in self.trades if t.net_pnl > 0)
        gross_loss = abs(sum(t.net_pnl for t in self.trades if t.net_pnl < 0))
        if gross_loss > 0:
            return gross_profit / gross_loss
        return None
    
    @property
    def avg_trade_pnl(self) -> Decimal:
        """Average P&L per trade."""
        if self.total_trades == 0:
            return Decimal("0")
        return self.net_pnl / Decimal(self.total_trades)


class BacktestEngine:
    """
    Main backtest engine for spread trading.
    
    Replays historical orderbook data and simulates spread trades.
    """
    
    def __init__(self, config: BacktestConfig):
        """
        Initialize backtest engine.
        
        Args:
            config: Backtest configuration
        """
        self.config = config
        
        # Create simulated exchanges
        self.exchanges: Dict[str, SimulatedExchange] = {}
        for exchange in config.exchanges:
            self.exchanges[exchange] = SimulatedExchange(exchange)
        
        # State
        self.orderbook_store = InMemoryOrderbookStore()
        self.slippage_calc = SlippageCalculator()
        self.open_positions: Dict[str, SpreadTrade] = {}
        self.closed_positions: List[SpreadTrade] = []
        
        # Metrics tracking
        self.equity_curve: List[Dict[str, Any]] = []
        self.peak_equity = Decimal("0")
        self.current_equity = Decimal("0")
        
        # Callbacks
        self._on_trade_open: Optional[Callable[[SpreadTrade], None]] = None
        self._on_trade_close: Optional[Callable[[SpreadTrade], None]] = None
        self._on_snapshot: Optional[Callable[[OrderbookSnapshot], None]] = None
    
    def on_trade_open(self, callback: Callable[[SpreadTrade], None]):
        """Register callback for trade opens."""
        self._on_trade_open = callback
    
    def on_trade_close(self, callback: Callable[[SpreadTrade], None]):
        """Register callback for trade closes."""
        self._on_trade_close = callback
    
    def on_snapshot(self, callback: Callable[[OrderbookSnapshot], None]):
        """Register callback for each snapshot processed."""
        self._on_snapshot = callback
    
    async def run(self) -> BacktestResult:
        """
        Run the backtest.
        
        Returns:
            BacktestResult with all metrics and trades
        """
        result = BacktestResult(config=self.config)
        
        logger.info(
            "backtest_starting",
            start=self.config.start_time.isoformat(),
            end=self.config.end_time.isoformat(),
            exchanges=self.config.exchanges,
            symbols=self.config.symbols
        )
        
        # Initialize orderbook playback
        playback = OrderbookPlayback(
            db_url=self.config.db_url,
            exchanges=self.config.exchanges,
            symbols=self.config.symbols,
            start_time=self.config.start_time,
            end_time=self.config.end_time,
        )
        
        await playback.connect()
        
        try:
            snapshot_count = 0
            spread_sum = Decimal("0")
            slippage_sum = Decimal("0")
            
            async for snapshot in playback:
                snapshot_count += 1
                
                # Update orderbook store
                self.orderbook_store.update(snapshot)
                
                # Update simulated exchanges
                if snapshot.exchange in self.exchanges:
                    self.exchanges[snapshot.exchange].update_orderbook(snapshot)
                
                # Check for exit conditions on open positions
                await self._check_exits(snapshot)
                
                # Look for new spread opportunities
                spread_info = await self._find_spread_opportunity(snapshot)
                
                if spread_info:
                    spread_sum += spread_info.get("spread_bps", Decimal("0"))
                    
                    # Check if we should enter
                    if self._should_enter(spread_info):
                        trade = await self._enter_spread(spread_info, snapshot.timestamp)
                        if trade:
                            slippage_sum += spread_info.get("total_slippage_bps", Decimal("0"))
                
                # Track equity
                self._update_equity(snapshot.timestamp)
                
                if self._on_snapshot:
                    self._on_snapshot(snapshot)
                
                # Progress logging
                if snapshot_count % 10000 == 0:
                    logger.debug("backtest_progress", snapshots=snapshot_count)
            
            result.total_snapshots_processed = snapshot_count
            
        finally:
            await playback.close()
        
        # Finalize results
        result.run_end = datetime.utcnow()
        result.trades = self.closed_positions + list(self.open_positions.values())
        result.total_trades = len(result.trades)
        result.winning_trades = sum(1 for t in result.trades if t.net_pnl > 0)
        result.losing_trades = sum(1 for t in result.trades if t.net_pnl < 0)
        result.gross_pnl = sum(t.gross_pnl for t in result.trades)
        result.total_fees = sum(t.fees for t in result.trades)
        result.net_pnl = sum(t.net_pnl for t in result.trades)
        
        if result.total_snapshots_processed > 0:
            result.avg_spread_bps = spread_sum / Decimal(result.total_snapshots_processed)
        
        if result.total_trades > 0:
            result.avg_slippage_bps = slippage_sum / Decimal(result.total_trades)
        
        # Calculate risk metrics
        self._calculate_risk_metrics(result)
        
        logger.info(
            "backtest_complete",
            trades=result.total_trades,
            win_rate=str(result.win_rate),
            net_pnl=str(result.net_pnl),
            sharpe=str(result.sharpe_ratio)
        )
        
        return result
    
    async def _find_spread_opportunity(self, snapshot: OrderbookSnapshot) -> Optional[Dict[str, Any]]:
        """
        Find spread opportunity for a symbol across exchanges.
        
        Args:
            snapshot: Latest orderbook snapshot
            
        Returns:
            Spread info dict if opportunity found
        """
        symbol = snapshot.symbol
        books = self.orderbook_store.get_all_for_symbol(symbol)
        
        if len(books) < 2:
            return None
        
        best_spread = None
        best_spread_bps = Decimal("-inf")
        
        # Check all exchange pairs
        exchanges = list(books.keys())
        for i, ex1 in enumerate(exchanges):
            for ex2 in exchanges[i+1:]:
                book1 = books[ex1]
                book2 = books[ex2]
                
                if not book1.best_bid or not book1.best_ask:
                    continue
                if not book2.best_bid or not book2.best_ask:
                    continue
                
                # Check both directions
                # Direction 1: Long ex1, Short ex2
                spread1 = (book2.best_bid.price - book1.best_ask.price) / book1.best_ask.price * Decimal("10000")
                
                # Direction 2: Long ex2, Short ex1
                spread2 = (book1.best_bid.price - book2.best_ask.price) / book2.best_ask.price * Decimal("10000")
                
                if spread1 > spread2 and spread1 > best_spread_bps:
                    best_spread_bps = spread1
                    
                    # Calculate slippage
                    long_slip = self.slippage_calc.calculate(
                        book1, TradeSide.BUY, self.config.size_in_coins
                    )
                    short_slip = self.slippage_calc.calculate(
                        book2, TradeSide.SELL, self.config.size_in_coins
                    )
                    
                    best_spread = {
                        "symbol": symbol,
                        "long_exchange": ex1,
                        "short_exchange": ex2,
                        "long_book": book1,
                        "short_book": book2,
                        "spread_bps": spread1,
                        "long_slippage": long_slip,
                        "short_slippage": short_slip,
                        "total_slippage_bps": long_slip.slippage_bps + short_slip.slippage_bps,
                        "can_execute": not (long_slip.insufficient_liquidity or short_slip.insufficient_liquidity),
                    }
                
                elif spread2 > best_spread_bps:
                    best_spread_bps = spread2
                    
                    long_slip = self.slippage_calc.calculate(
                        book2, TradeSide.BUY, self.config.size_in_coins
                    )
                    short_slip = self.slippage_calc.calculate(
                        book1, TradeSide.SELL, self.config.size_in_coins
                    )
                    
                    best_spread = {
                        "symbol": symbol,
                        "long_exchange": ex2,
                        "short_exchange": ex1,
                        "long_book": book2,
                        "short_book": book1,
                        "spread_bps": spread2,
                        "long_slippage": long_slip,
                        "short_slippage": short_slip,
                        "total_slippage_bps": long_slip.slippage_bps + short_slip.slippage_bps,
                        "can_execute": not (long_slip.insufficient_liquidity or short_slip.insufficient_liquidity),
                    }
        
        return best_spread
    
    def _should_enter(self, spread_info: Dict[str, Any]) -> bool:
        """Check if we should enter a spread trade."""
        # Already at max positions?
        if len(self.open_positions) >= self.config.max_concurrent_positions:
            return False
        
        # Check spread threshold
        if spread_info["spread_bps"] < self.config.entry_spread_threshold_bps:
            return False
        
        # Check slippage
        if spread_info["total_slippage_bps"] > self.config.max_slippage_bps:
            return False
        
        # Check liquidity
        if not spread_info["can_execute"]:
            return False
        
        # Check if we already have a position in this symbol
        symbol = spread_info["symbol"]
        for pos in self.open_positions.values():
            if pos.canonical_symbol == symbol:
                return False
        
        return True
    
    async def _enter_spread(
        self,
        spread_info: Dict[str, Any],
        timestamp: datetime
    ) -> Optional[SpreadTrade]:
        """
        Enter a spread trade.
        
        Args:
            spread_info: Spread opportunity info
            timestamp: Entry timestamp
            
        Returns:
            Created trade or None if entry failed
        """
        from uuid import uuid4
        
        long_slip = spread_info["long_slippage"]
        short_slip = spread_info["short_slippage"]
        
        trade = SpreadTrade(
            trade_id=str(uuid4()),
            canonical_symbol=spread_info["symbol"],
            long_exchange=spread_info["long_exchange"],
            short_exchange=spread_info["short_exchange"],
            entry_time=timestamp,
            size_in_coins=self.config.size_in_coins,
            long_entry_price=long_slip.actual_price,
            short_entry_price=short_slip.actual_price,
            entry_spread_bps=spread_info["spread_bps"],
            fees=long_slip.total_cost - long_slip.actual_price * long_slip.filled_quantity +
                 short_slip.total_cost - short_slip.actual_price * short_slip.filled_quantity,
        )
        
        self.open_positions[trade.trade_id] = trade
        
        logger.debug(
            "spread_trade_entered",
            trade_id=trade.trade_id,
            symbol=trade.canonical_symbol,
            long=trade.long_exchange,
            short=trade.short_exchange,
            spread_bps=str(trade.entry_spread_bps)
        )
        
        if self._on_trade_open:
            self._on_trade_open(trade)
        
        return trade
    
    async def _check_exits(self, snapshot: OrderbookSnapshot):
        """Check for exit conditions on open positions."""
        for trade_id, trade in list(self.open_positions.items()):
            if trade.canonical_symbol != snapshot.symbol:
                continue
            
            should_exit = False
            exit_reason = ""
            
            # Get current books
            long_book = self.orderbook_store.get(trade.long_exchange, trade.canonical_symbol)
            short_book = self.orderbook_store.get(trade.short_exchange, trade.canonical_symbol)
            
            if not long_book or not short_book:
                continue
            
            # Calculate current spread (inverted - we're closing)
            if long_book.best_bid and short_book.best_ask:
                # To close: sell long (get bid), buy short (pay ask)
                current_spread = (
                    (long_book.best_bid.price - short_book.best_ask.price) / 
                    short_book.best_ask.price * Decimal("10000")
                )
                
                # Check spread convergence
                if current_spread >= -self.config.exit_spread_threshold_bps:
                    should_exit = True
                    exit_reason = "spread_converged"
            
            # Check max hold time
            hold_time = snapshot.timestamp - trade.entry_time
            if hold_time > self.config.max_position_hold_time:
                should_exit = True
                exit_reason = "max_hold_time"
            
            if should_exit:
                await self._exit_spread(trade, snapshot.timestamp, long_book, short_book, exit_reason)
    
    async def _exit_spread(
        self,
        trade: SpreadTrade,
        timestamp: datetime,
        long_book: OrderbookSnapshot,
        short_book: OrderbookSnapshot,
        reason: str
    ):
        """Exit a spread trade."""
        # Calculate exit slippage
        # Close long = sell, close short = buy
        long_exit = self.slippage_calc.calculate(
            long_book, TradeSide.SELL, trade.size_in_coins
        )
        short_exit = self.slippage_calc.calculate(
            short_book, TradeSide.BUY, trade.size_in_coins
        )
        
        trade.exit_time = timestamp
        trade.long_exit_price = long_exit.actual_price
        trade.short_exit_price = short_exit.actual_price
        
        if long_exit.actual_price > 0 and short_exit.actual_price > 0:
            trade.exit_spread_bps = (
                (trade.long_exit_price - trade.short_exit_price) / 
                trade.short_exit_price * Decimal("10000")
            )
        
        # Calculate P&L
        # Long leg P&L: (exit - entry) * size
        long_pnl = (trade.long_exit_price - trade.long_entry_price) * trade.size_in_coins
        
        # Short leg P&L: (entry - exit) * size
        short_pnl = (trade.short_entry_price - trade.short_exit_price) * trade.size_in_coins
        
        trade.gross_pnl = long_pnl + short_pnl
        
        # Add exit fees
        exit_fees = (
            long_exit.total_cost - long_exit.actual_price * long_exit.filled_quantity +
            short_exit.total_cost - short_exit.actual_price * short_exit.filled_quantity
        )
        trade.fees += exit_fees
        
        trade.net_pnl = trade.gross_pnl - trade.fees
        trade.is_open = False
        
        # Move to closed
        del self.open_positions[trade.trade_id]
        self.closed_positions.append(trade)
        
        logger.debug(
            "spread_trade_exited",
            trade_id=trade.trade_id,
            reason=reason,
            net_pnl=str(trade.net_pnl)
        )
        
        if self._on_trade_close:
            self._on_trade_close(trade)
    
    def _update_equity(self, timestamp: datetime):
        """Update equity curve."""
        # Realized P&L from closed trades
        realized_pnl = sum(t.net_pnl for t in self.closed_positions)
        
        # Unrealized P&L from open trades (simplified - mark to market)
        unrealized_pnl = Decimal("0")
        for trade in self.open_positions.values():
            long_book = self.orderbook_store.get(trade.long_exchange, trade.canonical_symbol)
            short_book = self.orderbook_store.get(trade.short_exchange, trade.canonical_symbol)
            
            if long_book and long_book.best_bid and short_book and short_book.best_ask:
                long_mtm = (long_book.best_bid.price - trade.long_entry_price) * trade.size_in_coins
                short_mtm = (trade.short_entry_price - short_book.best_ask.price) * trade.size_in_coins
                unrealized_pnl += long_mtm + short_mtm
        
        self.current_equity = realized_pnl + unrealized_pnl
        
        # Track peak for drawdown
        if self.current_equity > self.peak_equity:
            self.peak_equity = self.current_equity
        
        self.equity_curve.append({
            "timestamp": timestamp,
            "equity": self.current_equity,
            "peak": self.peak_equity,
            "drawdown": self.peak_equity - self.current_equity,
        })
    
    def _calculate_risk_metrics(self, result: BacktestResult):
        """Calculate risk-adjusted performance metrics."""
        if len(self.equity_curve) < 2:
            return
        
        # Max drawdown
        max_dd = Decimal("0")
        for point in self.equity_curve:
            dd = point["drawdown"]
            if dd > max_dd:
                max_dd = dd
        
        result.max_drawdown = max_dd
        
        if self.peak_equity > 0:
            result.max_drawdown_pct = (max_dd / self.peak_equity) * Decimal("100")
        
        # Calculate returns for Sharpe
        if len(result.trades) >= 2:
            returns = [t.net_pnl for t in result.trades]
            
            avg_return = sum(returns) / len(returns)
            
            # Standard deviation
            variance = sum((r - avg_return) ** 2 for r in returns) / len(returns)
            std_dev = variance ** Decimal("0.5")
            
            if std_dev > 0:
                # Annualize assuming 252 trading days
                trades_per_day = len(result.trades) / max(1, (self.config.end_time - self.config.start_time).days)
                annualization_factor = Decimal(str((252 * trades_per_day) ** 0.5))
                
                result.sharpe_ratio = (avg_return / std_dev) * annualization_factor
                
                # Sortino (downside deviation only)
                downside_returns = [r for r in returns if r < 0]
                if downside_returns:
                    downside_variance = sum(r ** 2 for r in downside_returns) / len(downside_returns)
                    downside_std = downside_variance ** Decimal("0.5")
                    if downside_std > 0:
                        result.sortino_ratio = (avg_return / downside_std) * annualization_factor
