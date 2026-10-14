// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

// Older code let two runs hold one source name or one upload at once. The
// import-session migration settles each such shape before it builds the
// indexes, so a deploy over that data does not stop on a unique violation.

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/dbmigrate"
)

// causeSourceTaken is the text the backfill writes on a stopped commit whose
// upload another run kept.
const causeSourceTaken = "another run of this upload holds its file, so this one cannot resume; upload the file again to go on"

func TestTheNewerOfTwoCommitsOfOneNameKeepsIt(t *testing.T) {
	conn := connect(t, ownerDSNOf(t))
	rewindImportSession(t, conn)
	stale := seedLegacyRun(t, conn, legacyRun{
		status: "running", sourceRef: "ws/import/stale", sourceName: "Fair 2026", age: 2 * time.Hour,
	})
	live := seedLegacyRun(t, conn, legacyRun{
		status: "running", sourceRef: "ws/import/live", sourceName: "Fair 2026", age: time.Hour,
	})
	replayImportSession(t, conn)

	if got := readSessionRow(t, conn, stale); got.status != "failed" || got.runError != oldCauseStopped ||
		!got.approved || got.released {
		t.Errorf("the older commit reads %+v, want failed, approved and holding its file, so approve resumes it", got)
	}
	if got := readSessionRow(t, conn, live); got.status != "running" {
		t.Errorf("the newer commit reads %s, want it left running", got.status)
	}
}

func TestAnUndoKeepsItsNameAgainstACommit(t *testing.T) {
	conn := connect(t, ownerDSNOf(t))
	rewindImportSession(t, conn)
	undo := seedLegacyRun(t, conn, legacyRun{
		status: "undoing", sourceRef: "ws/import/undo", sourceName: "Fair 2026", age: 2 * time.Hour,
	})
	commit := seedLegacyRun(t, conn, legacyRun{
		status: "running", sourceRef: "ws/import/commit", sourceName: "Fair 2026", age: time.Hour,
	})
	replayImportSession(t, conn)

	if got := readSessionRow(t, conn, undo); got.status != "undoing" {
		t.Errorf("the undo reads %s, want it left undoing: no other state resumes a reversal", got.status)
	}
	if got := readSessionRow(t, conn, commit); got.status != "failed" || got.runError != oldCauseStopped {
		t.Errorf("the commit beside the undo reads %+v, want failed and resumable", got)
	}
}

func TestTwoUndosOfOneNameStopTheMigrationNamingThem(t *testing.T) {
	conn := connect(t, ownerDSNOf(t))
	rewindImportSession(t, conn)
	first := seedLegacyRun(t, conn, legacyRun{status: "undoing", sourceRef: "ws/import/a", sourceName: "Fair 2026"})
	second := seedLegacyRun(t, conn, legacyRun{status: "undoing", sourceRef: "ws/import/b", sourceName: "Fair 2026"})
	seedLegacyRun(t, conn, legacyRun{status: "undoing", sourceRef: "ws/import/c", sourceName: "Expo"})

	_, err := dbmigrate.Up(context.Background(), conn, customNamespace(t))
	if err == nil || !strings.Contains(err.Error(), first) || !strings.Contains(err.Error(), second) {
		t.Fatalf("migrating over two undos of one name = %v, want a stop naming %s and %s", err, first, second)
	}
	if _, err := conn.Exec(context.Background(), `UPDATE import_run SET status = 'undone' WHERE id::text = $1`, first); err != nil {
		t.Fatal(err)
	}
	replayImportSession(t, conn)
}

func TestTheNewerOfTwoStoppedCommitsOfOneUploadKeepsTheFile(t *testing.T) {
	conn := connect(t, ownerDSNOf(t))
	rewindImportSession(t, conn)
	older := seedLegacyRun(t, conn, legacyRun{
		status: "failed", sourceRef: "ws/import/same", runError: oldCauseStopped, age: 2 * time.Hour,
	})
	newer := seedLegacyRun(t, conn, legacyRun{
		status: "failed", sourceRef: "ws/import/same", runError: oldCauseStopped, age: time.Hour,
	})
	replayImportSession(t, conn)

	if got := readSessionRow(t, conn, newer); got.status != "failed" || got.released || got.runError != oldCauseStopped {
		t.Errorf("the newer stopped commit reads %+v, want failed, holding its file and resumable", got)
	}
	if got := readSessionRow(t, conn, older); got.status != "failed" || !got.released || got.runError != causeSourceTaken {
		t.Errorf("the older stopped commit reads %+v, want failed and released, saying why it cannot resume", got)
	}
}

func TestACommitInFlightKeepsItsUploadAgainstAStoppedOne(t *testing.T) {
	conn := connect(t, ownerDSNOf(t))
	rewindImportSession(t, conn)
	inFlight := seedLegacyRun(t, conn, legacyRun{status: "running", sourceRef: "ws/import/same", age: 2 * time.Hour})
	stopped := seedLegacyRun(t, conn, legacyRun{
		status: "failed", sourceRef: "ws/import/same", runError: oldCauseStopped, age: time.Hour,
	})
	replayImportSession(t, conn)

	if got := readSessionRow(t, conn, inFlight); got.status != "running" || got.released {
		t.Errorf("the commit in flight reads %+v, want running with its file", got)
	}
	if got := readSessionRow(t, conn, stopped); !got.released || got.runError != causeSourceTaken {
		t.Errorf("the stopped commit reads %+v, want released, saying why it cannot resume", got)
	}
}
