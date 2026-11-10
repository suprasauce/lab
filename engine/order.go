package engine

import (
	"optionsbacktest/models"
	"optionsbacktest/selector"
)

// Order adds runtime contract selection to the order's domain data.
type Order struct {
	models.Order
	Selector selector.Selector
}
