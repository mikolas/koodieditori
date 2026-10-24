package markdown

import (
	"strings"
	"testing"
)

func TestIsMarkdownFile(t *testing.T) {
	if !IsMarkdownFile("README.md") {
		t.Fatal("expected README.md to be markdown")
	}
	if !IsMarkdownFile("docs/SPEC.markdown") {
		t.Fatal("expected .markdown to be markdown")
	}
	if IsMarkdownFile("main.go") {
		t.Fatal("expected main.go not to be markdown")
	}
}

func TestToggleCheckbox(t *testing.T) {
	line1 := "- [ ] Write tests"
	toggled1, ok1 := ToggleCheckbox(line1)
	if !ok1 || toggled1 != "- [x] Write tests" {
		t.Fatalf("expected '- [x] Write tests', got %q (ok=%v)", toggled1, ok1)
	}

	toggledBack, okBack := ToggleCheckbox(toggled1)
	if !okBack || toggledBack != "- [ ] Write tests" {
		t.Fatalf("expected '- [ ] Write tests', got %q", toggledBack)
	}

	// Asterisk checklist
	line2 := "* [x] Finished task"
	toggled2, ok2 := ToggleCheckbox(line2)
	if !ok2 || toggled2 != "* [ ] Finished task" {
		t.Fatalf("expected '* [ ] Finished task', got %q", toggled2)
	}

	// Line without checkbox
	line3 := "Regular text without checkbox"
	toggled3, ok3 := ToggleCheckbox(line3)
	if ok3 || toggled3 != line3 {
		t.Fatal("expected no change on regular line")
	}
}

func TestConcealLine(t *testing.T) {
	// Active cursor line should NEVER conceal
	raw := "- [ ] **Bold** and `code` and [link](http://example.com)"
	if ConcealLine(raw, true) != raw {
		t.Fatal("active cursor line must not be concealed")
	}

	// Inactive line concealment
	concealed := ConcealLine(raw, false)
	if !strings.Contains(concealed, "☐") {
		t.Fatalf("expected ☐ in concealed line, got %q", concealed)
	}
	if strings.Contains(concealed, "**") {
		t.Fatalf("expected ** removed, got %q", concealed)
	}
	if strings.Contains(concealed, "`") {
		t.Fatalf("expected backticks removed, got %q", concealed)
	}
	if strings.Contains(concealed, "http://example.com") {
		t.Fatalf("expected url concealed, got %q", concealed)
	}
	if !strings.Contains(concealed, "link") {
		t.Fatalf("expected link text preserved, got %q", concealed)
	}

	// Checked checkbox
	checkedLine := "- [x] Done item"
	cChecked := ConcealLine(checkedLine, false)
	if !strings.Contains(cChecked, "☑") {
		t.Fatalf("expected ☑ in concealed line, got %q", cChecked)
	}
}

func TestConcealLinePreservesSnakeCaseAndGlobs(t *testing.T) {
	// 1. Inline code containing underscores must keep underscores intact
	line1 := "2. **100% Load-Image Parity:** (`vasmm68k_mot` + `scripts/verify_parity.py`)."
	got1 := ConcealLine(line1, false)
	want1 := "2. 100% Load-Image Parity: (vasmm68k_mot + scripts/verify_parity.py)."
	if got1 != want1 {
		t.Fatalf("expected %q, got %q", want1, got1)
	}

	// 2. Inline code containing glob asterisk + underscore must keep *_ intact
	line2 := "3. **Self-Contained Synthetic Tests:** All unit tests in `*_test.go` and `:memory:`."
	got2 := ConcealLine(line2, false)
	want2 := "3. Self-Contained Synthetic Tests: All unit tests in *_test.go and :memory:."
	if got2 != want2 {
		t.Fatalf("expected %q, got %q", want2, got2)
	}

	// 3. Unbackticked snake_case identifiers and path globs must not be treated as emphasis
	line3 := "Run vasmm68k_mot and verify_parity.py on disasm/*.asm src/*.asm"
	got3 := ConcealLine(line3, false)
	if got3 != line3 {
		t.Fatalf("expected unbackticked snake_case and globs unchanged %q, got %q", line3, got3)
	}
}

func TestConcealLineWithMappingAndMapSpan(t *testing.T) {
	raw := "review [AGENTS.md](AGENTS.md) and [ARCHITECTURE.md](ARCHITECTURE.md) now"
	concealed, mapping := ConcealLineWithMapping(raw, false)
	want := "review AGENTS.md and ARCHITECTURE.md now"
	if concealed != want {
		t.Fatalf("expected %q, got %q", want, concealed)
	}

	// Title 1: "AGENTS.md" in raw is at [8, 17)
	cStart1, cEnd1 := MapSpan(mapping, 8, 17)
	if concealed[cStart1:cEnd1] != "AGENTS.md" {
		t.Fatalf("expected mapped span 1 to be 'AGENTS.md', got %q ([%d:%d])", concealed[cStart1:cEnd1], cStart1, cEnd1)
	}

	// URL 1: "AGENTS.md" in raw is at [19, 28) -> should map to empty span
	uStart1, uEnd1 := MapSpan(mapping, 19, 28)
	if uStart1 != uEnd1 {
		t.Fatalf("expected concealed URL span to collapse to empty, got [%d, %d)", uStart1, uEnd1)
	}

	// Title 2: "ARCHITECTURE.md" in raw is at [35, 50)
	cStart2, cEnd2 := MapSpan(mapping, 35, 50)
	if concealed[cStart2:cEnd2] != "ARCHITECTURE.md" {
		t.Fatalf("expected mapped span 2 to be 'ARCHITECTURE.md', got %q ([%d:%d])", concealed[cStart2:cEnd2], cStart2, cEnd2)
	}

	// URL 2: "ARCHITECTURE.md" in raw is at [52, 67) -> should map to empty span
	uStart2, uEnd2 := MapSpan(mapping, 52, 67)
	if uStart2 != uEnd2 {
		t.Fatalf("expected second concealed URL span to collapse to empty, got [%d, %d)", uStart2, uEnd2)
	}
}

func TestFenceStateTracking(t *testing.T) {
	lines := []string{
		"Paragraph before",
		"```bash",
		"vasmm68k_mot -o build/my_target.bin",
		"```",
		"Paragraph after",
	}

	inFence := false
	var fenceChar byte
	fenceLen := 0

	var states []bool
	for _, l := range lines {
		wasIn := inFence
		var isDelim bool
		inFence, fenceChar, fenceLen, isDelim = NextFenceState(l, inFence, fenceChar, fenceLen)
		states = append(states, wasIn || isDelim)
	}

	want := []bool{false, true, true, true, false}
	for i := range want {
		if states[i] != want[i] {
			t.Fatalf("line %d (%q): expected fenced=%v, got %v", i, lines[i], want[i], states[i])
		}
	}
}
