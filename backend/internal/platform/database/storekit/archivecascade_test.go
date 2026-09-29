// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestOnlyAConstraintRefusalIsSwallowedAsNotRestored(t *testing.T) {
	for code, refused := range map[string]bool{
		"23505": true, "23503": true, "23514": true, "23502": true, "23P01": true,
		"22P02": false, "40P01": false, "57014": false,
	} {
		err := fmt.Errorf("restore a row: %w", &pgconn.PgError{Code: code})
		if got := isIntegrityRefusal(err); got != refused {
			t.Errorf("SQLSTATE %s: integrity refusal = %v, want %v", code, got, refused)
		}
	}
	if isIntegrityRefusal(errors.New("connection reset")) {
		t.Error("an error that is no Postgres error was read as a constraint refusal")
	}
}

func TestARestoreRefusalIsAConflictThatNamesItsReason(t *testing.T) {
	bare := &RestoreRefusal{Reason: RestoreMerged}
	named := &RestoreRefusal{Reason: RestoreValueTaken, Detail: "email"}
	if !errors.Is(bare, apperrors.ErrConflict) || !errors.Is(named, apperrors.ErrConflict) {
		t.Error("a restore refusal does not answer as a conflict")
	}
	if !strings.Contains(bare.Error(), "merged") {
		t.Errorf("%q does not name its reason", bare.Error())
	}
	if !strings.Contains(named.Error(), "value_taken") || !strings.Contains(named.Error(), "email") {
		t.Errorf("%q does not name its reason and the field", named.Error())
	}
}

func TestARestoreRowNamesOnlyWhatItHasToSay(t *testing.T) {
	archive := ids.NewV7()
	plain := RestoreReport{}.Evidence(archive)
	if len(plain) != 1 || plain[EvidenceKeyRestoresArchive] != archive {
		t.Errorf("a complete restore records %v, want only the archive it reversed", plain)
	}
	full := RestoreReport{
		LeftBehind: []LeftBehind{{Kind: "tag", ID: ids.NewV7()}},
		Relinked:   []ids.UUID{ids.NewV7()},
	}.Evidence(archive)
	if _, left := full[EvidenceKeyLeftBehind]; !left {
		t.Error("a restore that left something behind does not say so")
	}
	if _, relinked := full[EvidenceKeyRelinked]; !relinked {
		t.Error("a restore that brought back another record's link does not say so")
	}
	if _, cascade := (ArchiveCascade{}).Evidence()[EvidenceKeyArchiveCascade]; !cascade {
		t.Error("an archive's evidence does not carry its cascade")
	}
}
