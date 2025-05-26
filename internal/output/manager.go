package output

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/tanq16/danzo/internal/utils"
)

type FunctionOutput struct {
	ID          int
	URL         string
	Status      string
	Message     string
	StreamLines []string
	Complete    bool
	StartTime   time.Time
	LastUpdated time.Time
	Error       error
	Index       int
}

type ErrorReport struct {
	FunctionName string
	Error        error
	Time         time.Time
}

type Manager struct {
	outputs       map[string]*FunctionOutput
	mutex         sync.RWMutex
	lastLineCount int
	maxStreams    int // Max output stream lines per function
	errors        []ErrorReport
	doneCh        chan struct{} // Channel to signal stopping the display
	pauseCh       chan bool     // Channel to pause/resume display updates
	isPaused      bool
	displayTick   time.Duration // Interval between display updates
	functionCount int
	displayWg     sync.WaitGroup // WaitGroup for display goroutine shutdown
}

func NewManager() *Manager {
	return &Manager{
		outputs:       make(map[string]*FunctionOutput),
		errors:        []ErrorReport{},
		maxStreams:    10,
		doneCh:        make(chan struct{}),
		pauseCh:       make(chan bool),
		isPaused:      false,
		displayTick:   300 * time.Millisecond,
		functionCount: 0,
	}
}

func (m *Manager) Pause() {
	if !m.isPaused {
		m.pauseCh <- true
		m.isPaused = true
	}
}

func (m *Manager) Resume() {
	if m.isPaused {
		m.pauseCh <- false
		m.isPaused = false
	}
}

func (m *Manager) RegisterFunction(url string) int {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.functionCount++
	m.outputs[fmt.Sprint(m.functionCount)] = &FunctionOutput{
		ID:          m.functionCount,
		URL:         url,
		Status:      "pending",
		StreamLines: []string{},
		StartTime:   time.Now(),
		LastUpdated: time.Now(),
		Index:       m.functionCount,
	}
	return m.functionCount
}

func (m *Manager) SetMessage(id int, message string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if info, exists := m.outputs[fmt.Sprint(id)]; exists {
		info.Message = message
		info.LastUpdated = time.Now()
	}
}

func (m *Manager) SetStatus(id int, status string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if info, exists := m.outputs[fmt.Sprint(id)]; exists {
		info.Status = status
		info.LastUpdated = time.Now()
	}
}

func (m *Manager) GetStatus(id int) string {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	if info, exists := m.outputs[fmt.Sprint(id)]; exists {
		return info.Status
	}
	return "unknown"
}

func (m *Manager) Complete(id int, message string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if info, exists := m.outputs[fmt.Sprint(id)]; exists {
		info.StreamLines = []string{}
		if message == "" {
			info.Message = fmt.Sprintf("Completed %s", info.URL)
		} else {
			info.Message = message
		}
		info.Complete = true
		info.Status = "success"
		info.LastUpdated = time.Now()
	}
}

func (m *Manager) ReportError(id int, err error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if info, exists := m.outputs[fmt.Sprint(id)]; exists {
		info.Complete = true
		info.Status = "error"
		info.Error = err
		info.LastUpdated = time.Now()
		m.errors = append(m.errors, ErrorReport{
			FunctionName: info.URL,
			Error:        err,
			Time:         time.Now(),
		})
	}
}

func (m *Manager) AddStreamLine(id int, line string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if info, exists := m.outputs[fmt.Sprint(id)]; exists {
		info.StreamLines = append(info.StreamLines, line)
		if len(info.StreamLines) > m.maxStreams {
			info.StreamLines = info.StreamLines[len(info.StreamLines)-m.maxStreams:]
		}
		info.LastUpdated = time.Now()
	}
}

func (m *Manager) AddProgressBarToStream(id int, outof, final int64, text string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if info, exists := m.outputs[fmt.Sprint(id)]; exists {
		progressBar := PrintProgressBar(max(0, outof), final, 30)
		elapsed := time.Since(info.StartTime).Round(time.Second).Seconds()
		display := fmt.Sprintf("%s%s %s %s", progressBar, debugStyle.Render(text), StyleSymbols["bullet"], debugStyle.Render(utils.FormatSpeed(outof, elapsed)))
		info.StreamLines = []string{display}
		info.LastUpdated = time.Now()
	}
}

// func (m *Manager) ClearAll() {
// 	m.mutex.Lock()
// 	defer m.mutex.Unlock()
// 	for id := range m.outputs {
// 		m.outputs[id].StreamLines = []string{}
// 	}
// }

func (m *Manager) GetStatusIndicator(status string) string {
	switch status {
	case "success", "pass":
		return successStyle.Render(StyleSymbols["pass"])
	case "error", "fail":
		return errorStyle.Render(StyleSymbols["fail"])
	case "warning":
		return warningStyle.Render(StyleSymbols["warning"])
	case "pending":
		return pendingStyle.Render(StyleSymbols["pending"])
	default:
		return infoStyle.Render(StyleSymbols["bullet"])
	}
}

