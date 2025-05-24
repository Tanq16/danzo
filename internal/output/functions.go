package output

import (
	"fmt"
	"os"
	"strings"
	"unicode/utf8"

	"golang.org/x/term"
)

// FormatBytes converts bytes to human-readable format
func FormatBytes(bytes uint64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := uint64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// FormatSpeed calculates and formats download speed
func FormatSpeed(bytes int64, elapsed float64) string {
	if elapsed == 0 {
		return "0 B/s"
	}
	bps := float64(bytes) / elapsed
	formatted := FormatBytes(uint64(bps))
	return formatted[:len(formatted)-1] + "B/s" // Replace "B" with "B/s"
}

// PrintProgressBar creates a progress bar string
func PrintProgressBar(current, total int64, width int) string {
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

func getTerminalWidth() int {
	width, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || width <= 0 {
		return 80 // Default fallback width
	}
	return width
}

func getTerminalHeight() int {
	height, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || height <= 0 {
		return 24 // Default fallback height
	}
	return height
}

func wrapText(text string, indent int) []string {
	termWidth := getTerminalWidth()
	maxWidth := termWidth - indent - 2 // Account for indentation
	if maxWidth <= 10 {
		maxWidth = 80
	}
	if utf8.RuneCountInString(text) <= maxWidth {
		return []string{text}
	}
	var lines []string
	currentLine := ""
	currentWidth := 0
	for _, r := range text {
		runeWidth := 1
		// If adding this rune would exceed max width, flush the line
		if currentWidth+runeWidth > maxWidth {
			lines = append(lines, currentLine)
			currentLine = string(r)
			currentWidth = runeWidth
		} else {
			currentLine += string(r)
			currentWidth += runeWidth
		}
	}
	if currentLine != "" {
		lines = append(lines, currentLine)
	}
	return lines
}
