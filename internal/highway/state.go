package highway

import (
	"encoding/json"
	"fmt"
	"os"
)

type persistedState struct {
	Completed []string       `json:"completed"`
	Pending   []persistedJob `json:"pending"`
}

type persistedJob struct {
	ID   string          `json:"id"`
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

func (h *Highway) LoadState() error {
	data, err := os.ReadFile(h.statePath)
	if err != nil {
		return err
	}

	var state persistedState
	if err := json.Unmarshal(data, &state); err != nil {
		return err
	}

	h.mu.Lock()
	for _, id := range state.Completed {
		h.completed[id] = true
	}
	h.mu.Unlock()

	for _, pj := range state.Pending {
		unmarshal, ok := h.unmarshalers[pj.Type]
		if !ok {
			return fmt.Errorf("unknown job type: %s (register with RegisterType)", pj.Type)
		}

		job, err := unmarshal(pj.Data)
		if err != nil {
			return fmt.Errorf("failed to unmarshal job %s: %w", pj.ID, err)
		}

		h.Submit(job)
	}

	return nil
}

func (h *Highway) saveState() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	var completedIDs []string
	for id := range h.completed {
		completedIDs = append(completedIDs, id)
	}

	var pendingJobs []persistedJob
	for _, job := range h.pending {
		if h.completed[job.ID()] {
			continue
		}

		data, err := job.Marshal()
		if err != nil {
			continue
		}

		pendingJobs = append(pendingJobs, persistedJob{
			ID:   job.ID(),
			Type: job.Type(),
			Data: data,
		})
	}

	state := persistedState{
		Completed: completedIDs,
		Pending:   pendingJobs,
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}

	if err := os.WriteFile(h.statePath, data, 0644); err != nil {
		return err
	}

	fmt.Printf("\nState saved to %s\n", h.statePath)
	return nil
}

func (h *Highway) deleteState() {
	os.Remove(h.statePath)
}
