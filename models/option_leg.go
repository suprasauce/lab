package models

type OptionLeg struct {
	BaseLeg

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
