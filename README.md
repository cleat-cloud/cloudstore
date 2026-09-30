# cloudstore

Full-stack Go app built with [Amarra](https://github.com/puppe1990/amarra-cais): HTML templates, Tailwind, and SQLite.

## Stack

- Go 1.26 (net/http stdlib) + Amarra Views + Drive
- HTML pages (`web/templates/pages/`) + `view.Write` + `amarra.js`
- Tailwind CSS 3.x
- SQLite (modernc.org/sqlite, no CGO)

## Quick start

```bash
amarra-cais install  # npm install + go mod tidy
amarra-cais dev        # http://localhost:8080
amarra-cais test       # full test suite
amarra-cais build      # bin/server
```

## Amarra CLI

This app was scaffolded with the Amarra CLI. Useful commands:

```bash
amarra-cais install               # npm install + go mod tidy
amarra-cais css                   # build Tailwind
amarra-cais dev                   # air + tailwind watch
amarra-cais server                # go run ./cmd/server
amarra-cais console               # interactive Go REPL + SQL
amarra-cais g handler <name>      # handler + test + page template
amarra-cais g resource <name>     # model + migration + admin CRUD
amarra-cais g page <name>         # page template only
amarra-cais g migration <name>    # SQL migration file
amarra-cais test                  # go test ./...
amarra-cais doctor                # verify setup
```

## CI and pre-commit

GitHub Actions runs Go tests, `golangci-lint`, Prettier, and `npm test` on every push/PR to `main`.

```bash
make pre-commit-install   # once: installs git hooks
make ci                   # test + lint + format-check locally
```

Pre-commit hooks run: trailing whitespace, Prettier, `goimports`, `go test`, `golangci-lint`, and `npm test`.
(Install goimports once: `go install golang.org/x/tools/cmd/goimports@latest`.)

## Structure

```
AGENTS.md          → conventions for LLM/coding agents
pkg/cais/          → framework (via dependency)
internal/app/      → bootstrap and routes
internal/handlers/ → HTTP handlers (view.Write)
internal/store/    → SQLite + migrations
web/templates/layouts/ → Amarra layout
web/templates/pages/   → HTML pages
web/static/            → CSS + amarra.js + PWA
cmd/server/        → entry point
```

See [AGENTS.md](AGENTS.md) for TDD, Amarra views, flash/CSRF, and generator conventions.

## Templates

`view.Load` parses `web/templates/` once at boot:

- `layouts/*.html` — one layout per file; `view.Page{Layout: "app"}` selects it
- `pages/*.html` and `pages/*/*.html` — the name is the path under `pages/` without `.html`, so `pages/blog/post.html` renders as `view.Page{Name: "blog/post"}`
- `partials/*.html` — flat only; a nested partial never loads. A partial declares a named template that pages invoke
- `components/*.html` — flat only; overrides the shipped kit component with the same file name; an unknown `<.x>` tag fails at boot

## Demo data

Seeds and fixtures can use `github.com/puppe1990/amarra-cais/pkg/cais/fakedata` (`Name`, `Title`, `Email`, `Sentence`, `URL`, `Date`); `fakedata.Seed(42)` makes values reproducible in tests.

## Environment variables

| Variable | Default       | Description      |
| -------- | ------------- | ---------------- |
| PORT     | :8080         | Server port      |
| DB_PATH  | ./data/app.db | SQLite file path |
| ENV      | development   | Environment      |

Health check: GET /health → {"status":"ok"}

Jobs dashboard: GET /jobs (localhost only) — queue counts, failed retry/discard.

## Testing on phone (LAN)

1. Run `amarra-cais dev` and note the **LAN** URL printed at boot (e.g. `http://192.168.1.10:8080`).
2. Open that URL in mobile Safari/Chrome on the same Wi‑Fi.
3. After template or SSE changes, run `amarra-cais pwa --bump` and reinstall the PWA (or clear site data) so the service worker cache refreshes.
4. Run `amarra-cais doctor --mobile` to catch flash markup, font CSP, and SW cache issues.

## Brand assets

Replace the scaffold placeholders before sharing the site: `web/static/favicon.svg`, `web/static/icons/icon.png`, `icon-192.png`, `icon-512.png`, the padded `icon-512-maskable.png`, and the 1200x630 `web/static/og.png`. `amarra-cais doctor` warns while they are still the defaults.
