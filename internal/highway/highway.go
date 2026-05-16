package highway

import (
	"context"
	"errors"
	"sync"
)

type Job interface {
	ID() string
	Type() string
	Run(ctx context.Context, progress chan<- Progress) error
	Marshal() ([]byte, error)
}

type JobUnmarshaler func(data []byte) (Job, error)

type Highway struct {
	workers      int
	statePath    string
	unmarshalers map[string]JobUnmarshaler

	mu        sync.Mutex
	pending   []Job
	completed map[string]bool
	failures  []error
	progress  chan Progress
}

func New(workers int, statePath string) *Highway {
	if workers < 1 {
		workers = 1
	}
	return &Highway{
		workers:      workers,
		statePath:    statePath,
		unmarshalers: make(map[string]JobUnmarshaler),
		completed:    make(map[string]bool),
		progress:     make(chan Progress, 100),
	}
}

func (h *Highway) RegisterType(jobType string, unmarshal JobUnmarshaler) {
	h.unmarshalers[jobType] = unmarshal
}

func (h *Highway) Submit(jobs ...Job) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.pending = append(h.pending, jobs...)
}

func (h *Highway) Progress() <-chan Progress {
	return h.progress
}

func (h *Highway) PendingJobIDs() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	ids := make([]string, len(h.pending))
	for i, j := range h.pending {
		ids[i] = j.ID()
	}
	return ids
}

func (h *Highway) Run(ctx context.Context) error {
	jobCh := make(chan Job)
	var wg sync.WaitGroup

	for range h.workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobCh {
				h.executeJob(ctx, job)
			}
		}()
	}

	go func() {
		defer close(jobCh)
		h.mu.Lock()
		jobs := h.pending
		h.mu.Unlock()

		for _, job := range jobs {
			if h.isCompleted(job.ID()) {
				continue
			}

			select {
			case <-ctx.Done():
				return
			case jobCh <- job:
			}
		}
	}()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		close(h.progress)
		if ctx.Err() != nil {
			return h.saveState()
		}
		if err := h.failureError(); err != nil {
			h.deleteState()
			return err
		}
		h.deleteState()
		return nil
	case <-ctx.Done():
		wg.Wait()
		close(h.progress)
		return h.saveState()
	}
}

func (h *Highway) executeJob(ctx context.Context, job Job) {
	err := job.Run(ctx, h.progress)

	if err != nil {
		h.progress <- Progress{
			JobID:  job.ID(),
			Done:   true,
			Error:  err,
			ErrMsg: err.Error(),
		}
		if ctx.Err() != nil && errors.Is(err, ctx.Err()) {
			return
		}
		h.markFailed(job.ID(), err)
	} else {
		h.markCompleted(job.ID())
	}
}

func (h *Highway) isCompleted(id string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.completed[id]
}

func (h *Highway) markCompleted(id string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.completed[id] = true
}

func (h *Highway) markFailed(id string, err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.completed[id] = true
	h.failures = append(h.failures, err)
}

func (h *Highway) failureError() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	return errors.Join(h.failures...)
}
