"""
Orderbook models and playback engine for backtesting.

Provides L2 orderbook snapshots and historical playback capability.
"""

from dataclasses import dataclass, field
from datetime import datetime
from decimal import Decimal
from typing import List, Dict, Optional, Iterator, Callable
from enum import Enum
import asyncio
import json
import asyncpg
from structlog import get_logger

logger = get_logger(__name__)


class OrderbookSide(Enum):
    BID = "bid"
    ASK = "ask"


@dataclass
class OrderbookLevel:
    """Single price level in the orderbook."""
    price: Decimal
    quantity: Decimal
    
    def __post_init__(self):
        if isinstance(self.price, (int, float, str)):
            self.price = Decimal(str(self.price))
        if isinstance(self.quantity, (int, float, str)):
            self.quantity = Decimal(str(self.quantity))


@dataclass
class OrderbookSnapshot:
    """
    L2 orderbook snapshot at a specific point in time.
    
    Attributes:
        exchange: Exchange identifier (e.g., 'binance', 'bybit')
        symbol: Canonical symbol (e.g., 'BTC-USDT-PERP')
        timestamp: Snapshot timestamp
        bids: List of bid levels, sorted by price descending
        asks: List of ask levels, sorted by price ascending
        sequence: Exchange sequence number for ordering
    """
    exchange: str
    symbol: str
    timestamp: datetime
    bids: List[OrderbookLevel] = field(default_factory=list)
    asks: List[OrderbookLevel] = field(default_factory=list)
    sequence: int = 0
    
    @property
    def best_bid(self) -> Optional[OrderbookLevel]:
        """Get the best (highest) bid."""
        return self.bids[0] if self.bids else None
    
    @property
    def best_ask(self) -> Optional[OrderbookLevel]:
        """Get the best (lowest) ask."""
        return self.asks[0] if self.asks else None
    
    @property
    def mid_price(self) -> Optional[Decimal]:
        """Calculate mid price."""
        if self.best_bid and self.best_ask:
            return (self.best_bid.price + self.best_ask.price) / 2
        return None
    
    @property
    def spread(self) -> Optional[Decimal]:
        """Calculate absolute spread."""
        if self.best_bid and self.best_ask:
            return self.best_ask.price - self.best_bid.price
        return None
    
    @property
    def spread_bps(self) -> Optional[Decimal]:
        """Calculate spread in basis points."""
        if self.mid_price and self.spread:
            return (self.spread / self.mid_price) * Decimal("10000")
        return None
    
    def depth_at_price(self, side: OrderbookSide, price: Decimal) -> Decimal:
        """Calculate cumulative depth up to a price level."""
        levels = self.bids if side == OrderbookSide.BID else self.asks
        total = Decimal("0")
        
        for level in levels:
            if side == OrderbookSide.BID:
                if level.price >= price:
                    total += level.quantity
                else:
                    break
            else:  # ASK
                if level.price <= price:
                    total += level.quantity
                else:
                    break
        
        return total
    
    def total_depth(self, side: OrderbookSide, levels: int = 10) -> Decimal:
        """Calculate total depth for top N levels."""
        book = self.bids if side == OrderbookSide.BID else self.asks
        return sum(level.quantity for level in book[:levels])
    
    @classmethod
    def from_dict(cls, data: dict) -> "OrderbookSnapshot":
        """Create snapshot from dictionary."""
        return cls(
            exchange=data["exchange"],
            symbol=data["symbol"],
            timestamp=datetime.fromisoformat(data["timestamp"]) if isinstance(data["timestamp"], str) else data["timestamp"],
            bids=[OrderbookLevel(price=Decimal(str(b[0])), quantity=Decimal(str(b[1]))) for b in data.get("bids", [])],
            asks=[OrderbookLevel(price=Decimal(str(a[0])), quantity=Decimal(str(a[1]))) for a in data.get("asks", [])],
            sequence=data.get("sequence", 0)
        )
    
    def to_dict(self) -> dict:
        """Convert to dictionary."""
        return {
            "exchange": self.exchange,
            "symbol": self.symbol,
            "timestamp": self.timestamp.isoformat(),
            "bids": [[str(b.price), str(b.quantity)] for b in self.bids],
            "asks": [[str(a.price), str(a.quantity)] for a in self.asks],
            "sequence": self.sequence
        }


