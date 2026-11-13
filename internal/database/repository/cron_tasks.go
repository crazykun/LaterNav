package repository

import (
	"ai-later-nav/internal/models"
	"database/sql"
	"errors"
	"fmt"
	"time"
	"unicode/utf8"
)

type CronTaskRepository struct{ db *sql.DB }

func NewCronTaskRepository(db *sql.DB) *CronTaskRepository { return &CronTaskRepository{db: db} }

func scanCronTask(scan func(...any) error) (models.CronTask, error) {
	var task models.CronTask
	var runAt sql.NullTime
	if err := scan(&task.Key, &task.Name, &task.Spec, &task.Enabled, &runAt, &task.LastError); err != nil {
		return task, err
	}
	if runAt.Valid {
		t := timestampFromSQL(runAt.Time)
		task.LastRunAt = &t
	}
	return task, nil
}

func (r *CronTaskRepository) Get(key string) (models.CronTask, error) {
	return scanCronTask(r.db.QueryRow("SELECT task_key,name,cron_spec,enabled,last_run_at,last_error FROM cron_tasks WHERE task_key = ?", key).Scan)
}

func (r *CronTaskRepository) List() ([]models.CronTask, error) {
	rows, err := r.db.Query("SELECT task_key,name,cron_spec,enabled,last_run_at,last_error FROM cron_tasks ORDER BY task_key")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tasks []models.CronTask
	for rows.Next() {
		task, err := scanCronTask(rows.Scan)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	return tasks, rows.Err()
}

func (r *CronTaskRepository) Update(key, spec string, enabled bool) error {
	result, err := r.db.Exec("UPDATE cron_tasks SET cron_spec = ?, enabled = ? WHERE task_key = ?", spec, enabled, key)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		// MySQL reports changed rows by default; an unchanged form is a valid save.
		var exists int
		if err := r.db.QueryRow("SELECT 1 FROM cron_tasks WHERE task_key = ?", key).Scan(&exists); err != nil {
			return err
		}
	}
	return nil
}

func (r *CronTaskRepository) RecordRun(key string, runErr error) error {
	message := ""
	if runErr != nil {
		message = runErr.Error()
		if utf8.RuneCountInString(message) > 1000 {
			message = string([]rune(message)[:1000])
		}
	}
	result, err := r.db.Exec("UPDATE cron_tasks SET last_run_at = ?, last_error = ? WHERE task_key = ?", timestampForSQL(time.Now()), message, key)
	if err != nil {
		return fmt.Errorf("record cron run: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return errors.New("cron task missing")
	}
	return nil
}
