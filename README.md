# espigol

TUI and server for managing the annual subsidy budget of the Cooperativa d'Estellencs, a small agricultural cooperative on the island of Mallorca.

- `espigol` launches the admin TUI.
- `espigol --server` launches the socis web server.

Configuration and data live in `~/.config/espigol` by default, overridable with `$ESPIGOL_HOME`.

See `docs/superpowers/specs/` for the design and `docs/superpowers/plans/` for the phased implementation plans.

## Deployment

`make dist` cross-compiles static, CGO-free binaries for `linux/amd64`, `linux/arm64`,
and `darwin/arm64` into `dist/<os>-<arch>/espigol`. The typical update flow on a VPS is:

```bash
make dist
scp dist/linux-amd64/espigol root@your-vps:/usr/local/bin/espigol
ssh root@your-vps 'systemctl restart espigol-server'
```

`espigol --server` is meant to run under `systemd`, behind Caddy for TLS, with the
admin TUI run over SSH against the same database. See
[`docs/ops/DEPLOY.md`](docs/ops/DEPLOY.md) for the full provisioning runbook and
[`deployments/`](deployments/) for the systemd units, Caddy config, and backup script.
