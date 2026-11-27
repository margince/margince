// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Sources beyond the content gate corpus. The reader was copied on the first.
// The second is for the whole workspace, the third names a contact under a
// processing restriction, and the fourth is shared with the reader alone.
const (
	copiedSource = ownSource + 1 + iota
	workspaceSource
	restrictedSource
	selectedSource
)

// A content-gated list renders the reply channel without a source gate, and a
// discover-gated one keeps it. Either way the list holds the rows its gate
// admits, and each names the channel the source audience arm names.
func TestAListNamesTheReplyChannelTheSourceArmAdmits(t *testing.T) {
	t.Parallel()
	e := setupLoad(t)
	private := e.privateContactOf(t, e.other)
	restricted := e.buyer(t)
	e.exec(t, `INSERT INTO communication_suppression (contact_id, kind, source, captured_by, decided_by_level)
		VALUES ($1, 'processing_restriction', 'test', 'human:test', 'subject')`, restricted)
	sources := e.seedReplySources(t, restricted)
	var rows []ids.UUID
	origin := map[ids.UUID]int{}
	for _, shape := range everyGateShape() {
		if shape.kind != "task" {
			continue
		}
		for _, source := range replySourcesFor(shape.source) {
			shape.source = source
			row := e.seedGateShape(t, shape, sources, private, restricted)
			rows, origin[row] = append(rows, row), source
		}
	}
	e.exec(t, `UPDATE activity SET task_type = 'reply' WHERE id = ANY($1)`, rows)
	for name, ctx := range map[string]context.Context{
		"own human":  e.readerAt(principal.RowScopeOwn, principal.PrincipalHuman),
		"all human":  e.readerAt(principal.RowScopeAll, principal.PrincipalHuman),
		"own agent":  e.readerAt(principal.RowScopeOwn, principal.PrincipalAgent),
		"the system": e.readerAt(principal.RowScopeAll, principal.PrincipalSystem),
	} {
		for _, readable := range []bool{true, false} {
			gate := auth.ActivityDiscoverClause
			if readable {
				gate = auth.ActivityContentClause
			}
			named := e.compareReplyChannels(ctx, t, name, rows, origin, readable, gate)
			if name == "own agent" && named[restrictedSource] != 0 {
				t.Errorf("%s, readable only %v: an agent is named the channel of mail about a restricted contact", name, readable)
			}
			if name != "own human" {
				continue
			}
			if named[copiedSource] == 0 || named[workspaceSource] == 0 || named[selectedSource] == 0 || named[theirSource] != 0 {
				t.Errorf("%s, readable only %v: channels named per source %v, want the copied and workspace sources and never the other seat's private mail",
					name, readable, named)
			}
		}
	}
}

// compareReplyChannels lists rows and checks each against the source arm, and
// counts the channels named per source.
func (e *loadEnv) compareReplyChannels(
	ctx context.Context, t *testing.T, reader string, rows []ids.UUID, origin map[ids.UUID]int, readable bool, gate clauseRenderer,
) map[int]int {
	t.Helper()
	listed, want := e.listedReplyChannels(ctx, t, rows, readable), e.sourceArmReplyChannels(ctx, t, rows, gate)
	if len(listed) != len(want) {
		t.Errorf("%s, readable only %v: the list shows %d rows, its gate admits %d", reader, readable, len(listed), len(want))
	}
	named := map[int]int{}
	for row, channel := range want {
		got, ok := listed[row]
		if !ok {
			t.Errorf("%s, readable only %v: a row from source %d is admitted but not listed", reader, readable, origin[row])
			continue
		}
		if got != channel {
			t.Errorf("%s, readable only %v: a row from source %d names channel %q, the source arm names %q", reader, readable, origin[row], got, channel)
		}
		if channel != "" {
			named[origin[row]]++
		}
	}
	return named
}

