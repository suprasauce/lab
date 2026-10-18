package repository

const queryGetAllUnderlyings = `
	SELECT id, symbol
	FROM underlyings
	ORDER BY symbol;
`
