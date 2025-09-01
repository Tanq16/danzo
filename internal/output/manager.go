package output

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/tanq16/danzo/internal/utils"
)

type JobStatus string

const (
	StatusPending JobStatus = "pending"
	StatusActive  JobStatus = "active"
	StatusSuccess JobStatus = "success"
	StatusError   JobStatus = "error"
)

type JobOutput struct {
	ID            int
	Name          string
	Status        JobStatus
	Message       string
	StreamLines   []string
	StartTime     time.Time
	CompletedTime time.Duration
}

type Manager struct {
	jobs           map[int]*JobOutput
	mu             sync.RWMutex
	lastLineCount  int
	maxStreamLines int
	doneCh         chan struct{}
	pauseCh        chan bool
	isPaused       bool
	wg             sync.WaitGroup
	jobCounter     int
}

func NewManager() *Manager {
	return &Manager{
		jobs:           make(map[int]*JobOutput),
		maxStreamLines: 10,
		doneCh:         make(chan struct{}),
		pauseCh:        make(chan bool),
		isPaused:       false,
	}
}

func (m *Manager) RegisterFunction(name string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.jobCounter++
	id := m.jobCounter
	m.jobs[id] = &JobOutput{
		ID:          id,
		Name:        name,
		Status:      StatusPending,
		StreamLines: []string{},
		StartTime:   time.Now(),
	}
	return id
}

func (m *Manager) SetMessage(id int, message string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if job, exists := m.jobs[id]; exists {
		job.Message = message
		if job.Status == StatusPending {
			job.Status = StatusActive
		}
	}
}

func (m *Manager) SetStatus(id int, status string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if job, exists := m.jobs[id]; exists {
		switch status {
		case "pending":
			job.Status = StatusPending
		case "success":
			job.Status = StatusSuccess
		case "error":
			job.Status = StatusError
		default:
			job.Status = StatusActive
		}
	}
}

func (m *Manager) Complete(id int, message string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if job, exists := m.jobs[id]; exists {
		job.Status = StatusSuccess
		job.CompletedTime = time.Since(job.StartTime).Round(time.Second)
		job.StreamLines = []string{} // Clear streams on completion
		if message != "" {
			job.Message = message
		} else {
			job.Message = fmt.Sprintf("Completed %s", job.Name)
		}
	}
}

func (m *Manager) ReportError(id int, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if job, exists := m.jobs[id]; exists {
		job.Status = StatusError
		job.Message = fmt.Sprintf("Failed: %v", err)
	}
}

func (m *Manager) AddStreamLine(id int, line string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if job, exists := m.jobs[id]; exists {
		job.StreamLines = append(job.StreamLines, line)
		if len(job.StreamLines) > m.maxStreamLines {
			job.StreamLines = job.StreamLines[len(job.StreamLines)-m.maxStreamLines:]
		}
	}
}

func (m *Manager) AddProgressBarToStream(id int, current, total int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if job, exists := m.jobs[id]; exists {
		progressBar := printProgressBar(current, total, 30)
		elapsed := time.Since(job.StartTime).Seconds()
		speed := utils.FormatSpeed(current, elapsed)
		sizeDisplay := fmt.Sprintf("%s / %s", utils.FormatBytes(uint64(current)), utils.FormatBytes(uint64(total)))
		display := fmt.Sprintf("%s%s %s %s", progressBar, debugStyle.Render(sizeDisplay), StyleSymbols["bullet"], debugStyle.Render(speed))
		job.StreamLines = []string{display}
	}
}

func (m *Manager) Pause() {
	m.pauseCh <- true
}

func (m *Manager) Resume() {
	m.pauseCh <- false
}

func (m *Manager) StartDisplay() {
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		timePerUpdate := 300 * time.Millisecond
		if utils.GlobalDebugFlag {
			timePerUpdate = 3 * time.Second // slow refresh for debug mode
		}
		ticker := time.NewTicker(timePerUpdate)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if !m.isPaused {
					m.updateDisplay()
				}
			case m.isPaused = <-m.pauseCh:
			case <-m.doneCh:
				m.updateDisplay()
				m.showSummary()
				return
			}
		}
	}()
}

