package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/elmosgot/kube-commander/internal/applog"
	"github.com/elmosgot/kube-commander/internal/config"
	"github.com/elmosgot/kube-commander/internal/ui"
)

// version is set at link time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	configPath := flag.String("config", "", "path to kube-commander config (default: ~/.config/kube-commander/config.yaml)")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return
	}

	applog.SetVersion(version)
	if err := applog.Init(); err != nil {
		fmt.Fprintf(os.Stderr, "kube-commander: session log: %v\n", err)
	}
	defer applog.Close()

	examplePath := findExampleConfig()
	cfg, resolved, err := config.Load(*configPath, examplePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "kube-commander: %v\n", err)
		os.Exit(1)
	}
	applog.Infof("config=%s", resolved)

	model := ui.New(cfg, resolved)
	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())
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
