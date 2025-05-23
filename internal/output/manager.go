package output

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/term"
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
	numLines      int
	maxStreams    int // Max output stream lines per function
	errors        []ErrorReport
	doneCh        chan struct{} // Channel to signal stopping the display
	pauseCh       chan bool     // Channel to pause/resume display updates
	isPaused      bool
	displayTick   time.Duration // Interval between display updates
	functionCount int
	displayWg     sync.WaitGroup // WaitGroup for display goroutine shutdown
	enableLogging bool
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
		enableLogging: false,
	}
}

func (m *Manager) EnableLogging() {
	m.enableLogging = true
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
		// Add to global error list
		m.errors = append(m.errors, ErrorReport{
			FunctionName: info.URL,
			Error:        err,
			Time:         time.Now(),
		})
	}
}

func (m *Manager) UpdateStreamOutput(id int, output []string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if info, exists := m.outputs[fmt.Sprint(id)]; exists {
		info.StreamLines = append(info.StreamLines, output...)
		if len(info.StreamLines) > m.maxStreams {
			info.StreamLines = info.StreamLines[len(info.StreamLines)-m.maxStreams:]
		}
		info.LastUpdated = time.Now()
	}
}

func (m *Manager) AddStreamLine(id int, line string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if info, exists := m.outputs[fmt.Sprint(id)]; exists {
		// Wrap the line with indentation
		wrappedLines := wrapText(line, 2+4)
		info.StreamLines = append(info.StreamLines, wrappedLines...)
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
		display := fmt.Sprintf("%s%s %s %s", progressBar, debugStyle.Render(text), StyleSymbols["bullet"], debugStyle.Render(FormatSpeed(outof, elapsed)))
		info.StreamLines = []string{display} // Set as only stream so nothing else is displayed
		info.LastUpdated = time.Now()
	}
}

func (m *Manager) ClearLines(n int) {
	if n <= 0 {
		return
	}
	fmt.Printf("\033[%dA\033[J", min(m.numLines, n))
	m.numLines = max(m.numLines-n, 0)
}

func (m *Manager) ClearFunction(id int) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if info, exists := m.outputs[fmt.Sprint(id)]; exists {
		info.StreamLines = []string{}
		info.Message = ""
	}
}

