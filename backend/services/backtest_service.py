"""Generic multi-leg option backtest service."""

from __future__ import annotations

from dataclasses import dataclass
from datetime import date, datetime, time, timedelta
import logging
from typing import Any

import pandas as pd

from backend.common.nse_calendar import entry_date_for_expiry, exit_date_for_expiry, iter_monthly_expiries, trading_days_between
from backend.common.utils import MARKET_CLOSE, MARKET_OPEN, bar_end_time, bar_start_for_end_time
from backend.config.settings import lot_size_for_date
from backend.services import market_data_service
from backend.services.metrics_service import (
    build_average_mtm_by_expiry,
    build_backtest_metrics,
    build_equity_curve,
    build_expiry_pnl_curve,
    build_trade_metrics,
)
from backend.services.result_service import save_run
from backend.services.strike_selection_service import StrikeSelectionContext, resolve_strike, round_atm
from backend.services.vix_service import build_vix_curve

logger = logging.getLogger(__name__)
CUSTOM_STRATEGY_ID = "custom_multi_leg"
CUSTOM_STRATEGY_NAME = "Custom Multi-Leg Strategy"


@dataclass(frozen=True)
class PositionConfig:
    leg_role: str
    option_type: str
    side: str
    quantity: int | None
    strike_selection: str
    strike_params: dict[str, float | None]
    entry_dte: int
    entry_time: time
    exit_dte: int
    exit_time: time


@dataclass(frozen=True)
class StrikeSelectionConfig:
    method: str
    params: dict[str, float | None]


@dataclass(frozen=True)
class TriggerConfig:
    type: str
    value: float


@dataclass(frozen=True)
class ActionConfig:
    type: str
    strike_selection: StrikeSelectionConfig


@dataclass(frozen=True)
class AdjustmentConfig:
    trigger: TriggerConfig
    action: ActionConfig
    max_adjustments: int | None = None
    exit_if_net_credit_lte_zero: bool = False


@dataclass
class ActiveLeg:
    config: PositionConfig
    strike: dict
    price_df: pd.DataFrame
    price_lookup: dict[datetime, float]
    entry_price: float
    entry_datetime: datetime
    quantity: int
    spot_at_entry: float
    atm_at_entry: int
    strike_selection: StrikeSelectionConfig
    adjustment_counts: dict[int, int] | None = None


@dataclass(frozen=True)
class AdjustmentEvent:
    leg_role: str
    adjustment_index: int
    adjustment: AdjustmentConfig


def list_strategies() -> list[dict]:
    return [
        {
            "id": CUSTOM_STRATEGY_ID,
            "name": CUSTOM_STRATEGY_NAME,
            "description": "Build a strategy by adding option legs with independent entry, exit, and strike rules.",
        }
    ]


def get_strategy(strategy_id: str) -> dict:
    strategy = list_strategies()[0]
    if strategy_id != CUSTOM_STRATEGY_ID:
        raise ValueError(f"Unknown strategy: {strategy_id}")
    return strategy


def run_backtest_for_strategy(
    *,
    strategy_id: str,
    start_date: date,
    end_date: date,
    positions: list[PositionConfig],
    adjustments: list[AdjustmentConfig] | None = None,
    include_mtm: bool = True,
) -> tuple[str, dict[str, Any]]:
    if not positions:
        raise ValueError("Add at least one leg.")

    expiries = iter_monthly_expiries(start_date, end_date)
    logger.info(
        "Backtest started strategy=%s start=%s end=%s expiries=%s legs=%s",
        strategy_id,
        start_date,
        end_date,
        len(expiries),
        len(positions),
    )

    adjustments = adjustments or []
    results = run_backtest(
        start_date=start_date,
        end_date=end_date,
        positions=positions,
        adjustments=adjustments,
        include_mtm=include_mtm,
    )
    if not include_mtm:
        logger.info("MTM calculation skipped")
        results["daily_mtm"] = pd.DataFrame(columns=_mtm_columns())

    results["metrics"] = build_backtest_metrics(
        trades=results["trades"],
        skipped_expiries=results["skipped_expiries"],
        daily_mtm=results["daily_mtm"],
        include_mtm=include_mtm,
    )
    results["equity_curve"] = build_equity_curve(results["trades"])
    results["expiry_pnl_curve"] = build_expiry_pnl_curve(results["trades"])
    results["average_mtm_by_expiry"] = (
        build_average_mtm_by_expiry(trades=results["trades"], daily_mtm=results["daily_mtm"])
        if include_mtm
        else []
    )
    results["trade_metrics"] = (
        build_trade_metrics(trades=results["trades"], daily_mtm=results["daily_mtm"])
        if include_mtm
        else []
    )
    results["vix_curve"] = (
        build_vix_curve(
            equity_curve=results["equity_curve"],
            trade_metrics=results["trade_metrics"],
        )
        if include_mtm
        else []
    )

    run_id = datetime.now().strftime("%Y%m%d_%H%M%S")
    metadata = {
        "run_id": run_id,
        "strategy_id": CUSTOM_STRATEGY_ID,
        "strategy_name": CUSTOM_STRATEGY_NAME,
        "created_at": datetime.now().isoformat(timespec="seconds"),
        "start_date": start_date.isoformat(),
        "end_date": end_date.isoformat(),
        "include_mtm": include_mtm,
        "positions": [_position_metadata(position) for position in positions],
        "adjustments": [_adjustment_metadata(adjustment) for adjustment in adjustments],
        "trade_rows": len(results["trades"]),
        "skipped_expiries": len(results["skipped_expiries"]),
        "daily_mtm_rows": len(results["daily_mtm"]),
    }
    save_run(run_id, metadata, results)
    logger.info("Backtest completed run_id=%s", run_id)
    return run_id, results


