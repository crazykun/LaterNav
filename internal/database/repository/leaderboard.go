package repository

import (
	"ai-later-nav/internal/models"
	"context"
	"database/sql"
	"fmt"
	"time"
)

type LeaderboardRepository struct{ db *sql.DB }

func NewLeaderboardRepository(db *sql.DB) *LeaderboardRepository {
	return &LeaderboardRepository{db: db}
}

// DATETIME has no zone; MySQL's loc=Local parses its UTC wall clock as local.
func timestampFromSQL(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), time.UTC)
}

func timestampForSQL(t time.Time) time.Time {
	u := t.UTC()
	return time.Date(u.Year(), u.Month(), u.Day(), u.Hour(), u.Minute(), u.Second(), u.Nanosecond(), time.Local)
}

func syncDateBefore(last, now time.Time) bool {
	y1, m1, d1 := last.UTC().Date()
	y2, m2, d2 := now.UTC().Date()
	return y1 < y2 || y1 == y2 && (m1 < m2 || m1 == m2 && d1 < d2)
}

// Replace publishes an automatic daily snapshot.
func (r *LeaderboardRepository) Replace(board models.Leaderboard) error {
	return r.replace(board, false)
}

// ReplaceManual publishes a validated snapshot even if today's run already succeeded.
func (r *LeaderboardRepository) ReplaceManual(board models.Leaderboard) error {
	return r.replace(board, true)
}

// replace makes the new snapshot visible only after all rows have been written.
func (r *LeaderboardRepository) replace(board models.Leaderboard, manual bool) error {
	if len(board.Models) == 0 {
		return fmt.Errorf("empty leaderboard")
	}
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("begin leaderboard transaction: %w", err)
	}
	defer tx.Rollback()
	// The sync row serializes concurrent importers, including across processes.
	var previous sql.NullInt64
	var generated, synced sql.NullTime
	if err := tx.QueryRow("SELECT current_snapshot, generated_at, synced_at FROM leaderboard_sync WHERE id = 1 FOR UPDATE").Scan(&previous, &generated, &synced); err != nil {
		return fmt.Errorf("lock leaderboard: %w", err)
	}
	// Recheck inside the lock: another process may have published while we fetched.
	if !manual && synced.Valid && !syncDateBefore(timestampFromSQL(synced.Time), time.Now()) {
		return nil
	}
	if generated.Valid && board.GeneratedAt.Before(timestampFromSQL(generated.Time)) {
		return fmt.Errorf("refusing older leaderboard snapshot")
	}
	snapshot := uint64(time.Now().UnixNano())
	stmt, err := tx.Prepare("INSERT INTO leaderboard_models (snapshot_id,model_key,name,creator,overall_rank,score,evidence_status,source_url) VALUES (?,?,?,?,?,?,?,?)")
	if err != nil {
		return fmt.Errorf("prepare leaderboard insert: %w", err)
	}
	for _, m := range board.Models {
		if _, err := stmt.Exec(snapshot, m.Key, m.Name, m.Creator, m.Rank, m.Score, m.EvidenceStatus, m.URL); err != nil {
			stmt.Close()
			return fmt.Errorf("insert leaderboard model: %w", err)
		}
	}
	if err := stmt.Close(); err != nil {
		return fmt.Errorf("close leaderboard insert: %w", err)
	}
	if _, err := tx.Exec("UPDATE leaderboard_sync SET current_snapshot = ?, generated_at = ?, synced_at = ? WHERE id = 1", snapshot, timestampForSQL(board.GeneratedAt), timestampForSQL(time.Now())); err != nil {
		return fmt.Errorf("publish leaderboard: %w", err)
	}
	if previous.Valid {
		if _, err := tx.Exec("DELETE FROM leaderboard_models WHERE snapshot_id = ?", previous.Int64); err != nil {
			return fmt.Errorf("remove old leaderboard: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit leaderboard: %w", err)
	}
	return nil
}

func (r *LeaderboardRepository) LastSync() (time.Time, error) {
	var t sql.NullTime
	if err := r.db.QueryRow("SELECT synced_at FROM leaderboard_sync WHERE id = 1").Scan(&t); err != nil {
		return time.Time{}, err
	}
	if !t.Valid {
		return time.Time{}, nil
	}
	return timestampFromSQL(t.Time), nil
}

func (r *LeaderboardRepository) Current() (models.Leaderboard, error) {
	board := models.Leaderboard{}
	// A repeatable-read transaction keeps the pointer and model rows on the same snapshot.
	tx, err := r.db.BeginTx(context.Background(), &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return board, err
	}
	defer tx.Rollback()
	var snapshot sql.NullInt64
	var generated, synced sql.NullTime
	if err := tx.QueryRow("SELECT current_snapshot,generated_at,synced_at FROM leaderboard_sync WHERE id = 1").Scan(&snapshot, &generated, &synced); err != nil {
		return board, err
	}
	if snapshot.Valid {
		board.GeneratedAt = timestampFromSQL(generated.Time)
		board.SyncedAt = timestampFromSQL(synced.Time)
		rows, err := tx.Query("SELECT model_key,name,creator,overall_rank,score,evidence_status,source_url FROM leaderboard_models WHERE snapshot_id = ? ORDER BY overall_rank ASC", snapshot.Int64)
		if err != nil {
			return board, err
		}
		for rows.Next() {
			var m models.RankedModel
			if err := rows.Scan(&m.Key, &m.Name, &m.Creator, &m.Rank, &m.Score, &m.EvidenceStatus, &m.URL); err != nil {
				rows.Close()
				return board, err
			}
			board.Models = append(board.Models, m)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return board, err
		}
		if err := rows.Close(); err != nil {
			return board, err
		}
	}
	if err := tx.Commit(); err != nil {
		return board, err
	}
	return board, nil
}
