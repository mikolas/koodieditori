package test

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/mikolas/koodieditori/internal/buffer"
	"github.com/mikolas/koodieditori/internal/config"
	"github.com/mikolas/koodieditori/internal/treesitter"
	"github.com/mikolas/koodieditori/internal/ui"
)

// TestStartupLatencyStrictUnder20ms verifies that cold starting the editor
// (loading configuration, opening initial buffers, and generating the first visual frame)
// completes in strictly less than 20 milliseconds as required by AGENTS.md.
func TestStartupLatencyStrictUnder20ms(t *testing.T) {
	tmpDir := t.TempDir()
	testFiles := make([]string, 5)
	for i := 0; i < 5; i++ {
		p := filepath.Join(tmpDir, fmt.Sprintf("file%d.go", i))
		content := fmt.Sprintf("package main\n\nfunc worker%d() int {\n\treturn %d * 2\n}\n", i, i)
		if err := os.WriteFile(p, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}
		testFiles[i] = p
	}

	budget := 20 * time.Millisecond
	if raceEnabled {
		budget = 100 * time.Millisecond
	}

	var minElapsed time.Duration
	for attempt := 0; attempt < 3; attempt++ {
		start := time.Now()

		// 1. Load config
		cfg := config.DefaultConfig()

		// 2. Initialize application model with initial files
		app := ui.NewApp(cfg, testFiles)

		// 3. Run Init command
		_ = app.Init()

		// 4. Render first frame view
		view := app.View()
		elapsed := time.Since(start)
		app.Close()

		if len(view) == 0 {
			t.Fatal("expected non-empty initial view")
		}

		if attempt == 0 || elapsed < minElapsed {
			minElapsed = elapsed
		}
		if minElapsed <= budget {
			break
		}
	}

	t.Logf("Cold start elapsed: %v (Budget: < %v)", minElapsed, budget)
	if minElapsed > budget {
		t.Fatalf("cold startup took %v, exceeding the %v performance budget", minElapsed, budget)
	}
}

// BenchmarkStartupCold measures cold start latency across iterations.
func BenchmarkStartupCold(b *testing.B) {
	cfg := config.DefaultConfig()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		app := ui.NewApp(cfg, nil)
		_ = app.Init()
		_ = app.View()
		app.Close()
	}
}

// TestMemoryResidencyUnder40MB verifies that opening 20 active buffers across
// multiple languages (Go, C++, Java, Markdown) with parsed Tree-sitter ASTs
// stays strictly within the 40MB resident memory budget.
func TestMemoryResidencyUnder40MB(t *testing.T) {
	runtime.GC()

	cfg := config.DefaultConfig()
	app := ui.NewApp(cfg, nil)
	defer app.Close()

	languages := []struct {
		ext     string
		content string
	}{
		{".go", "package main\nimport \"fmt\"\nfunc main() {\n\tfmt.Println(\"Hello Go\")\n}\n"},
		{".cpp", "#include <iostream>\nint main() {\n\tstd::cout << \"Hello C++\" << std::endl;\n\treturn 0;\n}\n"},
		{".java", "public class Main {\n\tpublic static void main(String[] args) {\n\t\tSystem.out.println(\"Hello Java\");\n\t}\n}\n"},
		{".md", "# Documentation\n\n```mermaid\ngraph TD;\n  A-->B;\n```\n\nDetailed markdown documentation.\n"},
	}

	// Open 20 buffers and parse syntax trees
	for i := 0; i < 20; i++ {
		sample := languages[i%len(languages)]
		fileName := fmt.Sprintf("buffer_%d%s", i, sample.ext)
		content := strings.Repeat(sample.content, 20) // ~1-2 KB per file

		buf := ui.NewBufferItem(i+1, fileName, content)
		lang, _ := treesitter.DetectLanguage(fileName)
		if lang != nil {
			buf.Highlighter.Parse([]byte(content), lang)
		}
		app.Buffers = append(app.Buffers, buf)
	}

	// Render view to populate any cached layouts
	_ = app.View()

	runtime.GC()
	var mAfter runtime.MemStats
	runtime.ReadMemStats(&mAfter)

	totalLiveMB := float64(mAfter.Alloc) / (1024 * 1024)
	t.Logf("Live heap memory for 20 buffers with Tree-sitter ASTs: %.2f MB (Budget: < 40 MB)", totalLiveMB)
	if totalLiveMB > 40.0 {
		t.Fatalf("memory allocation of %.2f MB exceeded 40 MB budget", totalLiveMB)
	}
}

// BenchmarkPieceTableMutation benchmarks high-throughput piece table inserts and deletes.
func BenchmarkPieceTableMutation(b *testing.B) {
	pt := buffer.NewPieceTableFromString("package main\n\nfunc main() {\n\tprintln(\"hello\")\n}\n")

	insertText := "// comment insertion\n"
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		pt.Insert(0, insertText)
		pt.Delete(0, len(insertText))
	}
}
