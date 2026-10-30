package models

import "time"

type NavItem struct {
	ID          int64     `json:"id" db:"id"`
	Name        string    `json:"name" db:"name"`
	URL         string    `json:"url" db:"url"`
	Description string    `json:"description" db:"description"`
	Icon        string    `json:"icon" db:"icon"`
	SortOrder   int       `json:"sort_order" db:"sort_order"`
	Enabled     bool      `json:"enabled" db:"enabled"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}
