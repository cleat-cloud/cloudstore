# cloudstore — AI Conventions

Primary reader is often an LLM agent. Prefer small greps, small modules, and headless tests.

## Rule #1: TDD is mandatory

Before writing production code:

1. Write the test (`*_test.go`)
2. Run focused: `go test ./... -v -run TestName`
3. Confirm it **fails** for the right reason
4. Write the **minimal** code to pass
5. Run `amarra-cais test`

## Clean code for agents

| Priority | Rule                                                                                                   |
| -------- | ------------------------------------------------------------------------------------------------------ |
| 1        | **Small units** — functions ~4–20 lines; files target 200–300 lines, hard cap ~500                     |
| 2        | **SRP** — one reason to change per file/package                                                        |
| 3        | **Greppable names** — unique domain nouns; avoid `data`, `handler`, `Manager`, `util` as primary names |
| 4        | **Comments = WHY** — security, SQLite, CSRF/cookie, Drive vs full HTML. No narrating WHAT              |
| 5        | **Inject deps** — handlers take `Store`, `*view.Renderer`, `cais.Config` via constructor               |
| 6        | **Early returns** — max ~2 nesting levels                                                              |
| 7        | **Errors with values** — `fmt.Errorf("...: %w", err)`                                                  |
| 8        | **Headless tests** — SQLite `:memory:`; no manual seed for unit tests                                  |

## Demo data

Seeds and fixtures come from `pkg/cais/fakedata` (`Name`, `Title`, `Email`, `Sentence`, `URL`, `Date`) — `fakedata.Seed(42)` pins a reproducible sequence for tests.

## Layout

| Path                        | Responsibility                 |
| --------------------------- | ------------------------------ |
| `cmd/server/`               | Entry point                    |
| `internal/app/`             | Bootstrap, `registerRoutes`    |
| `internal/handlers/`        | HTTP handlers (`view.Write`)   |
| `internal/store/`           | SQLite + migrations            |
| `internal/models/`          | Domain structs                 |
| `web/templates/layouts/`    | Amarra layout (`#amarra-main`) |
| `web/templates/pages/`      | HTML pages                     |
| `web/templates/components/` | App component overrides        |
| `web/static/`               | CSS, `amarra.js`, PWA          |

Patch markers (do not remove): `registerRoutes`, `Close() error`, `<!-- cais:nav -->`, `// cais:live-views`.

## Amarra HTML

Handlers render HTML via `view.Write`:

```go
view.Write(w, r, h.views, view.Page{
  Layout: "app",
  Name:   "contact",
  Data: map[string]any{
    "Title":     h.catalog.T("contact.title"),
    "Site":      meta.ForRequest(h.site, r),
    "CSRFToken": csrf.TokenFromRequest(r),
    "Flash":     flashMsg,
  },
}, h.cfg)

// Validation — same page, status 422, `.Errors` on inputs
// Flash on redirect — cais cookie API only
flash.Set(w, "notice", "Saved!", cfg.CookieSecure())
http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
```

