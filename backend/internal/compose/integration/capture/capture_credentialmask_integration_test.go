// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	capturemod "github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const credentialMailID = "portal-access-1@acme.example"

func portalAccessMail() []byte {
	return []byte(strings.Join([]string{
		"From: Anna Acme <anna@acme.example>",
		"To: " + captureOwner,
		"Subject: Portal access, password: Quokka7Subject",
		"Date: Wed, 04 Jun 2026 08:00:00 +0000",
		"Message-ID: <" + credentialMailID + ">",
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=utf-8",
		"",
		"Hallo, here is your Kolibri portal access.",
		"",
		"Login: anna.portal",
		"Passwort: Quokka7Body",
		"",
	}, "\r\n"))
}

// A captured mail stores its subject and body with the login and password
// masked, so no reader of the mail and no search finds them. The provider
// original keeps them until its own retention ends.
func TestACapturedMailStoresItsCredentialsMasked(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	env.sync(t, portalAccessMail())
	activityID := oneActivityID(t, e)

	var subject, body string
	var raw []byte
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		if err := tx.QueryRow(context.Background(),
			`SELECT subject, body FROM activity WHERE id = $1`, activityID).Scan(&subject, &body); err != nil {
			return err
		}
		return tx.QueryRow(context.Background(),
			`SELECT payload FROM raw_capture WHERE source_id = $1`, credentialMailID).Scan(&raw)
	}); err != nil {
		t.Fatalf("reading the captured mail: %v", err)
	}
	if subject != "Portal access, password: [masked]" {
		t.Errorf("subject = %q, want the password masked behind its label", subject)
	}
	for _, secret := range []string{"Quokka7Body", "anna.portal"} {
		if strings.Contains(body, secret) {
			t.Errorf("body holds %q: %q", secret, body)
		}
	}
	for _, masked := range []string{"Login: [masked]", "Passwort: [masked]"} {
		if !strings.Contains(body, masked) {
			t.Errorf("body = %q, want %q", body, masked)
		}
	}
	if !strings.Contains(string(raw), "Passwort: Quokka7Body") {
		t.Errorf("raw_capture.payload lost the original: %q", raw)
	}

	owner := readerCtx(e, e.Rep1)
	for query, want := range map[string]int{"Kolibri": 1, "Quokka7Body": 0, "Quokka7Subject": 0} {
		page, err := e.Store.Search(owner, search.Input{Query: query, Types: []string{"activity"}})
		if err != nil {
			t.Fatalf("search %q: %v", query, err)
		}
		if len(page.Hits) != want {
			t.Errorf("search %q found %d hits, want %d", query, len(page.Hits), want)
		}
	}
}

func passwordMail(msgID, cc, password string) []byte {
	return plainMail("anna@acme.example", cc, msgID, "Passwort: "+password)
}

// plainMail is addressed to the capturing seat, captureOwner.
func plainMail(from, cc, msgID, text string) []byte {
	return []byte(strings.Join([]string{
		"From: " + from,
		"To: " + captureOwner,
		"Cc: " + cc,
		"Subject: Portal access",
		"Date: Wed, 04 Jun 2026 08:00:00 +0000",
		"Message-ID: <" + msgID + ">",
		"Content-Type: text/plain; charset=utf-8",
		"",
		text,
		"",
	}, "\r\n"))
}

// passwordMailWithFile is a mail to both seats carrying one file whose bytes are file.
func passwordMailWithFile(msgID, file string) []byte {
	return []byte(strings.Join([]string{
		"From: anna@acme.example",
		"To: " + captureOwner,
		"Cc: " + secondSeatAddress,
		"Subject: Portal access",
		"Date: Wed, 04 Jun 2026 08:00:00 +0000",
		"Message-ID: <" + msgID + ">",
		"MIME-Version: 1.0",
		`Content-Type: multipart/mixed; boundary="F"`,
		"",
		"--F",
		"Content-Type: text/plain; charset=utf-8",
		"",
		"Passwort: AlphaSecret",
		"--F",
		"Content-Type: text/plain",
		`Content-Disposition: attachment; filename="access.txt"`,
		"",
		file,
		"--F--",
		"",
	}, "\r\n"))
}

func importedBy(t *testing.T, e *integration.SearchEnv, activityID, user ids.UUID) bool {
	t.Helper()
	return slices.Contains(importRowsOf(t, e, activityID), user)
}

