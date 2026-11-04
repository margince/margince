// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A voice sample read from sent mail is a copy of that mail's words. Each path
// that erases the mail's content empties the sample too, through the eraser
// and retention service compose builds for production.

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// learnedMail is a built voice that learned from one sent mail to Pat.
type learnedMail struct {
	env     *voiceBuildEnv
	o       *owedEnv
	mail    ids.UUID
	contact ids.UUID
}

func voiceLearnedFromMail(t *testing.T) learnedMail {
	t.Helper()
	const quote = "Erasure quote."
	env := builtVoiceEnv(t, quote)
	o := &owedEnv{e: env.e, owner: env.e.Rep1, seat: seatAddress(t, env.e, env.e.Rep1), now: clocktest.Now(t)}
	env.e.WsExec(t, `UPDATE capture_connection SET mail_posture = 'shared' WHERE user_id = $1`, o.owner)
	pat := o.contact(t, "Pat Buyer", "pat@customer.example")
	mail := o.weWrite(t, owedMail{
		to: "pat@customer.example", subject: "Angebot für Pat", at: o.now.Add(-time.Hour),
		body: "Hallo Pat, anbei das Angebot, ruf mich gern an.",
	})

	learner := ai.NewVoiceStore(InstallationDB(env.e.Pool)).WithSentMail(voiceSentMail(env.e.Pool))
	learned, err := learner.LearnFromSentMail(env.e.As(o.owner, nil, sentMailOwnerPerms), env.profile.ID)
	if err != nil || learned.Added != 1 {
		t.Fatalf("learning from the mail: %+v, %v", learned, err)
	}
	// Built again, so the profile is ready and only the erasure can make it stale.
	build, err := env.store.CreateBuild(env.owner, env.profile.ID, ai.CreateVoiceBuildInput{Reason: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	worker := newVoiceBuildWorker(env.e.Pool, &scriptedBuildBrain{judgeScore: 0.9, quote: quote}, slog.New(slog.DiscardHandler))
	if err := worker.Work(context.Background(), voiceBuildJob(env, build)); err != nil {
		t.Fatalf("rebuilding: %v", err)
	}
	if status := env.e.WsScalar(t, `SELECT status FROM voice_profile WHERE id = $1`, env.profile.ID); status != "ready" {
		t.Fatalf("profile reads %q after the rebuild, want ready", status)
	}
	return learnedMail{env: env, o: o, mail: mail, contact: pat}
}

// assertSampleErased checks that the sample holds none of the mail and that the
// profile waits for a rebuild.
func (l learnedMail) assertSampleErased(t *testing.T) {
	t.Helper()
	if n := l.o.count(t, `SELECT count(*) FROM voice_corpus_source WHERE source_ref = $1
		AND content IS NULL AND content_erased_at IS NOT NULL AND archived_at IS NOT NULL
		AND source_label NOT LIKE '%Pat%'`, "sent-mail:"+l.mail.String()); n != 1 {
		t.Errorf("%d erased samples for the mail, want 1: the voice still holds its words or subject", n)
	}
	if status := l.env.e.WsScalar(t, `SELECT status FROM voice_profile WHERE id = $1`, l.env.profile.ID); status != "stale" {
		t.Errorf("profile reads %q after the erasure, want stale so it rebuilds without the mail", status)
	}
}

func TestPurgingASentMailEmptiesTheVoiceSampleReadFromIt(t *testing.T) {
	l := voiceLearnedFromMail(t)
	retention := NewRetentionServiceFor(InstallationDB(l.env.e.Pool), nil, slog.Default())
	if _, err := retention.PurgeActivities(l.env.e.Admin(), []ids.UUID{l.mail}, privacy.PurgeOwnerRule); err != nil {
		t.Fatalf("purging the mail: %v", err)
	}
	l.assertSampleErased(t)
}

func TestErasingTheRecipientEmptiesTheVoiceSampleOfTheirMail(t *testing.T) {
	l := voiceLearnedFromMail(t)
	if err := ComposedEraser(l.env.e.Pool, nil, nil).EraseContact(l.env.e.Admin(), l.contact, "Subject requested erasure"); err != nil {
		t.Fatalf("erasing the contact: %v", err)
	}
	l.assertSampleErased(t)
}

// scrubbedSample reports whether a sample no longer names the mail.
func (l learnedMail) scrubbedSample(t *testing.T, sourceID ids.UUID) bool {
	t.Helper()
	return l.o.count(t, `SELECT count(*) FROM voice_corpus_source WHERE id = $1
		AND source_label NOT LIKE '%Pat%' AND content_hash = ''`, sourceID) == 1
}

// A sample the owner removed is emptied already, but its label named the mail.
// The owner's remove scrubs it, and so does the erasure of the mail for a
// sample whose label survived its removal. The erasure audits only what it
// emptied, so the removed sample gains no second record.
func TestAnErasureScrubsASampleTheOwnerRemovedFirst(t *testing.T) {
	l := voiceLearnedFromMail(t)
	sourceID, err := ids.Parse(l.env.e.WsScalar(t,
		`SELECT id FROM voice_corpus_source WHERE source_ref = $1`, "sent-mail:"+l.mail.String()))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := l.env.store.DeleteSource(l.env.owner, l.env.profile.ID, sourceID, nil); err != nil {
		t.Fatalf("removing the sample: %v", err)
	}
	if !l.scrubbedSample(t, sourceID) {
		t.Error("the owner's remove left the mail's subject or text hash on the sample")
	}
	// The state of a sample removed before removal scrubbed its label.
	l.env.e.WsExec(t, `UPDATE voice_corpus_source SET source_label = 'Angebot für Pat', content_hash = 'h'
		WHERE id = $1`, sourceID)

	retention := NewRetentionServiceFor(InstallationDB(l.env.e.Pool), nil, slog.Default())
	if _, err := retention.PurgeActivities(l.env.e.Admin(), []ids.UUID{l.mail}, privacy.PurgeOwnerRule); err != nil {
		t.Fatalf("purging the mail: %v", err)
	}
	if !l.scrubbedSample(t, sourceID) {
		t.Error("the erasure left the mail's subject or text hash on a removed sample")
	}
	if n := l.o.count(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'voice_corpus_source'
		AND entity_id = $1 AND action = 'erase'`, sourceID); n != 0 {
		t.Errorf("%d erase records for a sample the owner had already emptied, want 0", n)
	}
}
