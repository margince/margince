// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// bulk_update_records' four modes: which seam method each reaches, and the
// arguments each refuses. What the advertised schema admits per mode is held in
// backend/gates/bulktoolschema_test.go, where a schema validator may be used. The engine behind the seam is proven
// over a real database in compose's bulk suites.

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// recordingChanger answers each seam method with the method's own name.
type recordingChanger struct {
	batch ids.UUID
	token string
}

func (c *recordingChanger) PreviewBulkChange(context.Context, BulkChangeCommand) (json.RawMessage, error) {
	return json.RawMessage(`"preview"`), nil
}

func (c *recordingChanger) ExecuteBulkChange(context.Context, BulkChangeCommand) (json.RawMessage, error) {
	return json.RawMessage(`"execute"`), nil
}

func (c *recordingChanger) PreviewBulkUndo(_ context.Context, batch ids.UUID) (json.RawMessage, error) {
	c.batch = batch
	return json.RawMessage(`"undo_preview"`), nil
}

func (c *recordingChanger) UndoBulkChange(_ context.Context, batch ids.UUID, token string) (json.RawMessage, error) {
	c.batch, c.token = batch, token
	return json.RawMessage(`"undo"`), nil
}

const changeArgs = `"record_type":"contact","verb":"archive","items":[{"id":"019ff000-0000-7000-8000-000000000001","version":1}]`

const undoBatch = "019ff000-0000-7000-8000-0000000000b1"

func TestEachBulkModeReachesItsOwnSeamMethod(t *testing.T) {
	for _, tc := range []struct{ args, want string }{
		{`{"mode":"preview",` + changeArgs + `}`, `"preview"`},
		{`{"mode":"execute",` + changeArgs + `}`, `"execute"`},
		{`{"mode":"undo_preview","batch_id":"` + undoBatch + `"}`, `"undo_preview"`},
		{`{"mode":"undo","batch_id":"` + undoBatch + `","confirm_token":"tok"}`, `"undo"`},
	} {
		changer := &recordingChanger{}
		got, err := bulkUpdateRecords{changer: changer}.Handle(context.Background(), json.RawMessage(tc.args))
		if err != nil || string(got) != tc.want {
			t.Errorf("%s → %s, %v; want %s", tc.args, got, err, tc.want)
		}
		if tc.want == `"undo"` && (changer.batch.String() != undoBatch || changer.token != "tok") {
			t.Errorf("undo reached the seam with batch %s and token %q", changer.batch, changer.token)
		}
	}
}

func TestTheBulkToolRefusesArgumentsOfTheOtherMode(t *testing.T) {
	for _, args := range []string{
		`{"mode":"preview",` + changeArgs + `,"batch_id":"` + undoBatch + `"}`,
		`{"mode":"preview",` + changeArgs + `,"confirm_token":"t"}`,
		`{"mode":"undo","batch_id":"` + undoBatch + `",` + changeArgs + `}`,
		`{"mode":"undo_preview","batch_id":"` + undoBatch + `","confirm_token":"t"}`,
		`{"mode":"undo"}`,
		`{"mode":"preview"}`,
		`{"mode":"rollback"}`,
	} {
		_, err := bulkUpdateRecords{changer: &recordingChanger{}}.Handle(context.Background(), json.RawMessage(args))
		var bad *BadArgsError
		if !errors.As(err, &bad) {
			t.Errorf("%s → %v, want a bad-arguments refusal", args, err)
		}
	}
}