def parse_date(value: str) -> date:
    return datetime.strptime(value, "%Y-%m-%d").date()


def parse_time(value: str) -> time:
    return datetime.strptime(value, "%H:%M").time()


def run_backtest(
    *,
    start_date: date,
    end_date: date,
    positions: list[PositionConfig],
    adjustments: list[AdjustmentConfig] | None = None,
    include_mtm: bool = True,
) -> dict[str, pd.DataFrame]:
    expiries = iter_monthly_expiries(start_date, end_date)
    if not expiries:
        return {
            "trades": pd.DataFrame(columns=_trade_columns()),
            "skipped_expiries": pd.DataFrame(columns=_skipped_columns()),
            "daily_mtm": pd.DataFrame(columns=_mtm_columns()),
        }

    results = []
    adjustments = adjustments or []
    for expiry in expiries:
        try:
            results.append((expiry, _run_expiry(expiry, positions, adjustments, include_mtm)))
            logger.info("Expiry completed expiry=%s", expiry)
        except Exception:
            logger.exception("Expiry failed expiry=%s", expiry)
            raise

    return {
        "trades": _concat([result["trades"] for _, result in results], _trade_columns()),
        "skipped_expiries": _concat(
            [result["skipped_expiries"] for _, result in results],
            _skipped_columns(),
        ),
        "daily_mtm": _concat([result["daily_mtm"] for _, result in results], _mtm_columns()),
    }


def _run_expiry(
    expiry: date,
    positions: list[PositionConfig],
    adjustments: list[AdjustmentConfig],
    include_mtm: bool,
) -> dict[str, pd.DataFrame]:
    logger.info("Expiry started expiry=%s legs=%s", expiry, len(positions))
    trades: list[dict] = []
    mtm_rows: list[dict] = []
    skipped: list[dict] = []
    active_legs = []
    for position in positions:
        entry_date = entry_date_for_expiry(expiry, position.entry_dte)
        exit_date = exit_date_for_expiry(expiry, position.exit_dte)
        base = _base_row(position, expiry, entry_date, exit_date)
        if entry_date >= expiry:
            skipped.append(_skipped_row(base, "entry_on_or_after_expiry"))
            continue
        if exit_date < entry_date:
            skipped.append(_skipped_row(base, "exit_before_entry"))
            continue

        leg = enter_position(
            position=position,
            strike_selection=StrikeSelectionConfig(position.strike_selection, position.strike_params),
            expiry=expiry,
            timestamp=datetime.combine(entry_date, position.entry_time),
        )
        if leg is None:
            skipped.append(_skipped_row(base, "entry_price_missing"))
            continue
        active_legs.append(leg)

    if active_legs:
        net_credit = sum(_entry_credit(leg) for leg in active_legs)
        exit_expiry = False
        current_timeline_day = None
        for timestamp in _build_timeline(active_legs, expiry):
            if not active_legs or exit_expiry:
                break
            if timestamp.date() != current_timeline_day:
                current_timeline_day = timestamp.date()
                logger.info("Timeline day started expiry=%s date=%s", expiry, current_timeline_day)
            prices = load_prices(active_legs, timestamp, expiry)
            if not prices:
                continue
            if include_mtm:
                mtm_rows.extend(record_mtm(active_legs, prices, timestamp, expiry))

            events = evaluate_events(
                active_legs=active_legs,
                prices=prices,
                timestamp=timestamp,
                adjustments=adjustments,
            )
            exit_expiry, net_credit = apply_leg_events(
                events=events,
                active_legs=active_legs,
                prices=prices,
                timestamp=timestamp,
                expiry=expiry,
                trades=trades,
                net_credit=net_credit,
            )

        if not exit_expiry:
            close_remaining_legs_at_scheduled_exit(active_legs, expiry, trades)

    return {
        "trades": pd.DataFrame(trades, columns=_trade_columns()),
        "skipped_expiries": pd.DataFrame(skipped, columns=_skipped_columns()),
        "daily_mtm": pd.DataFrame(mtm_rows, columns=_mtm_columns()),
    }


