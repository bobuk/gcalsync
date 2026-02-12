# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

gcalsync is a Go CLI tool that synchronizes events across multiple Google Calendar accounts by creating "blocker" events (prefixed with "O_o") in other calendars to prevent double-booking.

## Build & Run

```bash
go build                     # produces ./gcalsync binary
./gcalsync add               # add a calendar to sync
./gcalsync sync              # synchronize calendars
./gcalsync desync            # remove all blocker events
./gcalsync cleanup           # clean blocker events from calendars
./gcalsync list              # list synchronized calendars
```

No tests or linting are configured. The project uses Go 1.22.1 with CGO (sqlite3 dependency).

## Architecture

All source files are in the `main` package at the repository root — there are no sub-packages.

| File | Purpose |
|------|---------|
| `main.go` | Command dispatcher — routes to add/sync/desync/cleanup/list |
| `common.go` | Config loading (TOML), OAuth2 flow, SQLite helpers, token management |
| `sync.go` | Core sync logic: fetches events, creates/updates blocker events across calendars |
| `add.go` | Registers a new calendar with sync mode (validates access via API, stores in DB) |
| `desync.go` | Deletes all blocker events from Google Calendar and DB |
| `cleanup.go` | Scans calendars for "O_o" events and removes them |
| `list.go` | Lists registered calendars with blocker event counts |
| `dbinit.go` | SQLite schema creation and versioned migrations (v0→v5) |

### Sync Flow

1. For each registered account, authenticate via OAuth2 (tokens stored as JSON in SQLite)
2. Fetch events from current + next month for each calendar
3. Filter out: existing blockers ("O_o" prefix), `workingLocation` events, optionally birthdays
4. For each real event in calendar X (mode=`read` or `both`), create/update a blocker in every other calendar with mode=`write` or `both`
5. Blocker events preserve timing, description, response status, visibility, and reminder settings
6. Clean up blockers whose original events were deleted/cancelled

### Database (SQLite)

Schema in `dbinit.go` with progressive migrations. Key tables:
- `tokens` — OAuth2 tokens per account
- `calendars` — registered (account_name, calendar_id, mode) tuples; mode is `read`, `write`, or `both`
- `blocker_events` — tracks created blockers with origin event IDs, calendar IDs, response status

### Configuration

Loaded from `.gcalsync.toml` in cwd or `~/.config/gcalsync/`. Sectioned format (`[general]` + `[google]`). Old flat format is auto-migrated. See `backup.gcalsync.toml` for template.

Key settings: `disable_reminders`, `block_event_visibility`, `authorized_ports` (OAuth callback), `verbosity_level` (1-3), `ignore_birthdays`. Per-calendar sync mode (`read`/`write`/`both`) is stored in the database, not in the config file.

## Key Dependencies

- `github.com/BurntSushi/toml` — config parsing
- `github.com/mattn/go-sqlite3` — database (requires CGO)
- `golang.org/x/oauth2` + `google.golang.org/api` — Google Calendar API
