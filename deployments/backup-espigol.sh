#!/usr/bin/env bash
# Nightly SQLite backup: an online, consistent .backup snapshot, gzipped,
# with old backups pruned. Run as the espigol system user (see
# backup-espigol.service). Off-box copy is a separate, manual step -- see
# docs/ops/DEPLOY.md.
set -euo pipefail

ESPIGOL_HOME="${ESPIGOL_HOME:-/var/lib/espigol}"
DB="$ESPIGOL_HOME/espigol.db"
OUT_DIR="$ESPIGOL_HOME/backups"
KEEP_DAYS="${ESPIGOL_BACKUP_KEEP_DAYS:-30}"

mkdir -p "$OUT_DIR"
stamp="$(date +%Y%m%d-%H%M%S)"
tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT

sqlite3 "$DB" ".backup '$tmp'"
gzip -c "$tmp" > "$OUT_DIR/espigol-$stamp.db.gz"

find "$OUT_DIR" -name 'espigol-*.db.gz' -mtime "+$KEEP_DAYS" -delete

echo "backup written: $OUT_DIR/espigol-$stamp.db.gz"
