package models

type LegStatus string

const (
	LegPending LegStatus = "PENDING"
	LegOpen    LegStatus = "OPEN"
	LegClosed  LegStatus = "CLOSED"
)

// LegExecution stores runtime status and position history for one leg during an expiry.
type LegExecution struct {
	Leg    *OptionLeg
	Status LegStatus
	Trades []Trade
}
