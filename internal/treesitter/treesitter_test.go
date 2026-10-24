package treesitter

import (
	"testing"

	"github.com/mikolas/koodieditori/internal/buffer"
)

func TestDetectLanguage(t *testing.T) {
	cases := []struct {
		path     string
		expected string
	}{
		{"main.go", LangGo},
		{"server.c", LangC},
		{"header.h", LangC},
		{"engine.cpp", LangCPP},
		{"config.hpp", LangCPP},
		{"App.java", LangJava},
		{"README.md", LangMarkdown},
		{"package.json", LangJSON},
		{"config.toml", LangTOML},
		{"deploy.yaml", LangYAML},
		{"service.yml", LangYAML},
		{"pom.xml", LangXML},
		{"build.sh", LangBash},
		{".bashrc", LangBash},
		{".zshrc", LangZsh},
		{"script.zsh", LangZsh},
		{"Makefile", LangMakefile},
		{"GNUmakefile", LangMakefile},
		{"rules.mk", LangMakefile},
		{"CMakeLists.txt", LangCMake},
		{"FindPackage.cmake", LangCMake},
		{"notes.txt", LangUnknown},
		{"go.mod", LangGo},
	}

	for _, tc := range cases {
		_, id := DetectLanguage(tc.path)
		if id != tc.expected {
			t.Errorf("for path %q: expected lang %q, got %q", tc.path, tc.expected, id)
		}
	}

	// Test content-based / shebang detection
	if _, id := DetectLanguageWithContent("script", "#!/bin/bash"); id != LangBash {
		t.Errorf("expected LangBash for shebang bash, got %q", id)
	}
	if _, id := DetectLanguageWithContent("zscript", "#!/usr/bin/env zsh"); id != LangZsh {
		t.Errorf("expected LangZsh for shebang zsh, got %q", id)
	}
	if _, id := DetectLanguageWithContent("data", "<?xml version=\"1.0\"?>"); id != LangXML {
		t.Errorf("expected LangXML for XML header, got %q", id)
	}
}

func TestHighlighterGo(t *testing.T) {
	code := `package main

// comment here
func add(a int, b int) int {
	return a + b
}
`
	lang, _ := DetectLanguage("main.go")
	hl := NewHighlighter()
	defer hl.Close()

	hl.Parse([]byte(code), lang)

	// Line 0: "package main"
	spans0 := hl.GetLineHighlights(0)
	hasPackageDecl := false
	for _, s := range spans0 {
		if s.Type == TokenDeclaration {
			hasPackageDecl = true
		}
	}
	if !hasPackageDecl {
		t.Fatal("expected TokenDeclaration on line 0 for 'package'")
	}

	// Line 2: "// comment here"
	spans2 := hl.GetLineHighlights(2)
	hasComment := false
	for _, s := range spans2 {
		if s.Type == TokenComment {
			hasComment = true
		}
	}
	if !hasComment {
		t.Fatal("expected TokenComment on line 2")
	}

	// Line 3: "func add(a int, b int) int {"
	spans3 := hl.GetLineHighlights(3)
	hasFuncDecl := false
	hasAddFunction := false
	hasType := false
	for _, s := range spans3 {
		if s.Type == TokenDeclaration {
			hasFuncDecl = true
		}
		if s.Type == TokenFunction {
			hasAddFunction = true
		}
		if s.Type == TokenTypeId {
			hasType = true
		}
	}
	if !hasFuncDecl {
		t.Fatal("expected TokenDeclaration for 'func' on line 3")
	}
	if !hasAddFunction {
		t.Fatal("expected TokenFunction for 'add' on line 3")
	}
	if !hasType {
		t.Fatal("expected TokenTypeId for 'int' on line 3")
	}

	// Line 4: "	return a + b"
	spans4 := hl.GetLineHighlights(4)
	hasReturnKeyword := false
	for _, s := range spans4 {
		if s.Type == TokenKeyword {
			hasReturnKeyword = true
		}
	}
	if !hasReturnKeyword {
		t.Fatal("expected TokenKeyword for 'return' on line 4")
	}
}

