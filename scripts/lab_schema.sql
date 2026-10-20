-- Fresh database setup. Existing installations use migrations instead.
BEGIN;
CREATE SCHEMA IF NOT EXISTS lab;
SET LOCAL search_path TO lab, pg_catalog;

CREATE TYPE option_type_enum AS ENUM ('CALL', 'PUT');
CREATE TYPE timeframe_enum AS ENUM ('1m', '5m', '15m', '1d');
CREATE TYPE expiry_type AS ENUM ('MONTHLY', 'WEEKLY');

CREATE TABLE underlyings (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,

    symbol VARCHAR(50) NOT NULL UNIQUE,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);


CREATE TABLE option_expiry (
    id BIGSERIAL PRIMARY KEY,
    underlying_id BIGINT NOT NULL REFERENCES underlyings(id),
    expiry_date DATE NOT NULL,
    expiry_type expiry_type NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (underlying_id, expiry_date)
);

CREATE TABLE option_contracts (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,

    underlying_id BIGINT NOT NULL
        REFERENCES underlyings(id),

    expiry_id BIGINT NOT NULL REFERENCES option_expiry(id),

    strike_price BIGINT NOT NULL,

   option_type option_type_enum NOT NULL,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT unique_option_contract UNIQUE (
        expiry_id,
        strike_price,
        option_type
    )
);

CREATE TABLE underlying_candles (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,

    underlying_id BIGINT NOT NULL
        REFERENCES underlyings(id),

    start_time TIMESTAMPTZ NOT NULL,
    timeframe timeframe_enum NOT NULL,

    open BIGINT NOT NULL,
    high BIGINT NOT NULL,
    low BIGINT NOT NULL,
    close BIGINT NOT NULL,
    volume BIGINT NOT NULL,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT unique_underlying_candle UNIQUE (
        underlying_id, timeframe, start_time
    )
);

CREATE TABLE option_candles (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,

    option_contract_id BIGINT NOT NULL
        REFERENCES option_contracts(id),

    start_time TIMESTAMPTZ NOT NULL,
    timeframe timeframe_enum NOT NULL,

    open BIGINT NOT NULL,
    high BIGINT NOT NULL,
    low BIGINT NOT NULL,
    close BIGINT NOT NULL,
    volume BIGINT NOT NULL,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT unique_option_candle UNIQUE (
        option_contract_id, timeframe, start_time
    )
);

COMMIT;
