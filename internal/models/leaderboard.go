package models

import "time"

// RankedModel is a model in the current BenchLM overall ranking.
type RankedModel struct {
	Key            string
	Name           string
	Creator        string
	Rank           int
	Score          float64
	EvidenceStatus string
	URL            string
}

// Leaderboard holds the most recent successful import.
type Leaderboard struct {
	Models      []RankedModel
	GeneratedAt time.Time
	SyncedAt    time.Time
}
