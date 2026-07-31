"""Backtest metrics derived from saved result rows."""

from __future__ import annotations

from typing import Any

import numpy as np
import pandas as pd


def build_backtest_metrics(
    *,
    trades: pd.DataFrame,
    skipped_expiries: pd.DataFrame,
    daily_mtm: pd.DataFrame,
    include_mtm: bool = True,
) -> dict[str, Any]:
    expiry_pnl = _expiry_pnl(trades)
    mtm_curve = _daily_portfolio_mtm(daily_mtm) if include_mtm else pd.DataFrame()
    skip_counts = _skip_counts(skipped_expiries)

    traded_expiries = int(expiry_pnl["expiry_date"].nunique()) if not expiry_pnl.empty else 0
    skipped_count = _skipped_expiry_count(skipped_expiries)
    total_expiries = traded_expiries + skipped_count
    winning_expiries = int((expiry_pnl["pnl"] > 0).sum()) if not expiry_pnl.empty else 0
    losing_expiries = int((expiry_pnl["pnl"] < 0).sum()) if not expiry_pnl.empty else 0
    total_pnl = _round(expiry_pnl["pnl"].sum()) if not expiry_pnl.empty else 0.0
    average_profit = _average_profit(expiry_pnl)
    average_loss = _average_loss(expiry_pnl)
    win_rate_decimal = winning_expiries / traded_expiries if traded_expiries else 0.0
    loss_rate_decimal = losing_expiries / traded_expiries if traded_expiries else 0.0

    return {
        "total_expiries": total_expiries,
        "traded_expiries": traded_expiries,
        "skipped_expiries": skipped_count,
        "total_trades": int(trades["trade_id"].nunique()) if not trades.empty else 0,
        "total_pnl": total_pnl,
        "average_pnl_per_expiry": _round(total_pnl / traded_expiries) if traded_expiries else 0.0,
        "best_expiry_pnl": _round(expiry_pnl["pnl"].max()) if not expiry_pnl.empty else 0.0,
        "worst_expiry_pnl": _round(expiry_pnl["pnl"].min()) if not expiry_pnl.empty else 0.0,
        "winning_expiries": winning_expiries,
        "losing_expiries": losing_expiries,
        "win_rate": _round((winning_expiries / traded_expiries) * 100) if traded_expiries else 0.0,
        "average_profit": average_profit,
        "average_loss": average_loss,
        "risk_reward_ratio": (
            _round(average_loss / average_profit)
            if average_profit and average_profit > 0
            else None
        ),
        "expectancy": _round((win_rate_decimal * average_profit) - (loss_rate_decimal * average_loss)),
        "max_drawdown": _max_drawdown(mtm_curve) if include_mtm else None,
        "max_profit_seen": _round(mtm_curve["mtm"].max()) if include_mtm and not mtm_curve.empty else None,
        "max_loss_seen": _round(mtm_curve["mtm"].min()) if include_mtm and not mtm_curve.empty else None,
        "ending_mtm": _round(mtm_curve["mtm"].iloc[-1]) if include_mtm and not mtm_curve.empty else None,
        "best_mtm_day": _day_for_extreme(mtm_curve, "max") if include_mtm else None,
        "worst_mtm_day": _day_for_extreme(mtm_curve, "min") if include_mtm else None,
        "most_common_skip_reason": _most_common_skip_reason(skip_counts),
        "skip_reason_counts": skip_counts,
    }


def build_equity_curve(trades: pd.DataFrame) -> list[dict[str, Any]]:
    realized = _realized_pnl_by_exit_date(trades)
    return _equity_curve(realized)


def build_expiry_pnl_curve(trades: pd.DataFrame) -> list[dict[str, Any]]:
    expiry_pnl = _expiry_pnl(trades)
    if expiry_pnl.empty:
        return []
    rows = (
        expiry_pnl.groupby("expiry_date", as_index=False)["pnl"]
        .sum()
        .sort_values("expiry_date")
    )
    return [
        {"date": str(row["expiry_date"]), "pnl": _round(row["pnl"])}
        for _, row in rows.iterrows()
    ]


def build_average_mtm_by_expiry(*, trades: pd.DataFrame, daily_mtm: pd.DataFrame) -> list[dict[str, Any]]:
    if trades.empty or daily_mtm.empty:
        return []
    rows = daily_mtm.copy()
    rows["mtm"] = pd.to_numeric(rows["mtm"], errors="coerce")
    rows = rows.dropna(subset=["expiry_date", "mtm_date", "mtm_time", "mtm"])
    if rows.empty:
        return []
    premium_by_expiry = _premium_by_expiry(trades)
    intraday_expiry_mtm = rows.groupby(["expiry_date", "mtm_date", "mtm_time"], as_index=False)["mtm"].sum()
    average_mtm = (
        intraday_expiry_mtm.groupby("expiry_date", as_index=False)["mtm"]
        .mean()
        .sort_values("expiry_date")
    )
    result = []
    for _, row in average_mtm.iterrows():
        expiry = str(row["expiry_date"])
        premium = premium_by_expiry.get(expiry)
        value = None
        if premium is not None and abs(premium) > 0:
            value = _round((float(row["mtm"]) / abs(premium)) * 100)
        result.append({"date": expiry, "average_mtm_pct_of_premium": value})
    return result


