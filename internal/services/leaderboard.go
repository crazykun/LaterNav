package services

import (
	"ai-later-nav/internal/models"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

const BenchLMLeaderboardURL = "https://benchlm.ai/data/leaderboard.json"
const maxLeaderboardBytes int64 = 8 << 20

type leaderboardDocument struct {
	SchemaVersion string    `json:"schemaVersion"`
	GeneratedAt   time.Time `json:"generatedAt"`
	Items         []struct {
		Key            string   `json:"canonicalModelKey"`
		Name           string   `json:"model"`
		Creator        string   `json:"creator"`
		Score          *float64 `json:"displayScore"`
		Rank           int      `json:"overallRank"`
		EvidenceStatus string   `json:"evidenceStatus"`
		Eligible       bool     `json:"rankingEligible"`
		URL            string   `json:"url"`
	} `json:"items"`
}

func ParseLeaderboard(r io.Reader) (models.Leaderboard, error) {
	var doc leaderboardDocument
	decoder := json.NewDecoder(io.LimitReader(r, maxLeaderboardBytes+1))
	if err := decoder.Decode(&doc); err != nil {
		return models.Leaderboard{}, fmt.Errorf("decode leaderboard: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return models.Leaderboard{}, errors.New("leaderboard exceeds size limit or contains trailing data")
	}
	if doc.SchemaVersion != "1.0" || doc.GeneratedAt.IsZero() || len(doc.Items) == 0 {
		return models.Leaderboard{}, errors.New("invalid leaderboard metadata")
	}
	board := models.Leaderboard{GeneratedAt: doc.GeneratedAt}
	seen := map[string]bool{}
	ranks := map[int]bool{}
	for _, item := range doc.Items {
		if !item.Eligible {
			continue
		}
		u, err := url.Parse(item.URL)
		if item.Key == "" || len(item.Key) > 255 || item.Name == "" || len(item.Name) > 255 || len(item.Creator) > 255 || item.Score == nil || math.IsNaN(*item.Score) || math.IsInf(*item.Score, 0) || *item.Score < 0 || *item.Score > 100 || item.Rank <= 0 || seen[item.Key] || ranks[item.Rank] || err != nil || len(item.URL) > 500 || u.Scheme != "https" || u.Host != "benchlm.ai" || !strings.HasPrefix(u.Path, "/models/") || (item.EvidenceStatus != "estimated" && item.EvidenceStatus != "supported") {
			return models.Leaderboard{}, fmt.Errorf("invalid ranked model %q", item.Key)
		}
		seen[item.Key] = true
		ranks[item.Rank] = true
		board.Models = append(board.Models, models.RankedModel{Key: item.Key, Name: item.Name, Creator: item.Creator, Rank: item.Rank, Score: *item.Score, EvidenceStatus: item.EvidenceStatus, URL: item.URL})
	}
	if len(board.Models) == 0 {
		return models.Leaderboard{}, errors.New("no eligible ranked models")
	}
	sort.Slice(board.Models, func(i, j int) bool { return board.Models[i].Rank < board.Models[j].Rank })
	return board, nil
}

func FetchLeaderboard(ctx context.Context, client *http.Client, endpoint string) (models.Leaderboard, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return models.Leaderboard{}, err
	}
	res, err := client.Do(req)
	if err != nil {
		return models.Leaderboard{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return models.Leaderboard{}, fmt.Errorf("leaderboard response: %s", res.Status)
	}
	return ParseLeaderboard(res.Body)
}

func SyncDue(lastSync, now time.Time) bool {
	if lastSync.IsZero() {
		return true
	}
	y1, m1, d1 := lastSync.UTC().Date()
	y2, m2, d2 := now.UTC().Date()
	return y1 != y2 || m1 != m2 || d1 != d2
}

type LeaderboardStore interface {
	LastSync() (time.Time, error)
	Replace(models.Leaderboard) error
}

func SyncIfDue(ctx context.Context, store LeaderboardStore, now time.Time, fetch func(context.Context) (models.Leaderboard, error)) error {
	last, err := store.LastSync()
	if err != nil {
		return err
	}
	if !SyncDue(last, now) {
		return nil
	}
	board, err := fetch(ctx)
	if err != nil {
		return err
	}
	return store.Replace(board)
}

// RetrySync makes one initial attempt and at most five more, with bounded backoff.
func RetrySync(ctx context.Context, sync func(context.Context) error, wait func(context.Context, time.Duration) error) error {
	var err error
	for attempt := 0; attempt <= 5; attempt++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if attempt > 0 {
			if e := wait(ctx, time.Duration(1<<(attempt-1))*time.Second); e != nil {
				return e
			}
		}
		if err = sync(ctx); err == nil {
			return nil
		}
	}
	return fmt.Errorf("leaderboard sync failed after 6 attempts: %w", err)
}
