package services

import (
	"ai-later-nav/internal/models"
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
)

const BenchLMTaskKey = "benchlm_leaderboard"

var ErrTaskRunning = errors.New("任务正在运行")

type CronTaskStore interface {
	Get(key string) (models.CronTask, error)
	List() ([]models.CronTask, error)
	Update(key, spec string, enabled bool) error
	RecordRun(key string, runErr error) error
}

type CronTaskScheduler struct {
	mu         sync.Mutex
	store      CronTaskStore
	jobs       map[string]func(context.Context) error
	manualJobs map[string]func(context.Context) error
	running    map[string]bool
	cron       *cron.Cron
	entries    map[string]cron.EntryID
	loaded     map[string]models.CronTask
	parser     cron.Parser
	ctx        context.Context
}

func NewCronTaskScheduler(store CronTaskStore, jobs map[string]func(context.Context) error) *CronTaskScheduler {
	return &CronTaskScheduler{
		store: store, jobs: jobs, manualJobs: make(map[string]func(context.Context) error), running: make(map[string]bool), entries: make(map[string]cron.EntryID), loaded: make(map[string]models.CronTask),
		parser: cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow),
		ctx:    context.Background(),
		cron:   cron.New(cron.WithLocation(time.UTC), cron.WithChain(cron.SkipIfStillRunning(cron.DefaultLogger))),
	}
}

// validateSpec accepts only daily fixed-time expressions; minute-level jobs could flood the upstream.
func (s *CronTaskScheduler) validateSpec(spec string) error {
	fields := strings.Fields(spec)
	if len(fields) != 5 {
		return errors.New("需要五段 cron 表达式（分 时 日 月 周）")
	}
	if _, err := s.parser.Parse(spec); err != nil {
		return fmt.Errorf("无效 cron 表达式: %w", err)
	}
	minute, minuteErr := strconv.Atoi(fields[0])
	hour, hourErr := strconv.Atoi(fields[1])
	if minuteErr != nil || hourErr != nil || minute < 0 || minute > 59 || hour < 0 || hour > 23 || fields[0] != strconv.Itoa(minute) || fields[1] != strconv.Itoa(hour) || fields[2] != "*" || fields[3] != "*" || fields[4] != "*" {
		return errors.New("目前只支持每天固定时间（UTC），例如 0 3 * * *")
	}
	return nil
}

func (s *CronTaskScheduler) NextRun(task models.CronTask, now time.Time) (time.Time, error) {
	schedule, err := s.parser.Parse(task.Spec)
	if err != nil {
		return time.Time{}, err
	}
	return schedule.Next(now.UTC()), nil
}

func (s *CronTaskScheduler) Update(key, spec string, enabled bool) error {
	if _, ok := s.jobs[key]; !ok {
		return errors.New("未知任务")
	}
	if err := s.validateSpec(spec); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.store.Update(key, spec, enabled); err != nil {
		return err
	}
	return s.reloadLocked(key)
}

func (s *CronTaskScheduler) SetContext(ctx context.Context) { s.ctx = ctx }

func (s *CronTaskScheduler) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tasks, err := s.store.List()
	if err != nil {
		return err
	}
	for _, task := range tasks {
		if err := s.loadLocked(task); err != nil {
			return err
		}
	}
	s.cron.Start()
	return nil
}

func (s *CronTaskScheduler) Stop() {
	<-s.cron.Stop().Done()
}

// Reconcile picks up admin changes made on other instances without duplicating jobs.
func (s *CronTaskScheduler) Reconcile() error {
	tasks, err := s.store.List()
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, task := range tasks {
		previous, ok := s.loaded[task.Key]
		if ok && previous.Spec == task.Spec && previous.Enabled == task.Enabled {
			continue
		}
		if err := s.loadLocked(task); err != nil {
			return err
		}
	}
	return nil
}