func (m *Manager) sortFunctions() (active, pending, completed []*FunctionOutput) {
	var allFuncs []*FunctionOutput
	for _, info := range m.outputs {
		allFuncs = append(allFuncs, info)
	}
	sort.Slice(allFuncs, func(i, j int) bool {
		return allFuncs[i].Index < allFuncs[j].Index
	})

	for _, f := range allFuncs {
		if f.Complete {
			completed = append(completed, f)
		} else if f.Status == "pending" && f.Message == "" {
			pending = append(pending, f)
		} else {
			active = append(active, f)
		}
	}
	return active, pending, completed
}

func (m *Manager) updateDisplay() {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	// Clear previous display
	if m.lastLineCount > 0 {
		fmt.Printf("\033[%dA\033[J", m.lastLineCount)
	}

	// Build display content
	var lines []string
	termHeight := getTerminalHeight() - 2
	termWidth := getTerminalWidth() - 2

	activeFuncs, pendingFuncs, completedFuncs := m.sortFunctions()

	// Add active functions
	for _, f := range activeFuncs {
		statusDisplay := m.GetStatusIndicator(f.Status)
		elapsed := time.Since(f.StartTime).Round(time.Second).String()

		var styledMessage string
		switch f.Status {
		case "success":
			styledMessage = successStyle.Render(f.Message)
		case "error":
			styledMessage = errorStyle.Render(f.Message)
		case "warning":
			styledMessage = warningStyle.Render(f.Message)
		default:
			styledMessage = pendingStyle.Render(f.Message)
		}

		lines = append(lines, fmt.Sprintf("  %s %s %s", statusDisplay, debugStyle.Render(elapsed), styledMessage))

		// Add stream lines
		for _, streamLine := range f.StreamLines {
			lines = append(lines, fmt.Sprintf("      %s", streamStyle.Render(streamLine)))
		}
	}

	// Add pending functions
	for _, f := range pendingFuncs {
		statusDisplay := m.GetStatusIndicator(f.Status)
		lines = append(lines, fmt.Sprintf("  %s %s", statusDisplay, pendingStyle.Render("Waiting...")))
	}

	// Add completed functions summary if many
	if len(completedFuncs) > 10 {
		lines = append(lines, infoStyle.Render(fmt.Sprintf("  %d links completed with varying hidden status ...", len(completedFuncs)-8)))
		completedFuncs = completedFuncs[len(completedFuncs)-8:]
	}

	// Add completed functions
	for _, f := range completedFuncs {
		statusDisplay := m.GetStatusIndicator(f.Status)
		totalTime := f.LastUpdated.Sub(f.StartTime).Round(time.Second).String()

		var styledMessage string
		switch f.Status {
		case "success":
			styledMessage = successStyle.Render(f.Message)
		case "error":
			styledMessage = errorStyle.Render(f.Message)
		case "warning":
			styledMessage = warningStyle.Render(f.Message)
		default:
			styledMessage = pendingStyle.Render(f.Message)
		}

		lines = append(lines, fmt.Sprintf("  %s %s %s", statusDisplay, debugStyle.Render(totalTime), styledMessage))
	}

	// Calculate total lines needed (accounting for line wrapping)
	totalLines := 0
	for _, line := range lines {
		visualLen := len(line)
		if visualLen > termWidth {
			totalLines += (visualLen + termWidth - 1) / termWidth
		} else {
			totalLines++
		}
	}

	// Limit to terminal height
	if len(lines) > termHeight {
		lines = lines[:termHeight]
		totalLines = len(lines)
	}

	// Print all at once
	if len(lines) > 0 {
		fmt.Print(strings.Join(lines, "\n"))
		fmt.Println()
	}

	m.lastLineCount = totalLines
}

func (m *Manager) StartDisplay() {
	m.displayWg.Add(1)
	go func() {
		defer m.displayWg.Done()
		ticker := time.NewTicker(m.displayTick)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if !m.isPaused {
					m.updateDisplay()
				}
			case pauseState := <-m.pauseCh:
				m.isPaused = pauseState
			case <-m.doneCh:
				m.updateDisplay()
				m.ShowSummary()
				return
			}
		}
	}()
}

func (m *Manager) StopDisplay() {
	close(m.doneCh)
	m.displayWg.Wait()
}

func (m *Manager) displayErrors() {
	if len(m.errors) == 0 {
		return
	}
	fmt.Println()
	fmt.Println("  " + errorStyle.Bold(true).Render("Errors:"))
	for i, err := range m.errors {
		fmt.Printf("    %s %s %s\n",
			errorStyle.Render(fmt.Sprintf("%d.", i+1)),
			debugStyle.Render(fmt.Sprintf("[%s]", err.Time.Format("15:04:05"))),
			errorStyle.Render(fmt.Sprintf("Function: %s", err.FunctionName)))
		fmt.Printf("      %s\n", errorStyle.Render(fmt.Sprintf("Error: %v", err.Error)))
	}
}

func (m *Manager) ShowSummary() {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	fmt.Println()
	var success, failures int
	for _, info := range m.outputs {
		if info.Status == "success" {
			success++
		} else if info.Status == "error" {
			failures++
		}
	}
	fmt.Println("  " + success2Style.Render(fmt.Sprintf("Completed %d of %d", success, len(m.outputs))))
	if failures > 0 {
		fmt.Println("  " + errorStyle.Render(fmt.Sprintf("Failed %d of %d", failures, len(m.outputs))))
	}
	m.displayErrors()
	fmt.Println()
}
