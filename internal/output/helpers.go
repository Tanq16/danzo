package output

import (
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"
)

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
