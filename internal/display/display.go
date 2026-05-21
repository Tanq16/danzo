package display

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/tanq16/danzo/internal/highway"
	"github.com/tanq16/danzo/utils"
)

var (
	colorBlue     = lipgloss.ANSIColor(12)
	colorGreen    = lipgloss.ANSIColor(10)
	colorRed      = lipgloss.ANSIColor(9)
	colorMauve    = lipgloss.ANSIColor(13)
	colorSapphire = lipgloss.ANSIColor(14)
	colorText     = lipgloss.ANSIColor(15)
	colorSubtext0 = lipgloss.ANSIColor(7)
	colorSubtext1 = lipgloss.ANSIColor(7)
	colorOverlay0 = lipgloss.ANSIColor(8)
	colorOverlay1 = lipgloss.ANSIColor(8)
	colorSurface1 = lipgloss.ANSIColor(8)
)

type JobStatus int

const (
	StatusPending JobStatus = iota
	StatusRunning
	StatusCompleted
	StatusFailed
)

type JobState struct {
	ID         string
	Status     JobStatus
	UpdateType highway.ProgressType
	Message    string
	SubStatus  string
	Current    int64
	Total      int64
	Extra      string
}

type Config struct {
	MaxVisibleJobs int
	RefreshRate    time.Duration
	BoxWidth       int
}

func DefaultConfig() Config {
	return Config{
		MaxVisibleJobs: 5,
		RefreshRate:    200 * time.Millisecond,
		BoxWidth:       72,
	}
}

type Display struct {
	config Config

	mu        sync.RWMutex
	jobs      map[string]*JobState
	order     []string
	running   []string
	pending   []string
	completed []string
	failed    []string

	lastLineCount int
	paused        bool
	stopCh        chan struct{}
	doneCh        chan struct{}
}

func New(config Config) *Display {
	return &Display{
		config: config,
		jobs:   make(map[string]*JobState),
		stopCh: make(chan struct{}),
		doneCh: make(chan struct{}),
	}
}

func (d *Display) RegisterJob(id string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if _, exists := d.jobs[id]; !exists {
		d.jobs[id] = &JobState{
			ID:     id,
			Status: StatusPending,
		}
		d.order = append(d.order, id)
		d.pending = append(d.pending, id)
	}
}

func (d *Display) Update(update highway.Progress) {
	d.mu.Lock()
	defer d.mu.Unlock()

	job, exists := d.jobs[update.JobID]
	if !exists {
		job = &JobState{ID: update.JobID}
		d.jobs[update.JobID] = job
		d.order = append(d.order, update.JobID)
	}

	if update.Done {
		if update.Error != nil {
			job.Status = StatusFailed
			job.Message = update.Error.Error()
			d.removeFromSlice(&d.running, update.JobID)
			d.removeFromSlice(&d.pending, update.JobID)
			d.failed = append(d.failed, update.JobID)
		} else {
			job.Status = StatusCompleted
			job.Message = "Done"
			d.removeFromSlice(&d.running, update.JobID)
			d.removeFromSlice(&d.pending, update.JobID)
			d.completed = append(d.completed, update.JobID)
		}
	} else {
		if job.Status == StatusPending {
			d.removeFromSlice(&d.pending, update.JobID)
			d.running = append(d.running, update.JobID)
		}
		job.Status = StatusRunning
		job.UpdateType = update.Type
		job.Message = update.Message
		job.SubStatus = update.SubStatus
		job.Current = update.Current
		job.Total = update.Total
		job.Extra = update.Extra
	}
}

func (d *Display) removeFromSlice(slice *[]string, id string) {
	for i, v := range *slice {
		if v == id {
			*slice = append((*slice)[:i], (*slice)[i+1:]...)
			return
		}
	}
}

func (d *Display) Start(updates <-chan highway.Progress) {
	if utils.GlobalForAIFlag {
		d.startAI(updates)
		return
	}

	go func() {
		for update := range updates {
			d.Update(update)
		}
	}()

	go func() {
		defer close(d.doneCh)
		ticker := time.NewTicker(d.config.RefreshRate)
		defer ticker.Stop()

		for {
			select {
			case <-d.stopCh:
				d.clearDisplay()
				d.renderFinal()
				return
			case <-ticker.C:
				d.render()
			}
		}
	}()
}

func (d *Display) Stop() {
	if utils.GlobalForAIFlag {
		<-d.doneCh
		d.renderFinal()
		return
	}
	close(d.stopCh)
	<-d.doneCh
}

