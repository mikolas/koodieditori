package tooling

import (
	"testing"
)

func TestParseLineGCCAndClang(t *testing.T) {
	lines := []string{
		"src/main.cpp:24:10: error: 'vector' was not declared in this scope",
		"include/util.h:10:5: warning: unused variable 'x' [-Wunused-variable]",
		"server.c:15: fatal error: stdio.h: No such file or directory",
		"main.cpp:50: note: candidate function not viable",
	}

	expected := []struct {
		file string
		line int
		col  int
		sev  string
	}{
		{"src/main.cpp", 23, 9, "error"},
		{"include/util.h", 9, 4, "warning"},
		{"server.c", 14, 0, "error"},
		{"main.cpp", 49, 0, "note"},
	}

	for i, l := range lines {
		diag := ParseLine(l)
		if diag == nil {
			t.Fatalf("failed to parse line %d: %q", i, l)
		}
		if diag.FilePath != expected[i].file {
			t.Errorf("line %d: expected file %q, got %q", i, expected[i].file, diag.FilePath)
		}
		if diag.Line != expected[i].line {
			t.Errorf("line %d: expected line %d, got %d", i, expected[i].line, diag.Line)
		}
		if diag.Col != expected[i].col {
			t.Errorf("line %d: expected col %d, got %d", i, expected[i].col, diag.Col)
		}
		if diag.Severity != expected[i].sev {
			t.Errorf("line %d: expected sev %q, got %q", i, expected[i].sev, diag.Severity)
		}
	}
}

func TestParseLineGo(t *testing.T) {
	lines := []string{
		"main.go:14:2: undefined: foo",
		"cmd/koodi/app.go:50: syntax error: unexpected semicolon",
		"    buffer_test.go:102: expected 10, got 20",
	}

	expected := []struct {
		file string
		line int
		col  int
	}{
		{"main.go", 13, 1},
		{"cmd/koodi/app.go", 49, 0},
		{"buffer_test.go", 101, 0},
	}

	for i, l := range lines {
		diag := ParseLine(l)
		if diag == nil {
			t.Fatalf("failed to parse Go line %d: %q", i, l)
		}
		if diag.FilePath != expected[i].file {
			t.Errorf("line %d: expected file %q, got %q", i, expected[i].file, diag.FilePath)
		}
		if diag.Line != expected[i].line {
			t.Errorf("line %d: expected line %d, got %d", i, expected[i].line, diag.Line)
		}
		if diag.Col != expected[i].col {
			t.Errorf("line %d: expected col %d, got %d", i, expected[i].col, diag.Col)
		}
	}
}

func TestParseLineJava(t *testing.T) {
	lines := []string{
		"[ERROR] /home/user/src/App.java:[42,15] cannot find symbol",
		"[WARNING] /home/user/src/App.java:[10,5] deprecation",
		"MyClass.java:88: error: ';' expected",
	}

	expected := []struct {
		file string
		line int
		col  int
		sev  string
	}{
		{"/home/user/src/App.java", 41, 14, "error"},
		{"/home/user/src/App.java", 9, 4, "warning"},
		{"MyClass.java", 87, 0, "error"},
	}

	for i, l := range lines {
		diag := ParseLine(l)
		if diag == nil {
			t.Fatalf("failed to parse Java line %d: %q", i, l)
		}
		if diag.FilePath != expected[i].file {
			t.Errorf("line %d: expected file %q, got %q", i, expected[i].file, diag.FilePath)
		}
		if diag.Line != expected[i].line {
			t.Errorf("line %d: expected line %d, got %d", i, expected[i].line, diag.Line)
		}
		if diag.Col != expected[i].col {
			t.Errorf("line %d: expected col %d, got %d", i, expected[i].col, diag.Col)
		}
		if diag.Severity != expected[i].sev {
			t.Errorf("line %d: expected sev %q, got %q", i, expected[i].sev, diag.Severity)
		}
	}
}

func TestParseOutput(t *testing.T) {
	output := `Building project...
src/foo.cpp:10:5: error: missing return statement
compilation terminated.
src/bar.cpp:20: warning: unused variable
Finished with errors.
`
	diags := ParseOutput(output)
	if len(diags) != 2 {
		t.Fatalf("expected 2 diagnostics, got %d", len(diags))
	}
	if diags[0].FilePath != "src/foo.cpp" || diags[0].Line != 9 {
		t.Errorf("unexpected first diagnostic: %+v", diags[0])
	}
	if diags[1].FilePath != "src/bar.cpp" || diags[1].Line != 19 {
		t.Errorf("unexpected second diagnostic: %+v", diags[1])
	}
}

func TestParseLineCMakeAndMake(t *testing.T) {
	lines := []string{
		"CMake Error at CMakeLists.txt:24 (find_package): Could not find package",
		"CMake Warning at cmake/Modules.cmake:10: Deprecated module",
		"Makefile:42: *** missing separator (did you mean TAB?). Stop.",
	}

	expected := []struct {
		file string
		line int
		sev  string
	}{
		{"CMakeLists.txt", 23, "error"},
		{"cmake/Modules.cmake", 9, "warning"},
		{"Makefile", 41, "error"},
	}

	for i, l := range lines {
		diag := ParseLine(l)
		if diag == nil {
			t.Fatalf("failed to parse line %d: %q", i, l)
		}
		if diag.FilePath != expected[i].file {
			t.Errorf("line %d: expected file %q, got %q", i, expected[i].file, diag.FilePath)
		}
		if diag.Line != expected[i].line {
			t.Errorf("line %d: expected line %d, got %d", i, expected[i].line, diag.Line)
		}
		if diag.Severity != expected[i].sev {
			t.Errorf("line %d: expected sev %q, got %q", i, expected[i].sev, diag.Severity)
		}
	}
}
