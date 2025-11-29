"""
Backtest report generator.

Produces detailed reports in various formats (JSON, HTML, CSV).
"""

from dataclasses import dataclass, asdict
from datetime import datetime
from decimal import Decimal
from typing import Optional, List, Dict, Any
from pathlib import Path
import json
import csv

from structlog import get_logger

from .backtest import BacktestResult, SpreadTrade

logger = get_logger(__name__)


class DecimalEncoder(json.JSONEncoder):
    """JSON encoder that handles Decimal types."""
    
    def default(self, obj):
        if isinstance(obj, Decimal):
            return str(obj)
        if isinstance(obj, datetime):
            return obj.isoformat()
        return super().default(obj)


@dataclass
class BacktestReport:
    """
    Formatted backtest report.
    
    Attributes:
        result: The backtest result
        generated_at: Report generation timestamp
        title: Report title
        notes: Additional notes
    """
    result: BacktestResult
    generated_at: datetime
    title: str = "CrossSpread Backtest Report"
    notes: str = ""
    
    def to_summary_dict(self) -> Dict[str, Any]:
        """Get summary as dictionary."""
        r = self.result
        return {
            "report": {
                "title": self.title,
                "generated_at": self.generated_at.isoformat(),
                "notes": self.notes,
            },
            "config": {
                "start_time": r.config.start_time.isoformat(),
                "end_time": r.config.end_time.isoformat(),
                "exchanges": r.config.exchanges,
                "symbols": r.config.symbols,
                "size_in_coins": str(r.config.size_in_coins),
                "entry_threshold_bps": str(r.config.entry_spread_threshold_bps),
                "exit_threshold_bps": str(r.config.exit_spread_threshold_bps),
            },
            "performance": {
                "total_trades": r.total_trades,
                "winning_trades": r.winning_trades,
                "losing_trades": r.losing_trades,
                "win_rate_pct": str(r.win_rate),
                "profit_factor": str(r.profit_factor) if r.profit_factor else None,
                "avg_trade_pnl": str(r.avg_trade_pnl),
            },
            "pnl": {
                "gross_pnl": str(r.gross_pnl),
                "total_fees": str(r.total_fees),
                "net_pnl": str(r.net_pnl),
            },
            "risk": {
                "max_drawdown": str(r.max_drawdown),
                "max_drawdown_pct": str(r.max_drawdown_pct),
                "sharpe_ratio": str(r.sharpe_ratio) if r.sharpe_ratio else None,
                "sortino_ratio": str(r.sortino_ratio) if r.sortino_ratio else None,
            },
            "execution": {
                "snapshots_processed": r.total_snapshots_processed,
                "avg_spread_bps": str(r.avg_spread_bps),
                "avg_slippage_bps": str(r.avg_slippage_bps),
                "run_duration_seconds": (r.run_end - r.run_start).total_seconds() if r.run_end else None,
            },
        }
    
    def to_trades_list(self) -> List[Dict[str, Any]]:
        """Get trades as list of dictionaries."""
        trades = []
        for t in self.result.trades:
            trades.append({
                "trade_id": t.trade_id,
                "symbol": t.canonical_symbol,
                "long_exchange": t.long_exchange,
                "short_exchange": t.short_exchange,
                "entry_time": t.entry_time.isoformat(),
                "exit_time": t.exit_time.isoformat() if t.exit_time else None,
                "size_in_coins": str(t.size_in_coins),
                "long_entry_price": str(t.long_entry_price) if t.long_entry_price else None,
                "short_entry_price": str(t.short_entry_price) if t.short_entry_price else None,
                "long_exit_price": str(t.long_exit_price) if t.long_exit_price else None,
                "short_exit_price": str(t.short_exit_price) if t.short_exit_price else None,
                "entry_spread_bps": str(t.entry_spread_bps) if t.entry_spread_bps else None,
                "exit_spread_bps": str(t.exit_spread_bps) if t.exit_spread_bps else None,
                "gross_pnl": str(t.gross_pnl),
                "fees": str(t.fees),
                "net_pnl": str(t.net_pnl),
                "pnl_bps": str(t.pnl_bps),
                "duration_seconds": t.duration.total_seconds() if t.duration else None,
                "is_open": t.is_open,
            })
        return trades


