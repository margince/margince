// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A meeting read before the reading split its principals filed every item as
// a system principal named agent:transcript-proposer. Reading it again now
// files the requester's own items as the agent, and other colleagues' as
// system:transcript_read. What a reading already filed is found by its
// evidence: the locator, the meeting, the assignee. It is never found by who
// filed it, so the re-read files nothing twice.

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// readAsBeforeTheSplit runs one reading the way the worker bound it before:
// one system principal under the reader agent's id, acting for Rep1.
func (e *transcriptEnv) readAsBeforeTheSplit(t *testing.T, brain completer) {
	t.Helper()
	started, _, err := e.Activities.StartTranscriptReadQueued(e.ctx, e.activity, "human:"+e.Rep1.String(), nil)
	if err != nil {
		t.Fatalf("starting the reading: %v", err)
	}
	old := principal.WithActor(principal.WithWorkspaceID(context.Background(), e.WS), principal.Principal{
		Type: principal.PrincipalSystem, ID: transcriptProposalActor, UserID: e.Rep1, OnBehalfOf: e.Rep1,
	})
	proposer := NewTranscriptProposer(e.Pool, brain, e.svc, func() time.Time { return clocktest.Now(t) }, slog.New(slog.DiscardHandler))
	if err := proposer.Read(principal.WithCorrelationID(old, started.ID), e.Activities, started.ID, e.activity); err != nil {
		t.Fatalf("reading as before the split: %v", err)
	}
}

// filed counts what a reading leaves behind: tasks, claims and proposals.
func (e *transcriptEnv) filed(t *testing.T) int {
	t.Helper()
	return e.WsCount(t, `SELECT (SELECT count(*) FROM activity WHERE kind = 'task')
		+ (SELECT count(*) FROM conversation_claim WHERE archived_at IS NULL)
		+ (SELECT count(*) FROM approval)`)
}

func TestReReadingAMeetingAnOldIDRunFiledFilesNothingTwice(t *testing.T) {
	for name, promiser := range map[string]func(t *testing.T, e *transcriptEnv) (string, string){
		"the requester's own promise": func(_ *testing.T, e *transcriptEnv) (string, string) {
			return "Priya", "seat:" + e.Rep1.String()
		},
		"another colleague's promise": func(t *testing.T, e *transcriptEnv) (string, string) {
			e.WsExec(t, `UPDATE app_user SET display_name = 'Priya Raman' WHERE id = $1`, e.Rep2)
			return "Priya Raman", "seat:" + e.Rep2.String()
		},
	} {
		t.Run(name, func(t *testing.T) {
			e := setupTranscript(t)
			owner, ref := promiser(t, e)
			brain := cannedBrain{reply: attributedReply(t, owner, ref, 0.9)}
			e.readAsBeforeTheSplit(t, brain)
			before := e.filed(t)
			if before == 0 {
				t.Fatal("the old-id run filed nothing, so the re-read has nothing to find")
			}

			e.read(t, brain)
			if after := e.filed(t); after != before {
				t.Errorf("re-reading filed %d items, want the %d the old-id run already filed", after, before)
			}
		})
	}
}