def enter_position(
    *,
    position: PositionConfig,
    strike_selection: StrikeSelectionConfig,
    expiry: date,
    timestamp: datetime,
) -> ActiveLeg | None:
    spot = _underlying_price(timestamp)
    if spot is None:
        return None
    atm = round_atm(spot)
    logger.info(
        "Resolving strike expiry=%s leg=%s method=%s",
        expiry,
        position.leg_role,
        strike_selection.method,
    )
    strike = resolve_strike(
        StrikeSelectionContext(
            option_type=position.option_type,
            expiry=expiry,
            entry_date=timestamp.date(),
            entry_time=timestamp.time(),
            spot=spot,
            atm=atm,
            method=strike_selection.method,
            params=strike_selection.params,
        )
    )
    if strike is None:
        return None

    exit_at = _scheduled_exit_datetime(position, expiry)
    price_df = market_data_service.get_option_5m_range(
        symbol="NIFTY",
        exchange="NFO",
        expiry=expiry,
        strike=int(strike["strike"]),
        right=position.option_type,
        start=timestamp.date(),
        end=exit_at.date(),
    )
    price_lookup = _price_lookup(price_df)
    entry_price = price_lookup.get(bar_start_for_end_time(timestamp.date(), timestamp.time()))
    if entry_price is None:
        return None
    quantity = position.quantity if position.quantity is not None else lot_size_for_date(timestamp.date())
    return ActiveLeg(
        config=position,
        strike=strike,
        price_df=price_df,
        price_lookup=price_lookup,
        entry_price=entry_price,
        entry_datetime=timestamp,
        quantity=quantity,
        spot_at_entry=spot,
        atm_at_entry=atm,
        strike_selection=strike_selection,
    )


def _price_lookup(price_df: pd.DataFrame) -> dict[datetime, float]:
    if price_df is None or price_df.empty or "datetime" not in price_df.columns or "close" not in price_df.columns:
        return {}
    df = price_df[["datetime", "close"]].copy()
    df["datetime"] = pd.to_datetime(df["datetime"], errors="coerce")
    if getattr(df["datetime"].dt, "tz", None) is not None:
        df["datetime"] = df["datetime"].dt.tz_localize(None)
    df["close"] = pd.to_numeric(df["close"], errors="coerce")
    df = df.dropna(subset=["datetime", "close"])
    df = df[df["close"] > 0]
    return dict(zip(df["datetime"].dt.to_pydatetime(), df["close"].astype(float)))


def _build_timeline(
    active_legs: list[ActiveLeg],
    expiry: date,
) -> list[datetime]:
    start = min(bar_start_for_end_time(leg.entry_datetime.date(), leg.entry_datetime.time()) for leg in active_legs)
    end = max(_scheduled_exit_bar(leg.config, expiry) for leg in active_legs)
    timestamps = []
    session_end = bar_start_for_end_time(start.date(), MARKET_CLOSE)
    for trading_day in trading_days_between(start.date(), end.date()):
        current = datetime.combine(trading_day, MARKET_OPEN)
        day_end = datetime.combine(trading_day, session_end.time())
        while current <= day_end:
            if start <= current <= end:
                timestamps.append(current)
            current += timedelta(minutes=5)
    return timestamps


def load_prices(
    active_legs: list[ActiveLeg],
    timestamp: datetime,
    expiry: date,
) -> dict[str, float]:
    prices = {}
    for leg in active_legs:
        if not _is_leg_active_at(leg, timestamp, expiry):
            continue
        price = leg.price_lookup.get(timestamp)
        if price is not None:
            prices[leg.config.leg_role] = price
    return prices


