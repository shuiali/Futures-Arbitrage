"""
CrossSpread Backtest API

FastAPI application for running backtests and retrieving results.
"""

import os
from contextlib import asynccontextmanager
from datetime import datetime, timedelta
from decimal import Decimal
from typing import List, Optional

from fastapi import FastAPI, HTTPException, BackgroundTasks
from fastapi.middleware.cors import CORSMiddleware
from pydantic import BaseModel, Field
import structlog
from prometheus_client import Counter, Histogram, Gauge, generate_latest, CONTENT_TYPE_LATEST
from starlette.responses import Response

from engine.backtest import BacktestEngine, BacktestConfig, BacktestResult
from engine.slippage import SlippageCalculator, calculate_spread_slippage
from engine.orderbook import OrderbookSnapshot, OrderbookLevel
from engine.report import ReportGenerator

# Configure structured logging
structlog.configure(
    processors=[
        structlog.stdlib.filter_by_level,
        structlog.stdlib.add_logger_name,
        structlog.stdlib.add_log_level,
        structlog.stdlib.PositionalArgumentsFormatter(),
        structlog.processors.TimeStamper(fmt="iso"),
        structlog.processors.StackInfoRenderer(),
        structlog.processors.format_exc_info,
        structlog.processors.UnicodeDecoder(),
        structlog.processors.JSONRenderer()
    ],
    wrapper_class=structlog.stdlib.BoundLogger,
    context_class=dict,
    logger_factory=structlog.stdlib.LoggerFactory(),
    cache_logger_on_first_use=True,
)

logger = structlog.get_logger(__name__)

# Prometheus metrics
BACKTEST_RUNS = Counter("backtest_runs_total", "Total backtest runs", ["status"])
BACKTEST_DURATION = Histogram("backtest_duration_seconds", "Backtest run duration")
BACKTEST_TRADES = Histogram("backtest_trades_count", "Number of trades per backtest")
SLIPPAGE_CALCULATIONS = Counter("slippage_calculations_total", "Total slippage calculations")
ACTIVE_BACKTESTS = Gauge("active_backtests", "Currently running backtests")

# Store for running backtests
running_backtests: dict = {}
completed_backtests: dict = {}


@asynccontextmanager
async def lifespan(app: FastAPI):
    """Application lifespan handler."""
    logger.info("backtest_service_starting")
    yield
    logger.info("backtest_service_stopping")


app = FastAPI(
    title="CrossSpread Backtest API",
    description="Backtest and simulation engine for spread trading strategies",
    version="1.0.0",
    lifespan=lifespan,
)

app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)


# Request/Response Models

class SlippageRequest(BaseModel):
    """Request for slippage calculation."""
    exchange: str
    symbol: str
    side: str = Field(..., pattern="^(buy|sell)$")
    size_in_coins: str
    orderbook: dict = Field(..., description="Orderbook with bids and asks")
    include_fees: bool = True


class SlippageResponse(BaseModel):
    """Response for slippage calculation."""
    expected_price: str
    actual_price: str
    slippage_abs: str
    slippage_bps: str
    total_cost: str
    filled_quantity: str
    unfilled_quantity: str
    fill_rate: str
    insufficient_liquidity: bool


class SpreadSlippageRequest(BaseModel):
    """Request for spread slippage calculation."""
    long_exchange: str
    short_exchange: str
    symbol: str
    size_in_coins: str
    long_orderbook: dict
    short_orderbook: dict
    include_fees: bool = True


class BacktestRequest(BaseModel):
    """Request to start a backtest."""
    start_time: datetime
    end_time: datetime
    exchanges: List[str]
    symbols: List[str]
    size_in_coins: str
    entry_spread_threshold_bps: str = "10"
    exit_spread_threshold_bps: str = "2"
    max_position_hold_hours: int = 24
    max_concurrent_positions: int = 5
    max_slippage_bps: str = "5"
    slice_count: int = 5
    slice_interval_ms: int = 100


