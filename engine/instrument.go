package engine

import "optionsbacktest/models"

// GetLotSize returns expiry-specific option units per lot, or one for an underlying.
func (e *Execution) GetLotSize(symbol string) (int64, error) {
	instrument, err := e.resolveInstrument(symbol)
	if err != nil {
		return 0, err
	}
	return int64(instrument.multiplier()), nil
}

// instrument holds resolved metadata needed for candles and lot-size accounting.
type instrument struct {
	TradingSymbol string
	Contract      *models.Option
	Underlying    *models.Underlying
}

func (i *instrument) multiplier() models.Price {
	if i.Contract != nil {
		return models.Price(i.Contract.LotSize)
	}
	return 1
}
