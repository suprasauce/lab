"""Synchronous HTTP endpoints for historical candle ingestion."""

import json
import logging
import os
from decimal import Decimal
from pathlib import Path

from dotenv import dotenv_values
from flask import Flask, jsonify, request
from werkzeug.exceptions import HTTPException

from providers import create_provider

app = Flask(__name__, static_folder=None)
logger = logging.getLogger(__name__)


def request_fields(required):
    if not request.is_json:
        raise ValueError("Content-Type must be application/json")
    try:
        body = json.loads(request.get_data(), parse_float=Decimal)
    except (ValueError, UnicodeError) as exc:
        raise ValueError("Request body must be valid JSON") from exc
    if not isinstance(body, dict) or set(body) != set(required):
        raise ValueError("Expected exactly these fields: " + ", ".join(required))
    return body


@app.post("/underlyings/fetch")
def underlyings_fetch():
    return jsonify(app.config["DATA_PROVIDER"].fetch_underlying(**request_fields(("underlying", "start_date", "end_date"))))


@app.post("/option-chain/fetch")
def option_chain_fetch():
    return jsonify(app.config["DATA_PROVIDER"].fetch_option_chain(**request_fields(("underlying", "expiry"))))


def safe_message(error):
    message = str(error)
    for name in ("DATABASE_URL", "BREEZE_API_KEY", "BREEZE_API_SECRET", "BREEZE_SESSION_TOKEN", "UPSTOX_ACCESS_TOKEN"):
        value = os.environ.get(name)
        if value:
            message = message.replace(value, "[redacted]")
    return message


@app.errorhandler(ValueError)
def invalid_request(error):
    return jsonify(error=safe_message(error)), 400


@app.errorhandler(NotImplementedError)
def not_implemented(error):
    return jsonify(error=safe_message(error)), 501


@app.errorhandler(RuntimeError)
def upstream_failure(error):
    message = safe_message(error)
    logger.error("Ingestion failed: %s", message)
    return jsonify(error=message), 502


@app.errorhandler(Exception)
def unexpected_failure(error):
    if isinstance(error, HTTPException):
        return jsonify(error=error.description), error.code
    # Driver/SDK exceptions may expose credentials; do not log their raw text.
    logger.error("Ingestion failed (%s)", type(error).__name__)
    return jsonify(error="Ingestion failed; check service logs and database configuration"), 500


# Select one provider when the server loads. DATABASE_URL stays OS-only.
values = dotenv_values(Path(__file__).with_name(".env"))
provider_name = os.environ.get("MARKET_DATA_PROVIDER", values.get("MARKET_DATA_PROVIDER") or "breeze").strip().lower()
credentials = {
    "breeze": ("BREEZE_API_KEY", "BREEZE_API_SECRET", "BREEZE_SESSION_TOKEN"),
    "upstox": ("UPSTOX_ACCESS_TOKEN",),
}
for name in credentials.get(provider_name, ()):
    if values.get(name) is not None:
        os.environ.setdefault(name, values[name])
app.config["DATA_PROVIDER"] = create_provider(provider_name)


if __name__ == "__main__":
    logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(message)s")
    # One process/thread preserves the Breeze SDK's per-process pacing/budget.
    app.run(host="127.0.0.1", port=8000, threaded=False, use_reloader=False)