func (m *Manager) ClearAll() {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	for id := range m.outputs {
		m.outputs[id].StreamLines = []string{}
	}
}

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
	// Sort by index (registration order)
	for _, info := range m.outputs {
		allFuncs = append(allFuncs, info)
	}
	sort.Slice(allFuncs, func(i, j int) bool {
		return allFuncs[i].Index < allFuncs[j].Index
	})
	// Group functions by status
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

	// Get terminal height to limit output
	_, termHeight, _ := term.GetSize(int(os.Stdout.Fd()))
	if termHeight <= 0 {
		termHeight = 24 // Default fallback
	}
	availableLines := termHeight - 3 // Leave some buffer for prompt

	if m.numLines > 0 {
		fmt.Printf("\033[%dA\033[J", m.numLines)
	}

	lineCount := 0
	activeFuncs, pendingFuncs, completedFuncs := m.sortFunctions()

	// Calculate how many lines we need
	totalNeeded := 0
	for _, f := range activeFuncs {
		totalNeeded += 1 + len(f.StreamLines)
	}
	for _, f := range pendingFuncs {
		totalNeeded += 1 + len(f.StreamLines)
	}
	totalNeeded += len(completedFuncs)

	// If we need more than available, trim completed functions
	if totalNeeded > availableLines {
		maxCompleted := availableLines - (totalNeeded - len(completedFuncs))
		if maxCompleted < 0 {
			maxCompleted = 0
		}
		if len(completedFuncs) > maxCompleted {
			completedFuncs = completedFuncs[len(completedFuncs)-maxCompleted:]
		}
	}

	// Display active functions
	for _, f := range activeFuncs {
		if lineCount >= availableLines {
			break
		}
		info := f
		statusDisplay := m.GetStatusIndicator(info.Status)
		elapsed := time.Since(info.StartTime).Round(time.Second)
		if info.Complete {
			elapsed = info.LastUpdated.Sub(info.StartTime).Round(time.Second)
		}
		elapsedStr := elapsed.String()

		// Style the message based on status
		var styledMessage string
		switch info.Status {
		case "success":
			styledMessage = successStyle.Render(info.Message)
		case "error":
			styledMessage = errorStyle.Render(info.Message)
		case "warning":
			styledMessage = warningStyle.Render(info.Message)
		default: // pending or other
			styledMessage = pendingStyle.Render(info.Message)
		}
		fmt.Printf("%s%s %s %s\n", strings.Repeat(" ", 2), statusDisplay, debugStyle.Render(elapsedStr), styledMessage)
		lineCount++

		// Print stream lines with indentation
		if len(info.StreamLines) > 0 && lineCount < availableLines {
			indent := strings.Repeat(" ", 2+4) // Additional indentation for stream output
			for _, line := range info.StreamLines {
				if lineCount >= availableLines {
					break
				}
				fmt.Printf("%s%s\n", indent, streamStyle.Render(line))
				lineCount++
			}
		}
	}

	// Display pending functions
	for _, f := range pendingFuncs {
		if lineCount >= availableLines {
			break
		}
		info := f
		statusDisplay := m.GetStatusIndicator(info.Status)
		fmt.Printf("%s%s %s\n", strings.Repeat(" ", 2), statusDisplay, pendingStyle.Render("Waiting..."))
		lineCount++
		if len(info.StreamLines) > 0 && lineCount < availableLines {
			indent := strings.Repeat(" ", 2+4)
			for _, line := range info.StreamLines {
				if lineCount >= availableLines {
					break
				}
				fmt.Printf("%s%s\n", indent, streamStyle.Render(line))
				lineCount++
			}
		}
	}

	// Display completed functions summary if many
	if len(completedFuncs) > 10 && lineCount < availableLines {
		PrintInfo(fmt.Sprintf("%s%d links completed with varying hidden status ...", strings.Repeat(" ", 2), len(completedFuncs)-8))
		completedFuncs = completedFuncs[len(completedFuncs)-8:]
		lineCount++
	}

	// Display completed functions
	for _, f := range completedFuncs {
		if lineCount >= availableLines {
			break
		}
		info := f
		statusDisplay := m.GetStatusIndicator(info.Status)
		totalTime := info.LastUpdated.Sub(info.StartTime).Round(time.Second)
		timeStr := totalTime.String()

		// Style message based on status
		var styledMessage string
		if info.Status == "success" {
			styledMessage = successStyle.Render(info.Message)
		} else if info.Status == "error" {
			styledMessage = errorStyle.Render(info.Message)
		} else if info.Status == "warning" {
			styledMessage = warningStyle.Render(info.Message)
		} else { // pending or other
			styledMessage = pendingStyle.Render(info.Message)
		}
		fmt.Printf("%s%s %s %s\n", strings.Repeat(" ", 2), statusDisplay, debugStyle.Render(timeStr), styledMessage)
		lineCount++

		// Print stream lines with indentation
		if len(info.StreamLines) > 0 && lineCount < availableLines {
			indent := strings.Repeat(" ", 2+4)
			for _, line := range info.StreamLines {
				if lineCount >= availableLines {
					break
				}
				fmt.Printf("%s%s\n", indent, streamStyle.Render(line))
				lineCount++
			}
		}
	}
	m.numLines = lineCount
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
				m.ClearAll()
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
	fmt.Println(strings.Repeat(" ", 2) + errorStyle.Bold(true).Render("Errors:"))
	for i, err := range m.errors {
		fmt.Printf("%s%s %s %s\n",
			strings.Repeat(" ", 2+2),
			errorStyle.Render(fmt.Sprintf("%d.", i+1)),
			debugStyle.Render(fmt.Sprintf("[%s]", err.Time.Format("15:04:05"))),
			errorStyle.Render(fmt.Sprintf("Function: %s", err.FunctionName)))
		fmt.Printf("%s%s\n", strings.Repeat(" ", 2+4), errorStyle.Render(fmt.Sprintf("Error: %v", err.Error)))
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
	succeeded := fmt.Sprintf("Completed %d of %d", success, len(m.outputs))
	failed := fmt.Sprintf("Failed %d of %d", failures, len(m.outputs))
	fmt.Println(strings.Repeat(" ", 2) + success2Style.Render(succeeded))
	if failures > 0 {
		fmt.Println(strings.Repeat(" ", 2) + errorStyle.Render(failed))
	}
	m.displayErrors()
	fmt.Println()
}
