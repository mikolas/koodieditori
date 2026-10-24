# Agent Guidelines & Engineering Contract: Koodieditori

![Koodieditori — The Terminal Code Editor](docs/logo.jpeg)

This document defines the strict architectural rules, coding standards, and operational guidelines for **AI coding agents** working on the `koodieditori` codebase. All agents must read and strictly adhere to these rules before making any code modifications.

---

## 1. Absolute Prohibitions (Hard Constraints)

1. **NO Web Technologies:** Absolutely zero Electron, Tauri, Node.js, WebAssembly, HTML, CSS, or browser-based runtimes.
2. **NO Python, NO Rust:** The editor is written in **pure Go (1.22+)** with standard CGo allowed only for native C libraries (e.g. Tree-sitter).
3. **NO AltGr or Dead Keys for Core Bindings:** Never introduce keybindings that require `AltGr` combinations (e.g., `[`, `]`, `{`, `}`, `\`, `|`) or dead keys (`` ` ``, `~`, `^`). All bindings must be functional and ergonomic on **Scandinavian/Nordic ISO laptop keyboards**. Consult [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) and [docs/USER_MANUAL.md](docs/USER_MANUAL.md).
4. **NO Embedded Scripting Engine Bloat:** Do not attempt to embed Lua, Python, or JS engines into the binary. Scripting and automation are handled exclusively via the **Unix Domain Socket IPC API** (`koodi-cli`).
5. **NO Persistent Terminal Splits:** Do not add permanent terminal panes that waste screen space. All output/task views must be ephemeral drawers or popovers that dismiss with a single key (`Esc` or `q`).
6. **NO `git status` Execution:** Never run `git status` or `git status -s`. Always use `git diff --stat`, `git branch`, or `git ls-files` instead.
7. **NO Unsafe Overwrite / Data Loss:** All buffer writes must be atomic via temporary file + `fsync` + atomic `os.Rename`. Never discard dirty edits without explicit user confirmation (`Space b d` warns on dirty; `Space b D` force closes; tree item deletion requires pressing `d` twice within 3 seconds).

---

## 2. Architecture & Code Organization

- **`internal/buffer/`:** All text buffer operations belong here. 
  - Never manipulate raw strings directly in UI models.
  - Every buffer modification must route through `PieceTable.Insert()` or `PieceTable.Delete()`.
  - Group multi-step user edits (like typing a word or batch formatting) inside `BeginTransaction()` and `CommitTransaction()` for clean undo/redo history.
- **`internal/fsm/`:** The Finite State Machine controls all interaction states (`NORMAL`, `INSERT`, `SELECT`, `LEADER`, `MATCH`, `DRAWER`, `PICKER`, `TREE`, `HOVER`, `COMPLETION`, `ACTION`, `RENAME`, `SEARCH`, `HELP`). 
  - Every keyboard or mouse event must be dispatched deterministically according to [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).
  - Dynamic user key remaps from TOML (`[keys.<mode>]`) are parsed via `fsm.ParseActionType` and dispatched before falling back to default handlers.
  - Never hardcode state logic directly inside view renderers.
- **`internal/ui/`:** Bubble Tea models and components.
  - Follow the Elm architecture (`Init`, `Update`, `View`).
  - View rendering must return deterministic string layouts using box-drawing characters and Lip Gloss styles.
  - Viewport rendering separates visual display column progression from rune indices to handle wide characters (CJK, emojis) and tabs (`\t`) cleanly.