def record_mtm(
    active_legs: list[ActiveLeg],
    prices: dict[str, float],
    timestamp: datetime,
    expiry: date,
) -> list[dict]:
    return [
        _mtm_row(leg.config, expiry, leg.strike, leg.entry_price, leg.quantity, timestamp, prices[leg.config.leg_role])
        for leg in active_legs
        if leg.config.leg_role in prices
    ]


def evaluate_events(
    *,
    active_legs: list[ActiveLeg],
    prices: dict[str, float],
    timestamp: datetime,
    adjustments: list[AdjustmentConfig],
) -> list[AdjustmentEvent]:
    events = []
    for leg in active_legs:
        price = prices.get(leg.config.leg_role)
        if price is None or timestamp <= bar_start_for_end_time(leg.entry_datetime.date(), leg.entry_datetime.time()):
            continue
        event = _evaluate_adjustments(leg, price, adjustments)
        if event is not None:
            events.append(event)
    return events


def _evaluate_adjustments(
    leg: ActiveLeg,
    current_price: float,
    adjustments: list[AdjustmentConfig],
) -> AdjustmentEvent | None:
    counts = leg.adjustment_counts or {}
    for index, adjustment in enumerate(adjustments):
        if adjustment.action.type != "roll_triggered_leg":
            continue
        if adjustment.max_adjustments is not None and counts.get(index, 0) >= adjustment.max_adjustments:
            continue
        if _trigger_hit(adjustment.trigger, leg.entry_price, current_price):
            return AdjustmentEvent(leg_role=leg.config.leg_role, adjustment_index=index, adjustment=adjustment)
    return None


def apply_leg_events(
    *,
    events: list[AdjustmentEvent],
    active_legs: list[ActiveLeg],
    prices: dict[str, float],
    timestamp: datetime,
    expiry: date,
    trades: list[dict],
    net_credit: float,
) -> tuple[bool, float]:
    rolled_roles = set()
    for event in events:
        if event.leg_role in rolled_roles:
            continue
        leg = _active_leg_by_role(active_legs, event.leg_role)
        if leg is None or leg.config.leg_role not in prices:
            continue
        exit_price = prices[leg.config.leg_role]
        replacement = enter_position(
            position=leg.config,
            strike_selection=event.adjustment.action.strike_selection,
            expiry=expiry,
            timestamp=bar_end_datetime(timestamp),
        )
        if replacement is not None:
            next_net_credit = net_credit + _exit_credit(leg, exit_price) + _entry_credit(replacement)
            if event.adjustment.exit_if_net_credit_lte_zero and next_net_credit <= 0:
                close_all_active_legs(active_legs, prices, timestamp, expiry, trades, "net_credit_exit")
                logger.info(
                    "Adjustment exited strategy expiry=%s leg=%s net_credit_after_roll=%s",
                    expiry,
                    leg.config.leg_role,
                    round(next_net_credit, 2),
                )
                return True, net_credit

        close_leg(leg, exit_price, timestamp, expiry, trades, "adjustment")
        active_legs.remove(leg)
        if replacement is not None:
            replacement.adjustment_counts = dict(leg.adjustment_counts or {})
            replacement.adjustment_counts[event.adjustment_index] = (
                replacement.adjustment_counts.get(event.adjustment_index, 0) + 1
            )
            active_legs.append(replacement)
            net_credit = next_net_credit
            logger.info(
                "Adjustment completed expiry=%s leg=%s new_strike=%s entry_price=%s",
                expiry,
                replacement.config.leg_role,
                int(replacement.strike["strike"]),
                round(replacement.entry_price, 2),
            )
        elif trades:
            trades[-1]["exit_reason"] = "adjustment_reentry_failed"
        rolled_roles.add(event.leg_role)
    return False, net_credit


def close_all_active_legs(
    active_legs: list[ActiveLeg],
    prices: dict[str, float],
    timestamp: datetime,
    expiry: date,
    trades: list[dict],
    reason: str,
) -> None:
    for leg in list(active_legs):
        price = prices.get(leg.config.leg_role, leg.price_lookup.get(timestamp))
        if price is not None:
            close_leg(leg, price, timestamp, expiry, trades, reason)
        active_legs.remove(leg)


