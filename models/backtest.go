package models

type BacktestStatus string

const (
	BacktestPending   BacktestStatus = "PENDING"
	BacktestCompleted BacktestStatus = "COMPLETED"
	BacktestFailed    BacktestStatus = "FAILED"
)

type Backtest struct {
	ID     int64
	Script string
}
