"""Web page routes."""

from __future__ import annotations

from fastapi import APIRouter, Form, HTTPException, Request
from fastapi.responses import HTMLResponse, RedirectResponse
from jinja2 import Environment, FileSystemLoader, select_autoescape

from backend.config.settings import BACKTEST_ROOT
from backend.services.backtest_service import (
    get_strategy,
    list_strategies,
    parse_date,
    parse_time,
    run_backtest_for_strategy,
)
from backend.services.metrics_service import metric_cards
from backend.services.result_service import (
    dataframe_columns,
    dataframe_records,
    load_run,
    load_trade_mtm,
)

router = APIRouter()
templates = Environment(
    loader=FileSystemLoader(BACKTEST_ROOT / "frontend" / "templates"),
    autoescape=select_autoescape(["html"]),
)


@router.get("/", response_class=HTMLResponse)
def strategies_page(request: Request):
    return _render("strategies.html", request=request, strategies=list_strategies())


@router.get("/strategies/{strategy_id}", response_class=HTMLResponse)
def strategy_page(request: Request, strategy_id: str):
    try:
        strategy = get_strategy(strategy_id)
    except ValueError as exc:
        raise HTTPException(status_code=404, detail=str(exc)) from exc
    return _render(
        "strategy_detail.html",
        request=request,
        strategy=strategy,
        form=_default_form(),
        error=None,
        run=None,
        metric_cards=[],
        metrics={},
        equity_curve=[],
    )


@router.post("/strategies/{strategy_id}/run", response_class=HTMLResponse)
def run_strategy(
    request: Request,
    strategy_id: str,
    start_date: str = Form(...),
    end_date: str = Form(...),
    entry_dte: int = Form(...),
    entry_time: str = Form(...),
    exit_time: str = Form(...),
    strike_offset: int = Form(...),
    lot_size: str = Form(""),
):
    form = {
        "start_date": start_date,
        "end_date": end_date,
        "entry_dte": entry_dte,
        "entry_time": entry_time,
        "exit_time": exit_time,
        "strike_offset": strike_offset,
        "lot_size": lot_size,
    }
    try:
        strategy = get_strategy(strategy_id)
        lot_size_value = int(lot_size) if lot_size.strip() else None
        run_id, results = run_backtest_for_strategy(
            strategy_id=strategy_id,
            start_date=parse_date(start_date),
            end_date=parse_date(end_date),
            entry_dte=entry_dte,
            entry_time=parse_time(entry_time),
            exit_time=parse_time(exit_time),
            strike_offset=strike_offset,
            lot_size=lot_size_value,
        )
    except Exception as exc:
        try:
            strategy = get_strategy(strategy_id)
        except ValueError as strategy_exc:
            raise HTTPException(status_code=404, detail=strategy_exc.args[0]) from strategy_exc
        return _render(
            "strategy_detail.html",
            status_code=400,
            request=request,
            strategy=strategy,
            form=form,
            error=str(exc),
            run=None,
            metric_cards=[],
            metrics={},
            equity_curve=[],
        )

    run = load_run(run_id)
    return _render(
        "strategy_detail.html",
        request=request,
        strategy=strategy,
        form=form,
        error=None,
        run=run["metadata"],
        metric_cards=metric_cards(run["metrics"]),
        metrics=run["metrics"],
        equity_curve=run["equity_curve"],
    )


@router.get("/backtests/{run_id}", response_class=HTMLResponse)
def run_page(request: Request, run_id: str):
    try:
        run = load_run(run_id)
    except FileNotFoundError as exc:
        raise HTTPException(status_code=404, detail=str(exc)) from exc
    strategy = get_strategy(run["metadata"].get("strategy_id", "nifty_short_strangle"))
    return _render(
        "strategy_detail.html",
        request=request,
        strategy=strategy,
        form=run["metadata"],
        error=None,
        run=run["metadata"],
        metric_cards=metric_cards(run["metrics"]),
        metrics=run["metrics"],
        equity_curve=run["equity_curve"],
    )


@router.get("/backtests/{run_id}/trades", response_class=HTMLResponse)
def detailed_trades_page(request: Request, run_id: str):
    try:
        run = load_run(run_id)
    except FileNotFoundError as exc:
        raise HTTPException(status_code=404, detail=str(exc)) from exc
    return _render(
        "detailed_trades.html",
        request=request,
        run=run["metadata"],
        trades_columns=dataframe_columns(run["trades"]),
        trades_rows=dataframe_records(run["trades"]),
    )


@router.get("/backtests/{run_id}/skipped-expiries", response_class=HTMLResponse)
def skipped_expiries_page(request: Request, run_id: str):
    try:
        run = load_run(run_id)
    except FileNotFoundError as exc:
        raise HTTPException(status_code=404, detail=str(exc)) from exc
    return _render(
        "skipped_expiries.html",
        request=request,
        run=run["metadata"],
        skipped_columns=dataframe_columns(run["skipped_expiries"]),
        skipped_rows=dataframe_records(run["skipped_expiries"]),
    )


@router.get("/backtests/{run_id}/trades/{trade_id}/mtm", response_class=HTMLResponse)
def trade_mtm_page(request: Request, run_id: str, trade_id: str):
    try:
        detail = load_trade_mtm(run_id, trade_id)
    except FileNotFoundError as exc:
        raise HTTPException(status_code=404, detail=str(exc)) from exc
    return _render(
        "mtm.html",
        request=request,
        run=detail["metadata"],
        trade_id=trade_id,
        trades_columns=dataframe_columns(detail["trades"]),
        trades_rows=dataframe_records(detail["trades"]),
        mtm_columns=dataframe_columns(detail["daily_mtm"]),
        mtm_rows=dataframe_records(detail["daily_mtm"]),
    )


@router.get("/health")
def health():
    return {"status": "ok"}


@router.get("/favicon.ico")
def favicon():
    return RedirectResponse(url="/")


def _default_form() -> dict:
    return {
        "start_date": "2026-01-01",
        "end_date": "2026-02-28",
        "entry_dte": 45,
        "entry_time": "09:30",
        "exit_time": "15:30",
        "strike_offset": 6,
        "lot_size": "",
    }


def _render(template_name: str, status_code: int = 200, **context) -> HTMLResponse:
    template = templates.get_template(template_name)
    return HTMLResponse(template.render(**context), status_code=status_code)