func TestHighlighterCPPAndJava(t *testing.T) {
	cppCode := `#include <iostream>
int main() {
	int count = 42;
	return 0;
}
`
	langCPP, _ := DetectLanguage("main.cpp")
	hl := NewHighlighter()
	defer hl.Close()

	hl.Parse([]byte(cppCode), langCPP)

	// Line 1: int main() {
	spans1 := hl.GetLineHighlights(1)
	hasMainFunc := false
	for _, s := range spans1 {
		if s.Type == TokenFunction {
			hasMainFunc = true
		}
	}
	if !hasMainFunc {
		t.Fatal("expected TokenFunction for 'main' in C++")
	}

	spans2 := hl.GetLineHighlights(2)
	hasNumber := false
	for _, s := range spans2 {
		if s.Type == TokenNumber {
			hasNumber = true
		}
	}
	if !hasNumber {
		t.Fatal("expected TokenNumber for '42' in C++")
	}

	javaCode := `public class Main {
	public static void main(String[] args) {
		System.out.println("Hello Java");
	}
}`
	langJava, _ := DetectLanguage("Main.java")
	hl.Parse([]byte(javaCode), langJava)
	spans0 := hl.GetLineHighlights(0)
	hasDecl := false
	for _, s := range spans0 {
		if s.Type == TokenDeclaration {
			hasDecl = true
		}
	}
	if !hasDecl {
		t.Fatal("expected TokenDeclaration in Java class definition")
	}

	// Line 1: public static void main(String[] args)
	spansJava1 := hl.GetLineHighlights(1)
	hasJavaMainFunc := false
	for _, s := range spansJava1 {
		if s.Type == TokenFunction {
			hasJavaMainFunc = true
		}
	}
	if !hasJavaMainFunc {
		t.Fatal("expected TokenFunction for Java main method")
	}
}

func TestTextObjectsASTAndFallback(t *testing.T) {
	code := `package main

func calculate() {
	numbers := []int{1, 2, 3}
	msg := "hello world"
	println(numbers, msg)
}
`
	lang, _ := DetectLanguage("calc.go")
	hl := NewHighlighter()
	defer hl.Close()

	hl.Parse([]byte(code), lang)
	pt := buffer.NewPieceTableFromString(code)

	// 1. Text object inside block: 'm i b' while inside calculate()
	cursorInBlock := buffer.Position{Line: 3, Col: 10}
	rng, ok := FindTextObject(hl.Tree(), pt, cursorInBlock, "i", "b")
	if !ok {
		t.Fatal("expected block text object to be found")
	}
	if rng.Start.Line != 2 || rng.End.Line != 6 {
		t.Fatalf("unexpected block range: %+v", rng)
	}

	// 2. Text object inside quote: 'm i q' while inside "hello world"
	cursorInQuote := buffer.Position{Line: 4, Col: 12}
	rngQ, ok := FindTextObject(hl.Tree(), pt, cursorInQuote, "i", "q")
	if !ok {
		t.Fatal("expected quote text object to be found")
	}
	if rngQ.Start.Line != 4 {
		t.Fatalf("expected quote on line 4, got %+v", rngQ)
	}

	// 3. Fallback: Buffer text object with nil tree
	plainText := "function(param1, param2) { return [a, b, c]; }"
	ptPlain := buffer.NewPieceTableFromString(plainText)

	// Inside array [a, b, c]
	cursorInArr := buffer.Position{Line: 0, Col: 38}
	rngArr, ok := FindTextObject(nil, ptPlain, cursorInArr, "i", "a")
	if !ok {
		t.Fatal("expected fallback array text object")
	}
	startOff, _ := ptPlain.PositionToOffset(rngArr.Start)
	endOff, _ := ptPlain.PositionToOffset(rngArr.End)
	sub, _ := ptPlain.Substr(startOff, endOff-startOff)
	if sub != "a, b, c" {
		t.Fatalf("expected 'a, b, c', got %q", sub)
	}

	// Around array [a, b, c]
	rngArrAround, ok := FindTextObject(nil, ptPlain, cursorInArr, "a", "a")
	if !ok {
		t.Fatal("expected fallback around array text object")
	}
	sOff, _ := ptPlain.PositionToOffset(rngArrAround.Start)
	eOff, _ := ptPlain.PositionToOffset(rngArrAround.End)
	subA, _ := ptPlain.Substr(sOff, eOff-sOff)
	if subA != "[a, b, c]" {
		t.Fatalf("expected '[a, b, c]', got %q", subA)
	}
}

