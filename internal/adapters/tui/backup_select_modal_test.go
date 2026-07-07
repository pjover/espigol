package tui

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestBackupSelectModal_EnterStagesRestore(t *testing.T) {
	deps, _ := testDeps(t)
	// Create a real backup to select.
	src, err := deps.Backup.Backup(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	files, err := deps.Backup.ListBackups()
	if err != nil || len(files) == 0 {
		t.Fatalf("ListBackups: %v (n=%d)", err, len(files))
	}

	m := newBackupSelectModal(deps, files)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected a command after Enter")
	}
	// The batch includes stageRestoreCmd (produces restoreStagedMsg) and
	// closeModalCmd; run the batch and find the restoreStagedMsg.
	msg := runCmd(t, cmd)
	staged := findRestoreStaged(t, msg)
	if staged.err != nil {
		t.Fatalf("stage error: %v", staged.err)
	}
	if staged.name != filepath.Base(src) {
		t.Errorf("staged name = %q, want %q", staged.name, filepath.Base(src))
	}
	pending := filepath.Join(deps.Cfg.Home, "restore-pending.db")
	if _, err := os.Stat(pending); err != nil {
		t.Errorf("restore-pending.db not written: %v", err)
	}
}

// findRestoreStaged extracts a restoreStagedMsg from a possibly-batched message.
func findRestoreStaged(t *testing.T, msg tea.Msg) restoreStagedMsg {
	t.Helper()
	switch m := msg.(type) {
	case restoreStagedMsg:
		return m
	case tea.BatchMsg:
		for _, c := range m {
			if c == nil {
				continue
			}
			if rs := findRestoreStaged(t, c()); rs.name != "" || rs.err != nil {
				return rs
			}
		}
	}
	return restoreStagedMsg{}
}
