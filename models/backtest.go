package models

import "encoding/json"

type BacktestStatus string

const (
	BacktestPending   BacktestStatus = "PENDING"
	BacktestCompleted BacktestStatus = "COMPLETED"
	BacktestFailed    BacktestStatus = "FAILED"
)

type Backtest struct {
	ID      int64
	Script  string
	Status  BacktestStatus
	Results json.RawMessage
}