func TestHighlighterExpandedLanguages(t *testing.T) {
	hl := NewHighlighter()
	defer hl.Close()

	// 1. JSON (Chroma)
	jsonCode := `{"name": "koodi", "version": 2, "active": true}`
	hl.Parse([]byte(jsonCode), nil, LangJSON, "package.json")
	spansJSON := hl.GetLineHighlights(0)
	if len(spansJSON) == 0 {
		t.Fatal("expected JSON highlight spans")
	}

	// 2. TOML (Tree-sitter)
	tomlCode := `[package]
name = "koodi"
version = "1.0.0"
`
	langTOML, _ := DetectLanguage("Cargo.toml")
	hl.Parse([]byte(tomlCode), langTOML, LangTOML, "Cargo.toml")
	spansTOML := hl.GetLineHighlights(1)
	if len(spansTOML) == 0 {
		t.Fatal("expected TOML highlight spans")
	}

	// 3. YAML (Tree-sitter)
	yamlCode := `services:
  app:
    image: golang:1.24
    ports:
      - "8080:8080"
`
	langYAML, _ := DetectLanguage("docker-compose.yaml")
	hl.Parse([]byte(yamlCode), langYAML, LangYAML, "docker-compose.yaml")
	spansYAML := hl.GetLineHighlights(0)
	if len(spansYAML) == 0 {
		t.Fatal("expected YAML highlight spans")
	}

	// 4. XML (Chroma)
	xmlCode := `<project xmlns="http://maven.apache.org/POM/4.0.0">
  <groupId>com.koodi</groupId>
  <version>1.0.0</version>
</project>`
	hl.Parse([]byte(xmlCode), nil, LangXML, "pom.xml")
	spansXML := hl.GetLineHighlights(0)
	if len(spansXML) == 0 {
		t.Fatal("expected XML highlight spans")
	}

	// 5. Bash / Zsh (Tree-sitter)
	bashCode := `#!/bin/bash
function deploy() {
    echo "Deploying..."
    for file in *.tar.gz; do
        tar -xzf "$file"
    done
}
`
	langBash, _ := DetectLanguage("deploy.sh")
	hl.Parse([]byte(bashCode), langBash, LangBash, "deploy.sh")
	spansBash := hl.GetLineHighlights(1)
	if len(spansBash) == 0 {
		t.Fatal("expected Bash highlight spans")
	}

	// 6. Makefile (Chroma)
	makeCode := `all: build test

build:
	go build -o bin/koodi ./cmd/koodi

test:
	go test ./...
`
	hl.Parse([]byte(makeCode), nil, LangMakefile, "Makefile")
	spansMake := hl.GetLineHighlights(0)
	if len(spansMake) == 0 {
		t.Fatal("expected Makefile highlight spans")
	}

	// 7. CMake (Chroma)
	cmakeCode := `cmake_minimum_required(VERSION 3.20)
project(koodi C CXX)
add_executable(koodi main.c)
`
	hl.Parse([]byte(cmakeCode), nil, LangCMake, "CMakeLists.txt")
	spansCMake := hl.GetLineHighlights(0)
	if len(spansCMake) == 0 {
		t.Fatal("expected CMake highlight spans")
	}
}

func TestTextObjectsExpandedLanguages(t *testing.T) {
	hl := NewHighlighter()
	defer hl.Close()

	// Test Bash function and compound block text objects
	bashCode := `function setup() {
    echo "Init"
    echo "Ready"
}
`
	langBash, _ := DetectLanguage("setup.sh")
	hl.Parse([]byte(bashCode), langBash, LangBash, "setup.sh")
	pt := buffer.NewPieceTableFromString(bashCode)

	// Inside block: 'm i b' while on line 1
	cursorInBlock := buffer.Position{Line: 1, Col: 6}
	rngBlock, ok := FindTextObject(hl.Tree(), pt, cursorInBlock, "i", "b")
	if !ok {
		t.Fatal("expected block text object in bash function")
	}
	if rngBlock.Start.Line > rngBlock.End.Line {
		t.Fatalf("invalid block range: %+v", rngBlock)
	}

	// Test TOML array text object
	tomlCode := `ports = [8080, 8443, 9090]
`
	langTOML, _ := DetectLanguage("config.toml")
	hl.Parse([]byte(tomlCode), langTOML, LangTOML, "config.toml")
	ptTOML := buffer.NewPieceTableFromString(tomlCode)

	cursorInArray := buffer.Position{Line: 0, Col: 12}
	rngArray, ok := FindTextObject(hl.Tree(), ptTOML, cursorInArray, "i", "a")
	if !ok {
		t.Fatal("expected array text object in toml")
	}
	startOff, _ := ptTOML.PositionToOffset(rngArray.Start)
	endOff, _ := ptTOML.PositionToOffset(rngArray.End)
	sub, _ := ptTOML.Substr(startOff, endOff-startOff)
	if sub != "8080, 8443, 9090" {
		t.Fatalf("expected inside array '8080, 8443, 9090', got %q", sub)
	}
}

