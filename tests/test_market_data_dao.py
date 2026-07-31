from datetime import date

from backend.dao.market_data_dao import MarketDataDao


def test_mark_and_read_missing_underlying_day(tmp_path):
    dao = MarketDataDao(tmp_path / "market.duckdb")

    assert not dao.is_day_marked_missing(
        symbol="NIFTY",
        exchange="NSE",
        instrument_type="underlying",
        data_date=date(2026, 1, 2),
    )

    dao.mark_day_missing(
        symbol="NIFTY",
        exchange="NSE",
        instrument_type="underlying",
        data_date=date(2026, 1, 2),
        reason="provider_no_rows",
    )

    assert dao.is_day_marked_missing(
        symbol="NIFTY",
        exchange="NSE",
        instrument_type="underlying",
        data_date=date(2026, 1, 2),
    )


def test_mark_and_read_missing_option_day(tmp_path):
    dao = MarketDataDao(tmp_path / "market.duckdb")

    dao.mark_day_missing(
        symbol="NIFTY",
        exchange="NFO",
        instrument_type="option",
        expiry=date(2026, 1, 27),
        strike=24500,
        right="call",
        data_date=date(2026, 1, 2),
        reason="provider_no_rows",
    )

    assert dao.is_day_marked_missing(
        symbol="NIFTY",
        exchange="NFO",
        instrument_type="option",
        expiry=date(2026, 1, 27),
        strike=24500,
        right="call",
        data_date=date(2026, 1, 2),
    )
    assert not dao.is_day_marked_missing(
        symbol="NIFTY",
        exchange="NFO",
        instrument_type="option",
        expiry=date(2026, 1, 27),
        strike=24550,
        right="call",
        data_date=date(2026, 1, 2),
    )


def test_mark_and_read_option_contract_coverage(tmp_path):
    dao = MarketDataDao(tmp_path / "market.duckdb")

    assert dao.option_contract_fetched_from(
        symbol="NIFTY",
        exchange="NFO",
        expiry=date(2026, 1, 27),
        strike=24500,
        right="call",
    ) is None

    dao.mark_option_contract_fetched_from(
        symbol="NIFTY",
        exchange="NFO",
        expiry=date(2026, 1, 27),
        strike=24500,
        right="call",
        fetched_from_date=date(2026, 1, 10),
    )

    assert dao.option_contract_fetched_from(
        symbol="NIFTY",
        exchange="NFO",
        expiry=date(2026, 1, 27),
        strike=24500,
        right="call",
    ) == date(2026, 1, 10)

    dao.mark_option_contract_fetched_from(
        symbol="NIFTY",
        exchange="NFO",
        expiry=date(2026, 1, 27),
        strike=24500,
        right="call",
        fetched_from_date=date(2026, 1, 15),
    )

    assert dao.option_contract_fetched_from(
        symbol="NIFTY",
        exchange="NFO",
        expiry=date(2026, 1, 27),
        strike=24500,
        right="call",
    ) == date(2026, 1, 10)

    dao.mark_option_contract_fetched_from(
        symbol="NIFTY",
        exchange="NFO",
        expiry=date(2026, 1, 27),
        strike=24500,
        right="call",
        fetched_from_date=date(2026, 1, 5),
    )

    assert dao.option_contract_fetched_from(
        symbol="NIFTY",
        exchange="NFO",
        expiry=date(2026, 1, 27),
        strike=24500,
        right="call",
    ) == date(2026, 1, 5)
