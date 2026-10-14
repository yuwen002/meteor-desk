# MeteorDesk

**Your Local Developer Workbench.**

MeteorDesk is a lightweight, local-first desktop workbench for developers and operators. It brings large-log exploration, local process orchestration, and everyday developer utilities into one native desktop app.

## Goals

- **Local-first:** files, process output, and settings stay on the user's machine by default.
- **Large-log friendly:** stream files incrementally instead of loading an entire multi-GB file into memory.
- **Low overhead:** Go backend with the system WebView through Wails.
- **Composable:** keep log analysis, process management, and small developer utilities as separate modules.

## MVP roadmap

1. **Foundation:** Wails + Go, Vue 3 + TypeScript + Vite, dark desktop shell.
2. **Log Explorer:** streaming search, literal/regex modes, context lines, matching-line navigation, JSON pretty view.
3. **Process Manager:** saved local commands, start/stop/restart, stdout/stderr streaming, per-process status.
4. **Developer Toolkit:** timestamp conversion, JSON formatting, Base64, JWT claims inspection (decode only; no signature verification).
5. **Later:** reusable search sessions, cross-file comparison, dependency-aware service groups, optional configuration export/import.

## Tech stack

- Go
- [Wails v2](https://wails.io/) (stable track)
- Vue 3 + TypeScript + Vite
- Native system WebView (WebView2 on Windows, WebKit on macOS/Linux)

Wails v3 is actively in beta; this starter uses the stable v2 track to reduce early project risk. Re-evaluate after the first working MVP.

## Development prerequisites

Install Go, Node.js (LTS), npm, and the native build prerequisites for your platform. Follow the [Wails v2 installation guide](https://wails.io/docs/gettingstarted/installation).

## Quick start

From the repository root:

```bash
cd frontend
npm install
npm run build
cd ..
go mod tidy
wails dev
```

To build a production bundle:

```bash
cd frontend && npm run build && cd ..
wails build
```

The first frontend build creates `frontend/dist`, which is embedded into the Go executable.

## Repository layout

```text
.
├── app.go                  # Wails bindings exposed to the frontend
├── main.go                 # Desktop application entry point
├── internal/
│   ├── logx/               # Streaming log search primitives
│   └── process/            # Local process management domain
├── frontend/
│   ├── src/                # Vue application
│   └── dist/               # Generated frontend bundle (not committed)
└── docs/
    └── ROADMAP.md
```

## Project principles

- Do not read an entire log file into memory just to search it.
- Do not execute arbitrary shell text implicitly; process actions should use explicit executable/argument fields.
- Keep searches cancellable and avoid unbounded result accumulation.
- No telemetry or cloud dependency by default.
- Treat logs as potentially sensitive: avoid uploading or writing their contents to diagnostic logs.

## Status

Early scaffold. The desktop shell and module boundaries are in place; feature behavior will be added incrementally.