func TestMarkdownHighlightingAndFencedBlocks(t *testing.T) {
	hl := NewHighlighter()
	defer hl.Close()

	md := "## Contributing\n" +
		"Please review [AGENTS.md](AGENTS.md) and [ARCHITECTURE.md](ARCHITECTURE.md).\n" +
		"1. **Branching & Commits:** Never commit to `main`/`master` (`vasmm68k_mot`).\n" +
		"```bash\n" +
		"# Build target\n" +
		"vasmm68k_mot -Fhunkexe disasm/target.asm\n" +
		"```\n"

	hl.Parse([]byte(md), nil, LangMarkdown, "README.md")

	// Line 0: heading
	spans0 := hl.GetLineHighlights(0)
	if len(spans0) == 0 || spans0[0].Type != TokenDeclaration {
		t.Fatalf("expected heading TokenDeclaration on line 0, got %+v", spans0)
	}

	// Line 1: links
	line1 := "Please review [AGENTS.md](AGENTS.md) and [ARCHITECTURE.md](ARCHITECTURE.md)."
	spans1 := hl.GetLineHighlights(1)
	foundAgentsTitle := false
	foundArchTitle := false
	for _, s := range spans1 {
		text := line1[s.StartCol:s.EndCol]
		if text == "AGENTS.md" && s.Type == TokenDeclaration {
			foundAgentsTitle = true
		}
		if text == "ARCHITECTURE.md" && s.Type == TokenDeclaration {
			foundArchTitle = true
		}
	}
	if !foundAgentsTitle || !foundArchTitle {
		t.Fatalf("expected link title spans on line 1, got %+v", spans1)
	}

	// Line 2: list marker, bold, inline code
	line2 := "1. **Branching & Commits:** Never commit to `main`/`master` (`vasmm68k_mot`)."
	spans2 := hl.GetLineHighlights(2)
	foundMain := false
	foundMaster := false
	foundVasm := false
	for _, s := range spans2 {
		text := line2[s.StartCol:s.EndCol]
		if text == "`main`" && s.Type == TokenString {
			foundMain = true
		}
		if text == "`master`" && s.Type == TokenString {
			foundMaster = true
		}
		if text == "`vasmm68k_mot`" && s.Type == TokenString {
			foundVasm = true
		}
	}
	if !foundMain || !foundMaster || !foundVasm {
		t.Fatalf("expected exact inline code spans on line 2, got %+v", spans2)
	}

	// Line 5: bash command inside fenced code block
	line5 := "vasmm68k_mot -Fhunkexe disasm/target.asm"
	spans5 := hl.GetLineHighlights(5)
	foundCmd := false
	for _, s := range spans5 {
		if line5[s.StartCol:s.EndCol] == "vasmm68k_mot" && s.Type == TokenFunction {
			foundCmd = true
		}
	}
	if !foundCmd {
		t.Fatalf("expected bash command highlight inside fenced code block on line 5, got %+v", spans5)
	}
}

func TestChromaUTF8ByteOffsets(t *testing.T) {
	hl := NewHighlighter()
	defer hl.Close()

	// Multi-byte UTF-8 characters before numeric value in JSON
	jsonLine := `{"title": "Ääkköset — test", "count": 42}`
	hl.Parse([]byte(jsonLine), nil, LangJSON, "data.json")
	spans := hl.GetLineHighlights(0)
	foundNumber := false
	for _, s := range spans {
		if s.Type == TokenNumber && jsonLine[s.StartCol:s.EndCol] == "42" {
			foundNumber = true
		}
	}
	if !foundNumber {
		t.Fatalf("expected exact byte offset for '42' after multi-byte UTF-8 string, got %+v", spans)
	}
}
