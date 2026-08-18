package task

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/dbpilot/dbpilot/internal/persistence"
	"github.com/dbpilot/dbpilot/internal/redact"
)

type Logger interface {
	Infof(format string, args ...any)
	Errorf(format string, args ...any)
}

// HandlerFunc executes a task inside a worker goroutine. Returning a
// *PartialError marks the task partial_success.
type HandlerFunc func(ctx context.Context, task *Task, payload []byte, log Logger) error

type PartialError struct{ Err error }

func (e *PartialError) Error() string { return e.Err.Error() }
func (e *PartialError) Unwrap() error { return e.Err }

type Manager struct {
	store    *Store
	mu       sync.RWMutex
	handlers map[string]HandlerFunc
	queue    chan int64
	wg       sync.WaitGroup
}

func NewManager(db *persistence.DB) *Manager {
	return &Manager{
		store:    NewStore(db),
		handlers: make(map[string]HandlerFunc),
		queue:    make(chan int64, 256),
	}
}

func (m *Manager) Store() *Store { return m.store }

func (m *Manager) Register(typ string, h HandlerFunc) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.handlers[typ] = h
}

func (m *Manager) Start(ctx context.Context, workers int) {
	if workers <= 0 {
		workers = 1
	}
	for i := 0; i < workers; i++ {
		m.wg.Add(1)
		go m.worker(ctx)
	}
}

func (m *Manager) Enqueue(ctx context.Context, typ, resourceType, resourceID string, payload any) (*Task, error) {
	var raw string
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		raw = string(b)
	}
	t, err := m.store.Create(ctx, typ, resourceType, resourceID, raw)
	if err != nil {
		return nil, err
	}
	select {
	case m.queue <- t.ID:
	default:
		return nil, errors.New("task queue is full")
	}
	return t, nil
}

func (m *Manager) worker(ctx context.Context) {
	defer m.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case id := <-m.queue:
			m.run(ctx, id)
		}
	}
}

func (m *Manager) run(ctx context.Context, id int64) {
	t, err := m.store.Get(ctx, id)
	if err != nil {
		return
	}
	m.mu.RLock()
	h, ok := m.handlers[t.Type]
	m.mu.RUnlock()
	if !ok {
		slog.Error("no handler registered", "task_id", id, "type", t.Type)
		_ = m.store.Finish(ctx, id, StatusFailed, "no_handler", "no handler registered", "")
		return
	}

	payload, _ := m.store.Payload(ctx, id)
	if err := m.store.MarkRunning(ctx, id); err != nil {
		return
	}

	log := &dbLogger{store: m.store, id: id}
	stopFlush := make(chan struct{})
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stopFlush:
				return
			case <-ticker.C:
				log.flushDelta(ctx)
			}
		}
	}()

	err = h(ctx, t, []byte(payload), log)
	close(stopFlush)
	log.flushDelta(ctx)

	var partial *PartialError
	switch {
	case err == nil:
		_ = m.store.Finish(ctx, id, StatusSuccess, "", "", log.String())
	case errors.As(err, &partial):
		_ = m.store.Finish(ctx, id, StatusPartialSuccess, "partial_success", partial.Error(), log.String())
	default:
		_ = m.store.Finish(ctx, id, StatusFailed, "failed", redact.String(err.Error()), log.String())
	}
}

type dbLogger struct {
	store   *Store
	id      int64
	mu      sync.Mutex
	buf     []byte
	flushed int
}

func (l *dbLogger) Infof(format string, args ...any) {
	l.append("INFO  " + redact.String(fmt.Sprintf(format, args...)))
}

func (l *dbLogger) Errorf(format string, args ...any) {
	l.append("ERROR " + redact.String(fmt.Sprintf(format, args...)))
}

func (l *dbLogger) append(line string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.buf = append(l.buf, line...)
	l.buf = append(l.buf, '\n')
}

func (l *dbLogger) flushDelta(ctx context.Context) {
	l.mu.Lock()
	delta := l.buf[l.flushed:]
	l.flushed = len(l.buf)
	l.mu.Unlock()
	if len(delta) > 0 {
		_ = l.store.AppendLog(ctx, l.id, string(delta))
	}
}

func (l *dbLogger) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return string(l.buf)
}
