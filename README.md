# MeteorDesk

**Your Local Developer Workbench.**

MeteorDesk is a lightweight, local-first desktop workbench for developers and operators. It brings large-log exploration, local process orchestration, and everyday developer utilities into one native desktop app.

## Goals

- **Local-first:** files, process output, and settings stay on your machine by default.
- **Large-log friendly:** scan sequentially instead of loading a multi-GB file wholesale.
- **Low overhead:** Go backend with the system WebView through Wails.
- **Composable:** keep log analysis, process management, and developer utilities as separate modules.

## MVP

1. **Log Explorer:** streaming literal/regex search, line numbers, bounded previews and native file picker.
2. **Process Manager:** save local commands, start/stop services and inspect stdout/stderr.
3. **Dev Toolkit:** timestamp conversion, JSON, Base64 and JWT claims inspection.
4. **Later:** log comparison/indexing, dependency-aware startup groups, optional config export/import.

## Tech stack

- Go
- [Wails v2](https://wails.io/)
- Vue 3 + TypeScript + Vite
- Native system WebView (WebView2 on Windows, WebKit on macOS/Linux)

## Prerequisites

Install Go 1.23+, Node.js LTS, npm, and the native prerequisites for your OS. Follow the [Wails v2 installation guide](https://wails.io/docs/gettingstarted/installation).

## Quick start

```bash
# Build the Vue frontend once so it can be embedded.
cd frontend
npm install
npm run build
cd ..

# Download Go dependencies and run the desktop app.
go mod tidy
wails dev
```

Production build:

```bash
cd frontend && npm install && npm run build && cd ..
wails build
```

## Layout

```text
.
├── app.go
├── main.go
├── internal/
│   ├── logx/       # Streaming search primitives + tests
│   └── process/    # Process manager domain types
├── frontend/
│   ├── src/        # Vue application
│   └── dist/       # Embedded production assets (generated)
└── docs/ROADMAP.md
```

## Notes

- Search results are capped to prevent unbounded UI memory growth.
- Current search reads sequentially, but a single exceptionally long line can temporarily allocate memory while it is assembled; chunk-safe matching is a planned optimization.
- No log content is intentionally uploaded or sent to a remote service. UI assets do not require remote fonts or telemetry.

## Status

Early scaffold: the desktop shell and first log-search slice are present. Process orchestration and the developer toolkit are not implemented yet.
