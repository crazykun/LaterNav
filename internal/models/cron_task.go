package models

import "time"

type CronTask struct {
	Key       string
	Name      string
	Spec      string
	Enabled   bool
	LastRunAt *time.Time
	LastError string
}
