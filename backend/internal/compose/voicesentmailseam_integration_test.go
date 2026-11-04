// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Mail goes through the production parser and capture sink. The press goes
// through the real reader and voice store. So each case is a mail the product
// really stores.

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

var sentMailOwnerPerms = principal.Permissions{
	RoleKeys: []string{"rep"},
	Objects: map[string]principal.ObjectGrant{
		"voice_profile": {Create: true, Read: true, Update: true},
		"activity":      {Read: true, Update: true},
		"contact":       {Read: true},
	},
	RowScope: principal.RowScopeAll,
}

// sentMailVoice is the owner's voice store with the production reader,
// counting how often the reader runs.
type sentMailVoice struct {
	store *ai.VoiceStore
	reads int
	owner context.Context
}

func newSentMailVoice(t *testing.T, o *owedEnv) (*sentMailVoice, ids.UUID) {
	t.Helper()
	v := &sentMailVoice{owner: o.e.As(o.owner, nil, sentMailOwnerPerms)}
	read := voiceSentMail(o.e.Pool)
	v.store = ai.NewVoiceStore(InstallationDB(o.e.Pool)).WithSentMail(
		func(ctx context.Context, tx pgx.Tx, limit int) ([]ai.SentMail, error) {
			v.reads++
			return read(ctx, tx, limit)
		})
	profile, err := v.store.CreateProfile(v.owner, ai.CreateVoiceProfileInput{})
	if err != nil {
		t.Fatalf("creating the owner's voice profile: %v", err)
	}
	return v, profile.ID
}

func (v *sentMailVoice) press(t *testing.T, profileID ids.UUID) ai.SentMailLearned {
	t.Helper()
	learned, err := v.store.LearnFromSentMail(v.owner, profileID)
	if err != nil {
		t.Fatalf("learning from sent mail: %v", err)
	}
	return learned
}

// stored reads the ref and text of the live mail sources, as the admin.
func stored(t *testing.T, o *owedEnv, profileID ids.UUID) map[string]string {
	t.Helper()
	out := map[string]string{}
	if err := database.WithWorkspaceTx(o.e.Admin(), o.e.Pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(context.Background(), `SELECT source_ref, coalesce(content, '') FROM voice_corpus_source
			WHERE voice_profile_id = $1 AND archived_at IS NULL AND origin = 'capture' AND kind = 'email'`, profileID)
		if err != nil {
			return err
		}
		var ref, content string
		_, err = pgx.ForEachRow(rows, []any{&ref, &content}, func() error {
			out[ref] = content
			return nil
		})
		return err
	}); err != nil {
		t.Fatalf("reading the stored sources: %v", err)
	}
	return out
}

// threadOf is the thread key capture filed a mail under.
func threadOf(t *testing.T, o *owedEnv, activityID ids.UUID) string {
	t.Helper()
	return o.e.WsScalar(t, `SELECT thread_key FROM activity WHERE id = $1`, activityID)
}

