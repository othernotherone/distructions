package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Command represents a project command with description
type Command struct {
	Name        string `json:"name"`
	Command     string `json:"command"`
	Description string `json:"description"`
}

// Config represents the project configuration
type Config struct {
	ProjectName string    `json:"projectName"`
	Commands    []Command `json:"commands"`
}

// Model represents the application state
type Model struct {
	config     Config
	cursor     int
	selected   map[int]struct{}
	quitting   bool
	err        error
}

var (
	titleStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#FF75B7")).
		MarginBottom(1)

	itemStyle = lipgloss.NewStyle().
		PaddingLeft(4)

	selectedItemStyle = lipgloss.NewStyle().
		PaddingLeft(2).
		Foreground(lipgloss.Color("#FF75B7")).
		SetString("→ ")

	descriptionStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#666666")).
		PaddingLeft(6)
)

func initialModel() Model {
	config, err := loadConfig()
	return Model{
		config:   config,
		selected: make(map[int]struct{}),
		err:      err,
	}
}

func loadConfig() (Config, error) {
	data, err := os.ReadFile(".project-commands.json")
	if err != nil {
		return Config{}, err
	}

	var config Config
	err = json.Unmarshal(data, &config)
	return config, err
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			m.quitting = true
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.config.Commands)-1 {
				m.cursor++
			}
		case "enter":
			// Execute the selected command
			if m.cursor < len(m.config.Commands) {
				cmd := m.config.Commands[m.cursor].Command
				return m, tea.ExecProcess(exec.Command("sh", "-c", cmd), nil)
			}
		}
	}
	return m, nil
}

func (m Model) View() string {
	if m.err != nil {
		return fmt.Sprintf("Error: %v\n", m.err)
	}

	if m.quitting {
		return "Bye!\n"
	}

	s := titleStyle.Render(m.config.ProjectName) + "\n\n"

	for i, cmd := range m.config.Commands {
		cursor := " "
		if m.cursor == i {
			cursor = selectedItemStyle.String()
		}
		s += fmt.Sprintf("%s%s\n", cursor, itemStyle.Render(cmd.Name))
		s += descriptionStyle.Render(cmd.Description) + "\n\n"
	}

	s += "\nPress q to quit.\n"

	return s
}

func main() {
	p := tea.NewProgram(initialModel())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v", err)
		os.Exit(1)
	}
} 