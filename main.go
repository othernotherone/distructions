package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"gopkg.in/yaml.v3"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"strings"
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
		MarginBottom(1).
		SetString("distructions")

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

type PackageJSON struct {
	Scripts map[string]string `json:"scripts"`
}

// ConfigGenerator handles detecting and generating config
type ConfigGenerator struct {
	projectRoot string
}

func NewConfigGenerator() *ConfigGenerator {
	return &ConfigGenerator{
		projectRoot: ".",
	}
}

func (g *ConfigGenerator) Generate() error {
	// Check if .project-commands.json already exists
	if _, err := os.Stat(".project-commands.json"); err == nil {
		return nil // Config already exists
	}

	config := Config{
		ProjectName: getRepoName(),
		Commands:    []Command{},
	}

	// Detect and add commands from various sources
	g.detectNodeCommands(&config)
	g.detectDockerCommands(&config)
	g.detectGoCommands(&config)
	// Add more detectors as needed

	// Only save if we found any commands
	if len(config.Commands) > 0 {
		return g.saveConfig(config)
	}

	return nil
}

func (g *ConfigGenerator) detectNodeCommands(config *Config) {
	data, err := os.ReadFile("package.json")
	if err != nil {
		return
	}

	var pkg PackageJSON
	if err := json.Unmarshal(data, &pkg); err != nil {
		return
	}

	for name, script := range pkg.Scripts {
		config.Commands = append(config.Commands, Command{
			Name:        fmt.Sprintf("npm: %s", name),
			Command:     fmt.Sprintf("npm run %s", name),
			Description: fmt.Sprintf("Run npm script: %s", script),
		})
	}
}

func (g *ConfigGenerator) detectDockerCommands(config *Config) {
	files := []string{"docker-compose.yml", "docker-compose.yaml"}
	var composeFile string
	
	for _, file := range files {
		if _, err := os.Stat(file); err == nil {
			composeFile = file
			break
		}
	}

	if composeFile == "" {
		return
	}

	// Read docker-compose file to get service names
	data, err := os.ReadFile(composeFile)
	if err != nil {
		return
	}

	var compose map[string]interface{}
	if err := yaml.Unmarshal(data, &compose); err != nil {
		return
	}

	services, ok := compose["services"].(map[string]interface{})
	if !ok {
		return
	}

	// Add generic docker-compose commands
	config.Commands = append(config.Commands,
		Command{
			Name:        "Docker: Start All",
			Command:     "docker-compose up",
			Description: "Start all Docker containers",
		},
		Command{
			Name:        "Docker: Start All (Detached)",
			Command:     "docker-compose up -d",
			Description: "Start all Docker containers in detached mode",
		},
		Command{
			Name:        "Docker: Stop All",
			Command:     "docker-compose down",
			Description: "Stop all Docker containers",
		},
	)

	// Add service-specific commands
	for serviceName := range services {
		config.Commands = append(config.Commands,
			Command{
				Name:        fmt.Sprintf("Docker: Start %s", serviceName),
				Command:     fmt.Sprintf("docker-compose up %s", serviceName),
				Description: fmt.Sprintf("Start the %s service", serviceName),
			},
		)
	}
}

func (g *ConfigGenerator) detectGoCommands(config *Config) {
	if _, err := os.Stat("go.mod"); err == nil {
		config.Commands = append(config.Commands,
			Command{
				Name:        "Go: Run",
				Command:     "go run .",
				Description: "Run the Go application",
			},
			Command{
				Name:        "Go: Test",
				Command:     "go test ./...",
				Description: "Run all tests",
			},
			Command{
				Name:        "Go: Build",
				Command:     "go build",
				Description: "Build the Go application",
			},
		)
	}
}

func (g *ConfigGenerator) saveConfig(config Config) error {
	data, err := json.MarshalIndent(config, "", "    ")
	if err != nil {
		return err
	}
	return os.WriteFile(".project-commands.json", data, 0644)
}

func initialModel() Model {
	config, err := loadConfig()
	return Model{
		config:   config,
		selected: make(map[int]struct{}),
		err:      err,
	}
}

func loadConfig() (Config, error) {
	// Try to generate config if it doesn't exist
	generator := NewConfigGenerator()
	if err := generator.Generate(); err != nil {
		return Config{}, err
	}

	// Now try to load the config (whether it existed before or was just generated)
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

func getRepoName() string {
	// Try to get the remote origin URL
	cmd := exec.Command("git", "config", "--get", "remote.origin.url")
	output, err := cmd.Output()
	if err == nil {
		// Clean the URL and get the last part
		url := strings.TrimSpace(string(output))
		url = strings.TrimSuffix(url, ".git")
		parts := strings.Split(url, "/")
		if len(parts) > 0 {
			return parts[len(parts)-1]
		}
	}

	// Fallback: try to get the directory name
	dir, err := os.Getwd()
	if err == nil {
		return filepath.Base(dir)
	}

	return "Unknown Project"
}

func main() {
	p := tea.NewProgram(initialModel())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v", err)
		os.Exit(1)
	}
} 