class ReportGenerator:
    """
    Generates reports in various formats.
    """
    
    def __init__(self, output_dir: str = "./reports"):
        """
        Initialize generator.
        
        Args:
            output_dir: Directory to save reports
        """
        self.output_dir = Path(output_dir)
        self.output_dir.mkdir(parents=True, exist_ok=True)
    
    def generate(
        self,
        result: BacktestResult,
        title: str = "CrossSpread Backtest Report",
        notes: str = ""
    ) -> BacktestReport:
        """
        Generate a report from backtest result.
        
        Args:
            result: Backtest result
            title: Report title
            notes: Additional notes
            
        Returns:
            BacktestReport object
        """
        return BacktestReport(
            result=result,
            generated_at=datetime.utcnow(),
            title=title,
            notes=notes,
        )
    
    def save_json(self, report: BacktestReport, filename: Optional[str] = None) -> Path:
        """
        Save report as JSON.
        
        Args:
            report: The report to save
            filename: Optional filename (auto-generated if not provided)
            
        Returns:
            Path to saved file
        """
        if not filename:
            timestamp = report.generated_at.strftime("%Y%m%d_%H%M%S")
            filename = f"backtest_report_{timestamp}.json"
        
        filepath = self.output_dir / filename
        
        data = {
            "summary": report.to_summary_dict(),
            "trades": report.to_trades_list(),
        }
        
        with open(filepath, "w") as f:
            json.dump(data, f, cls=DecimalEncoder, indent=2)
        
        logger.info("report_saved_json", path=str(filepath))
        return filepath
    
    def save_csv(self, report: BacktestReport, filename: Optional[str] = None) -> Path:
        """
        Save trades as CSV.
        
        Args:
            report: The report to save
            filename: Optional filename
            
        Returns:
            Path to saved file
        """
        if not filename:
            timestamp = report.generated_at.strftime("%Y%m%d_%H%M%S")
            filename = f"backtest_trades_{timestamp}.csv"
        
        filepath = self.output_dir / filename
        trades = report.to_trades_list()
        
        if not trades:
            logger.warning("no_trades_to_export")
            return filepath
        
        with open(filepath, "w", newline="") as f:
            writer = csv.DictWriter(f, fieldnames=trades[0].keys())
            writer.writeheader()
            writer.writerows(trades)
        
        logger.info("report_saved_csv", path=str(filepath))
        return filepath
    
    def save_html(self, report: BacktestReport, filename: Optional[str] = None) -> Path:
        """
        Save report as HTML.
        
        Args:
            report: The report to save
            filename: Optional filename
            
        Returns:
            Path to saved file
        """
        if not filename:
            timestamp = report.generated_at.strftime("%Y%m%d_%H%M%S")
            filename = f"backtest_report_{timestamp}.html"
        
        filepath = self.output_dir / filename
        
        summary = report.to_summary_dict()
        trades = report.to_trades_list()
        
        html = self._generate_html(summary, trades, report)
        
        with open(filepath, "w") as f:
            f.write(html)
        
        logger.info("report_saved_html", path=str(filepath))
        return filepath
    
    def _generate_html(
        self,
        summary: Dict[str, Any],
        trades: List[Dict[str, Any]],
        report: BacktestReport
    ) -> str:
        """Generate HTML report content."""
        
        # Determine P&L color
        net_pnl = Decimal(summary["pnl"]["net_pnl"])
        pnl_color = "green" if net_pnl >= 0 else "red"
        
        trades_html = ""
        for t in trades[:100]:  # Limit to 100 trades in HTML
            pnl = Decimal(t["net_pnl"])
            row_class = "win" if pnl >= 0 else "loss"
            trades_html += f"""
            <tr class="{row_class}">
                <td>{t["entry_time"][:19]}</td>
                <td>{t["symbol"]}</td>
                <td>{t["long_exchange"]} → {t["short_exchange"]}</td>
                <td>{t["size_in_coins"]}</td>
                <td>{t["entry_spread_bps"] or '-'}</td>
                <td>{t["exit_spread_bps"] or '-'}</td>
                <td style="color: {"green" if pnl >= 0 else "red"}">{t["net_pnl"]}</td>
            </tr>
            """
        
        return f"""
<!DOCTYPE html>
<html>
<head>
    <title>{report.title}</title>
    <style>
        body {{
            font-family: 'Segoe UI', Arial, sans-serif;
            margin: 40px;
            background: #1a1a2e;
            color: #eee;
        }}
        h1, h2, h3 {{
            color: #00d4ff;
        }}
        .container {{
            max-width: 1200px;
            margin: 0 auto;
        }}
        .grid {{
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(280px, 1fr));
            gap: 20px;
            margin-bottom: 30px;
        }}
        .card {{
            background: #16213e;
            border-radius: 10px;
            padding: 20px;
            box-shadow: 0 4px 6px rgba(0,0,0,0.3);
        }}
        .card h3 {{
            margin-top: 0;
            border-bottom: 2px solid #00d4ff;
            padding-bottom: 10px;
        }}
        .metric {{
            display: flex;
            justify-content: space-between;
            padding: 8px 0;
            border-bottom: 1px solid #333;
        }}
        .metric-label {{
            color: #888;
        }}
        .metric-value {{
            font-weight: bold;
        }}
        .pnl-positive {{
            color: #00ff88;
        }}
        .pnl-negative {{
            color: #ff4444;
        }}
        table {{
            width: 100%;
            border-collapse: collapse;
            margin-top: 20px;
        }}
        th, td {{
            padding: 12px;
            text-align: left;
            border-bottom: 1px solid #333;
        }}
        th {{
            background: #0f3460;
            color: #00d4ff;
        }}
        tr:hover {{
            background: #1f4068;
        }}
        tr.win td:last-child {{
            color: #00ff88;
        }}
        tr.loss td:last-child {{
            color: #ff4444;
        }}
        .footer {{
            margin-top: 40px;
            padding-top: 20px;
            border-top: 1px solid #333;
            color: #666;
            text-align: center;
        }}
    </style>
</head>
<body>
    <div class="container">
        <h1>📊 {report.title}</h1>
        <p>Generated: {summary["report"]["generated_at"]}</p>
        
        <div class="grid">
            <div class="card">
                <h3>📈 Performance</h3>
                <div class="metric">
                    <span class="metric-label">Total Trades</span>
                    <span class="metric-value">{summary["performance"]["total_trades"]}</span>
                </div>
                <div class="metric">
                    <span class="metric-label">Win Rate</span>
                    <span class="metric-value">{summary["performance"]["win_rate_pct"]}%</span>
                </div>
                <div class="metric">
                    <span class="metric-label">Profit Factor</span>
                    <span class="metric-value">{summary["performance"]["profit_factor"] or "N/A"}</span>
                </div>
                <div class="metric">
                    <span class="metric-label">Avg Trade P&L</span>
                    <span class="metric-value">{summary["performance"]["avg_trade_pnl"]}</span>
                </div>
            </div>
            
            <div class="card">
                <h3>💰 P&L</h3>
                <div class="metric">
                    <span class="metric-label">Gross P&L</span>
                    <span class="metric-value">{summary["pnl"]["gross_pnl"]}</span>
                </div>
                <div class="metric">
                    <span class="metric-label">Total Fees</span>
                    <span class="metric-value">{summary["pnl"]["total_fees"]}</span>
                </div>
                <div class="metric">
                    <span class="metric-label">Net P&L</span>
                    <span class="metric-value {pnl_color}">{summary["pnl"]["net_pnl"]}</span>
                </div>
            </div>
            
            <div class="card">
                <h3>⚠️ Risk Metrics</h3>
                <div class="metric">
                    <span class="metric-label">Max Drawdown</span>
                    <span class="metric-value">{summary["risk"]["max_drawdown"]}</span>
                </div>
                <div class="metric">
                    <span class="metric-label">Max DD %</span>
                    <span class="metric-value">{summary["risk"]["max_drawdown_pct"]}%</span>
                </div>
                <div class="metric">
                    <span class="metric-label">Sharpe Ratio</span>
                    <span class="metric-value">{summary["risk"]["sharpe_ratio"] or "N/A"}</span>
                </div>
                <div class="metric">
                    <span class="metric-label">Sortino Ratio</span>
                    <span class="metric-value">{summary["risk"]["sortino_ratio"] or "N/A"}</span>
                </div>
            </div>
            
            <div class="card">
                <h3>⚡ Execution</h3>
                <div class="metric">
                    <span class="metric-label">Snapshots</span>
                    <span class="metric-value">{summary["execution"]["snapshots_processed"]:,}</span>
                </div>
                <div class="metric">
                    <span class="metric-label">Avg Spread (bps)</span>
                    <span class="metric-value">{summary["execution"]["avg_spread_bps"]}</span>
                </div>
                <div class="metric">
                    <span class="metric-label">Avg Slippage (bps)</span>
                    <span class="metric-value">{summary["execution"]["avg_slippage_bps"]}</span>
                </div>
                <div class="metric">
                    <span class="metric-label">Run Time</span>
                    <span class="metric-value">{summary["execution"]["run_duration_seconds"]:.1f}s</span>
                </div>
            </div>
        </div>
        
        <h2>📋 Trade History</h2>
        <table>
            <thead>
                <tr>
                    <th>Entry Time</th>
                    <th>Symbol</th>
                    <th>Direction</th>
                    <th>Size</th>
                    <th>Entry Spread</th>
                    <th>Exit Spread</th>
                    <th>Net P&L</th>
                </tr>
            </thead>
            <tbody>
                {trades_html}
            </tbody>
        </table>
        
        <div class="footer">
            <p>CrossSpread Backtest Engine v1.0 | {len(trades)} total trades</p>
        </div>
    </div>
</body>
</html>
        """
