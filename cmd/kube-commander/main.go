package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/elwin/kube-commander/internal/config"
	"github.com/elwin/kube-commander/internal/ui"
)

func main() {
	configPath := flag.String("config", "", "path to kube-commander config (default: ~/.config/kube-commander/config.yaml)")
	flag.Parse()

	examplePath := findExampleConfig()
	cfg, resolved, err := config.Load(*configPath, examplePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "kube-commander: %v\n", err)
		os.Exit(1)
	}

	model := ui.New(cfg, resolved)
	p := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "kube-commander: %v\n", err)
		os.Exit(1)
	}
}

func findExampleConfig() string {
	candidates := []string{
		"configs/example.yaml",
	}
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(dir, "configs", "example.yaml"),
			filepath.Join(dir, "..", "configs", "example.yaml"),
			filepath.Join(dir, "..", "..", "configs", "example.yaml"),
		)
	}
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates,
			filepath.Join(wd, "configs", "example.yaml"),
		)
	}
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			abs, err := filepath.Abs(c)
			if err == nil {
				return abs
			}
			return c
		}
	}
	return "configs/example.yaml"
}
