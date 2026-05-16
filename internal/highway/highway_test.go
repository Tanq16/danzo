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