def close_remaining_legs_at_scheduled_exit(
    active_legs: list[ActiveLeg],
    expiry: date,
    trades: list[dict],
) -> None:
    for leg in list(active_legs):
        price = leg.price_lookup.get(_scheduled_exit_bar(leg.config, expiry))
        if price is not None:
            close_leg(leg, price, _scheduled_exit_bar(leg.config, expiry), expiry, trades, "scheduled_exit")


def close_leg(
    leg: ActiveLeg,
    price: float,
    timestamp: datetime,
    expiry: date,
    trades: list[dict],
    reason: str,
) -> None:
    exit_at = bar_end_datetime(timestamp)
    trades.append(
        _trade_row(
            position=leg.config,
            expiry=expiry,
            entry_date=leg.entry_datetime.date(),
            exit_date=exit_at.date(),
            entry_time=leg.entry_datetime.time(),
            exit_time=exit_at.time(),
            strike=leg.strike,
            entry_price=leg.entry_price,
            exit_price=price,
            quantity=leg.quantity,
            exit_reason=reason,
            spot=leg.spot_at_entry,
            atm=leg.atm_at_entry,
            strike_selection=leg.strike_selection,
        )
    )


def _entry_credit(leg: ActiveLeg) -> float:
    credit = leg.entry_price * leg.quantity
    return credit if leg.config.side.lower() == "sell" else -credit


def _exit_credit(leg: ActiveLeg, price: float) -> float:
    credit = price * leg.quantity
    return -credit if leg.config.side.lower() == "sell" else credit


def _trigger_hit(trigger: TriggerConfig, entry_price: float, current_price: float) -> bool:
    if trigger.type != "premium_increase_pct" or entry_price <= 0:
        return False
    return current_price >= entry_price * (1 + float(trigger.value) / 100)


def _underlying_price(timestamp: datetime) -> float | None:
    return _close(
        market_data_service.get_underlying_candle(
            symbol="NIFTY",
            exchange="NSE",
            candle_date=timestamp.date(),
            candle_time=timestamp.time(),
        )
    )


def _scheduled_exit_datetime(position: PositionConfig, expiry: date) -> datetime:
    return datetime.combine(exit_date_for_expiry(expiry, position.exit_dte), position.exit_time)


def _scheduled_exit_bar(position: PositionConfig, expiry: date) -> datetime:
    return bar_start_for_end_time(exit_date_for_expiry(expiry, position.exit_dte), position.exit_time)


def _is_leg_active_at(leg: ActiveLeg, timestamp: datetime, expiry: date) -> bool:
    entry_bar = bar_start_for_end_time(leg.entry_datetime.date(), leg.entry_datetime.time())
    return entry_bar <= timestamp <= _scheduled_exit_bar(leg.config, expiry)


def _active_leg_by_role(active_legs: list[ActiveLeg], leg_role: str | None) -> ActiveLeg | None:
    for leg in active_legs:
        if leg.config.leg_role == leg_role:
            return leg
    return None


def bar_end_datetime(bar_start: datetime) -> datetime:
    return datetime.combine(bar_start.date(), bar_end_time(bar_start))


def _close(candle: dict | None) -> float | None:
    if candle is None or candle.get("close") is None:
        return None
    return _clean_price(candle["close"])


def _clean_price(value: Any) -> float | None:
    close = float(value)
    return close if close > 0 else None


def _trade_row(
    *,
    position: PositionConfig,
    expiry: date,
    entry_date: date,
    exit_date: date,
    entry_time: time,
    exit_time: time,
    strike: dict,
    entry_price: float,
    exit_price: float,
    quantity: int,
    exit_reason: str,
    spot: float,
    atm: int,
    strike_selection: StrikeSelectionConfig | None = None,
) -> dict:
    strike_selection = strike_selection or StrikeSelectionConfig(position.strike_selection, position.strike_params)
    return _base_row(position, expiry, entry_date, exit_date, entry_time, exit_time) | {
        "option_type": position.option_type,
        "side": position.side,
        "strike": int(strike["strike"]),
        "entry_price": round(entry_price, 2),
        "exit_price": round(exit_price, 2),
        "lot_size": quantity,
        "exit_reason": exit_reason,
        "entry_delta": _round_or_none(strike.get("delta"), 4),
        "entry_iv": _round_or_none(strike.get("iv"), 4),
        "spot_at_entry": round(spot, 2),
        "atm_strike": atm,
        "strike_selection": strike_selection.method,
        "strike_params": _format_strike_params(strike_selection.params),
    }


