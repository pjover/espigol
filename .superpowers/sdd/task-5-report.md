# Task 5: `backup` Adapter — TDD Report

## Status
✅ COMPLETE — All tests passing, committed to `admin-panel`.

## Commits
- **fa51deb** `feat(backup): VACUUM INTO snapshots and staged restore`

## TDD Evidence

### RED Phase
Tests written and verified to fail before implementation:
```
$ go test ./internal/adapters/persistence/backup/ -v
github.com/pjover/espigol/internal/adapters/persistence/backup: 
  no non-test Go files in /home/pere/Projects/espigol/internal/adapters/persistence/backup
FAIL	github.com/pjover/espigol/internal/adapters/persistence/backup [build failed]
FAIL
```
**Result:** Tests failed as expected—`Service` and package undefined.

### GREEN Phase
Implementation created and verified to pass:
```
$ go test ./internal/adapters/persistence/backup/ -v
=== RUN   TestBackup_ProducesOpenableCopy
2026/07/07 08:34:42 OK   00001_init.sql (3.57ms)
2026/07/07 08:34:42 OK   00002_session.sql (407.71µs)
2026/07/07 08:34:42 goose: successfully migrated database to version: 2
2026/07/07 08:34:42 goose: no migrations to run. current version: 2
--- PASS: TestBackup_ProducesOpenableCopy (0.01s)
=== RUN   TestListBackups_NewestFirst
2026/07/07 08:34:42 OK   00001_init.sql (2.46ms)
2026/07/07 08:34:42 OK   00002_session.sql (291.12µs)
2026/07/07 08:34:42 goose: successfully migrated database to version: 2
--- PASS: TestListBackups_NewestFirst (0.00s)
=== RUN   TestStageRestore_WritesPendingAndSafetyBackup
2026/07/07 08:34:42 OK   00001_init.sql (2.14ms)
2026/07/07 08:34:42 OK   00002_session.sql (325.63µs)
2026/07/07 08:34:42 goose: successfully migrated database to version: 2
--- PASS: TestStageRestore_WritesPendingAndSafetyBackup (0.01s)
PASS
ok  	github.com/pjover/espigol/internal/adapters/persistence/backup	0.023s
```
**Result:** All 3 tests PASS.

## Test Summary
- **TestBackup_ProducesOpenableCopy**: Verifies `VACUUM INTO` creates a readable, valid SQLite database at the correct path.
- **TestListBackups_NewestFirst**: Confirms filtering (espigol-*.db only) and newest-first ordering by lexical descending sort.
- **TestStageRestore_WritesPendingAndSafetyBackup**: Validates `restore-pending.db` staging and automatic safety backup creation.

## Files Created
- `internal/adapters/persistence/backup/backup.go` (226 lines)
  - Exports: `Clock` interface, `BackupFile` struct, `Backuper` interface, `Service` type, `New()` constructor
  - Implements: `Backup()` (VACUUM INTO with timestamped filename), `ListBackups()` (espigol-*.db only, newest-first), `StageRestore()` (safety backup + copy to pending)
  - Helper funcs: `copyFile()`, `sqlQuote()` (SQL string literal escaping)
  
- `internal/adapters/persistence/backup/backup_test.go` (83 lines)
  - 3 test cases covering all public methods
  - Minimal mock (`fakeClock`) and test helpers (`newSvc()`)

## Concerns
None. All code matches the brief exactly, unused imports are absent, and all three tests pass with no warnings.

## Fix: copyFile partial-file cleanup

### Change
Modified `copyFile()` in `internal/adapters/persistence/backup/backup.go` to remove the destination file if `io.Copy` fails, preventing partial/corrupt `restore-pending.db` from being left behind.

**Before:**
```go
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
```

**After:**
```go
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		os.Remove(dst)
		return copyErr
	}
	return closeErr
```

### Verification
```
$ go test ./internal/adapters/persistence/backup/ -v
=== RUN   TestBackup_ProducesOpenableCopy
2026/07/07 08:38:40 OK   00001_init.sql (2.44ms)
2026/07/07 08:38:40 OK   00002_session.sql (315.43µs)
2026/07/07 08:38:40 goose: successfully migrated database to version: 2
2026/07/07 08:38:40 goose: no migrations to run. current version: 2
--- PASS: TestBackup_ProducesOpenableCopy (0.01s)
=== RUN   TestListBackups_NewestFirst
2026/07/07 08:38:40 OK   00001_init.sql (2.03ms)
2026/07/07 08:38:40 OK   00002_session.sql (302.35µs)
2026/07/07 08:38:40 goose: successfully migrated database to version: 2
--- PASS: TestListBackups_NewestFirst (0.00s)
=== RUN   TestStageRestore_WritesPendingAndSafetyBackup
2026/07/07 08:38:40 OK   00001_init.sql (2.05ms)
2026/07/07 08:38:40 OK   00002_session.sql (310.07µs)
2026/07/07 08:38:40 goose: successfully migrated database to version: 2
--- PASS: TestStageRestore_WritesPendingAndSafetyBackup (0.01s)
PASS
ok  	github.com/pjover/espigol/internal/adapters/persistence/backup	0.020s

$ go build ./...
```

**Result:** All 3 tests PASS, build succeeds.
