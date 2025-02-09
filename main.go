package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"bufio"
	"gopkg.in/yaml.v3"

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
	// Colors
	subtle    = lipgloss.AdaptiveColor{Light: "#D9DCCF", Dark: "#383838"}
	highlight = lipgloss.AdaptiveColor{Light: "#874BFD", Dark: "#7D56F4"}
	special   = lipgloss.AdaptiveColor{Light: "#43BF6D", Dark: "#73F59F"}

	// Borders and boxes
	boxStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(highlight).
		Padding(1).
		MarginBottom(1)

	titleStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(special).
		MarginLeft(2).
		MarginBottom(1).
		PaddingLeft(2).
		SetString("✨ ")

	itemStyle = lipgloss.NewStyle().
		PaddingLeft(4)

	selectedItemStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(highlight).
		PaddingLeft(2)

	descriptionStyle = lipgloss.NewStyle().
		Foreground(subtle).
		PaddingLeft(6)

	footerStyle = lipgloss.NewStyle().
		Foreground(subtle).
		Align(lipgloss.Center).
		MarginTop(1)

	// Icons for different command types
	icons = map[string]string{
		"npm":    "📦",
		"docker": "🐳",
		"go":     "🚀",
		"test":   "🧪",
		"build":  "🔨",
		"run":    "▶️ ",
		"deploy": "🚀",
	}
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

func promptUser(message string) bool {
	reader := bufio.NewReader(os.Stdin)
	fmt.Printf("\n%s (y/n): ", message)
	response, _ := reader.ReadString('\n')
	response = strings.ToLower(strings.TrimSpace(response))
	return response == "y" || response == "yes"
}

func loadConfig() (Config, error) {
	// Check if we're in a git repo first
	if _, err := os.Stat(".git"); err != nil {
		return Config{}, fmt.Errorf("not a git repository")
	}

	// Check if config exists
	if _, err := os.Stat(".project-commands.json"); err != nil {
		// Config doesn't exist - prompt user
		if promptUser("No .project-commands.json found. Would you like to generate one?") {
			generator := NewConfigGenerator()
			if err := generator.Generate(); err != nil {
				return Config{}, err
			}
		} else {
			return Config{}, fmt.Errorf("config generation cancelled by user")
		}
	}

	// Load the config (whether it existed or was just generated)
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
		return boxStyle.Render(fmt.Sprintf("Error: %v", m.err))
	}

	if m.quitting {
		return "Goodbye! 👋\n"
	}

	// Build the title section
	title := titleStyle.Render(m.config.ProjectName)
	
	// Build the commands section
	var commands strings.Builder
	for i, cmd := range m.config.Commands {
		// Determine the icon based on command name
		icon := "💫" // default icon
		for key, specificIcon := range icons {
			if strings.Contains(strings.ToLower(cmd.Name), key) {
				icon = specificIcon
				break
			}
		}

		// Style the command entry
		cursor := " "
		if m.cursor == i {
			cursor = "→"
			commands.WriteString(selectedItemStyle.Render(
				fmt.Sprintf("%s %s %s", cursor, icon, cmd.Name),
			))
		} else {
			commands.WriteString(itemStyle.Render(
				fmt.Sprintf("%s %s %s", cursor, icon, cmd.Name),
			))
		}
		commands.WriteString("\n")
		
		// Add description with subtle styling
		commands.WriteString(descriptionStyle.Render(cmd.Description))
		commands.WriteString("\n\n")
	}

	// Build the footer
	footer := footerStyle.Render("↑/↓: navigate • enter: run • q: quit")

	// Combine all sections in a box
	content := fmt.Sprintf("%s\n%s\n%s", title, commands.String(), footer)
	return boxStyle.Render(content)
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
		if err.Error() == "config generation cancelled by user" {
			fmt.Println("Cancelled by user. Run 'distructions' again if you change your mind!")
			return
		}
		if err.Error() == "not a git repository" {
			fmt.Println("Not a git repository. Distructions only works in git repositories.")
			return
		}
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
} 