class BacktestStatusResponse(BaseModel):
    """Response for backtest status."""
    backtest_id: str
    status: str
    progress: Optional[float] = None
    started_at: Optional[datetime] = None
    completed_at: Optional[datetime] = None
    error: Optional[str] = None


class BacktestSummaryResponse(BaseModel):
    """Summary of backtest results."""
    backtest_id: str
    total_trades: int
    winning_trades: int
    losing_trades: int
    win_rate: str
    profit_factor: Optional[str]
    gross_pnl: str
    total_fees: str
    net_pnl: str
    max_drawdown: str
    sharpe_ratio: Optional[str]
    sortino_ratio: Optional[str]


# Endpoints

@app.get("/health")
async def health_check():
    """Health check endpoint."""
    return {"status": "healthy", "service": "sim-backtest"}


@app.get("/metrics")
async def metrics():
    """Prometheus metrics endpoint."""
    return Response(generate_latest(), media_type=CONTENT_TYPE_LATEST)


@app.post("/api/v1/slippage/calculate", response_model=SlippageResponse)
async def calculate_slippage(request: SlippageRequest):
    """
    Calculate expected slippage for an order.
    
    Walks the orderbook to determine the volume-weighted average
    execution price and resulting slippage.
    """
    SLIPPAGE_CALCULATIONS.inc()
    
    try:
        # Parse orderbook
        book = OrderbookSnapshot(
            exchange=request.exchange,
            symbol=request.symbol,
            timestamp=datetime.utcnow(),
            bids=[OrderbookLevel(price=Decimal(str(b[0])), quantity=Decimal(str(b[1]))) 
                  for b in request.orderbook.get("bids", [])],
            asks=[OrderbookLevel(price=Decimal(str(a[0])), quantity=Decimal(str(a[1]))) 
                  for a in request.orderbook.get("asks", [])],
        )
        
        from engine.slippage import TradeSide
        
        calc = SlippageCalculator()
        side = TradeSide.BUY if request.side == "buy" else TradeSide.SELL
        size = Decimal(request.size_in_coins)
        
        result = calc.calculate(book, side, size, request.include_fees)
        
        return SlippageResponse(
            expected_price=str(result.expected_price),
            actual_price=str(result.actual_price),
            slippage_abs=str(result.slippage_abs),
            slippage_bps=str(result.slippage_bps),
            total_cost=str(result.total_cost),
            filled_quantity=str(result.filled_quantity),
            unfilled_quantity=str(result.unfilled_quantity),
            fill_rate=str(result.fill_rate),
            insufficient_liquidity=result.insufficient_liquidity,
        )
    except Exception as e:
        logger.error("slippage_calculation_failed", error=str(e))
        raise HTTPException(status_code=400, detail=str(e))


@app.post("/api/v1/slippage/spread")
async def calculate_spread_slippage_endpoint(request: SpreadSlippageRequest):
    """
    Calculate slippage for a spread trade (both legs).
    
    Returns expected execution prices and slippage for entering
    a spread position across two exchanges.
    """
    SLIPPAGE_CALCULATIONS.inc()
    
    try:
        long_book = OrderbookSnapshot(
            exchange=request.long_exchange,
            symbol=request.symbol,
            timestamp=datetime.utcnow(),
            bids=[OrderbookLevel(price=Decimal(str(b[0])), quantity=Decimal(str(b[1]))) 
                  for b in request.long_orderbook.get("bids", [])],
            asks=[OrderbookLevel(price=Decimal(str(a[0])), quantity=Decimal(str(a[1]))) 
                  for a in request.long_orderbook.get("asks", [])],
        )
        
        short_book = OrderbookSnapshot(
            exchange=request.short_exchange,
            symbol=request.symbol,
            timestamp=datetime.utcnow(),
            bids=[OrderbookLevel(price=Decimal(str(b[0])), quantity=Decimal(str(b[1]))) 
                  for b in request.short_orderbook.get("bids", [])],
            asks=[OrderbookLevel(price=Decimal(str(a[0])), quantity=Decimal(str(a[1]))) 
                  for a in request.short_orderbook.get("asks", [])],
        )
        
        size = Decimal(request.size_in_coins)
        
        result = calculate_spread_slippage(
            long_book, short_book, size, request.include_fees
        )
        
        return result
    except Exception as e:
        logger.error("spread_slippage_failed", error=str(e))
        raise HTTPException(status_code=400, detail=str(e))