def _mtm_row(
    position: PositionConfig,
    expiry: date,
    strike: dict,
    entry_price: float,
    quantity: int,
    candle_start: datetime,
    current_price: float,
) -> dict:
    side = position.side.lower()
    if side == "buy":
        mtm = round((current_price - entry_price) * quantity, 2)
    else:
        mtm = round((entry_price - current_price) * quantity, 2)
    return {
        "trade_id": f"{CUSTOM_STRATEGY_ID}_{expiry.isoformat()}",
        "expiry_date": expiry.isoformat(),
        "mtm_date": candle_start.date().isoformat(),
        "mtm_time": bar_end_time(candle_start).strftime("%H:%M"),
        "leg_role": position.leg_role,
        "option_type": position.option_type,
        "side": side,
        "strike": int(strike["strike"]),
        "entry_price": round(entry_price, 2),
        "current_price": round(current_price, 2),
        "lot_size": quantity,
        "mtm": mtm,
        "reason": "ok",
    }


def _base_row(
    position: PositionConfig,
    expiry: date,
    entry_date: date,
    exit_date: date,
    entry_time: time | None = None,
    exit_time: time | None = None,
) -> dict:
    return {
        "trade_id": f"{CUSTOM_STRATEGY_ID}_{expiry.isoformat()}",
        "expiry_date": expiry.isoformat(),
        "entry_date": entry_date.isoformat(),
        "exit_date": exit_date.isoformat(),
        "leg_role": position.leg_role,
        "strike_selection": position.strike_selection,
        "strike_params": _format_strike_params(position.strike_params),
        "entry_time": (entry_time or position.entry_time).strftime("%H:%M"),
        "exit_time": (exit_time or position.exit_time).strftime("%H:%M"),
    }


def _skipped_row(base: dict, reason: str) -> dict:
    row = {column: base.get(column) for column in _skipped_columns()}
    row["reason"] = reason
    return row


def _position_metadata(position: PositionConfig) -> dict:
    return {
        "leg_role": position.leg_role,
        "option_type": position.option_type,
        "side": position.side,
        "quantity": position.quantity,
        "strike_selection": position.strike_selection,
        "strike_params": position.strike_params,
        "entry_dte": position.entry_dte,
        "entry_time": position.entry_time.strftime("%H:%M"),
        "exit_dte": position.exit_dte,
        "exit_time": position.exit_time.strftime("%H:%M"),
    }


def _adjustment_metadata(adjustment: AdjustmentConfig) -> dict:
    return {
        "trigger": {
            "type": adjustment.trigger.type,
            "value": adjustment.trigger.value,
        },
        "action": {
            "type": adjustment.action.type,
            "strikeSelection": {
                "method": adjustment.action.strike_selection.method,
                "params": adjustment.action.strike_selection.params,
            },
        },
        "max_adjustments": adjustment.max_adjustments,
        "exit_if_net_credit_lte_zero": adjustment.exit_if_net_credit_lte_zero,
    }


def _round_or_none(value: Any, digits: int) -> float | None:
    return None if value is None else round(float(value), digits)


def _format_strike_params(params: dict[str, float | None]) -> str:
    values = [f"{key}={value}" for key, value in params.items() if value is not None]
    return ", ".join(values)


def _trade_columns() -> list[str]:
    return [
        "trade_id",
        "expiry_date",
        "entry_date",
        "exit_date",
        "leg_role",
        "option_type",
        "side",
        "strike",
        "entry_price",
        "exit_price",
        "lot_size",
        "exit_reason",
        "entry_delta",
        "entry_iv",
        "spot_at_entry",
        "atm_strike",
        "strike_selection",
        "strike_params",
        "entry_time",
        "exit_time",
    ]


def _skipped_columns() -> list[str]:
    return [
        "trade_id",
        "expiry_date",
        "entry_date",
        "exit_date",
        "leg_role",
        "reason",
        "spot_at_entry",
        "atm_strike",
        "strike_selection",
        "strike_params",
        "entry_time",
        "exit_time",
    ]


def _mtm_columns() -> list[str]:
    return [
        "trade_id",
        "expiry_date",
        "mtm_date",
        "mtm_time",
        "leg_role",
        "option_type",
        "side",
        "strike",
        "entry_price",
        "current_price",
        "lot_size",
        "mtm",
        "reason",
    ]


def _concat(frames: list[pd.DataFrame], columns: list[str]) -> pd.DataFrame:
    non_empty = [frame for frame in frames if not frame.empty]
    return pd.concat(non_empty, ignore_index=True) if non_empty else pd.DataFrame(columns=columns)