- **`internal/config/`:** Configuration loader, TOML schema (`config.toml` and `koodi.toml`), defaults, and key remaps.
- **`internal/treesitter/`:** Native Tree-sitter AST parsers and Chroma high-speed lexers for polyglot syntax highlighting and semantic text objects.
- **`internal/tooling/`:** Compiler/linter regex parsers (GCC, Clang, Javac, Go, ShellCheck, CMake, etc.), alternate file switcher, and command runner.
- **`internal/markdown/`:** Glamour markdown rendering, Unicode box-art and Kitty graphics Mermaid rendering, checklist conceal/toggle.
- **`internal/diff/`:** Unified diff computation, hunk parsing, and agent batch review management.
- **`internal/search/`:** In-buffer and project search & replace engine with regex capture groups and ripgrep integration (live grep requires >=2 query characters to prevent UI stutter).
- **`internal/lsp/`:** Zero-dependency JSON-RPC 2.0 LSP client (diagnostics, hover, completion, definition, code actions, rename).
  - All position coordinates (`Position.Character`) must strictly use UTF-16 code unit conversions (`RuneColToUTF16Col` / `UTF16ColToRuneCol`).
  - JSON-RPC request/response IDs support both integer and string (UUID) identifiers.
- **`internal/dap/`:** Debug Adapter Protocol client, breakpoint management, stack frames, variable scopes, and inspector drawer.
- **`internal/watcher/`:** File system inotify watcher.
  - Must filter out heavy build output folders (`target/`, `build/`, `.gradle/`, `.ccls-cache`, `node_modules/`, `.git/`, `.cache/`).
  - All disk change events must be debounced (minimum 50ms) to prevent event spam.
  - External file reloads must never clobber locally dirty buffers without user confirmation.
- **`internal/ipc/`:** Unix domain socket server and protocol handlers.
  - Socket path: `$XDG_RUNTIME_DIR/koodi/koodi-<session>.sock`.

---

## 3. Keyboard & Nordic Ergonomics Rules

- Standard text objects must use mnemonic letters, NOT braces/brackets:
  - `m i b`: Inside block (`{}`) / Shell compound block / YAML indented mapping
  - `m i a`: Inside array/bracket (`[]`) / TOML inline array / YAML sequence
  - `m i p`: Inside parens (`()`) / CMake arguments / subshell substitution / Makefile variable
  - `m i q`: Inside quote (`""`, `''`)
  - `m i f`: Inside function / Makefile rule recipe
  - `m i c`: Inside class/struct / TOML table section
- Quick stepping on Nordic ISO keyboards:
  - `ä` / `ö`: Quick step forward / backward between diagnostics or diff hunks
- Diagnostic and diff stepping must use space leader chords:
  - `Space n e` / `Space p e`: Next / Previous build error
  - `Space n d` / `Space p d`: Next / Previous LSP diagnostic
  - `Space n h` / `Space p h`: Next / Previous diff hunk

---

## 4. Concurrency & Performance Standards

1. **Thread Safety:** The buffer and shared state structures are accessed concurrently by the UI loop, LSP client, DAP debugger, inotify watcher, and IPC socket goroutines. All shared data must be protected using `sync.RWMutex` or Go channels.
2. **Race Detection:** Every PR or task must pass `go test -race ./...` with zero data races.
3. **Sub-20ms Startup:** Avoid heavy synchronous I/O during editor startup. Defer language server spawning, git status scanning, and tree building to background goroutines with Bubble Tea command messages (`tea.Cmd`).
4. **UTF-8 & Rune Safety:** Never assume 1 byte = 1 character. Always account for multibyte UTF-8 characters (`ä`, `ö`, `å`, emojis) when calculating column coordinates and character lengths. Use `utf8.RuneCount` and `lineindex.go` conversions.

---

## 5. Development Workflow for Agents

1. **Feature Branches:** Always create and work on dedicated feature branches for each milestone or task (e.g. `feature/...`).
2. **Test-Driven:** Write unit tests alongside implementation. Do not mark a task complete without passing unit tests.
3. **Commit Messages:** Use conventional commit messages:
   - `feat(...)`: New capability
   - `fix(...)`: Bug fix
   - `refactor(...)`: Code cleanup
   - `docs(...)`: Documentation changes
   - `test(...)`: Unit/integration tests
