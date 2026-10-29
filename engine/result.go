package engine

// BacktestResult retains runtime legs and their contract metadata, grouped by expiry.
type BacktestResult struct {
	Underlying string
	Executions [][]*Leg
}
