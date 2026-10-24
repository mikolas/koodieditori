package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/mikolas/koodieditori/internal/ipc"
)

// Version is the release version of koodi-cli.
var Version = "v1.0.0"

func printUsage() {
	fmt.Fprintf(os.Stderr, `koodi-cli %s - Command-line interface for Koodieditori

Usage:
  koodi-cli [--socket <path>] <command> [arguments...]

Commands:
  open <file>[:line[:col]]               Jump editor view to file, line, and column
  diff <file>                            Open interactive diff view for file
  reload [--all] [file]                  Reload buffer(s) from disk
  notify <message> [--level=info|warn|error] Display notification in editor
  status-badge <badge> [--busy]          Set agent statusline badge
  action <name> [key=val...]             Dispatch editor action by name

Options:
  --socket <path>  Path to Unix domain socket (default: $XDG_RUNTIME_DIR/koodi/koodi-default.sock)
  -v, --version    Print version information and exit
  -h, --help       Print this help message and exit
`, Version)
}

func main() {
	socketFlag := flag.String("socket", "", "Path to Unix domain socket")
	versionFlag := flag.Bool("version", false, "Print version information")
	vShortFlag := flag.Bool("v", false, "Print version information")
	flag.Usage = printUsage
	flag.Parse()

	if *versionFlag || *vShortFlag {
		fmt.Printf("koodi-cli %s\n", Version)
		os.Exit(0)
	}

	args := flag.Args()
	if len(args) == 0 {
		printUsage()
		os.Exit(1)
	}

	socketPath := *socketFlag
	if socketPath == "" {
		socketPath = ipc.DefaultSocketPath("default")
	}

	client, err := ipc.NewClient(socketPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error connecting to Koodieditori: %v\n", err)
		os.Exit(1)
	}
	defer client.Close()

	cmd := args[0]
	cmdArgs := args[1:]

	switch cmd {
	case "open":
		if len(cmdArgs) == 0 {
			fmt.Fprintf(os.Stderr, "Usage: koodi-cli open <file>[:line[:col]]\n")
			os.Exit(1)
		}
		file, line, col := parseFileLocation(cmdArgs[0])
		if err := client.Open(file, line, col); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Opened %s:%d:%d\n", file, line, col)

	case "diff":
		if len(cmdArgs) == 0 {
			fmt.Fprintf(os.Stderr, "Usage: koodi-cli diff <file>\n")
			os.Exit(1)
		}
		if err := client.Diff(cmdArgs[0]); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Requested diff view for %s\n", cmdArgs[0])

	case "reload":
		all := false
		file := ""
		for _, a := range cmdArgs {
			if a == "--all" || a == "-a" {
				all = true
			} else {
				file = a
			}
		}
		if err := client.Reload(all, file); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if all {
			fmt.Println("Reloaded all buffers")
		} else {
			fmt.Printf("Reloaded %s\n", file)
		}

	case "notify":
		if len(cmdArgs) == 0 {
			fmt.Fprintf(os.Stderr, "Usage: koodi-cli notify <message> [--level=info|warn|error]\n")
			os.Exit(1)
		}
		msg := cmdArgs[0]
		level := "info"
		for _, a := range cmdArgs[1:] {
			if strings.HasPrefix(a, "--level=") {
				level = strings.TrimPrefix(a, "--level=")
			}
		}
		if err := client.Notify(msg, level); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Notification sent: %q\n", msg)

	case "status-badge":
		if len(cmdArgs) == 0 {
			fmt.Fprintf(os.Stderr, "Usage: koodi-cli status-badge <badge> [--busy]\n")
			os.Exit(1)
		}
		badge := cmdArgs[0]
		busy := false
		for _, a := range cmdArgs[1:] {
			if a == "--busy" {
				busy = true
			}
		}
		if err := client.StatusBadge(badge, busy); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Status badge set to %q\n", badge)

	case "action":
		if len(cmdArgs) == 0 {
			fmt.Fprintf(os.Stderr, "Usage: koodi-cli action <name> [key=val...]\n")
			os.Exit(1)
		}
		name := cmdArgs[0]
		actionArgs := make(map[string]string)
		for _, kv := range cmdArgs[1:] {
			parts := strings.SplitN(kv, "=", 2)
			if len(parts) == 2 {
				actionArgs[parts[0]] = parts[1]
			}
		}
		if err := client.Action(name, actionArgs); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Action %q executed\n", name)

	default:
		fmt.Fprintf(os.Stderr, "Unknown command %q\n", cmd)
		printUsage()
		os.Exit(1)
	}
}

// parseFileLocation parses "path/file.go:12:5" into ("path/file.go", 12, 5).
func parseFileLocation(input string) (string, int, int) {
	parts := strings.Split(input, ":")
	file := parts[0]
	line := 1
	col := 1

	if len(parts) > 1 {
		if l, err := strconv.Atoi(parts[1]); err == nil && l > 0 {
			line = l
		}
	}
	if len(parts) > 2 {
		if c, err := strconv.Atoi(parts[2]); err == nil && c > 0 {
			col = c
		}
	}

	return file, line, col
}
