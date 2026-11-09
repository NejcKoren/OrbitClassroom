# Orbit Classroom – developer notes

## Razvoj brez Windows / Developing without Windows

```
go run ./cmd/master --no-window --dir /tmp/m
ORBIT_AGENT_DIR=/tmp/a go run ./cmd/agent --portable --config /tmp/a/agent.json
go test ./... && python3 tools/checki18n/check.py
```

Na Linuxu sta oba programa z lažnim zaslonom in brez grafičnih elementov.
Environment variables: `ORBIT_DEBUG_DUMP=dir` (save demo frames), `ORBIT_AUTO_ANSWER=n` (auto-answer polls), `ORBIT_TEST_HAND=1` (raise a hand), `ORBIT_LANG`, `ORBIT_TEST_W/H`.

## Layout

| Path | |
|---|---|
| `cmd/master`, `cmd/agent` | entry points |
| `internal/proto` | protocol: keys & login, delta-tile JPEG stream, discovery |
| `internal/agent` | agent API, commands, installer, student texts |
| `internal/master` | master: connections, local API, demo broadcaster, discovery, WOL |
| `internal/master/web` | UI (vanilla JS), `i18n/*.json`, `docs/*.md` |
| `internal/sys`, `internal/win`, `internal/screen` | Win32 (GDI, SendInput, lock, demo, cards, tray); stubs for Linux |
| `tools/` | icon generator, key generator, i18n checker |

## Checks before a release

`go test ./...`, `python3 tools/checki18n/check.py`, `./build.sh`.
