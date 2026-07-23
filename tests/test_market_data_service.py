from datetime import date, time

from backend.services import market_data_service


class FakeService:
    def __init__(self):
        self.calls = 0

    def get_underlying_candle(self, **kwargs):
        self.calls += 1
        return {"close": 1}

    def get_option_candle(self, **kwargs):
        self.calls += 1
        return {"close": 2}


class FakeDao:
    def __init__(self, marked_missing=False):
        self.marked_missing = marked_missing
        self.marked = []
        self.saved_derivatives = []
        self.saved_underlying = []

    def load_underlying_candle(self, **kwargs):
        return None

    def load_derivative_candle(self, **kwargs):
        return None

    def is_day_marked_missing(self, **kwargs):
        return self.marked_missing

    def mark_day_missing(self, **kwargs):
        self.marked.append(kwargs)

    def save_underlying_5m(self, **kwargs):
        self.saved_underlying.append(kwargs)

    def save_derivative_5m(self, **kwargs):
        self.saved_derivatives.append(kwargs)


class FakeClient:
    def __init__(self, rows=None):
        self.calls = 0
        self.rows = rows or []

    def get_historical_5min(self, **kwargs):
        self.calls += 1
        return self.rows


def test_market_data_service_reuses_global_service(monkeypatch):
    created = []

    def fake_create():
        service = FakeService()
        created.append(service)
        return service

    monkeypatch.setattr(market_data_service, "_create_market_data_service", fake_create)
    monkeypatch.setattr(market_data_service, "_service", None)

    assert market_data_service.get_underlying_candle(
        symbol="NIFTY",
        exchange="NSE",
        candle_date=None,
        candle_time=None,
    ) == {"close": 1}
    assert market_data_service.get_option_candle(
        symbol="NIFTY",
        exchange="NFO",
        expiry=None,
        strike=25000,
        right="call",
        candle_date=None,
        candle_time=None,
    ) == {"close": 2}

    assert len(created) == 1
    assert created[0].calls == 2


def test_missing_option_marker_skips_breeze_fetch():
    client = FakeClient()
    service = market_data_service._MarketDataService(client, FakeDao(marked_missing=True))

    candle = service.get_option_candle(
        symbol="NIFTY",
        exchange="NFO",
        expiry=date(2026, 1, 27),
        strike=25000,
        right="call",
        candle_date=date(2026, 1, 2),
        candle_time=time(9, 30),
    )

    assert candle is None
    assert client.calls == 0


def test_empty_breeze_response_marks_option_day_missing():
    client = FakeClient()
    dao = FakeDao(marked_missing=False)
    service = market_data_service._MarketDataService(client, dao)

    candle = service.get_option_candle(
        symbol="NIFTY",
        exchange="NFO",
        expiry=date(2026, 1, 27),
        strike=25000,
        right="call",
        candle_date=date(2026, 1, 2),
        candle_time=time(9, 30),
    )

    assert candle is None
    assert client.calls == 1
    assert dao.saved_derivatives == []
    assert dao.marked[0]["instrument_type"] == "option"
    assert dao.marked[0]["reason"] == "provider_no_rows"


def test_partial_breeze_response_marks_day_missing_with_candle_reason():
    client = FakeClient(
        rows=[
            {
                "datetime": "2026-01-02 09:15:00",
                "open": 1,
                "high": 1,
                "low": 1,
                "close": 1,
            }
        ]
    )
    dao = FakeDao(marked_missing=False)
    service = market_data_service._MarketDataService(client, dao)

    candle = service.get_option_candle(
        symbol="NIFTY",
        exchange="NFO",
        expiry=date(2026, 1, 27),
        strike=25000,
        right="call",
        candle_date=date(2026, 1, 2),
        candle_time=time(9, 30),
    )

    assert candle is None
    assert client.calls == 1
    assert len(dao.saved_derivatives) == 1
    assert dao.marked[0]["instrument_type"] == "option"
    assert dao.marked[0]["reason"] == "requested_candle_missing_after_fetch"