class OrderbookPlayback:
    """
    Replays historical orderbook snapshots for backtesting.
    
    Loads L2 snapshots from TimescaleDB and yields them in chronological order.
    """
    
    def __init__(
        self,
        db_url: str,
        exchanges: List[str],
        symbols: List[str],
        start_time: datetime,
        end_time: datetime,
        batch_size: int = 1000
    ):
        """
        Initialize playback engine.
        
        Args:
            db_url: PostgreSQL connection URL
            exchanges: List of exchanges to include
            symbols: List of canonical symbols to include
            start_time: Start of playback period
            end_time: End of playback period
            batch_size: Number of snapshots to load per batch
        """
        self.db_url = db_url
        self.exchanges = exchanges
        self.symbols = symbols
        self.start_time = start_time
        self.end_time = end_time
        self.batch_size = batch_size
        
        self._pool: Optional[asyncpg.Pool] = None
        self._current_batch: List[OrderbookSnapshot] = []
        self._batch_index = 0
        self._last_timestamp: Optional[datetime] = None
        self._exhausted = False
        
        # Callbacks for event handling
        self._on_snapshot: Optional[Callable[[OrderbookSnapshot], None]] = None
    
    async def connect(self):
        """Establish database connection pool."""
        self._pool = await asyncpg.create_pool(self.db_url, min_size=2, max_size=10)
        logger.info("orderbook_playback_connected", db_url=self.db_url[:30] + "...")
    
    async def close(self):
        """Close database connection pool."""
        if self._pool:
            await self._pool.close()
            logger.info("orderbook_playback_closed")
    
    def on_snapshot(self, callback: Callable[[OrderbookSnapshot], None]):
        """Register callback for each snapshot."""
        self._on_snapshot = callback
    
    async def _load_batch(self) -> List[OrderbookSnapshot]:
        """Load next batch of snapshots from database."""
        if not self._pool:
            raise RuntimeError("Not connected to database")
        
        start_ts = self._last_timestamp or self.start_time
        
        query = """
            SELECT 
                exchange,
                symbol,
                timestamp,
                bids,
                asks,
                sequence
            FROM orderbook_snapshots
            WHERE timestamp > $1 
              AND timestamp <= $2
              AND exchange = ANY($3)
              AND symbol = ANY($4)
            ORDER BY timestamp ASC
            LIMIT $5
        """
        
        async with self._pool.acquire() as conn:
            rows = await conn.fetch(
                query,
                start_ts,
                self.end_time,
                self.exchanges,
                self.symbols,
                self.batch_size
            )
        
        snapshots = []
        for row in rows:
            try:
                snapshot = OrderbookSnapshot(
                    exchange=row["exchange"],
                    symbol=row["symbol"],
                    timestamp=row["timestamp"],
                    bids=[OrderbookLevel(price=Decimal(str(b[0])), quantity=Decimal(str(b[1]))) 
                          for b in (json.loads(row["bids"]) if isinstance(row["bids"], str) else row["bids"])],
                    asks=[OrderbookLevel(price=Decimal(str(a[0])), quantity=Decimal(str(a[1]))) 
                          for a in (json.loads(row["asks"]) if isinstance(row["asks"], str) else row["asks"])],
                    sequence=row["sequence"]
                )
                snapshots.append(snapshot)
            except Exception as e:
                logger.error("failed_to_parse_snapshot", error=str(e), row=dict(row))
        
        if snapshots:
            self._last_timestamp = snapshots[-1].timestamp
            logger.debug("loaded_snapshot_batch", count=len(snapshots), last_ts=self._last_timestamp)
        else:
            self._exhausted = True
            logger.info("orderbook_playback_exhausted")
        
        return snapshots
    
    async def __aiter__(self) -> Iterator[OrderbookSnapshot]:
        """Async iterator over snapshots."""
        return self
    
    async def __anext__(self) -> OrderbookSnapshot:
        """Get next snapshot."""
        if self._exhausted:
            raise StopAsyncIteration
        
        # Load next batch if current is exhausted
        if self._batch_index >= len(self._current_batch):
            self._current_batch = await self._load_batch()
            self._batch_index = 0
            
            if not self._current_batch:
                raise StopAsyncIteration
        
        snapshot = self._current_batch[self._batch_index]
        self._batch_index += 1
        
        if self._on_snapshot:
            self._on_snapshot(snapshot)
        
        return snapshot
    
    async def play(self, speed_multiplier: float = 1.0, realtime: bool = False):
        """
        Play snapshots with optional timing.
        
        Args:
            speed_multiplier: Speed up playback (2.0 = 2x faster)
            realtime: If True, wait between snapshots based on timestamp gaps
        """
        last_ts: Optional[datetime] = None
        
        async for snapshot in self:
            if realtime and last_ts:
                gap = (snapshot.timestamp - last_ts).total_seconds()
                adjusted_gap = gap / speed_multiplier
                if adjusted_gap > 0:
                    await asyncio.sleep(adjusted_gap)
            
            last_ts = snapshot.timestamp
            
            if self._on_snapshot:
                self._on_snapshot(snapshot)
    
    async def get_snapshot_count(self) -> int:
        """Get total number of snapshots in the date range."""
        if not self._pool:
            raise RuntimeError("Not connected to database")
        
        query = """
            SELECT COUNT(*) 
            FROM orderbook_snapshots
            WHERE timestamp >= $1 
              AND timestamp <= $2
              AND exchange = ANY($3)
              AND symbol = ANY($4)
        """
        
        async with self._pool.acquire() as conn:
            count = await conn.fetchval(
                query,
                self.start_time,
                self.end_time,
                self.exchanges,
                self.symbols
            )
        
        return count or 0


class InMemoryOrderbookStore:
    """
    In-memory store for latest orderbooks during simulation.
    
    Maintains the current state of orderbooks for multiple exchanges/symbols.
    """
    
    def __init__(self):
        self._books: Dict[str, Dict[str, OrderbookSnapshot]] = {}
    
    def _key(self, exchange: str, symbol: str) -> str:
        return f"{exchange}:{symbol}"
    
    def update(self, snapshot: OrderbookSnapshot):
        """Update orderbook for an exchange/symbol pair."""
        if snapshot.exchange not in self._books:
            self._books[snapshot.exchange] = {}
        self._books[snapshot.exchange][snapshot.symbol] = snapshot
    
    def get(self, exchange: str, symbol: str) -> Optional[OrderbookSnapshot]:
        """Get current orderbook for an exchange/symbol pair."""
        return self._books.get(exchange, {}).get(symbol)
    
    def get_all_for_symbol(self, symbol: str) -> Dict[str, OrderbookSnapshot]:
        """Get orderbooks from all exchanges for a symbol."""
        result = {}
        for exchange, books in self._books.items():
            if symbol in books:
                result[exchange] = books[symbol]
        return result
    
    def clear(self):
        """Clear all stored orderbooks."""
        self._books.clear()
