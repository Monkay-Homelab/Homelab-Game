package models

import "time"

type BitcoinPrice struct {
	CurrentPrice int64     `json:"current_price"`
	Seed         int64     `json:"seed"`
	LastStepAt   time.Time `json:"last_step_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type BitcoinPricePoint struct {
	Time  time.Time `json:"time"`
	Price int64     `json:"price"`
}
