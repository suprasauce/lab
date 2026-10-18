package models

type PositionSide string

const (
	Long  PositionSide = "LONG"
	Short PositionSide = "SHORT"
)

type Leg interface {
	GetID() string
	GetPositionSide() PositionSide
	GetQuantity() int64
}

type BaseLeg struct {
	ID           string
	PositionSide PositionSide
	Quantity     int64
}

func (l BaseLeg) GetID() string                 { return l.ID }
func (l BaseLeg) GetPositionSide() PositionSide { return l.PositionSide }
func (l BaseLeg) GetQuantity() int64            { return l.Quantity }