func (d *Display) startAI(updates <-chan highway.Progress) {
	go func() {
		defer close(d.doneCh)
		for update := range updates {
			d.Update(update)
			if update.Done {
				if update.Error != nil {
					fmt.Printf("[ERROR] %s: %s\n", update.JobID, update.ErrMsg)
				} else {
					fmt.Printf("[OK] %s: Done\n", update.JobID)
				}
				continue
			}
			if update.Type == highway.ProgressTypeProgress && update.Total > 0 {
				percent := int(float64(update.Current) / float64(update.Total) * 100)
				if update.Extra != "" {
					fmt.Printf("[INFO] %s: %s %d%% %s\n", update.JobID, update.Message, percent, update.Extra)
				} else {
					fmt.Printf("[INFO] %s: %s %d%%\n", update.JobID, update.Message, percent)
				}
			} else if update.SubStatus != "" {
				fmt.Printf("[INFO] %s: %s - %s\n", update.JobID, update.Message, update.SubStatus)
			} else if update.Message != "" {
				fmt.Printf("[INFO] %s: %s\n", update.JobID, update.Message)
			}
		}
	}()
}

func (d *Display) Pause() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.clearDisplay()
	d.lastLineCount = 0
	d.paused = true
}

func (d *Display) Resume() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.paused = false
}

func (d *Display) clearDisplay() {
	if d.lastLineCount > 0 {
		for range d.lastLineCount {
			fmt.Print("\033[A")
			fmt.Print("\033[K")
		}
	}
}

func (d *Display) render() {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if d.paused {
		return
	}

	d.clearDisplay()

	lines := d.buildDisplay()
	output := strings.Join(lines, "\n")
	fmt.Println(output)

	d.lastLineCount = len(lines)
}

func (d *Display) renderFinal() {
	d.mu.RLock()
	defer d.mu.RUnlock()

	total := len(d.jobs)
	completedCount := len(d.completed)
	failedCount := len(d.failed)

	if utils.GlobalForAIFlag {
		if failedCount == 0 {
			fmt.Printf("[OK] All %d jobs completed successfully\n", total)
		} else {
			fmt.Printf("[OK] %d completed\n", completedCount)
			fmt.Printf("[ERROR] %d failed\n", failedCount)
		}
		return
	}

	successStyle := lipgloss.NewStyle().Foreground(colorGreen).Bold(true)
	failStyle := lipgloss.NewStyle().Foreground(colorRed).Bold(true)
	dimStyle := lipgloss.NewStyle().Foreground(colorSubtext0)

	if failedCount == 0 {
		fmt.Println(successStyle.Render(fmt.Sprintf("✓ All %d jobs completed successfully", total)))
	} else {
		fmt.Println(successStyle.Render(fmt.Sprintf("✓ %d completed", completedCount)) +
			dimStyle.Render("  ") +
			failStyle.Render(fmt.Sprintf("✗ %d failed", failedCount)))
	}
}

