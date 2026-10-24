package repository

const queryGetAllUnderlyings = `
	SELECT id, symbol
	FROM lab.underlyings
	ORDER BY symbol;
`

const queryGetExpiries = `
	SELECT expiry_date
	FROM lab.option_expiry
	WHERE underlying_id = $1
	  AND expiry_date BETWEEN $2::date AND $3::date
	ORDER BY expiry_date ASC;
`

const queryGetOptionContract = `
	SELECT c.id, c.underlying_id, c.expiry_id, e.expiry_date, c.strike_price, c.option_type
	FROM lab.option_contracts AS c
	JOIN lab.option_expiry AS e ON e.id = c.expiry_id
	WHERE c.underlying_id = $1
	  AND e.underlying_id = $1
	  AND e.expiry_date = $2::date
	  AND c.strike_price = $3
	  AND c.option_type = $4;
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

const queryGetUnderlyingCandles = `
	SELECT start_time, timeframe, open, high, low, close, volume
	FROM lab.underlying_candles
	WHERE underlying_id = $1
	  AND timeframe = $2
	  AND start_time >= $3
	  AND start_time < $4
	ORDER BY start_time ASC;
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
