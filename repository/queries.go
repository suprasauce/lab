package repository

const queryGetAllUnderlyings = `
	SELECT id, symbol
	FROM underlyings
	ORDER BY symbol;
`

const queryGetOptionContracts = `
	SELECT id, underlying_id, expiry, strike_price, option_type
	FROM option_contracts
	WHERE underlying_id = $1
	  AND option_type = $2
	  AND expiry BETWEEN $3 AND $4
	ORDER BY expiry ASC, strike_price ASC;
`
