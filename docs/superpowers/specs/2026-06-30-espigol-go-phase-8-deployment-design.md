# Espígol (Go) — Phase 8: Deployment — Design

**Status:** Approved for implementation · **Date:** 2026-06-30

Final phase of the Espígol Go rewrite roadmap. Deployment artifacts and operator
documentation for running `espigol --server` (socis web) and `espigol` (admin TUI, over
SSH) on a small VPS, plus the cross-compile build target that produces the deployable
binaries. Parent: `docs/superpowers/specs/2026-06-26-espigol-go-overview-design.md` (§9.2,
§9.3, §10 item 8).

**No application code changes** beyond a small `--version` flag. Phase 8 is config/docs
artifacts: a `make dist` cross-compile target, systemd units, a Caddy reverse-proxy
config, a nightly local SQLite backup (timer-driven), and an operator runbook
(`docs/ops/DEPLOY.md`) plus a short Deployment section in the top-level `README.md`.

---

## 1. Decisions (resolved during brainstorming)

| Decision | Choice |
|---|---|
| Scope | **Config artifacts + runbook + make targets.** No CI/CD pipeline, no deploy automation beyond local scripts/targets — the operator runs `make dist`, scps the binary, and `systemctl restart`s it by hand, per the runbook. |
| Build targets | **`linux/amd64`, `linux/arm64` (VPS), `darwin/arm64`** (local dev/TUI on Apple Silicon) — `make dist` builds all three, each as `dist/<os>-<arch>/espigol`, plus `dist/SHA256SUMS`. |
| Backup off-box copy | **Local-only**, with the off-box copy documented as a manual/operator-choice step in the runbook (no rclone/rsync baked into the script). |
| Domain in Caddy/OAuth | **Placeholder variable**, documented (`espigol.example.org`) — no real production domain committed to the repo. |
| Artifact layout | `deployments/` holds the systemd units, Caddyfile, backup script/timer, and env-file template together (not split into `init/` per golang-standards/project-layout) since they're one deployment story. |

---

## 2. Repo layout & artifacts

```
deployments/
├── espigol-server.service      # systemd unit: espigol --server
├── backup-espigol.service      # systemd oneshot: runs the backup script
├── backup-espigol.timer        # nightly trigger (OnCalendar, Persistent=true)
├── backup-espigol.sh           # sqlite3 .backup -> gzip -> prune
├── Caddyfile                   # TLS reverse-proxy (placeholder domain)
└── espigol.env.example         # EnvironmentFile template
docs/ops/
└── DEPLOY.md                   # the operator runbook
dist/                           # (gitignored) make-dist output
└── {linux-amd64,linux-arm64,darwin-arm64}/espigol  + SHA256SUMS
Makefile                        # + `dist` (and per-target) build targets
cmd/espigol/main.go             # + `--version` flag
README.md                       # + a short "Deployment" section
```

- `dist/` is build output only; added to `.gitignore`.
- No new application packages; the only source touch is `--version` (see §4).

---

## 3. On-box layout, systemd & Caddy

**On-box data model** (shared by the server and the admin TUI — one authoritative DB):

- Binary: `/usr/local/bin/espigol`.
- Dedicated system user `espigol` (no login shell).
- `ESPIGOL_HOME=/var/lib/espigol/` → `espigol.db`, `config.yaml`, `logo.png`, `reports/`,
  `backups/`.
- Secrets/env: `/etc/espigol/espigol.env` (root-owned, mode `0640`, group `espigol`).

**`deployments/espigol-server.service`** (`espigol --server`):

```ini
[Unit]
Description=Espigol socis web server
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=espigol
Group=espigol
EnvironmentFile=/etc/espigol/espigol.env
Environment=ESPIGOL_HOME=/var/lib/espigol
ExecStart=/usr/local/bin/espigol --server
Restart=on-failure
RestartSec=5
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true
ReadWritePaths=/var/lib/espigol

[Install]
WantedBy=multi-user.target
```

Migrations run automatically on process start (`db.Open` runs goose) — no separate
migrate step; first boot creates the schema. Seeding the first admin data (partners,
sections, a window) is done via the TUI, documented in the runbook.

**`deployments/Caddyfile`** (placeholder domain):

```
espigol.example.org {
    reverse_proxy localhost:8080
}
```

Caddy auto-provisions Let's Encrypt TLS; the espigol server stays bound to
`localhost:<port>` (only Caddy is publicly reachable). The runbook ties the chosen
domain to the Google OAuth redirect URL (`https://<domain>/oauth2/callback`) and the
`ESPIGOL_OAUTH_REDIRECT_URL` env var.

**TUI over SSH:** the admin runs the TUI as the `espigol` user against the same
`/var/lib/espigol/espigol.db`, e.g.:

```
sudo -u espigol -E ESPIGOL_HOME=/var/lib/espigol /usr/local/bin/espigol
```

So the server (socis) and TUI (admin) share one authoritative database, never two.

---

## 4. Build & versioning

