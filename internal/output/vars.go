package output

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

var (
	// Core styles
	successStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("37"))            // dark green
	success2Style = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))             // green
	errorStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))             // red
	warningStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))            // yellow
	pendingStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("12"))            // blue
	infoStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("14"))            // cyan
	debugStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("250"))           // light grey
	detailStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("13"))            // purple
	streamStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))           // grey
	headerStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("69")) // purple
)

var StyleSymbols = map[string]string{
	"pass":    "✓",
	"fail":    "✗",
	"warning": "!",
	"pending": "◉",
	"info":    "ℹ",
	"arrow":   "→",
	"bullet":  "•",
	"dot":     "·",
	"hline":   "━",
}

func PrintSuccess(text string) {
	fmt.Println(successStyle.Render(text))
}
func PrintSuccess2(text string) {
	fmt.Println(success2Style.Render(text))
}
func PrintError(text string) {
	fmt.Println(errorStyle.Render(text))
}
func PrintWarning(text string) {
	fmt.Println(warningStyle.Render(text))
}
func PrintPending(text string) {
	fmt.Println(pendingStyle.Render(text))
}
func PrintInfo(text string) {
	fmt.Println(infoStyle.Render(text))
}
func PrintDebug(text string) {
	fmt.Println(debugStyle.Render(text))
}
func PrintDetail(text string) {
	fmt.Println(detailStyle.Render(text))
}
func PrintStream(text string) {
	fmt.Println(streamStyle.Render(text))
}
func PrintHeader(text string) {
	fmt.Println(headerStyle.Render(text))
}
func FSuccess(text string) string {
	return successStyle.Render(text)
}
func FSuccess2(text string) string {
	return success2Style.Render(text)
}
func FError(text string) string {
	return errorStyle.Render(text)
}
func FWarning(text string) string {
	return warningStyle.Render(text)
}
func FPending(text string) string {
	return pendingStyle.Render(text)
}
func FInfo(text string) string {
	return infoStyle.Render(text)
}
func FDebug(text string) string {
	return debugStyle.Render(text)
}
func FDetail(text string) string {
	return detailStyle.Render(text)
}
func FStream(text string) string {
	return streamStyle.Render(text)
}
func FHeader(text string) string {
	return headerStyle.Render(text)
}
