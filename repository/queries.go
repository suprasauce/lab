package repository

const queryGetExpiry = `
	WITH expiries AS (
		SELECT e.expiry_date, ROW_NUMBER() OVER (ORDER BY e.expiry_date) AS ordinal
		FROM lab.option_expiry AS e
		JOIN lab.underlyings AS u ON u.id = e.underlying_id
		WHERE u.symbol = $1 AND e.expiry_type = $2
	)
	SELECT expiry_date
	FROM expiries
	WHERE ordinal = (
		SELECT MIN(ordinal) FROM expiries WHERE expiry_date >= $3::date
	) + $4::bigint;
`

const queryGetIndicatorCandles = `
	SELECT c.start_time, c.timeframe, c.open, c.high, c.low, c.close, c.volume
	FROM lab.underlying_candles AS c
	JOIN lab.underlyings AS u ON u.id = c.underlying_id
	WHERE u.symbol = $1 AND c.timeframe = '5m'
	UNION ALL
	SELECT c.start_time, c.timeframe, c.open, c.high, c.low, c.close, c.volume
	FROM lab.option_candles AS c
	JOIN lab.option_contracts AS o ON o.id = c.option_contract_id
	WHERE o.tradingsymbol = $1 AND c.timeframe = '5m'
	ORDER BY start_time ASC;
`

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

const queryGetOptionChainStatus = `
 SELECT e.id, e.option_chain_fetched
 FROM lab.option_expiry AS e
 JOIN lab.underlyings AS u ON u.id = e.underlying_id
 WHERE u.symbol = $1 AND e.expiry_date = $2::date;
`

const queryGetOptionContract = `
	SELECT c.id, c.underlying_id, c.expiry_id, e.expiry_date, c.strike_price, c.option_type, u.symbol, e.lot_size
	FROM lab.option_contracts AS c
	JOIN lab.option_expiry AS e ON e.id = c.expiry_id
 JOIN lab.underlyings AS u ON u.id = c.underlying_id
	WHERE c.underlying_id = $1
	  AND e.underlying_id = $1
	  AND e.expiry_date = $2::date
	  AND c.strike_price = $3
	  AND c.option_type = $4;
`

const queryGetOptionTradingSymbol = `
 SELECT c.tradingsymbol
 FROM lab.option_contracts AS c
 JOIN lab.option_expiry AS e ON e.id = c.expiry_id
 WHERE c.underlying_id = $1 AND e.underlying_id = $1
   AND e.expiry_date = $2::date
   AND c.strike_price = $3 AND c.option_type = $4;
`

const queryGetOptionContractByTradingSymbol = `
 SELECT c.id, c.underlying_id, c.expiry_id, e.expiry_date, c.strike_price, c.option_type, u.symbol, e.lot_size
 FROM lab.option_contracts AS c
 JOIN lab.option_expiry AS e ON e.id = c.expiry_id
 JOIN lab.underlyings AS u ON u.id = c.underlying_id
 WHERE c.tradingsymbol = $1;
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

const queryGetLatestCompletedUnderlyingCandle = `
 SELECT start_time, timeframe, open, high, low, close, volume
 FROM lab.underlying_candles
 WHERE underlying_id = $1
   AND timeframe = '5m'
   AND start_time <= $2::timestamptz - INTERVAL '5 minutes'
 ORDER BY start_time DESC
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

const queryOptionExpiryExists = `
 SELECT EXISTS (
  SELECT 1
  FROM lab.option_expiry AS e
  JOIN lab.underlyings AS u ON u.id = e.underlying_id
  WHERE u.symbol = $1 AND e.expiry_date = $2::date
 );
`

const queryGetOptionDeltaCandidates = `
 SELECT c.tradingsymbol, c.strike_price, oc.close
 FROM lab.option_contracts AS c
 JOIN lab.option_expiry AS e ON e.id = c.expiry_id
 JOIN lab.option_candles AS oc ON oc.option_contract_id = c.id
 WHERE c.underlying_id = $1 AND e.underlying_id = $1
   AND e.expiry_date = $2::date AND c.option_type = $3
   AND oc.timeframe = '5m' AND oc.start_time = $4
 ORDER BY c.strike_price ASC, c.id ASC;
`
