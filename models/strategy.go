package models

import "time"

type Strategy struct {
	ID          int64
	Name        string
	Description string `json:"description"`
	Script      string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}
