package repository

const queryGetAllUnderlyings = `
	SELECT id, symbol
	FROM lab.underlyings
	ORDER BY symbol;
`

const queryGetExpiries = `
	SELECT DISTINCT expiry
	FROM lab.option_contracts
	WHERE underlying_id = $1
	  AND expiry BETWEEN $2::date AND $3::date
	ORDER BY expiry ASC;
`

const queryGetOptionContract = `
	SELECT id, underlying_id, expiry, strike_price, option_type
	FROM lab.option_contracts
	WHERE underlying_id = $1
	  AND expiry = $2
	  AND strike_price = $3
	  AND option_type = $4;
`

const queryGetUnderlyingCandle = `
	SELECT start_time, timeframe, open, high, low, close, volume
	FROM lab.underlying_candles
	WHERE underlying_id = $1
	  AND timeframe = $2
	  AND start_time >= $3
	ORDER BY start_time ASC
	LIMIT 1;
`

const queryGetOptionCandles = `
	SELECT start_time, timeframe, open, high, low, close, volume
	FROM lab.option_candles
	WHERE option_contract_id = $1
	  AND timeframe = $2
	  AND start_time >= $3
	  AND start_time <= $4
	ORDER BY start_time ASC;
`
