package models

type Tag struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type TagUsage struct {
	Tag
	SiteCount int64 `json:"site_count"`
}

type TagPage struct {
	Tags       []TagUsage
	Total      int64
	Page       int
	PageSize   int
	TotalPages int
}
