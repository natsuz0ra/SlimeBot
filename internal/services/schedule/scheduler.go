package schedule

import (
	"context"
	"sync"
	"time"

	"slimebot/internal/logging"
)

// Scheduler periodically asks the service to run due tasks.
type Scheduler struct {
	service  *Service
	interval time.Duration

	mu      sync.Mutex
	stopped bool
	cancel  context.CancelFunc
	done    chan struct{}
	workers sync.WaitGroup
}

func NewScheduler(service *Service, interval time.Duration) *Scheduler {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	return &Scheduler{service: service, interval: interval}
}

func (s *Scheduler) Start(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.done != nil || s.stopped {
		return
	}
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	s.cancel = cancel
	s.done = done
	s.workers.Go(func() {
		defer cancel()
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()
		if err := s.service.RestoreRunningTasks(ctx); err != nil && ctx.Err() == nil {
			logging.Warn("scheduled_tasks_restore_running_failed", "err", err)
		}
		s.tick(ctx)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.tick(ctx)
			}
		}
	})
	go func() {
		s.workers.Wait()
		close(done)
	}()
}

// Stop prevents further starts, cancels the loop, and waits for the current tick.
func (s *Scheduler) Stop(ctx context.Context) error {
	s.mu.Lock()
	s.stopped = true
	if s.cancel != nil {
		s.cancel()
	}
	s.mu.Unlock()
	return s.Wait(ctx)
}

// Wait waits for a started scheduler to finish without requesting cancellation.
func (s *Scheduler) Wait(ctx context.Context) error {
	s.mu.Lock()
	done := s.done
	s.mu.Unlock()
	if done == nil {
		return nil
	}
	select {
	case <-done:
		return nil
	default:
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Scheduler) tick(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}
	if err := s.service.RunDue(ctx); err != nil && ctx.Err() == nil {
		logging.Warn("scheduled_tasks_tick_failed", "err", err)
	}
}
