package models

import "time"

type Category struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	SortOrder int       `json:"sort_order"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type CategoryUsage struct {
	Category
	SiteCount int64 `json:"site_count"`
}

type CategoryPage struct {
	Categories []CategoryUsage
	Total      int64
	Page       int
	PageSize   int
	TotalPages int
}
