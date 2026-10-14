# MeteorDesk Roadmap

The roadmap is intentionally MVP-first. Prefer a narrow, reliable workflow over adding many half-finished utilities.

## Phase 0 — Project foundation

- [x] Initialize Wails v2 + Go module.
- [x] Add Vue 3 + TypeScript + Vite frontend.
- [x] Add the dark desktop shell and navigation.
- [x] Add a basic streaming file-search implementation and unit tests.
- [x] Create a domain boundary for process management.
- [ ] Run the full build on Windows, macOS, and Linux.
- [ ] Set up CI for Go tests and frontend type-check/build.

## Phase 1 — Log Explorer

- [x] Native file picker.
- [x] Literal and Go regular-expression search.
- [x] Line numbers and bounded match previews.
- [ ] Chunk-safe matching for exceptionally long lines.
- [ ] Cancellation and progress updates for long scans.
- [ ] Better file encoding handling.
- [ ] Context lines and next/previous match navigation.
- [ ] Virtualized results and incremental result events.
- [ ] JSON / NDJSON pretty view and field filtering.
- [ ] Search history and saved queries.

## Phase 2 — Process Manager

- [ ] Saved process profiles (executable, args, working directory, environment).
- [ ] Start, stop, restart, and lifecycle state.
- [ ] Separate stdout/stderr streaming per process.
- [ ] Batch actions and dependency-aware startup groups.
- [ ] Graceful shutdown and OS-specific process-tree termination.
- [ ] Environment value masking in UI.

## Phase 3 — Developer Toolkit

- [ ] Timestamp and timezone conversion.
- [ ] JSON formatting, minifying, and diff.
- [ ] Base64 encode/decode.
- [ ] JWT claims inspection (decode only; no signature verification).
- [ ] URL and query-string utilities.

## Engineering principles

- Local-first, no telemetry by default.
- Never load an entire large file just to search it.
- Bound result counts and UI preview sizes.
- Keep potentially expensive work cancellable.
- Do not interpret arbitrary command strings as shell scripts implicitly.
- Never send log content to remote services.
- Document platform-specific behavior.
