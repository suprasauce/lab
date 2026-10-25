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
}
