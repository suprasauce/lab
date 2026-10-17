package models

import "time"

type Timeframe string

const (
	Timeframe1m  Timeframe = "1m"
	Timeframe5m  Timeframe = "5m"
	Timeframe15m Timeframe = "15m"
	Timeframe1d  Timeframe = "1d"
)

type Candle struct {
	Timestamp time.Time
	Timeframe Timeframe
	Open      Price
	High      Price
	Low       Price
	Close     Price
	Volume    int64
}