func (d *Display) buildDisplay() []string {
	var lines []string

	total := len(d.jobs)
	boxWidth := d.config.BoxWidth
	innerWidth := boxWidth - 4

	borderStyle := lipgloss.NewStyle().Foreground(colorOverlay0)
	titleStyle := lipgloss.NewStyle().Foreground(colorBlue).Bold(true)
	runningStyle := lipgloss.NewStyle().Foreground(colorGreen)
	pendingStyle := lipgloss.NewStyle().Foreground(colorOverlay1)
	jobIDStyle := lipgloss.NewStyle().Foreground(colorText)
	messageStyle := lipgloss.NewStyle().Foreground(colorSubtext0)
	substatusStyle := lipgloss.NewStyle().Foreground(colorMauve)
	progressFillStyle := lipgloss.NewStyle().Foreground(colorBlue)
	progressEmptyStyle := lipgloss.NewStyle().Foreground(colorSurface1)
	percentStyle := lipgloss.NewStyle().Foreground(colorSapphire)
	extraStyle := lipgloss.NewStyle().Foreground(colorSubtext1)

	title := fmt.Sprintf(" Processing %d jobs ", total)
	leftPad := 2
	rightPad := boxWidth - leftPad - len(title) - 2
	if rightPad < 0 {
		rightPad = 0
	}
	topBorder := borderStyle.Render("┌"+strings.Repeat("─", leftPad)) +
		titleStyle.Render(title) +
		borderStyle.Render(strings.Repeat("─", rightPad)+"┐")
	lines = append(lines, topBorder)

	lines = append(lines, d.emptyLine(boxWidth, borderStyle))

	visibleCount := 0
	for _, jobID := range d.running {
		if visibleCount >= d.config.MaxVisibleJobs {
			break
		}

		job := d.jobs[jobID]
		visibleCount++

		msgPart := ""
		if job.Message != "" {
			msgPart = " [" + job.Message + "]"
		}
		maxIDLen := innerWidth - 4 - len(msgPart)
		if maxIDLen < 10 {
			maxIDLen = 10
			msgPart = truncateString(msgPart, innerWidth-4-maxIDLen)
		}
		truncatedID := truncateString(job.ID, maxIDLen)

		jobLine := "  " + runningStyle.Render("●") + " " +
			jobIDStyle.Render(truncatedID) +
			messageStyle.Render(msgPart)
		lines = append(lines, d.padLine(jobLine, boxWidth, borderStyle))

		if job.UpdateType == highway.ProgressTypeProgress && job.Total > 0 {
			extraInfo := ""
			if job.Extra != "" {
				extraInfo = "  " + job.Extra
			}
			extraLen := len(extraInfo)

			progressWidth := innerWidth - 4 - 1 - 1 - 6 - extraLen
			if progressWidth < 10 {
				progressWidth = 10
				extraInfo = truncateString(extraInfo, innerWidth-4-1-1-6-progressWidth)
			}

			var pct float64
			if job.Total > 0 {
				pct = float64(job.Current) / float64(job.Total)
			}
			if pct > 1.0 {
				pct = 1.0
			} else if pct < 0 {
				pct = 0
			}
			filled := int(pct * float64(progressWidth))
			empty := progressWidth - filled

			progressLine := "    " +
				progressFillStyle.Render("●"+strings.Repeat("━", filled)) +
				progressEmptyStyle.Render(strings.Repeat(" ", empty)) +
				progressFillStyle.Render("●") +
				" " + percentStyle.Render(fmt.Sprintf("%3d%%", int(pct*100))) +
				extraStyle.Render(extraInfo)
			lines = append(lines, d.padLine(progressLine, boxWidth, borderStyle))
		} else if job.UpdateType == highway.ProgressTypeSubStatus {
			subText := job.SubStatus
			if subText == "" {
				subText = job.Message
			}
			subLine := "    " + substatusStyle.Render("└─ "+truncateString(subText, innerWidth-10))
			lines = append(lines, d.padLine(subLine, boxWidth, borderStyle))
		}

		lines = append(lines, d.emptyLine(boxWidth, borderStyle))
	}

	pendingToShow := d.config.MaxVisibleJobs - visibleCount
	if pendingToShow > 3 {
		pendingToShow = 3
	}
	for i := 0; i < pendingToShow && i < len(d.pending); i++ {
		jobID := d.pending[i]
		truncatedID := truncateString(jobID, innerWidth-14)
		pendingLine := "  " + pendingStyle.Render("○ "+truncatedID+" (queued)")
		lines = append(lines, d.padLine(pendingLine, boxWidth, borderStyle))
	}

	remainingPending := len(d.pending) - pendingToShow
	if remainingPending > 0 {
		moreLine := "    " + pendingStyle.Render(fmt.Sprintf("... and %d more queued", remainingPending))
		lines = append(lines, d.padLine(moreLine, boxWidth, borderStyle))
	}

	lines = append(lines, d.emptyLine(boxWidth, borderStyle))

	sepLine := borderStyle.Render("│  " + strings.Repeat("─", innerWidth-2) + "  │")
	lines = append(lines, sepLine)

	runCount := lipgloss.NewStyle().Foreground(colorGreen).Render(fmt.Sprintf("● %d running", len(d.running)))
	pendCount := lipgloss.NewStyle().Foreground(colorOverlay1).Render(fmt.Sprintf("○ %d pending", len(d.pending)))
	compCount := lipgloss.NewStyle().Foreground(colorGreen).Render(fmt.Sprintf("✓ %d completed", len(d.completed)))
	failCount := lipgloss.NewStyle().Foreground(colorRed).Render(fmt.Sprintf("✗ %d failed", len(d.failed)))
	summary := "  " + runCount + "  " + pendCount + "  " + compCount + "  " + failCount
	lines = append(lines, d.padLine(summary, boxWidth, borderStyle))

	bottomBorder := borderStyle.Render("└" + strings.Repeat("─", boxWidth-2) + "┘")
	lines = append(lines, bottomBorder)

	return lines
}

func (d *Display) emptyLine(boxWidth int, borderStyle lipgloss.Style) string {
	return borderStyle.Render("│") + strings.Repeat(" ", boxWidth-2) + borderStyle.Render("│")
}

func (d *Display) padLine(content string, boxWidth int, borderStyle lipgloss.Style) string {
	visibleLen := lipgloss.Width(content)
	padding := boxWidth - visibleLen - 2

	if padding < 0 {
		padding = 0
	}

	return borderStyle.Render("│") + content + strings.Repeat(" ", padding) + borderStyle.Render("│")
}

func truncateString(s string, maxLen int) string {
	if maxLen <= 3 {
		return s
	}
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
