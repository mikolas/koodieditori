package tooling

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindAlternateSameDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	cppFile := filepath.Join(tmpDir, "server.cpp")
	hppFile := filepath.Join(tmpDir, "server.hpp")

	_ = os.WriteFile(cppFile, []byte("int main() {}"), 0644)
	_ = os.WriteFile(hppFile, []byte("#pragma once"), 0644)

	alt, err := FindAlternate(cppFile, tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if alt != hppFile {
		t.Fatalf("expected alternate %q, got %q", hppFile, alt)
	}

	altBack, err := FindAlternate(hppFile, tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if altBack != cppFile {
		t.Fatalf("expected alternate %q, got %q", cppFile, altBack)
	}
}

func TestFindAlternateSrcInclude(t *testing.T) {
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "src", "network")
	includeDir := filepath.Join(tmpDir, "include", "network")

	_ = os.MkdirAll(srcDir, 0755)
	_ = os.MkdirAll(includeDir, 0755)

	cppFile := filepath.Join(srcDir, "socket.cpp")
	hFiled := filepath.Join(includeDir, "socket.h")

	_ = os.WriteFile(cppFile, []byte(""), 0644)
	_ = os.WriteFile(hFiled, []byte(""), 0644)

	alt, err := FindAlternate(cppFile, tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if alt != hFiled {
		t.Fatalf("expected alternate %q, got %q", hFiled, alt)
	}

	altBack, err := FindAlternate(hFiled, tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if altBack != cppFile {
		t.Fatalf("expected alternate %q, got %q", cppFile, altBack)
	}
}

func TestFindAlternateFallbackNonExistent(t *testing.T) {
	tmpDir := t.TempDir()
	cppFile := filepath.Join(tmpDir, "new_class.cpp")

	alt, err := FindAlternate(cppFile, tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := filepath.Join(tmpDir, "new_class.hpp")
	if alt != expected {
		t.Fatalf("expected fallback %q, got %q", expected, alt)
	}
}

func TestFindAlternateNotCpp(t *testing.T) {
	_, err := FindAlternate("main.go", "")
	if err == nil {
		t.Fatal("expected error for Go file")
	}
}