def build_trade_metrics(*, trades: pd.DataFrame, daily_mtm: pd.DataFrame) -> list[dict[str, Any]]:
    if trades.empty:
        return []

    premiums = _premium_by_trade(trades)
    trade_mtm = _trade_daily_mtm(daily_mtm)
    rows = []
    for _, trade in _trade_summary(trades).iterrows():
        trade_id = str(trade["trade_id"])
        premium = premiums.get(trade_id)
        mtm_rows = trade_mtm[trade_mtm["trade_id"] == trade_id]
        rows.append(
            {
                "trade_id": trade_id,
                "expiry_date": str(trade["expiry_date"]),
                "exit_date": str(trade["exit_date"]),
                "exit_reason": str(trade["exit_reason"]) if "exit_reason" in trade else "",
                "premium_received": None if premium is None else _round(premium),
                "maxMtm": _max_mtm(mtm_rows),
                "minMtm": _min_mtm(mtm_rows),
                "mtmVolatilityPctOfPremium": _mtm_volatility_pct_of_premium(
                    mtm_rows=mtm_rows,
                    premium_received=premium,
                ),
            }
        )
    return rows


def metric_cards(metrics: dict[str, Any]) -> list[dict[str, Any]]:
    cards = [
        {"label": "Total PnL", "value": metrics.get("total_pnl", 0.0)},
        {"label": "Win Rate", "value": f"{metrics.get('win_rate', 0.0)}%"},
        {"label": "RR", "value": metrics.get("risk_reward_ratio") or "n/a"},
        {"label": "Expectancy", "value": metrics.get("expectancy", 0.0)},
        {"label": "Best Expiry", "value": metrics.get("best_expiry_pnl", 0.0)},
        {"label": "Worst Expiry", "value": metrics.get("worst_expiry_pnl", 0.0)},
        {"label": "Avg PnL / Expiry", "value": metrics.get("average_pnl_per_expiry", 0.0)},
        {"label": "Traded Expiries", "value": metrics.get("traded_expiries", 0)},
        {"label": "Skipped Expiries", "value": metrics.get("skipped_expiries", 0)},
        {"label": "Most Common Skip", "value": metrics.get("most_common_skip_reason") or "none"},
    ]
    if metrics.get("max_drawdown") is not None:
        cards.insert(2, {"label": "Max Drawdown", "value": metrics.get("max_drawdown", 0.0)})
    return cards


def _expiry_pnl(trades: pd.DataFrame) -> pd.DataFrame:
    if trades.empty:
        return pd.DataFrame(columns=["trade_id", "expiry_date", "pnl"])
    rows = trades.copy()
    rows["leg_pnl"] = _leg_pnl(rows)
    return (
        rows.groupby(["trade_id", "expiry_date"], as_index=False)["leg_pnl"]
        .sum()
        .rename(columns={"leg_pnl": "pnl"})
    )


def _average_profit(expiry_pnl: pd.DataFrame) -> float:
    if expiry_pnl.empty:
        return 0.0
    wins = pd.to_numeric(expiry_pnl["pnl"], errors="coerce")
    wins = wins[wins > 0]
    return _round(wins.mean()) if not wins.empty else 0.0


def _average_loss(expiry_pnl: pd.DataFrame) -> float:
    if expiry_pnl.empty:
        return 0.0
    losses = pd.to_numeric(expiry_pnl["pnl"], errors="coerce")
    losses = losses[losses < 0].abs()
    return _round(losses.mean()) if not losses.empty else 0.0


def _premium_by_trade(trades: pd.DataFrame) -> dict[str, float]:
    if trades.empty:
        return {}
    rows = trades.copy()
    rows["premium"] = pd.to_numeric(rows["entry_price"], errors="coerce") * pd.to_numeric(
        rows["lot_size"], errors="coerce"
    )
    premium = rows.groupby("trade_id")["premium"].sum()
    return {str(trade_id): float(value) for trade_id, value in premium.items()}


def _premium_by_expiry(trades: pd.DataFrame) -> dict[str, float]:
    if trades.empty:
        return {}
    rows = trades.copy()
    rows["premium"] = pd.to_numeric(rows["entry_price"], errors="coerce") * pd.to_numeric(
        rows["lot_size"], errors="coerce"
    )
    premium = rows.groupby("expiry_date")["premium"].sum()
    return {str(expiry): float(value) for expiry, value in premium.items()}


def _trade_summary(trades: pd.DataFrame) -> pd.DataFrame:
    columns = ["trade_id", "expiry_date", "exit_date"]
    if "exit_reason" in trades.columns:
        columns.append("exit_reason")
    return (
        trades[columns]
        .drop_duplicates()
        .sort_values(["exit_date", "trade_id"])
    )


