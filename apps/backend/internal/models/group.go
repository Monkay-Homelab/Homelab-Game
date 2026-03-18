package models

import "time"

type Group struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	FounderID       string    `json:"founder_id"`
	MinContribution int64     `json:"min_contribution"`
	ProfitSplit     float64   `json:"profit_split"`
	CreatedAt       time.Time `json:"created_at"`
}

type GroupMember struct {
	GroupID  string    `json:"group_id"`
	UserID   string    `json:"user_id"`
	Role     string    `json:"role"`
	JoinedAt time.Time `json:"joined_at"`
}