Pages define a content block plus kit tags `<.form>` / `<.input>` / `<.button>` / `<.flash />` / `<.locale-toggle />`.
Kit attributes interpolate: `<.stat label="Potência" value="{{ .Power }} kWp" />` renders `5 kWp`; a control action in an attribute value fails at boot.
Templates load once at boot (`view.Load`): `layouts/*.html`; `pages/*.html` plus `pages/*/*.html` — `pages/blog/post.html` is `view.Page{Name: "blog/post"}`; `partials/*.html` and `components/*.html` are **flat only** (a nested partial never loads), and an unknown `<.x>` fails at boot.
Designed 404: register `r.NotFound(handler)` in `internal/app/routes.go` — it also serves path params that fail to parse (`IntParam`, `StringParam`); render with `writeView(..., http.StatusNotFound)`.
Drive morphs `#amarra-main` by default — plain links/forms and `linkTo` need no attribute; opt out with `data-amarra-skip` (#31). Do not check `HX-Request`.
Fullbleed pages (own header/footer/sidebar, e.g. marketing landing): Drive only swaps `#amarra-main`, so page-owned chrome outside it goes stale on Drive navigation — keep that chrome **inside** `#amarra-main` (wrap the whole page; the layout emits no chrome for those pages via `ActiveNav`/a second layout, #66). Inline `<script>` inside the morph runs. Escape hatch: `data-amarra-skip` (#31).
Inline page scripts re-run on **every** Drive morph into that page (#84): never top-level `let`/`const` (second visit throws `Identifier has already been declared` and the page goes dead) — wrap in an IIFE and expose handlers via `window.x = x`. Never `addEventListener` on morphed nodes (the listener stays on the orphaned node after the next morph) — use event delegation on `document`, registered once behind a `window` flag, resolving targets at click time.
Password fields: `<.password name="password" />` kit (input + eye toggle wired to `amarra-hook="password"`; `fieldPassword` stays as the Go form-builder path).
Shipped hooks: `bulk` (select-all: `amarra-hook="bulk"` + `data-amarra-bulk-all`/`-row`/`-bar`), `clipboard`, `dialog` (native `<dialog>` via `amarra-hook="dialog"` + `data-amarra-dialog-open`/`-target`/`-close`; `<.modal>` renders the target), `dropdown` (row actions: `amarra-hook="dropdown"` + `data-amarra-dropdown-button`/`-menu`), `nav` (re-sync active link after Drive morph: `amarra-hook="nav"` + `data-amarra-nav-on`/`-off` on the container), `password`, `reveal` (client show/hide, no Drive round-trip), `sidebar` (`amarra-hook="sidebar"` on the drawer button + `data-amarra-sidebar-target="#amarra-nav"`: toggles `data-amarra-sidebar-open` on the panel, `aria-expanded`, Esc and outside-click close), `theme` (`html.light` + `localStorage["amarra-theme"]`, per-element via `data-amarra-theme-key` / `-class` / `-color` / `-on-label` / `-off-label`).
Theme FOUC snippet belongs in the layout `<head>` before CSS:

```html
<script>
  try {
    if (localStorage.getItem("amarra-theme") === "light")
      document.documentElement.classList.add("light");
  } catch (e) {}
</script>
```

Parse bodies with `httpx.ParseFormOrJSON`.

## Auth, CSRF, flash

- Session middleware: `LoadSession` + `Flash` + `CSRF(cfg)`
- Protect routes: `middleware.RequireAuth("/login")` / `RequireAuthFunc`
- CSRF: double-submit cookie `cais_csrf` + form field or `X-CSRF-Token`
- Flash: **only** `flash.Set` + read via `flash.MessageFromRequest`
- Dev demo user (when seeded): `demo@example.com` / `password`

## New page / resource

```bash
amarra-cais g handler settings     # handler + test + web/templates/pages/settings.html + route
amarra-cais g page about           # HTML page only
amarra-cais g component input      # override a kit component with its shipped markup (--list shows them)
amarra-cais g resource bookmark --fields title:string,url:url,notes:text?
amarra-cais g model tag --fields name:string
amarra-cais g migration add_notes
amarra-cais g live counter         # WebSocket Live view + /live/counter
amarra-cais g stream chat --live   # chat with Live WS (SSE is the default)
amarra-cais g auth                 # if app was --blank/--minimal
amarra-cais db migrate
```

Or by hand:

1. Go test in `internal/handlers/`
2. HTML page in `web/templates/pages/`
3. Handler + route in `internal/app/routes.go`

## Commands

```bash
amarra-cais install          # npm + go mod tidy (+ Tailwind build)
amarra-cais dev              # air + tailwind watch
amarra-cais test             # go test ./...
make ci                     # test + lint + format-check
amarra-cais doctor [--mobile]
amarra-cais routes
amarra-cais db migrate | status | rollback | seed
amarra-cais jobs work | status
```

`GET /jobs` — localhost queue dashboard (heartbeats, retry/discard, prune, `?kind=`). Production: SSH tunnel.

## Do not

- Parse templates per request (`view.Load` once at boot)
- Use inline CSS (Tailwind classes)
- Mock the database (use SQLite `:memory:`)
- Grow files past ~500 lines without splitting
- Ship features without a headless test
