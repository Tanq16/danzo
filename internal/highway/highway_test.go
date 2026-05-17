package highway

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type fakeJob struct {
	JobID   string `json:"id"`
	JobType string `json:"type"`
	Err     error  `json:"-"`
}

func (j fakeJob) ID() string   { return j.JobID }
func (j fakeJob) Type() string { return j.JobType }

func (j fakeJob) Run(ctx context.Context, progress chan<- Progress) error {
	return j.Err
}

func (j fakeJob) Marshal() ([]byte, error) {
	return json.Marshal(j)
}

type blockingJob struct {
	JobID   string `json:"id"`
	JobType string `json:"type"`
	started chan struct{}
}

func (j blockingJob) ID() string   { return j.JobID }
func (j blockingJob) Type() string { return j.JobType }

func (j blockingJob) Run(ctx context.Context, progress chan<- Progress) error {
	close(j.started)
	<-ctx.Done()
	return ctx.Err()
}

func (j blockingJob) Marshal() ([]byte, error) {
	return json.Marshal(fakeJob{JobID: j.JobID, JobType: j.JobType})
}

func TestRunReturnsJobFailures(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.json")
	jobErr := errors.New("boom")
	hw := New(1, statePath)
	hw.Submit(fakeJob{JobID: "job-1", JobType: "fake", Err: jobErr})

	err := hw.Run(context.Background())
	if !errors.Is(err, jobErr) {
		t.Fatalf("expected job error, got %v", err)
	}

	if _, statErr := os.Stat(statePath); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("expected state file to be deleted, got %v", statErr)
	}
}

func TestRunDeletesStateAfterSuccessfulCompletion(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(statePath, []byte("stale"), 0644); err != nil {
		t.Fatal(err)
	}
	hw := New(1, statePath)
	hw.Submit(fakeJob{JobID: "job-1", JobType: "fake"})

	if err := hw.Run(context.Background()); err != nil {
		t.Fatalf("run: %v", err)
	}
	if _, statErr := os.Stat(statePath); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("expected stale state file to be deleted, got %v", statErr)
	}
}

func TestRunSavesPendingStateWhenContextIsCanceled(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.json")
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	hw := New(1, statePath)
	hw.Submit(blockingJob{JobID: "job-1", JobType: "fake", started: started})

	errCh := make(chan error, 1)
	go func() {
		errCh <- hw.Run(ctx)
	}()
	<-started
	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("run after cancellation: %v", err)
	}

	data, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("read saved state: %v", err)
	}
	var state persistedState
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatalf("unmarshal saved state: %v", err)
	}
	if len(state.Completed) != 0 {
		t.Fatalf("canceled job should not be marked completed: %#v", state.Completed)
	}
	if len(state.Pending) != 1 || state.Pending[0].ID != "job-1" || state.Pending[0].Type != "fake" {
		t.Fatalf("expected canceled job to remain pending, got %#v", state.Pending)
	}
}

func TestLoadStateSubmitsPendingJobs(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.json")
	state := persistedState{
		Pending: []persistedJob{
			{
				ID:   "job-1",
				Type: "fake",
				Data: json.RawMessage(`{"id":"job-1","type":"fake"}`),
			},
		},
	}

	data, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statePath, data, 0644); err != nil {
		t.Fatal(err)
	}

	hw := New(1, statePath)
	hw.RegisterType("fake", func(data []byte) (Job, error) {
		var job fakeJob
		if err := json.Unmarshal(data, &job); err != nil {
			return nil, err
		}
		return job, nil
	})

	if err := hw.LoadState(); err != nil {
		t.Fatalf("load state: %v", err)
	}

	ids := hw.PendingJobIDs()
	if len(ids) != 1 || ids[0] != "job-1" {
		t.Fatalf("expected pending job job-1, got %v", ids)
	}
}

func TestLoadStateRejectsCorruptJSON(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(statePath, []byte("{not-json"), 0644); err != nil {
		t.Fatal(err)
	}

	hw := New(1, statePath)
	if err := hw.LoadState(); err == nil {
		t.Fatalf("expected corrupt JSON error")
	}
}

func TestLoadStateRejectsUnknownJobTypes(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.json")
	state := persistedState{
		Pending: []persistedJob{
			{ID: "job-1", Type: "missing", Data: json.RawMessage(`{}`)},
		},
	}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statePath, data, 0644); err != nil {
		t.Fatal(err)
	}

	hw := New(1, statePath)
	if err := hw.LoadState(); err == nil {
		t.Fatalf("expected unknown job type error")
	}
}

func TestLoadStateWrapsJobUnmarshalFailures(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.json")
	state := persistedState{
		Pending: []persistedJob{
			{ID: "job-1", Type: "fake", Data: json.RawMessage(`{}`)},
		},
	}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statePath, data, 0644); err != nil {
		t.Fatal(err)
	}

	unmarshalErr := errors.New("cannot decode fake job")
	hw := New(1, statePath)
	hw.RegisterType("fake", func(data []byte) (Job, error) {
		return nil, unmarshalErr
	})

	err = hw.LoadState()
	if err == nil {
		t.Fatalf("expected job unmarshal error")
	}
	if !errors.Is(err, unmarshalErr) {
		t.Fatalf("expected wrapped unmarshal error, got %v", err)
	}
}
