"""CSV and HTML report generation."""

from __future__ import annotations

import base64
import io
from datetime import datetime
from pathlib import Path

import matplotlib

matplotlib.use("Agg")
import matplotlib.pyplot as plt
import pandas as pd

from settings import RESULTS_DIR, StrategyConfig
from strategy import SkippedTrade, Trade


def _trades_df(trades: list[Trade]) -> pd.DataFrame:
    if not trades:
        return pd.DataFrame()
    return pd.DataFrame([t.to_dict() for t in trades])


def _skipped_df(skipped: list[SkippedTrade]) -> pd.DataFrame:
    if not skipped:
        return pd.DataFrame()
    return pd.DataFrame([s.to_dict() for s in skipped])


def _equity_df(trades: list[Trade]) -> pd.DataFrame:
    if not trades:
        return pd.DataFrame(columns=["expiry_date", "pnl", "cumulative_pnl"])
    df = _trades_df(trades)[["expiry_date", "pnl"]].copy()
    df["cumulative_pnl"] = df["pnl"].cumsum()
    return df


def _summary(trades: list[Trade], skipped: list[SkippedTrade]) -> dict:
    if not trades:
        return {
            "total_pnl": 0.0,
            "trades_taken": 0,
            "trades_skipped": len(skipped),
            "win_rate": 0.0,
            "avg_pnl": 0.0,
            "best_trade": 0.0,
            "worst_trade": 0.0,
            "max_drawdown": 0.0,
            "avg_entry_credit": 0.0,
        }
    pnls = [t.pnl for t in trades]
    wins = sum(1 for p in pnls if p > 0)
    equity = _equity_df(trades)
    peak = equity["cumulative_pnl"].cummax()
    drawdown = equity["cumulative_pnl"] - peak
    return {
        "total_pnl": round(sum(pnls), 2),
        "trades_taken": len(trades),
        "trades_skipped": len(skipped),
        "win_rate": round(100 * wins / len(trades), 1),
        "avg_pnl": round(sum(pnls) / len(trades), 2),
        "best_trade": round(max(pnls), 2),
        "worst_trade": round(min(pnls), 2),
        "max_drawdown": round(float(drawdown.min()), 2),
        "avg_entry_credit": round(sum(t.entry_premium for t in trades) / len(trades), 2),
    }


def _equity_chart_b64(trades: list[Trade]) -> str:
    if not trades:
        return ""
    eq = _equity_df(trades)
    fig, ax = plt.subplots(figsize=(10, 4))
    ax.plot(eq["expiry_date"], eq["cumulative_pnl"], marker="o", linewidth=2)
    ax.set_title("Cumulative PnL")
    ax.set_xlabel("Expiry date")
    ax.set_ylabel("PnL (₹)")
    ax.grid(True, alpha=0.3)
    plt.xticks(rotation=45, ha="right")
    fig.tight_layout()
    buf = io.BytesIO()
    fig.savefig(buf, format="png", dpi=120)
    plt.close(fig)
    return base64.b64encode(buf.getvalue()).decode("ascii")


def _df_to_html_table(df: pd.DataFrame) -> str:
    if df.empty:
        return "<p><em>No rows.</em></p>"
    return df.to_html(index=False, classes="data-table", border=0)


def write_report(
    trades: list[Trade],
    skipped: list[SkippedTrade],
    config: StrategyConfig,
    run_dir: Path | None = None,
) -> Path:
    run_id = datetime.now().strftime("%Y%m%d_%H%M%S")
    out = run_dir or (RESULTS_DIR / run_id)
    out.mkdir(parents=True, exist_ok=True)

    trades_df = _trades_df(trades)
    skipped_df = _skipped_df(skipped)
    equity_df = _equity_df(trades)
    summary = _summary(trades, skipped)

    if not trades_df.empty:
        trades_df.to_csv(out / "trades.csv", index=False)
    if not skipped_df.empty:
        skipped_df.to_csv(out / "skipped.csv", index=False)
    if not equity_df.empty:
        equity_df.to_csv(out / "equity_curve.csv", index=False)

    chart_b64 = _equity_chart_b64(trades)
    chart_html = (
        f'<img src="data:image/png;base64,{chart_b64}" alt="Equity curve" style="max-width:100%;">'
        if chart_b64
        else "<p><em>No trades to chart.</em></p>"
    )

    cfg_html = f"""
    <ul>
      <li>Entry DTE: {config.entry_dte}</li>
      <li>Entry time: {config.entry_time.strftime('%H:%M')} IST</li>
      <li>Exit time: {config.exit_time.strftime('%H:%M')} IST</li>
      <li>Strike offset: ±{config.strike_offset}</li>
      <li>Date range: {config.start_date} → {config.end_date}</li>
    </ul>
    """

    cards = f"""
    <div class="cards">
      <div class="card"><div class="label">Total PnL</div><div class="value">₹{summary['total_pnl']:,.0f}</div></div>
      <div class="card"><div class="label">Trades taken</div><div class="value">{summary['trades_taken']}</div></div>
      <div class="card"><div class="label">Skipped</div><div class="value">{summary['trades_skipped']}</div></div>
      <div class="card"><div class="label">Win rate</div><div class="value">{summary['win_rate']}%</div></div>
      <div class="card"><div class="label">Avg PnL / trade</div><div class="value">₹{summary['avg_pnl']:,.0f}</div></div>
      <div class="card"><div class="label">Max drawdown</div><div class="value">₹{summary['max_drawdown']:,.0f}</div></div>
      <div class="card"><div class="label">Best / Worst</div><div class="value">₹{summary['best_trade']:,.0f} / ₹{summary['worst_trade']:,.0f}</div></div>
      <div class="card"><div class="label">Avg entry credit</div><div class="value">₹{summary['avg_entry_credit']:,.2f}</div></div>
    </div>
    """

    html = f"""<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <title>Nifty Short Strangle Backtest</title>
  <style>
    body {{ font-family: system-ui, sans-serif; margin: 2rem; background: #fafafa; color: #111; }}
    h1, h2 {{ margin-top: 1.5rem; }}
    .cards {{ display: grid; grid-template-columns: repeat(auto-fill, minmax(160px, 1fr)); gap: 1rem; margin: 1rem 0; }}
    .card {{ background: #fff; border: 1px solid #ddd; border-radius: 8px; padding: 1rem; }}
    .label {{ font-size: 0.85rem; color: #666; }}
    .value {{ font-size: 1.25rem; font-weight: 600; margin-top: 0.25rem; }}
    table.data-table {{ border-collapse: collapse; width: 100%; background: #fff; font-size: 0.9rem; }}
    table.data-table th, table.data-table td {{ border: 1px solid #ddd; padding: 0.4rem 0.6rem; text-align: left; }}
    table.data-table th {{ background: #eee; }}
    .section {{ background: #fff; border: 1px solid #ddd; border-radius: 8px; padding: 1rem; margin: 1rem 0; overflow-x: auto; }}
  </style>
</head>
<body>
  <h1>Nifty Short Strangle Backtest</h1>
  <p>Run: {run_id}</p>
  <div class="section"><h2>Configuration</h2>{cfg_html}</div>
  <div class="section"><h2>Summary</h2>{cards}</div>
  <div class="section"><h2>Equity curve</h2>{chart_html}</div>
  <div class="section"><h2>Trades</h2>{_df_to_html_table(trades_df)}</div>
  <div class="section"><h2>Skipped</h2>{_df_to_html_table(skipped_df)}</div>
</body>
</html>
"""
    (out / "report.html").write_text(html, encoding="utf-8")
    return out
