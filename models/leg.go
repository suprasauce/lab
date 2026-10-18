package models

type PositionSide string

const (
	Long  PositionSide = "LONG"
	Short PositionSide = "SHORT"
)

type Leg struct {
	ID           string
	Instrument   Instrument
	PositionSide PositionSide
	// Quantity is the configured full entry and exit size in units.
	Quantity           int64
	EntryCriteria      EntryCriteria
	ExitCriteria       ExitCriteria
	InstrumentSelector InstrumentSelector
	// Trades are in execution order, with nondecreasing timestamps. Slice order
	// breaks ties. A closing trade must not exceed the quantity currently open.
	Trades []Trade
}