func (s *CronTaskScheduler) reloadLocked(key string) error {
	task, err := s.store.Get(key)
	if err != nil {
		return err
	}
	return s.loadLocked(task)
}

// SetManualJob registers the explicit recovery path for a known scheduled task.
func (s *CronTaskScheduler) SetManualJob(key string, job func(context.Context) error) {
	s.manualJobs[key] = job
}

// RunNow executes an enabled scheduled task; disabled tasks remain disabled.
func (s *CronTaskScheduler) RunNow(ctx context.Context, key string) error {
	return s.run(ctx, key, false, "")
}

// RunManual bypasses the schedule and daily freshness guard without changing settings.
func (s *CronTaskScheduler) RunManual(ctx context.Context, key string) error {
	return s.run(ctx, key, true, "")
}

// StartManual queues a run independent of the HTTP request lifetime.
func (s *CronTaskScheduler) StartManual(key string) error {
	if _, ok := s.jobs[key]; !ok {
		return errors.New("未知任务")
	}
	if _, ok := s.manualJobs[key]; !ok {
		return errors.New("不支持手动运行该任务")
	}
	s.mu.Lock()
	if s.running[key] {
		s.mu.Unlock()
		return ErrTaskRunning
	}
	s.running[key] = true
	s.mu.Unlock()
	go func() {
		ctx, cancel := context.WithTimeout(s.ctx, 3*time.Minute)
		defer cancel()
		if err := s.runReserved(ctx, key, true, ""); err != nil {
			log.Printf("manual cron task %s failed: %v", key, err)
		}
	}()
	return nil
}

func (s *CronTaskScheduler) runScheduled(ctx context.Context, key, spec string) error {
	return s.run(ctx, key, false, spec)
}

func (s *CronTaskScheduler) run(ctx context.Context, key string, manual bool, scheduledSpec string) error {
	_, ok := s.jobs[key]
	if !ok {
		return errors.New("未知任务")
	}
	if manual {
		_, ok = s.manualJobs[key]
		if !ok {
			return errors.New("不支持手动运行该任务")
		}
	}
	s.mu.Lock()
	if s.running[key] {
		s.mu.Unlock()
		return ErrTaskRunning
	}
	s.running[key] = true
	s.mu.Unlock()
	return s.runReserved(ctx, key, manual, scheduledSpec)
}

func (s *CronTaskScheduler) runReserved(ctx context.Context, key string, manual bool, scheduledSpec string) error {
	defer func() {
		s.mu.Lock()
		delete(s.running, key)
		s.mu.Unlock()
	}()
	job := s.jobs[key]
	if manual {
		job = s.manualJobs[key]
	}
	task, err := s.store.Get(key)
	if err != nil {
		return err
	}
	if !manual && (!task.Enabled || scheduledSpec != "" && task.Spec != scheduledSpec) {
		return nil
	}
	runErr := job(ctx)
	if err := s.store.RecordRun(key, runErr); err != nil {
		log.Printf("record cron task %s: %v", key, err)
	}
	return runErr
}

func (s *CronTaskScheduler) loadLocked(task models.CronTask) error {
	if task.Enabled {
		if err := s.validateSpec(task.Spec); err != nil {
			return err
		}
	}
	if id := s.entries[task.Key]; id != 0 {
		s.cron.Remove(id)
		delete(s.entries, task.Key)
	}
	s.loaded[task.Key] = task
	_, ok := s.jobs[task.Key]
	if !ok || !task.Enabled {
		return nil
	}
	id, err := s.cron.AddFunc(task.Spec, func() {
		ctx, cancel := context.WithTimeout(s.ctx, 3*time.Minute)
		defer cancel()
		if err := s.runScheduled(ctx, task.Key, task.Spec); err != nil {
			log.Printf("cron task %s failed: %v", task.Key, err)
		}
	})
	if err != nil {
		return err
	}
	s.entries[task.Key] = id
	return nil
}