@app.post("/api/v1/backtest/start")
async def start_backtest(request: BacktestRequest, background_tasks: BackgroundTasks):
    """
    Start a new backtest run.
    
    The backtest runs in the background. Use the returned ID
    to check status and retrieve results.
    """
    from uuid import uuid4
    
    backtest_id = str(uuid4())
    
    config = BacktestConfig(
        start_time=request.start_time,
        end_time=request.end_time,
        exchanges=request.exchanges,
        symbols=request.symbols,
        size_in_coins=Decimal(request.size_in_coins),
        entry_spread_threshold_bps=Decimal(request.entry_spread_threshold_bps),
        exit_spread_threshold_bps=Decimal(request.exit_spread_threshold_bps),
        max_position_hold_time=timedelta(hours=request.max_position_hold_hours),
        max_concurrent_positions=request.max_concurrent_positions,
        max_slippage_bps=Decimal(request.max_slippage_bps),
        slice_count=request.slice_count,
        slice_interval_ms=request.slice_interval_ms,
        db_url=os.getenv("DATABASE_URL", "postgresql://crossspread:changeme@localhost:5432/crossspread"),
    )
    
    running_backtests[backtest_id] = {
        "status": "running",
        "started_at": datetime.utcnow(),
        "config": config,
    }
    
    ACTIVE_BACKTESTS.inc()
    BACKTEST_RUNS.labels(status="started").inc()
    
    background_tasks.add_task(run_backtest_task, backtest_id, config)
    
    logger.info("backtest_started", backtest_id=backtest_id)
    
    return {
        "backtest_id": backtest_id,
        "status": "running",
        "message": "Backtest started. Use GET /api/v1/backtest/{id}/status to check progress."
    }


async def run_backtest_task(backtest_id: str, config: BacktestConfig):
    """Background task to run backtest."""
    import time
    
    start_time = time.time()
    
    try:
        engine = BacktestEngine(config)
        result = await engine.run()
        
        duration = time.time() - start_time
        BACKTEST_DURATION.observe(duration)
        BACKTEST_TRADES.observe(result.total_trades)
        
        # Generate and store report
        generator = ReportGenerator()
        report = generator.generate(result)
        
        completed_backtests[backtest_id] = {
            "status": "completed",
            "completed_at": datetime.utcnow(),
            "result": result,
            "report": report,
        }
        
        BACKTEST_RUNS.labels(status="completed").inc()
        logger.info("backtest_completed", backtest_id=backtest_id, trades=result.total_trades)
        
    except Exception as e:
        logger.error("backtest_failed", backtest_id=backtest_id, error=str(e))
        
        completed_backtests[backtest_id] = {
            "status": "failed",
            "completed_at": datetime.utcnow(),
            "error": str(e),
        }
        
        BACKTEST_RUNS.labels(status="failed").inc()
    
    finally:
        ACTIVE_BACKTESTS.dec()
        if backtest_id in running_backtests:
            del running_backtests[backtest_id]


@app.get("/api/v1/backtest/{backtest_id}/status", response_model=BacktestStatusResponse)
async def get_backtest_status(backtest_id: str):
    """Get the status of a backtest run."""
    if backtest_id in running_backtests:
        bt = running_backtests[backtest_id]
        return BacktestStatusResponse(
            backtest_id=backtest_id,
            status="running",
            started_at=bt["started_at"],
        )
    
    if backtest_id in completed_backtests:
        bt = completed_backtests[backtest_id]
        return BacktestStatusResponse(
            backtest_id=backtest_id,
            status=bt["status"],
            completed_at=bt["completed_at"],
            error=bt.get("error"),
        )
    
    raise HTTPException(status_code=404, detail="Backtest not found")