func (m *Manager) StopDisplay() {
	close(m.doneCh)
	m.wg.Wait()
}

func (m *Manager) updateDisplay() {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.lastLineCount > 0 && !utils.GlobalDebugFlag {
		fmt.Printf("\033[%dA\033[J", m.lastLineCount)
	}

	var lines []string
	allJobs := make([]*JobOutput, 0, len(m.jobs))
	for _, job := range m.jobs {
		allJobs = append(allJobs, job)
	}
	sort.Slice(allJobs, func(i, j int) bool {
		return allJobs[i].ID < allJobs[j].ID
	})

	// Group jobs by status
	var active, pending, completed []*JobOutput
	for _, job := range allJobs {
		switch job.Status {
		case StatusPending:
			pending = append(pending, job)
		case StatusSuccess, StatusError:
			completed = append(completed, job)
		default:
			active = append(active, job)
		}
	}

	// Display active jobs
	for _, job := range active {
		elapsed := time.Since(job.StartTime).Round(time.Second)
		lines = append(lines, fmt.Sprintf("  %s %s %s", getStatusIcon(job.Status), debugStyle.Render(elapsed.String()), pendingStyle.Render(job.Message)))
		for _, stream := range job.StreamLines {
			lines = append(lines, fmt.Sprintf("      %s", streamStyle.Render(stream)))
		}
	}

	// Display pending jobs
	for _, job := range pending {
		lines = append(lines, fmt.Sprintf("  %s %s", getStatusIcon(job.Status), pendingStyle.Render("Waiting...")))
	}

	// Display completed jobs (limit to last 8 if too many)
	if len(completed) > 10 {
		lines = append(lines, infoStyle.Render(fmt.Sprintf("  %d jobs completed (showing last 8)...", len(completed))))
		completed = completed[len(completed)-8:]
	}
	for _, job := range completed {
		totalTime := job.CompletedTime
		if job.CompletedTime == 0 {
			totalTime = time.Since(job.StartTime).Round(time.Second)
			job.CompletedTime = totalTime
		}
		style := successStyle
		if job.Status == StatusError {
			style = errorStyle
		}
		lines = append(lines, fmt.Sprintf("  %s %s %s", getStatusIcon(job.Status), debugStyle.Render(totalTime.String()), style.Render(job.Message)))
	}

	// Print all lines
	if len(lines) > 0 {
		fmt.Println(strings.Join(lines, "\n"))
	}
	m.lastLineCount = len(lines)
}

func (m *Manager) showSummary() {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var success, errors int
	for _, job := range m.jobs {
		switch job.Status {
		case StatusSuccess:
			success++
		case StatusError:
			errors++
		}
	}
	fmt.Println()
	fmt.Println("  " + successStyle.Render(fmt.Sprintf("Completed %d of %d", success, len(m.jobs))))
	if errors > 0 {
		fmt.Println("  " + errorStyle.Render(fmt.Sprintf("Failed %d of %d", errors, len(m.jobs))))
	}
	fmt.Println()
}

func getStatusIcon(status JobStatus) string {
	switch status {
	case StatusSuccess:
		return successStyle.Render(StyleSymbols["pass"])
	case StatusError:
		return errorStyle.Render(StyleSymbols["fail"])
	case StatusPending:
		return pendingStyle.Render(StyleSymbols["pending"])
	default:
		return pendingStyle.Render(StyleSymbols["pending"])
	}
}

func printProgressBar(current, total int64, width int) string {
	if width <= 0 {
		width = 30
	}
	if total <= 0 {
		total = 1
	}
	if current < 0 {
		current = 0
	}
	if current > total {
		current = total
	}
	percent := float64(current) / float64(total)
	filled := max(0, min(int(percent*float64(width)), width))
	bar := StyleSymbols["bullet"]
	bar += strings.Repeat(StyleSymbols["hline"], filled)
	if filled < width {
		bar += strings.Repeat(" ", width-filled)
	}
	bar += StyleSymbols["bullet"]
	return debugStyle.Render(fmt.Sprintf("%s %.1f%% %s ", bar, percent*100, StyleSymbols["bullet"]))
}
