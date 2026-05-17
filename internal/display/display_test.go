package display

import (
	"errors"
	"testing"

	"github.com/tanq16/danzo/internal/highway"
)

func TestDisplayUpdateMovesJobThroughRunningAndCompletedStates(t *testing.T) {
	d := New(DefaultConfig())
	d.RegisterJob("job-1")

	if len(d.pending) != 1 || d.pending[0] != "job-1" {
		t.Fatalf("expected registered job to be pending, got %#v", d.pending)
	}

	d.Update(highway.Progress{
		JobID:   "job-1",
		Type:    highway.ProgressTypeProgress,
		Message: "Downloading",
		Current: 50,
		Total:   100,
		Extra:   "1 MB/s",
	})

	job := d.jobs["job-1"]
	if job.Status != StatusRunning {
		t.Fatalf("expected running status, got %v", job.Status)
	}
	if len(d.pending) != 0 || len(d.running) != 1 || d.running[0] != "job-1" {
		t.Fatalf("expected job in running set, pending=%#v running=%#v", d.pending, d.running)
	}
	if job.Message != "Downloading" || job.Current != 50 || job.Total != 100 || job.Extra != "1 MB/s" {
		t.Fatalf("progress fields were not preserved: %#v", job)
	}

	d.Update(highway.Progress{JobID: "job-1", Done: true})

	if job.Status != StatusCompleted || job.Message != "Done" {
		t.Fatalf("expected completed job, got %#v", job)
	}
	if len(d.running) != 0 || len(d.completed) != 1 || d.completed[0] != "job-1" {
		t.Fatalf("expected job in completed set, running=%#v completed=%#v", d.running, d.completed)
	}
}

func TestDisplayUpdateRecordsFailedJobsEvenWhenTheyWereNotRegistered(t *testing.T) {
	d := New(DefaultConfig())
	err := errors.New("network broke")

	d.Update(highway.Progress{JobID: "job-2", Done: true, Error: err})

	job := d.jobs["job-2"]
	if job == nil {
		t.Fatalf("expected failed job to be tracked")
	}
	if job.Status != StatusFailed || job.Message != err.Error() {
		t.Fatalf("expected failed status and message, got %#v", job)
	}
	if len(d.failed) != 1 || d.failed[0] != "job-2" {
		t.Fatalf("expected failed job list to contain job-2, got %#v", d.failed)
	}
}
