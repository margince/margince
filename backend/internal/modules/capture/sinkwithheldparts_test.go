// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// withheldOnlyKeeper records what RecordWithheld was asked to name, and fails
// it when told to; the stored-file half is never reached by these cases.
type withheldOnlyKeeper struct {
	named []CapturedFile
	fail  error
}

func (k *withheldOnlyKeeper) Stage(context.Context, []CapturedFile) ([]StagedFile, error) {
	return nil, errors.New("not reached")
}

func (k *withheldOnlyKeeper) Record(context.Context, pgx.Tx, ids.ActivityID, FileSource, []StagedFile) error {
	return errors.New("not reached")
}

func (k *withheldOnlyKeeper) RecordWithheld(_ context.Context, _ pgx.Tx, _ ids.ActivityID, _ FileSource, files []CapturedFile) error {
	k.named = files
	return k.fail
}

// A private message's files are named through the keeper, and a keeper that
// cannot name them fails the capture rather than losing the list quietly. A
// deployment with no keeper, or a message that carried nothing, names nothing.
func TestAWithheldFileIsNamedOrTheCaptureFails(t *testing.T) {
	rec := connector.NormalizedRecord{NaturalKey: connector.NaturalKey{SourceSystem: "imap", SourceID: "m-1"}}
	fields := ActivityFields{Kind: "email"}
	parts := []connector.Part{{Ordinal: 1, Filename: "payslip.pdf", Body: []byte("%PDF")}}
	activity := ids.From[ids.ActivityKind](ids.NewV7())

	keeper := &withheldOnlyKeeper{}
	if err := (&Sink{files: keeper}).recordWithheldParts(context.Background(), nil, activity, rec, fields, parts); err != nil {
		t.Fatalf("naming the withheld file: %v", err)
	}
	if len(keeper.named) != 1 || keeper.named[0].Filename != "payslip.pdf" || keeper.named[0].PartID != "part:1" {
		t.Errorf("named %+v, want payslip.pdf as part:1", keeper.named)
	}

	broken := &withheldOnlyKeeper{fail: errors.New("store down")}
	if err := (&Sink{files: broken}).recordWithheldParts(context.Background(), nil, activity, rec, fields, parts); err == nil {
		t.Error("a keeper that could not name the files let the capture through")
	}

	if err := (&Sink{}).recordWithheldParts(context.Background(), nil, activity, rec, fields, parts); err != nil {
		t.Errorf("no keeper wired: %v, want the message kept and nothing named", err)
	}
	if err := (&Sink{files: broken}).recordWithheldParts(context.Background(), nil, activity, rec, fields, nil); err != nil {
		t.Errorf("no parts withheld: %v, want nothing asked of the keeper", err)
	}
}
