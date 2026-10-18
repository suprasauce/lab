package models

type LegStatus string

const (
	LegPending LegStatus = "PENDING"
	LegOpen    LegStatus = "OPEN"
	LegClosed  LegStatus = "CLOSED"
)

type LegState struct {
	Status LegStatus
}

// LegExecution associates leg configuration with its state for one expiry cycle.
type LegExecution struct {
	Leg   *Leg
	State LegState
}