// Masking makes two different passwords read the same, so equal masked text
// proves no replay. A forger copies the colleague's headers, adds themselves on
// Cc and guesses the password; they get a row of their own. Only a party the
// stored row names joins it.
func TestMaskedTextIsNoProofThatTwoMailboxesHoldOneMessage(t *testing.T) {
	env := newCaptureEnv(t)
	e, sync := env.e, env.sync
	syncOther := secondMailbox(t, e, e.Rep3)

	sync(t, passwordMail("pw-forged@acme.example", "anna@acme.example", "AlphaSecret"))
	forged := activityIDOf(t, e, "pw-forged@acme.example")
	syncOther(t, passwordMail("pw-forged@acme.example", secondSeatAddress, "WrongGuess"))
	if importedBy(t, e, forged, e.Rep3) {
		t.Error("a forged twin with another password became an importer of the colleague's row")
	}
	if n := countRows(t, e, `SELECT count(*) FROM activity WHERE kind = 'email'`); n != 2 {
		t.Errorf("%d email rows after the forged twin, want 2: the forger keeps a row of their own", n)
	}

	sync(t, passwordMail("pw-both@acme.example", secondSeatAddress, "AlphaSecret"))
	syncOther(t, passwordMail("pw-both@acme.example", secondSeatAddress, "AlphaSecret"))
	if !importedBy(t, e, activityIDOf(t, e, "pw-both@acme.example"), e.Rep3) {
		t.Error("a colleague the masked mail was addressed to did not join its row")
	}

	sync(t, passwordMail("pw-older@acme.example", secondSeatAddress, "AlphaSecret"))
	older := activityIDOf(t, e, "pw-older@acme.example")
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(),
			`UPDATE activity SET body = replace(body, '[masked]', 'AlphaSecret') WHERE id = $1`, older)
		return err
	}); err != nil {
		t.Fatalf("storing the row as capture stored it before masking: %v", err)
	}
	syncOther(t, passwordMail("pw-older@acme.example", secondSeatAddress, "AlphaSecret"))
	if !importedBy(t, e, older, e.Rep3) {
		t.Error("a recipient's masked copy did not join a row stored before masking")
	}
}

// The masked proof asks for more than a matching address. The address must be
// the mailbox the provider attested for the seat, on the stored row's To or Cc
// line. Text a sender typed as "[masked]" with nothing masked keeps the
// equality proof.
func TestAMaskedReplayNeedsAnAttestedRecipient(t *testing.T) {
	env := newCaptureEnv(t)
	e, sync := env.e, env.sync
	syncOther := secondMailbox(t, e, e.Rep3)

	declareIdentity(t, e, e.Rep3, capturemod.IdentityKindAddress, "alias@myco.example")
	sync(t, passwordMail("pw-alias@acme.example", "alias@myco.example", "AlphaSecret"))
	syncOther(t, passwordMail("pw-alias@acme.example", "alias@myco.example", "AlphaSecret"))
	if importedBy(t, e, activityIDOf(t, e, "pw-alias@acme.example"), e.Rep3) {
		t.Error("a self-declared alias on the Cc line proved a masked replay")
	}

	sent := plainMail(secondSeatAddress, "anna@acme.example", "pw-sent@myco.example", "Passwort: AlphaSecret")
	sync(t, sent)
	syncOther(t, sent)
	if importedBy(t, e, activityIDOf(t, e, "pw-sent@myco.example"), e.Rep3) {
		t.Error("the seat that sent a masked mail became an importer of the recipient's row")
	}

	sync(t, plainMail("anna@acme.example", secondSeatAddress, "literal@acme.example", "hello"))
	syncOther(t, plainMail("anna@acme.example", secondSeatAddress, "literal@acme.example", "hello [masked]"))
	if importedBy(t, e, activityIDOf(t, e, "literal@acme.example"), e.Rep3) {
		t.Error("a typed [masked] with nothing masked skipped the equality proof and joined another text")
	}
}

// On the masked path, as on the text path, the stored files must be ones the
// arriving copy carried.
func TestAMaskedReplayNeedsTheSameFiles(t *testing.T) {
	e := integration.SetupSearch(t)
	seedCaptureRole(t, e)
	seedCaptureRoleFor(t, e, e.Rep3)
	cfg := compose.CaptureConfig{Blob: blobstore.NewMemory()}
	sync := mailboxFor(t, e, e.Rep1, captureOwner, cfg)
	syncOther := mailboxFor(t, e, e.Rep3, secondSeatAddress, cfg)

	sync(t, passwordMailWithFile("pw-file@acme.example", "the original file"))
	syncOther(t, passwordMailWithFile("pw-file@acme.example", "another file"))
	if importedBy(t, e, activityIDOf(t, e, "pw-file@acme.example"), e.Rep3) {
		t.Error("a masked copy carrying another file joined the row")
	}

	sync(t, passwordMailWithFile("pw-same-file@acme.example", "the original file"))
	syncOther(t, passwordMailWithFile("pw-same-file@acme.example", "the original file"))
	if !importedBy(t, e, activityIDOf(t, e, "pw-same-file@acme.example"), e.Rep3) {
		t.Error("a recipient's masked copy carrying the same file did not join the row")
	}
}