- **`make dist`** cross-compiles all three targets into `dist/<os>-<arch>/espigol`:
  `linux/amd64`, `linux/arm64` (VPS targets), `darwin/arm64` (local dev/TUI on Apple
  Silicon). Each build: `CGO_ENABLED=0` (modernc pure-Go SQLite → static binary, no libc
  dependency), `-trimpath`, `-ldflags "-s -w -X main.version=$(VERSION)"`. Emits
  `dist/SHA256SUMS` covering all three binaries (`sha256sum` in the same `dist/` tree).
  Per-target sub-targets (`dist-linux-amd64`, `dist-linux-arm64`, `dist-darwin-arm64`)
  let the operator build just one.
- **`VERSION`** = `git describe --tags --always --dirty` (Makefile variable), threaded via
  `-ldflags -X`. `cmd/espigol` gains a package-level `var version = "dev"` (default for a
  plain `go build`) and a `--version` flag handled by `app.ParseMode`/`main`: prints
  `espigol <version>` and exits 0 before any config/DB work.
- Existing Make targets (`build`, `run`, `test`, `fmt`, `tidy`, `sqlc-generate`, `tui`,
  `server`) are unchanged; `dist` and a `clean` (removing `dist/`) are added.

---

## 5. Backup

**`deployments/backup-espigol.sh`** (runs as the `espigol` user):

```sh
#!/usr/bin/env bash
set -euo pipefail
DB="${ESPIGOL_HOME:-/var/lib/espigol}/espigol.db"
OUT_DIR="${ESPIGOL_HOME:-/var/lib/espigol}/backups"
KEEP_DAYS="${ESPIGOL_BACKUP_KEEP_DAYS:-30}"
mkdir -p "$OUT_DIR"
stamp="$(date +%Y%m%d-%H%M%S)"
tmp="$(mktemp)"
sqlite3 "$DB" ".backup '$tmp'"
gzip -c "$tmp" > "$OUT_DIR/espigol-$stamp.db.gz"
rm -f "$tmp"
find "$OUT_DIR" -name 'espigol-*.db.gz' -mtime "+$KEEP_DAYS" -delete
```

`.backup` takes an online, consistent snapshot (safe against a running server).
`deployments/backup-espigol.timer` fires nightly (`OnCalendar=*-*-* 03:30:00`,
`Persistent=true` so a missed run catches up after downtime) and triggers
`backup-espigol.service` (a `Type=oneshot` running the script as `espigol`).

**Off-box copy is a documented manual step** in `docs/ops/DEPLOY.md` (operator's choice
of rclone/rsync/cloud storage/etc.) — intentionally not baked into the script.

---

## 6. Docs

- **`docs/ops/DEPLOY.md`** — the full runbook: provision the VPS → create the `espigol`
  system user + `/var/lib/espigol` + `/etc/espigol` → `make dist` + scp the right
  `dist/<os>-<arch>/espigol` to `/usr/local/bin/` → place `config.yaml`/`logo.png` +
  `espigol.env` (from `espigol.env.example`) → DNS for the chosen domain → install Caddy +
  the `Caddyfile` → Google Cloud Console OAuth client + redirect URL → enable/start
  `espigol-server.service` (migrations run automatically) → enable
  `backup-espigol.timer` → run the TUI over SSH to seed the first admin data → **update /
  rollback** (scp the new binary alongside the old one, `systemctl restart`, keep the
  previous binary for rollback) → the off-box backup step → troubleshooting
  (`journalctl -u espigol-server -f`, `systemctl status backup-espigol.timer`).
- **`README.md`** — a new **Deployment** section: a short quick-start (the `make dist` →
  scp → `systemctl restart` flow, the on-box layout at a glance) linking to
  `docs/ops/DEPLOY.md` for the full procedure.

---

## 7. Testing / validation

Deployment artifacts aren't unit-testable in the usual sense; validate what's mechanically
checkable, added to CI as a `dist` job alongside the existing test job:

- `make dist` builds all three binaries; assert each exists and is the right
  OS/arch (`file` or `go version -m` on the output).
- `shellcheck deployments/backup-espigol.sh`.
- `systemd-analyze verify` on the three unit files and `caddy validate` on the
  `Caddyfile`, **run only when those tools are available** on the runner (skip
  gracefully otherwise — they're not part of the Go toolchain).
- A small Go test for `--version`: running the binary (or calling the flag-handling
  function directly) prints the version string and exits 0 without touching config/DB.

---

## 8. Scope

**In:** `make dist` (3 targets) + checksums; `--version` flag; the five `deployments/`
artifacts (server unit, backup oneshot + timer + script, Caddyfile, env-file template);
`docs/ops/DEPLOY.md`; a README Deployment section; CI `dist` job + artifact validation.

**Out:** CI/CD release automation, container images, off-box backup tooling, a real
production domain in the repo, auto-close scheduler/notifications (already out of scope
for v1 per the overview).

---

## 9. References

- Overview: `docs/superpowers/specs/2026-06-26-espigol-go-overview-design.md` (§9.2 build,
  §9.3 deployment, §10 item 8).
- Phase 6: `internal/wire.Server` (the `--server` entrypoint this unit runs).
- Phase 7: `internal/wire.TUI` (the entrypoint the SSH runbook step runs).