@app.get("/api/v1/backtest/{backtest_id}/summary", response_model=BacktestSummaryResponse)
async def get_backtest_summary(backtest_id: str):
    """Get summary of backtest results."""
    if backtest_id not in completed_backtests:
        if backtest_id in running_backtests:
            raise HTTPException(status_code=202, detail="Backtest still running")
        raise HTTPException(status_code=404, detail="Backtest not found")
    
    bt = completed_backtests[backtest_id]
    
    if bt["status"] == "failed":
        raise HTTPException(status_code=500, detail=bt["error"])
    
    result = bt["result"]
    
    return BacktestSummaryResponse(
        backtest_id=backtest_id,
        total_trades=result.total_trades,
        winning_trades=result.winning_trades,
        losing_trades=result.losing_trades,
        win_rate=str(result.win_rate),
        profit_factor=str(result.profit_factor) if result.profit_factor else None,
        gross_pnl=str(result.gross_pnl),
        total_fees=str(result.total_fees),
        net_pnl=str(result.net_pnl),
        max_drawdown=str(result.max_drawdown),
        sharpe_ratio=str(result.sharpe_ratio) if result.sharpe_ratio else None,
        sortino_ratio=str(result.sortino_ratio) if result.sortino_ratio else None,
    )


@app.get("/api/v1/backtest/{backtest_id}/trades")
async def get_backtest_trades(backtest_id: str, limit: int = 100, offset: int = 0):
    """Get trades from a completed backtest."""
    if backtest_id not in completed_backtests:
        if backtest_id in running_backtests:
            raise HTTPException(status_code=202, detail="Backtest still running")
        raise HTTPException(status_code=404, detail="Backtest not found")
    
    bt = completed_backtests[backtest_id]
    
    if bt["status"] == "failed":
        raise HTTPException(status_code=500, detail=bt["error"])
    
    report = bt["report"]
    trades = report.to_trades_list()
    
    return {
        "total": len(trades),
        "limit": limit,
        "offset": offset,
        "trades": trades[offset:offset + limit],
    }


@app.get("/api/v1/backtest/{backtest_id}/report")
async def get_backtest_report(backtest_id: str, format: str = "json"):
    """
    Get full backtest report.
    
    Args:
        format: Report format (json, csv, html)
    """
    if backtest_id not in completed_backtests:
        if backtest_id in running_backtests:
            raise HTTPException(status_code=202, detail="Backtest still running")
        raise HTTPException(status_code=404, detail="Backtest not found")
    
    bt = completed_backtests[backtest_id]
    
    if bt["status"] == "failed":
        raise HTTPException(status_code=500, detail=bt["error"])
    
    report = bt["report"]
    
    if format == "json":
        return {
            "summary": report.to_summary_dict(),
            "trades": report.to_trades_list(),
        }
    elif format == "csv":
        generator = ReportGenerator()
        filepath = generator.save_csv(report, f"backtest_{backtest_id}.csv")
        return {"file": str(filepath)}
    elif format == "html":
        generator = ReportGenerator()
        filepath = generator.save_html(report, f"backtest_{backtest_id}.html")
        return {"file": str(filepath)}
    else:
        raise HTTPException(status_code=400, detail=f"Unknown format: {format}")


@app.get("/api/v1/backtest/list")
async def list_backtests():
    """List all backtest runs."""
    backtests = []
    
    for bt_id, bt in running_backtests.items():
        backtests.append({
            "backtest_id": bt_id,
            "status": "running",
            "started_at": bt["started_at"].isoformat(),
        })
    
    for bt_id, bt in completed_backtests.items():
        backtests.append({
            "backtest_id": bt_id,
            "status": bt["status"],
            "completed_at": bt["completed_at"].isoformat(),
        })
    
    return {"backtests": backtests}


if __name__ == "__main__":
    import uvicorn
    
    port = int(os.getenv("PORT", "8002"))
    uvicorn.run(app, host="0.0.0.0", port=port)
