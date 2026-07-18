"""HTML report generation."""

from __future__ import annotations

from datetime import datetime
from pathlib import Path

import pandas as pd

from common.settings import RESULTS_DIR


def write_report(results: dict[str, pd.DataFrame], strategy_name: str = "backtest", run_dir: Path | None = None) -> Path:
    run_id = datetime.now().strftime("%Y%m%d_%H%M%S")
    out = run_dir or (RESULTS_DIR / run_id)
    out.mkdir(parents=True, exist_ok=True)

    trades = results["trades"]
    skipped_expiries = results["skipped_expiries"]
    if not trades.empty:
        trades.to_csv(out / "trades.csv", index=False)
    if not skipped_expiries.empty:
        skipped_expiries.to_csv(out / "skipped_expiries.csv", index=False)

    html = f"""<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <title>{strategy_name} Backtest</title>
  <style>
    body {{ font-family: system-ui, sans-serif; margin: 2rem; background: #fafafa; color: #111; }}
    h1, h2 {{ margin-top: 1.5rem; }}
    table.data-table {{ border-collapse: collapse; width: 100%; background: #fff; font-size: 0.9rem; }}
    table.data-table th, table.data-table td {{ border: 1px solid #ddd; padding: 0.4rem 0.6rem; text-align: left; }}
    table.data-table th {{ background: #eee; }}
    .section {{ background: #fff; border: 1px solid #ddd; border-radius: 8px; padding: 1rem; margin: 1rem 0; overflow-x: auto; }}
  </style>
</head>
<body>
  <h1>{strategy_name} Backtest</h1>
  <p>Run: {run_id}</p>
  <div class="section"><h2>Trades</h2>{_df_to_html_table(trades)}</div>
  <div class="section"><h2>Skipped Expiries</h2>{_df_to_html_table(skipped_expiries)}</div>
</body>
</html>
"""
    (out / "report.html").write_text(html, encoding="utf-8")
    return out


def _df_to_html_table(df: pd.DataFrame) -> str:
    if df.empty:
        return "<p><em>No rows.</em></p>"
    return df.to_html(index=False, classes="data-table", border=0)
