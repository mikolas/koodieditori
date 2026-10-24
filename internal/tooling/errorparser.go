package tooling

import (
	"regexp"
	"strconv"
	"strings"
)

// DiagnosticError represents a parsed compiler or test error/warning location.
type DiagnosticError struct {
	RawLine  string
	FilePath string
	Line     int    // 0-indexed line number in editor buffer
	Col      int    // 0-indexed column number in editor buffer
	Severity string // "error", "warning", "note", "info"
	Message  string
}

// Regex patterns for various compiler and build tool outputs.
var (
	// Maven / Gradle: [ERROR] /path/to/File.java:[42,15] cannot find symbol
	reMaven = regexp.MustCompile(`^\[(ERROR|WARNING|INFO)\]\s+([^:\[\s]+(?:\s+[^:\[\s]+)*):\[(\d+),(\d+)\]\s*(.*)$`)

	// GCC / Clang / Go / Javac standard: file:line:col: [severity:] message
	reStandardWithCol = regexp.MustCompile(`^([^:\r\n\t]+):(\d+):(\d+):\s*(?:(error|fatal error|warning|note|info):\s*)?(.*)$`)

	// GCC / Clang / Go / Javac without col: file:line: [severity:] message
	reStandardNoCol = regexp.MustCompile(`^([^:\r\n\t]+):(\d+):\s*(?:(error|fatal error|warning|note|info):\s*)?(.*)$`)

	// Go test failure line: "    foo_test.go:42: assertion failed"
	reGoTest = regexp.MustCompile(`^\s+([a-zA-Z0-9_\-./]+\.go):(\d+):\s*(.*)$`)

	// CMake: CMake Error at CMakeLists.txt:24 (find_package): message
	reCMake = regexp.MustCompile(`^CMake\s+(Error|Warning)\s+at\s+([^:\r\n\t]+):(\d+)(?:\s+\([^)]+\))?:?\s*(.*)$`)
)

// ParseLine attempts to parse a single line of compiler output into a DiagnosticError.
// Returns nil if the line does not match any recognized diagnostic pattern.
func ParseLine(line string) *DiagnosticError {
	trimmed := strings.TrimRight(line, "\r\n")
	if len(trimmed) == 0 {
		return nil
	}

	// 0. CMake Error / Warning
	if m := reCMake.FindStringSubmatch(trimmed); len(m) > 0 {
		sev := strings.ToLower(m[1])
		filePath := strings.TrimSpace(m[2])
		lineNum, _ := strconv.Atoi(m[3])
		msg := strings.TrimSpace(m[4])
		return &DiagnosticError{
			RawLine:  trimmed,
			FilePath: filePath,
			Line:     max(0, lineNum-1),
			Col:      0,
			Severity: sev,
			Message:  msg,
		}
	}

	// 1. Maven / Gradle
	if m := reMaven.FindStringSubmatch(trimmed); len(m) > 0 {
		sev := strings.ToLower(m[1])
		filePath := strings.TrimSpace(m[2])
		lineNum, _ := strconv.Atoi(m[3])
		colNum, _ := strconv.Atoi(m[4])
		msg := strings.TrimSpace(m[5])
		return &DiagnosticError{
			RawLine:  trimmed,
			FilePath: filePath,
			Line:     max(0, lineNum-1),
			Col:      max(0, colNum-1),
			Severity: sev,
			Message:  msg,
		}
	}

	// 2. Go test indent
	if m := reGoTest.FindStringSubmatch(trimmed); len(m) > 0 {
		filePath := strings.TrimSpace(m[1])
		lineNum, _ := strconv.Atoi(m[2])
		msg := strings.TrimSpace(m[3])
		return &DiagnosticError{
			RawLine:  trimmed,
			FilePath: filePath,
			Line:     max(0, lineNum-1),
			Col:      0,
			Severity: "error",
			Message:  msg,
		}
	}

	// 3. Standard with Col
	if m := reStandardWithCol.FindStringSubmatch(trimmed); len(m) > 0 {
		filePath := strings.TrimSpace(m[1])
		// Avoid matching drive letters like C:\ or generic prefixes
		if len(filePath) > 0 && !strings.Contains(filePath, " ") {
			lineNum, _ := strconv.Atoi(m[2])
			colNum, _ := strconv.Atoi(m[3])
			sev := "error"
			if m[4] != "" {
				sev = strings.ToLower(m[4])
				if sev == "fatal error" {
					sev = "error"
				}
			}
			msg := strings.TrimSpace(m[5])
			return &DiagnosticError{
				RawLine:  trimmed,
				FilePath: filePath,
				Line:     max(0, lineNum-1),
				Col:      max(0, colNum-1),
				Severity: sev,
				Message:  msg,
			}
		}
	}

	// 4. Standard without Col
	if m := reStandardNoCol.FindStringSubmatch(trimmed); len(m) > 0 {
		filePath := strings.TrimSpace(m[1])
		if len(filePath) > 0 && !strings.Contains(filePath, " ") {
			lineNum, _ := strconv.Atoi(m[2])
			sev := "error"
			if m[3] != "" {
				sev = strings.ToLower(m[3])
				if sev == "fatal error" {
					sev = "error"
				}
			}
			msg := strings.TrimSpace(m[4])
			return &DiagnosticError{
				RawLine:  trimmed,
				FilePath: filePath,
				Line:     max(0, lineNum-1),
				Col:      0,
				Severity: sev,
				Message:  msg,
			}
		}
	}

	return nil
}

// ParseOutput splits an entire output string into lines and returns all parsed diagnostic errors.
func ParseOutput(output string) []*DiagnosticError {
	lines := strings.Split(output, "\n")
	var errs []*DiagnosticError
	for _, l := range lines {
		if diag := ParseLine(l); diag != nil {
			errs = append(errs, diag)
		}
	}
	return errs
}