// seedReplySources writes the sources a reply task can stand for.
func (e *loadEnv) seedReplySources(t *testing.T, restricted ids.UUID) map[int]ids.UUID {
	t.Helper()
	audiences := map[int]string{
		theirSource: "participants", ownSource: "participants", copiedSource: "participants",
		workspaceSource: "workspace", restrictedSource: "workspace", selectedSource: "selected",
	}
	sources := map[int]ids.UUID{}
	for source, audience := range audiences {
		sources[source] = ids.NewV7()
		e.exec(t, `INSERT INTO activity (id, kind, direction, subject, occurred_at, audience, source, captured_by)
			VALUES ($1, 'email', 'inbound', 'a source mail', now(), $2, 'seed', 'system')`, sources[source], audience)
	}
	for source, seat := range map[int]ids.UUID{theirSource: e.other, ownSource: e.rep, copiedSource: e.other} {
		e.exec(t, `INSERT INTO capture_import (activity_id, user_id) VALUES ($1, $2)`, sources[source], seat)
	}
	e.exec(t, `INSERT INTO activity_participant (activity_id, address, user_id, role) VALUES ($1, $2, $3, 'cc')`,
		sources[copiedSource], "rep-"+e.rep.String()+"@load.test", e.rep)
	// A party, not a link. A link also hides the mail behind the contact's row
	// scope, and the restriction must be what withholds it.
	e.exec(t, `INSERT INTO activity_participant (activity_id, contact_id, role) VALUES ($1, $2, 'from')`,
		sources[restrictedSource], restricted)
	e.exec(t, `INSERT INTO activity_audience_member (activity_id, subject_type, subject_id, created_by) VALUES ($1, 'user', $2, 'human:seed')`,
		sources[selectedSource], e.rep)
	return sources
}

// replySourcesFor widens the corpus: where the other seat's private mail stands,
// so does each of the sources above.
func replySourcesFor(source int) []int {
	if source == theirSource {
		return []int{theirSource, copiedSource, workspaceSource, restrictedSource, selectedSource}
	}
	return []int{source}
}

// listedReplyChannels reads rows through the activity list.
func (e *loadEnv) listedReplyChannels(ctx context.Context, t *testing.T, rows []ids.UUID, readable bool) map[ids.UUID]string {
	t.Helper()
	got := map[ids.UUID]string{}
	limit := 200
	err := storeKnowing(e).db.Tx(ctx, func(tx pgx.Tx) error {
		for start := 0; start < len(rows); start += limit {
			var chunk []ids.ActivityID
			for _, row := range rows[start:min(start+limit, len(rows))] {
				chunk = append(chunk, ids.From[ids.ActivityKind](row))
			}
			page, _, err := ListActivitiesTx(ctx, tx, ListActivitiesInput{IDs: chunk, ReadableOnly: readable, Limit: &limit})
			if err != nil {
				return err
			}
			for _, activity := range page {
				got[ids.UUID(activity.Id)] = replyChannelOf(activity.ReplyChannel)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("listing the reply tasks: %v", err)
	}
	return got
}

// sourceArmReplyChannels is what the list must show: the rows gate admits,
// each naming its channel under the source audience arm.
func (e *loadEnv) sourceArmReplyChannels(ctx context.Context, t *testing.T, rows []ids.UUID, gate clauseRenderer) map[ids.UUID]string {
	t.Helper()
	want := map[ids.UUID]string{}
	err := storeKnowing(e).db.Tx(ctx, func(tx pgx.Tx) error {
		args := []any{rows}
		arg := func(v any) int { args = append(args, v); return len(args) }
		admits, err := gate(ctx, "a", arg)
		if err != nil {
			return err
		}
		source, err := auth.ActivitySourceAudienceArm(ctx, "s", arg)
		if err != nil {
			return err
		}
		admitted, err := tx.Query(ctx, `SELECT a.id, coalesce(`+storekit.ReplyChannelSQL("a", source)+`, '')
			FROM activity a WHERE a.id = ANY($1) AND `+admits, args...)
		if err != nil {
			return err
		}
		defer admitted.Close()
		for admitted.Next() {
			var row ids.UUID
			var channel string
			if err := admitted.Scan(&row, &channel); err != nil {
				return err
			}
			want[row] = channel
		}
		return admitted.Err()
	})
	if err != nil {
		t.Fatalf("reading what the source arm admits: %v", err)
	}
	return want
}

func replyChannelOf(channel *crmcontracts.ActivityReplyChannel) string {
	if channel == nil {
		return ""
	}
	return string(*channel)
}
