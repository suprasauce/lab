package engine

// BacktestResult retains position histories and contract metadata per expiry.
type BacktestResult struct {
	Underlying string
	Executions [][]*Position
}
