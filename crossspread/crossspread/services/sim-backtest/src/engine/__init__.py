"""
CrossSpread Backtest & Simulation Engine

This module provides:
1. L2 orderbook playback from historical data
2. Simulated order execution with realistic slippage
3. Backtest reporting with PnL, Sharpe ratio, drawdown analysis
"""

from .orderbook import OrderbookSnapshot, OrderbookLevel, OrderbookPlayback
from .simulator import SimulatedExchange, SimulatedOrder, SimulatedFill
from .backtest import BacktestEngine, BacktestConfig, BacktestResult
from .slippage import SlippageCalculator, SlippageResult
from .report import BacktestReport, ReportGenerator

__version__ = "1.0.0"
__all__ = [
    "OrderbookSnapshot",
    "OrderbookLevel", 
    "OrderbookPlayback",
    "SimulatedExchange",
    "SimulatedOrder",
    "SimulatedFill",
    "BacktestEngine",
    "BacktestConfig",
    "BacktestResult",
    "SlippageCalculator",
    "SlippageResult",
    "BacktestReport",
    "ReportGenerator",
]
