package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mikolas/koodieditori/internal/config"
	"github.com/mikolas/koodieditori/internal/ui"
)

// Version is the release version of Koodieditori.
var Version = "v1.0.0"

func printUsage() {
	fmt.Fprintf(os.Stderr, `Koodieditori %s - High-performance terminal code editor

Usage:
  koodi [options] [file...]

Options:
  -v, --version  Print version information and exit
  -h, --help     Print this help message and exit
`, Version)
}

func main() {
	var initialFiles []string
	for _, arg := range os.Args[1:] {
		switch arg {
		case "-v", "--version":
			fmt.Printf("koodi %s\n", Version)
			os.Exit(0)
		case "-h", "--help":
			printUsage()
			os.Exit(0)
		default:
			initialFiles = append(initialFiles, arg)
		}
	}

	cfg := config.FindAndLoadConfig()
	app := ui.NewApp(cfg, initialFiles)

	programOpts := []tea.ProgramOption{
		tea.WithAltScreen(),
	}

	if cfg.Editor.Mouse {
		programOpts = append(programOpts, tea.WithMouseCellMotion())
	}

	p := tea.NewProgram(app, programOpts...)
	app.Program = p

	if cfg.Agent.SocketEnabled {
		_ = app.StartIPCServer("")
	}
	_ = app.StartWatcher()
	defer app.Close()

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running Koodieditori: %v\n", err)
		os.Exit(1)
	}
}