func TestAVoiceLearnsOnlyFromTheOwnersOwnSentMail(t *testing.T) {
	o := setupOwed(t)
	o.now = clocktest.Now(t).Add(-time.Hour)
	colleagueSeat := seatAddress(t, o.e, o.e.Rep2)
	// The colleague shares their mailbox too, so their own mail is workspace
	// mail the owner may read and only its sender keeps it out.
	o.e.WsExec(t, `UPDATE capture_connection SET mail_posture = 'shared' WHERE user_id = $1`, o.e.Rep2)
	v, profileID := newSentMailVoice(t, o)

	own := o.weWrite(t, owedMail{
		to: "pat@customer.example", subject: "Angebot", at: o.now,
		body: "Hallo Pat,\r\n\r\nanbei das Angebot, ruf mich gern an.\r\n\r\nLG Lars\r\n" +
			"-- \r\nLars Rep\r\n\r\nAm 01.11.2026 um 10:00 schrieb Pat <pat@customer.example>:\r\n> Schick mir bitte die Preise.",
	})
	// The colleague's own mail, received as a copy in the owner's mailbox.
	login := o.e.WsScalar(t, `SELECT email FROM app_user WHERE id = $1`, o.e.Rep2)
	o.e.WsExec(t, `INSERT INTO workspace_email_domain (domain, source, verified) VALUES ($1, 'admin', true)
		ON CONFLICT DO NOTHING`, login[strings.LastIndex(login, "@")+1:])
	copied := o.capture(t, owedMail{
		from: login, to: "pat@customer.example", cc: o.seat, subject: "Unsere Konditionen", authenticated: true,
		messageID: "terms-" + ids.NewV7().String() + "@ws.example", at: o.now,
		body: "Die Konditionen der Kollegin stehen hier ausführlich drin.",
	})
	// The colleague's own sent mail in their own mailbox.
	theirs := o.captureFor(t, o.e.Rep2, colleagueSeat, owedMail{
		from: colleagueSeat, to: "pat@customer.example", subject: "Rückfrage", at: o.now,
		messageID: "theirs-" + ids.NewV7().String() + "@ws.example",
		body:      "Der Kollege schreibt hier seine eigenen Worte.",
	})
	received := o.customerWrites(t, "pat@customer.example", "Frage zum Angebot", o.now)
	confidential := o.weWrite(t, owedMail{
		to: "pat@customer.example", subject: "[Vertraulich] Preise", at: o.now,
		body: "Die vertraulichen Preise stehen hier drin.",
	})
	limited := o.weWrite(t, owedMail{
		to: "pat@customer.example", subject: "Nur für dich", at: o.now,
		body: "Diese Mail ist nur für die Beteiligten bestimmt.",
	})
	if o.count(t, `SELECT count(*) FROM activity WHERE id = $1 AND sent_by_colleague = $2`, copied, o.e.Rep2) != 1 {
		t.Fatal("the fixture's colleague copy is not stamped as the colleague's mail")
	}
	if o.count(t, `SELECT count(*) FROM activity WHERE id = $1 AND audience = 'workspace'
		AND direction = 'outbound' AND counterparty_outbound_attested`, theirs) != 1 {
		t.Fatal("the colleague's own mail is not attested workspace mail, so its sender is not what keeps it out")
	}
	decideThread(t, o.e, o.owner, threadOf(t, o, limited), false)
	if o.count(t, `SELECT count(*) FROM activity WHERE id = $1 AND audience = 'participants'`, limited) != 1 {
		t.Fatal("the fixture's private thread is still open to the workspace")
	}

	if got := stored(t, o, profileID); len(got) != 0 || v.reads != 0 {
		t.Fatalf("before any press: %d sources and %d reads, want none", len(got), v.reads)
	}

	learned := v.press(t, profileID)
	got := stored(t, o, profileID)
	ownRef := "sent-mail:" + own.String()
	if learned.Added != 1 || len(got) != 1 {
		t.Fatalf("learned %+v and stored %v, want the owner's one mail", learned, got)
	}
	if content := got[ownRef]; !strings.Contains(content, "anbei das Angebot") ||
		strings.Contains(content, "Preise") || strings.Contains(content, "Lars Rep") {
		t.Errorf("stored text %q, want the owner's words without the quote or the signature", content)
	}
	for name, id := range map[string]ids.UUID{
		"a colleague's copy": copied, "a colleague's own mail": theirs, "a received mail": received,
		"a confidential mail": confidential, "a participants-only mail": limited,
	} {
		if _, fed := got["sent-mail:"+id.String()]; fed {
			t.Errorf("%s fed the owner's voice", name)
		}
	}
	if n := o.count(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'voice_corpus_source'
		AND action = 'create' AND after->>'source_ref' = $1`, ownRef); n != 1 {
		t.Errorf("%d audit rows for the filed mail, want 1", n)
	}
}

// The subject marker is the sender's own request for confidence. Here a
// workspace mail gets the marker later, by an edit to its subject. It stays
// out of the voice even though the workspace reads it.
func TestASentMailMarkedConfidentialLaterStaysOut(t *testing.T) {
	o := setupOwed(t)
	v, profileID := newSentMailVoice(t, o)
	mail := o.weWrite(t, owedMail{
		to: "pat@customer.example", subject: "Pricing", at: clocktest.Now(t).Add(-time.Hour),
		body: "These are the prices for your team, as discussed.",
	})
	marked := "Confidential: pricing"
	if _, err := o.e.Activities.UpdateActivity(v.owner, ids.ActivityID{UUID: mail},
		activities.UpdateActivityInput{Subject: &marked}); err != nil {
		t.Fatalf("marking the mail confidential: %v", err)
	}
	if o.count(t, `SELECT count(*) FROM activity WHERE id = $1 AND audience = 'workspace' AND subject = $2`,
		mail, marked) != 1 {
		t.Fatal("the fixture's mail is not workspace mail with a confidential subject")
	}
	if learned := v.press(t, profileID); learned.Read != 0 || learned.Added != 0 {
		t.Errorf("learned %+v from a mail marked confidential, want nothing read", learned)
	}
}

// A second press adds nothing, and leaves alone what the owner removed or
// excluded since the first.
func TestPressingAgainKeepsTheOwnersChoices(t *testing.T) {
	o := setupOwed(t)
	v, profileID := newSentMailVoice(t, o)
	at := clocktest.Now(t).Add(-time.Hour)
	removed := o.weWrite(t, owedMail{to: "pat@customer.example", subject: "Eins", at: at,
		body: "Das ist die erste Mail mit eigenen Worten."})
	excluded := o.weWrite(t, owedMail{to: "pat@customer.example", subject: "Zwei", at: at.Add(time.Minute),
		body: "Das ist die zweite Mail mit eigenen Worten."})

	if learned := v.press(t, profileID); learned.Added != 2 {
		t.Fatalf("first press %+v, want both mails", learned)
	}
	if learned := v.press(t, profileID); learned.Added != 0 || learned.AlreadyPresent != 2 {
		t.Fatalf("second press %+v, want both already present", learned)
	}
	sources, _, err := v.store.ListSources(v.owner, profileID)
	if err != nil {
		t.Fatal(err)
	}
	byRef := map[string]ai.VoiceCorpusSource{}
	for _, source := range sources {
		byRef[source.SourceRef] = source
	}
	if _, err := v.store.DeleteSource(v.owner, profileID, byRef["sent-mail:"+removed.String()].ID, nil); err != nil {
		t.Fatalf("removing a source: %v", err)
	}
	exclude := true
	if _, _, err := v.store.UpdateSource(v.owner, profileID, byRef["sent-mail:"+excluded.String()].ID,
		ai.UpdateSourceInput{Excluded: &exclude}); err != nil {
		t.Fatalf("excluding a source: %v", err)
	}

	if learned := v.press(t, profileID); learned.Added != 0 {
		t.Errorf("third press %+v brought back a mail the owner removed or excluded", learned)
	}
	if n := o.count(t, `SELECT count(*) FROM voice_corpus_source WHERE voice_profile_id = $1
		AND archived_at IS NULL AND NOT excluded`, profileID); n != 0 {
		t.Errorf("%d included sources after the owner's removal and exclusion, want 0", n)
	}

	if _, err := v.store.ClearCorpus(v.owner, profileID, nil); err != nil {
		t.Fatalf("clearing the corpus: %v", err)
	}
	if n := o.count(t, `SELECT count(*) FROM voice_corpus_source WHERE voice_profile_id = $1
		AND (content IS NOT NULL OR content_erased_at IS NULL OR content_hash <> ''
		  OR source_label LIKE 'Eins%' OR source_label LIKE 'Zwei%')`, profileID); n != 0 {
		t.Errorf("%d sent-mail sources keep their text, hash or subject after a clear, want 0", n)
	}
}

// Another user's profile answers not found, and the press reads no mail.
func TestAnotherUsersProfileReadsNoMail(t *testing.T) {
	o := setupOwed(t)
	v, profileID := newSentMailVoice(t, o)
	o.weWrite(t, owedMail{to: "pat@customer.example", subject: "Eins", at: clocktest.Now(t).Add(-time.Hour),
		body: "Das ist eine Mail mit eigenen Worten."})
	stranger := o.e.As(o.e.Rep2, nil, sentMailOwnerPerms)
	if _, err := v.store.LearnFromSentMail(stranger, profileID); err == nil {
		t.Fatal("a colleague pressed learn on the owner's profile and was not refused")
	}
	if v.reads != 0 {
		t.Errorf("the reader ran %d times for a refused press, want 0", v.reads)
	}
}
