package models

import "time"

type FriendLink struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	URL         string    `json:"url"`
	Description string    `json:"description"`
	Logo        string    `json:"logo"`
	SortOrder   int       `json:"sort_order"`
	Enabled     bool      `json:"enabled"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`

	// Color/Initials 是展示字段，不落库：Logo 缺失时用名称生成彩色首字头像。
	Color    string `json:"color"`
	Initials string `json:"initials"`
}
