CREATE TYPE option_type_enum AS ENUM ('CALL', 'PUT');
CREATE TYPE timeframe_enum AS ENUM ('1m', '5m', '15m', '1d');

CREATE TABLE underlyings (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,

    symbol VARCHAR(50) NOT NULL UNIQUE,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);


CREATE TABLE option_contracts (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,

    underlying_id BIGINT NOT NULL
        REFERENCES underlyings(id),

    expiry DATE NOT NULL,

    strike_price BIGINT NOT NULL,

   option_type option_type_enum NOT NULL,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT unique_option_contract UNIQUE (
        underlying_id,
        expiry,
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