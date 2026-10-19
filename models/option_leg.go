package models

type PositionSide string

const (
	Long  PositionSide = "LONG"
	Short PositionSide = "SHORT"
)

type OptionLeg struct {
	ID           string
	PositionSide PositionSide
	Quantity     int64

	OptionType OptionType
	EntryDTE   int
	// EntryHour and EntryMinute are clock components in Asia/Kolkata.
	EntryHour   int
	EntryMinute int
	ExitDTE     int
	// ExitTime is an HH:mm clock time in Asia/Kolkata.
	ExitTime           string
	InstrumentSelector InstrumentSelector
}
