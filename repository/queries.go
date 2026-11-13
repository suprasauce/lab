package repository

const QueryListTemplates = `
	SELECT id, name, script, created_at, updated_at, description
	FROM lab.strategies
	WHERE user_id = $1
	ORDER BY name ASC, id ASC;
`

const QueryCreateStrategy = `
	INSERT INTO lab.strategies (user_id, name, script, description)
	VALUES ($1, $2, $3, $4)
	RETURNING id;
`

const QueryListStrategies = `
	SELECT id, name, script, created_at, updated_at, description
	FROM lab.strategies
	WHERE user_id = $1
	ORDER BY updated_at DESC, id DESC;
`

const QueryUpdateStrategy = `
	UPDATE lab.strategies
	SET name = $1, script = $2, description = $5, updated_at = NOW()
	WHERE id = $3 AND user_id = $4
	RETURNING id;
`

const QueryPendingBacktestIDs = `
	SELECT id
	FROM lab.backtests
	WHERE status = 'PENDING'
	ORDER BY created_at ASC, id ASC;
`

const QueryListBacktests = `
	SELECT id, status, created_at
	FROM lab.backtests
	WHERE user_id = $1
	ORDER BY created_at DESC;
`

// QueryCreateBacktest inserts a backtest using the database defaults for status and results.
const QueryCreateBacktest = `
	INSERT INTO lab.backtests (user_id, script)
	VALUES ($1, $2)
	RETURNING id;
`

const QueryGetBacktest = `
	SELECT id, script
	FROM lab.backtests
	WHERE id = $1;
`

const QueryGetBacktestForUser = `
	SELECT id, status, results, script
	FROM lab.backtests
	WHERE id = $1 AND user_id = $2;
`

const QueryUpdateBacktest = `
	UPDATE lab.backtests
	SET status = $2, results = $3::jsonb, updated_at = NOW()
	WHERE id = $1;
`

const queryGetAllUnderlyings = `
	SELECT id, symbol
	FROM lab.underlyings
	ORDER BY symbol;
`

const queryGetUnderlying = `
	SELECT id, symbol
	FROM lab.underlyings
	WHERE symbol = $1;
`

const queryGetUnderlyingSymbol = `
	SELECT symbol
	FROM lab.underlyings
	WHERE id = $1;
`

const queryGetExpiries = `
	SELECT expiry_date
	FROM lab.option_expiry
	WHERE underlying_id = $1
	  AND expiry_date BETWEEN $2::date AND $3::date
	ORDER BY expiry_date ASC;
`

const queryHasOptionCandlesForDay = `
	SELECT EXISTS (
		SELECT 1 FROM lab.option_candles
		WHERE option_contract_id = $1 AND timeframe = '5m'
		  AND start_time >= $2 AND start_time < $3
	);
`

const queryGetOptionContract = `
	SELECT c.id, c.underlying_id, c.expiry_id, e.expiry_date, c.strike_price, c.option_type, c.synced
	FROM lab.option_contracts AS c
	JOIN lab.option_expiry AS e ON e.id = c.expiry_id
	WHERE c.underlying_id = $1
	  AND e.underlying_id = $1
	  AND e.expiry_date = $2::date
	  AND c.strike_price = $3
	  AND c.option_type = $4;
`

const queryGetOptionContractByID = `
	SELECT c.id, c.underlying_id, c.expiry_id, e.expiry_date, c.strike_price, c.option_type, c.synced
	FROM lab.option_contracts AS c
	JOIN lab.option_expiry AS e ON e.id = c.expiry_id
	WHERE c.id = $1;
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
