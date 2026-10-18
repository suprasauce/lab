package repository

const queryGetAllUnderlyings = `
	SELECT id, symbol
	FROM underlyings
	ORDER BY symbol;
`

const queryGetExpiries = `
	SELECT DISTINCT expiry
	FROM option_contracts
	WHERE underlying_id = $1
	  AND expiry BETWEEN $2 AND $3
	ORDER BY expiry ASC;
`

const queryGetOptionContract = `
	SELECT id, underlying_id, expiry, strike_price, option_type
	FROM option_contracts
	WHERE underlying_id = $1
	  AND expiry = $2
	  AND strike_price = $3
	  AND option_type = $4;
`
