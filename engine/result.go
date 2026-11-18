package engine

// BacktestResult retains the execution ledger and resolved order metadata per expiry.
type BacktestResult struct {
	Executions []*Execution
}