def _trade_daily_mtm(daily_mtm: pd.DataFrame) -> pd.DataFrame:
    if daily_mtm.empty:
        return pd.DataFrame(columns=["trade_id", "mtm_date", "mtm"])
    rows = daily_mtm.copy()
    rows["mtm"] = pd.to_numeric(rows["mtm"], errors="coerce").fillna(0.0)
    group_columns = ["trade_id", "mtm_date"]
    if "mtm_time" in rows.columns:
        group_columns.append("mtm_time")
    return (
        rows.groupby(group_columns, as_index=False)["mtm"]
        .sum()
        .sort_values(group_columns)
    )


def _mtm_volatility_pct_of_premium(
    *,
    mtm_rows: pd.DataFrame,
    premium_received: float | None,
) -> float | None:
    if premium_received is None or pd.isna(premium_received) or abs(premium_received) == 0:
        return None
    if len(mtm_rows) < 2:
        return 0.0

    sort_columns = ["mtm_date"]
    if "mtm_time" in mtm_rows.columns:
        sort_columns.append("mtm_time")
    mtm_values = pd.to_numeric(mtm_rows.sort_values(sort_columns)["mtm"], errors="coerce").dropna()
    if len(mtm_values) < 2:
        return 0.0

    daily_changes = mtm_values.diff().dropna()
    normalized = (daily_changes / abs(float(premium_received))) * 100
    return _round(np.std(normalized.to_numpy(), ddof=0))


def _max_mtm(mtm_rows: pd.DataFrame) -> float | None:
    if mtm_rows.empty:
        return None
    return _round(pd.to_numeric(mtm_rows["mtm"], errors="coerce").max())


def _min_mtm(mtm_rows: pd.DataFrame) -> float | None:
    if mtm_rows.empty:
        return None
    return _round(pd.to_numeric(mtm_rows["mtm"], errors="coerce").min())


def _realized_pnl_by_exit_date(trades: pd.DataFrame) -> pd.DataFrame:
    if trades.empty:
        return pd.DataFrame(columns=["date", "pnl"])
    rows = trades.copy()
    rows["pnl"] = _leg_pnl(rows)
    rows["date"] = rows["exit_date"].astype(str)
    return rows.groupby("date", as_index=False)["pnl"].sum().sort_values("date")


def _equity_curve(realized: pd.DataFrame) -> list[dict[str, Any]]:
    if realized.empty:
        return []
    rows = realized.copy()
    rows["equity"] = rows["pnl"].cumsum()
    return [
        {"date": str(row["date"]), "equity": _round(row["equity"])}
        for _, row in rows.iterrows()
    ]


def _daily_portfolio_mtm(daily_mtm: pd.DataFrame) -> pd.DataFrame:
    if daily_mtm.empty:
        return pd.DataFrame(columns=["mtm_date", "mtm"])
    rows = daily_mtm.copy()
    rows["mtm"] = pd.to_numeric(rows["mtm"], errors="coerce").fillna(0.0)
    group_columns = ["mtm_date"]
    if "mtm_time" in rows.columns:
        group_columns.append("mtm_time")
    return rows.groupby(group_columns, as_index=False)["mtm"].sum().sort_values(group_columns)


def _leg_pnl(rows: pd.DataFrame) -> pd.Series:
    entry = pd.to_numeric(rows["entry_price"], errors="coerce")
    exit_ = pd.to_numeric(rows["exit_price"], errors="coerce")
    quantity = pd.to_numeric(rows["lot_size"], errors="coerce")
    if "side" in rows.columns:
        side = rows["side"].fillna("sell").astype(str).str.lower()
    else:
        side = pd.Series(["sell"] * len(rows), index=rows.index)
    short_pnl = (entry - exit_) * quantity
    long_pnl = (exit_ - entry) * quantity
    return short_pnl.where(side != "buy", long_pnl)


def _skip_counts(skipped_expiries: pd.DataFrame) -> dict[str, int]:
    if skipped_expiries.empty or "reason" not in skipped_expiries.columns:
        return {}
    counts = skipped_expiries["reason"].fillna("unknown").value_counts()
    return {str(reason): int(count) for reason, count in counts.items()}


def _skipped_expiry_count(skipped_expiries: pd.DataFrame) -> int:
    if skipped_expiries.empty:
        return 0
    if "expiry_date" in skipped_expiries.columns:
        return int(skipped_expiries["expiry_date"].dropna().astype(str).nunique())
    return int(len(skipped_expiries))


def _most_common_skip_reason(skip_counts: dict[str, int]) -> str | None:
    if not skip_counts:
        return None
    return max(skip_counts.items(), key=lambda item: item[1])[0]


def _max_drawdown(mtm_curve: pd.DataFrame) -> float:
    if mtm_curve.empty:
        return 0.0
    running_peak = mtm_curve["mtm"].cummax()
    drawdown = mtm_curve["mtm"] - running_peak
    return _round(drawdown.min())


def _day_for_extreme(mtm_curve: pd.DataFrame, fn: str) -> str | None:
    if mtm_curve.empty:
        return None
    idx = mtm_curve["mtm"].idxmax() if fn == "max" else mtm_curve["mtm"].idxmin()
    return str(mtm_curve.loc[idx, "mtm_date"])


def _round(value) -> float:
    return round(float(value), 2)
