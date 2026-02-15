package utils

import (
	"fmt"
	"os"

	"github.com/charmbracelet/lipgloss"
	"github.com/rs/zerolog/log"
)

var (
	infoStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#89b4fa"))
	successStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#a6e3a1"))
	errorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#f38ba8"))
	warnStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#f9e2af"))
)

func PrintInfo(msg string) {
	if GlobalDebugFlag {
		log.Info().Msg(msg)
	} else {
		fmt.Println(infoStyle.Render("→ " + msg))
	}
}

func PrintSuccess(msg string) {
	if GlobalDebugFlag {
		log.Info().Msg(msg)
	} else {
		fmt.Println(successStyle.Render("✓ " + msg))
	}
}

func PrintError(msg string, err error) {
	if GlobalDebugFlag && err != nil {
		log.Error().Err(err).Msg(msg)
	} else {
		fmt.Println(errorStyle.Render("✗ " + msg))
	}
}

func PrintFatal(msg string, err error) {
	if GlobalDebugFlag && err != nil {
		log.Error().Err(err).Msg(msg)
	} else {
		fmt.Println(errorStyle.Render("✗ " + msg))
	}
	os.Exit(1)
}

func PrintWarn(msg string, err error) {
	if GlobalDebugFlag && err != nil {
		log.Warn().Err(err).Msg(msg)
	} else {
		fmt.Println(warnStyle.Render("! " + msg))
	}
}

func PrintGeneric(msg string) {
	fmt.Println(msg)
}
