"""FastAPI application entrypoint."""

from __future__ import annotations

from fastapi import FastAPI

from backend.controllers.web_controller import router as web_router

app = FastAPI(title="Backtest UI")
app.include_router(web_router)
