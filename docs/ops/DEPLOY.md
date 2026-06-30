# Deploying Espígol

This is the operator runbook for running `espigol --server` (the socis web app) and
the admin TUI (over SSH) on a single small VPS. See `deployments/` for the config
artifacts this runbook installs.

## 1. Provision the VPS

Any small Linux VPS works (1 vCPU / 512MB–1GB RAM is plenty for ~8 socis). Pick
`linux-amd64` or `linux-arm64` to match the VPS architecture in step 3.

```bash
ssh root@your-vps
useradd --system --create-home --home-dir /var/lib/espigol --shell /usr/sbin/nologin espigol
mkdir -p /var/lib/espigol/reports /var/lib/espigol/backups /etc/espigol
chown -R espigol:espigol /var/lib/espigol
chmod 750 /etc/espigol
```

## 2. Install Caddy and `sqlite3`

```bash
apt-get update
apt-get install -y caddy sqlite3
```

## 3. Build and install the binary

From your development machine:

```bash
make dist
scp dist/linux-amd64/espigol root@your-vps:/usr/local/bin/espigol   # or linux-arm64
scp deployments/backup-espigol.sh root@your-vps:/usr/local/bin/backup-espigol.sh
ssh root@your-vps 'chmod 755 /usr/local/bin/espigol /usr/local/bin/backup-espigol.sh'
```

## 4. Configuration

Copy `config.yaml`/`logo.png` (if you have a logo) into `/var/lib/espigol/`:

```bash
scp config.yaml logo.png root@your-vps:/var/lib/espigol/
ssh root@your-vps 'chown espigol:espigol /var/lib/espigol/config.yaml /var/lib/espigol/logo.png'
```

Copy and edit the env file:

```bash
scp deployments/espigol.env.example root@your-vps:/etc/espigol/espigol.env
ssh root@your-vps 'chmod 640 /etc/espigol/espigol.env && chgrp espigol /etc/espigol/espigol.env'
```
Edit `/etc/espigol/espigol.env` on the VPS: set the real domain in
`ESPIGOL_OAUTH_REDIRECT_URL`, the business name, the admin email, and (once you have
Google OAuth credentials -- step 6) the client id/secret. **Limits live in the database**
(set later via the TUI), not in this file.

## 5. DNS and Caddy

Point an A/AAAA record for your domain at the VPS IP. Then:

```bash
scp deployments/Caddyfile root@your-vps:/etc/caddy/Caddyfile
ssh root@your-vps "sed -i 's/espigol.example.org/your-real-domain.org/' /etc/caddy/Caddyfile"
ssh root@your-vps 'systemctl reload caddy'
```
Caddy will automatically obtain and renew a Let's Encrypt certificate for the domain on
first request -- no manual certbot step.

## 6. Google OAuth

In the [Google Cloud Console](https://console.cloud.google.com/), create an OAuth 2.0
client (type: Web application). Set the authorized redirect URI to:

```
https://your-real-domain.org/oauth2/callback
```

Put the resulting client ID and secret into `/etc/espigol/espigol.env`
(`ESPIGOL_OAUTH_CLIENT_ID`, `ESPIGOL_OAUTH_CLIENT_SECRET`). Leaving both empty runs the
server in **dev-login mode** (type-any-email bypass) -- do not deploy to production with
them empty.

## 7. Start the server

```bash
scp deployments/espigol-server.service root@your-vps:/etc/systemd/system/
ssh root@your-vps 'systemctl daemon-reload && systemctl enable --now espigol-server'
ssh root@your-vps 'systemctl status espigol-server'
```
The database and schema migrations are created/applied automatically on first start --
there is no separate migrate step.

## 8. Seed the first admin data (via the TUI, over SSH)

```bash
ssh root@your-vps
sudo -u espigol -E ESPIGOL_HOME=/var/lib/espigol /usr/local/bin/espigol
```
Use the TUI to create the first submission window, sections, partners, and board
authorizations. The TUI and the server share the same `/var/lib/espigol/espigol.db` --
there is only ever one authoritative database.

## 9. Enable nightly backups

```bash
scp deployments/backup-espigol.service deployments/backup-espigol.timer root@your-vps:/etc/systemd/system/
ssh root@your-vps 'systemctl daemon-reload && systemctl enable --now backup-espigol.timer'
ssh root@your-vps 'systemctl list-timers backup-espigol.timer'
```
Backups land in `/var/lib/espigol/backups/espigol-<timestamp>.db.gz`, pruned after
`ESPIGOL_BACKUP_KEEP_DAYS` (default 30).

**Off-box copy is not automated.** Copy `/var/lib/espigol/backups/` off the VPS
periodically using whatever tool you prefer (`rclone`, `rsync` to a second host, a
manual download, etc.) -- a single VPS with only local backups is not durable against
that VPS being lost entirely.

## 10. Updating / rolling back

```bash
make dist
scp dist/linux-amd64/espigol root@your-vps:/usr/local/bin/espigol.new
ssh root@your-vps 'mv /usr/local/bin/espigol /usr/local/bin/espigol.prev
                    mv /usr/local/bin/espigol.new /usr/local/bin/espigol
                    systemctl restart espigol-server
                    systemctl status espigol-server'
```
To roll back: `ssh root@your-vps 'mv /usr/local/bin/espigol.prev /usr/local/bin/espigol && systemctl restart espigol-server'`.

## 11. Troubleshooting

```bash
journalctl -u espigol-server -f          # server logs, live
journalctl -u backup-espigol.service     # last backup run's output
systemctl status backup-espigol.timer    # next/last scheduled backup
caddy validate --config /etc/caddy/Caddyfile   # check the Caddy config syntax
```
