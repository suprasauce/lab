package engine

import "optionsbacktest/models"

// Order holds the order's placement and execution data.
type Order struct {
	models.Order
}

func (o *Order) multiplier() models.Price {
	if o.Contract != nil {
		return models.Price(o.Contract.LotSize)
	}
	return 1
}
