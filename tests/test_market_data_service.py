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
