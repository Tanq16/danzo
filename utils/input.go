package utils

import (
	"bufio"
	"os"
	"strings"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

var stdinScanner *bufio.Scanner

func getStdinScanner() *bufio.Scanner {
	if stdinScanner == nil {
		stdinScanner = bufio.NewScanner(os.Stdin)
	}
	return stdinScanner
}

func ReadPipedInput() string {
	fi, err := os.Stdin.Stat()
	if err != nil || fi.Mode()&os.ModeCharDevice != 0 {
		return ""
	}
	scanner := getStdinScanner()
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func ReadPipedLine() string {
	fi, err := os.Stdin.Stat()
	if err != nil || fi.Mode()&os.ModeCharDevice != 0 {
		return ""
	}
	scanner := getStdinScanner()
	if scanner.Scan() {
		return strings.TrimSpace(scanner.Text())
	}
	return ""
}

type inputModel struct {
	textInput textinput.Model
	done      bool
	value     string
	initCmd   tea.Cmd
}

func (m inputModel) Init() tea.Cmd {
	return m.initCmd
}

func (m inputModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "enter":
			m.value = m.textInput.Value()
			m.done = true
			return m, tea.Quit
		case "ctrl+c", "esc":
			m.done = true
			return m, tea.Quit
		}
	}
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

func (m inputModel) View() tea.View {
	if m.done {
		return tea.NewView("")
	}
	return tea.NewView(m.textInput.View())
}

func PromptInput(prompt string, placeholder string) (string, error) {
	if GlobalForAIFlag {
		return ReadPipedLine(), nil
	}

	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.Prompt = prompt + " "
	focusCmd := ti.Focus()

	m := inputModel{textInput: ti, initCmd: focusCmd}
	finalModel, err := tea.NewProgram(m).Run()
	if err != nil {
		return "", err
	}

	result := finalModel.(inputModel)
	return strings.TrimSpace(result.value), nil
}

func PromptPassword(prompt string) (string, error) {
	if GlobalForAIFlag {
		return ReadPipedLine(), nil
	}

	ti := textinput.New()
	ti.Placeholder = "********"
	ti.Prompt = prompt + " "
	ti.EchoMode = textinput.EchoPassword
	focusCmd := ti.Focus()

	m := inputModel{textInput: ti, initCmd: focusCmd}
	finalModel, err := tea.NewProgram(m).Run()
	if err != nil {
		return "", err
	}

	result := finalModel.(inputModel)
	return result.value, nil
}

type textAreaModel struct {
	textarea textarea.Model
	done     bool
	value    string
	initCmd  tea.Cmd
}

func (m textAreaModel) Init() tea.Cmd {
	return m.initCmd
}

func (m textAreaModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+d":
			m.value = m.textarea.Value()
			m.done = true
			return m, tea.Quit
		case "ctrl+c", "esc":
			m.done = true
			return m, tea.Quit
		}
	}
	m.textarea, cmd = m.textarea.Update(msg)
	return m, cmd
}

func (m textAreaModel) View() tea.View {
	if m.done {
		return tea.NewView("")
	}
	return tea.NewView(m.textarea.View() + "\n(Ctrl+D to submit, Esc to cancel)")
}

func PromptTextArea(prompt string, placeholder string) (string, error) {
	if GlobalForAIFlag {
		return ReadPipedInput(), nil
	}

	PrintInfo(prompt)
	ta := textarea.New()
	ta.Placeholder = placeholder
	focusCmd := ta.Focus()

	m := textAreaModel{textarea: ta, initCmd: focusCmd}
	finalModel, err := tea.NewProgram(m).Run()
	if err != nil {
		return "", err
	}

	result := finalModel.(textAreaModel)
	return strings.TrimSpace(result.value), nil
}
