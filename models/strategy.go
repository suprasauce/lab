package models

import "time"

type Strategy struct {
	ID        string
	Name      string
	StartTime time.Time
	EndTime   time.Time
	Legs      []Leg
}
