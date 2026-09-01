// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// The audit image redaction, in both directions.
//
// The first version of this blanked the whole image for every activity row, and
// that destroyed the audit record of the audience change ITSELF — the one act a
// compliance reader most needs to see, and the one carrying no content at all.
// So the interesting cases here are the images that must SURVIVE, not the one
// that must not.

import (
	"encoding/json"
	"testing"
)

func TestRedactionKeepsGovernanceAndDropsContent(t *testing.T) {
	for _, tc := range []struct {
		name string
		// entityType selects the key set. Empty means "activity", so the cases
		// that predate the collateral types read exactly as they did.
		entityType string
		image      string
		wantKeys   []string
		gone       []string
	}{{
		// The audience change's own audit row. Written by SetAudience, and the
		// reason this test exists: withholding it tells a compliance reader
		// nothing about content and hides an administrative act from them.
		name:     "an audience change survives whole",
		image:    `{"audience":"participants","member_count":3}`,
		wantKeys: []string{"audience", "member_count"},
	}, {
		name:     "a relink survives whole",
		image:    `{"entity_type":"deal","entity_id":"a-uuid","replaced":true}`,
		wantKeys: []string{"entity_type", "entity_id", "replaced"},
	}, {
		// LogActivity's image. `subject` is the free text the audience governs.
		// `kind` SURVIVES, and that is not a concession: the activity read
		// surface answers kind on a withheld row and nils only subject, body and
		// source_id (activityread.go). The audit image follows the record's own
		// rule rather than inventing a stricter one.
		name:     "a subject is dropped while the kind marker survives",
		image:    `{"kind":"email","subject":"Q3 renewal terms"}`,
		wantKeys: []string{"kind", "content_state"},
		gone:     []string{"subject"},
	}, {
		// source_id is the other half of that rule: the read surface nils it
		// because it identifies the message at the provider, so this endpoint
		// must not answer it either.
		name:     "a provider message id is dropped like the read surface drops it",
		image:    `{"kind":"email","source_system":"gmail","source_id":"CADnq=abc@mail.gmail.com"}`,
		wantKeys: []string{"kind", "source_system", "content_state"},
		gone:     []string{"source_id"},
	}, {
		// updateDelta reduces body to a presence flag and says so, so the flag
		// itself discloses nothing and survives — while subject, which it does
		// NOT reduce, does not.
		name:     "a mixed delta keeps its metadata and loses its content",
		image:    `{"subject":"Q3 renewal terms","body":true,"is_done":false,"occurred_at":"2026-01-01T00:00:00Z"}`,
		wantKeys: []string{"body", "is_done", "occurred_at", "content_state"},
		gone:     []string{"subject"},
	}, {
		// The guard that does NOT depend on the writer. updateDelta reduces body
		// to a presence flag today and says so in a comment; a one-line edit
		// making it the body itself would, without this, hand an out-of-audience
		// admin the confidential text of a limited conversation.
		name:     "a body carrying text rather than presence is dropped",
		image:    `{"body":"the confidential text itself","is_done":true}`,
		wantKeys: []string{"is_done", "content_state"},
		gone:     []string{"body"},
	}, {
		name:     "a body that is still a presence flag survives",
		image:    `{"body":false,"is_done":true}`,
		wantKeys: []string{"body", "is_done"},
	}, {
		// JSON null is the shape a bare Unmarshal-into-bool admits, and it would
		// survive with NO marker — the only way a non-boolean body could pass
		// unannounced.
		name:     "a null body is dropped like any other non-boolean",
		image:    `{"body":null,"is_done":true}`,
		wantKeys: []string{"is_done", "content_state"},
		gone:     []string{"body"},
	}, {
		// Fail closed: a key nobody classified is content until proven otherwise.
		name:     "an unrecognised key is dropped rather than trusted",
		image:    `{"audience":"selected","some_new_field":"whatever this is"}`,
		wantKeys: []string{"audience", "content_state"},
		gone:     []string{"some_new_field"},
	}, {
		// The leak this closes. capturedfiles.go stores an email attachment
		// under its user-supplied filename, and that name is the thing a
		// termination letter or a diagnosis gives away on its own.
		name:       "an attachment filename is dropped while the file's shape survives",
		entityType: "attachment",
		image:      `{"entity_type":"activity","entity_id":"a-uuid","filename":"Kuendigung.pdf","byte_size":8241,"category":"email_attachment"}`,
		wantKeys:   []string{"entity_type", "entity_id", "byte_size", "category", "content_state"},
		gone:       []string{"filename"},
	}, {
		// A human-authored document title names the file's content as surely as
		// the filename does.
		name:       "a document title is dropped like a filename",
		entityType: "attachment",
		image:      `{"title":"Aufhebungsvertrag Entwurf","pinned":true}`,
		wantKeys:   []string{"pinned", "content_state"},
		gone:       []string{"title"},
	}, {
		// The direction that matters as much as the leak: withholding this
		// image WHOLE would destroy "who asked to read this file", which is the
		// governance record a compliance reader came for. Both keys survive and
		// nothing is marked withheld.
		name:       "an extraction request survives whole so the governance record is not destroyed",
		entityType: "attachment_extraction",
		image:      `{"attachment_id":"a-uuid","requested_by":"human:b-uuid"}`,
		wantKeys:   []string{"attachment_id", "requested_by"},
	}, {
		// line_count measures the transcript, so it is content by the same
		// argument that drops a subject: it tells a reader outside the audience
		// how long the held conversation was. It also survives the transcript's
		// own erasure — the purge deletes the routing row without a tombstone,
		// so the route resolves NULL and this same redactor runs again.
		name:       "a transcript read keeps who read it and loses how long it was",
		entityType: "transcript_read",
		image:      `{"activity_id":"a-uuid","requested_by":"human:b-uuid","status":"done","line_count":412}`,
		wantKeys:   []string{"activity_id", "requested_by", "status", "content_state"},
		gone:       []string{"line_count"},
	}, {
		name:       "a finished extraction keeps its job progress",
		entityType: "attachment_extraction",
		image:      `{"status":"done","grounded":4}`,
		wantKeys:   []string{"status", "grounded"},
	}, {
		name:       "a hidden deal document keeps which deal and which way",
		entityType: "attachment",
		image:      `{"deal_id":"a-uuid","hidden_from_deal":true}`,
		wantKeys:   []string{"deal_id", "hidden_from_deal"},
	}, {
		// scheduledsend.go writes the message's own subject line onto this
		// image, so the send is a content carrier exactly like the activity.
		name:       "a scheduled send loses its subject and keeps its schedule",
		entityType: "scheduled_send",
		image:      `{"scheduled_at":"2026-03-01T09:00:00Z","scheduled_tz":"Europe/Berlin","subject":"Re: Aufhebungsvertrag"}`,
		wantKeys:   []string{"scheduled_at", "scheduled_tz", "content_state"},
		gone:       []string{"subject"},
	}, {
		// Fail closed at the TYPE level, not just the key level: a governed type
		// nobody gave keys to withholds everything rather than falling through
		// to a permissive default.
		name:       "an entity type with no key set withholds every key",
		entityType: "some_new_collateral",
		image:      `{"attachment_id":"a-uuid","requested_by":"human:b-uuid"}`,
		wantKeys:   []string{"content_state"},
		gone:       []string{"attachment_id", "requested_by"},
	}} {
		t.Run(tc.name, func(t *testing.T) {
			entityType := tc.entityType
			if entityType == "" {
				entityType = "activity"
			}
			got := redactAuditImage([]byte(tc.image), governanceKeysFor(entityType))
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(got, &fields); err != nil {
				t.Fatalf("redacted image is not an object: %s (%v)", got, err)
			}
			for _, key := range tc.wantKeys {
				if _, ok := fields[key]; !ok {
					t.Errorf("%q was redacted away; the audience has no claim over it: %s", key, got)
				}
			}
			for _, key := range tc.gone {
				if _, ok := fields[key]; ok {
					t.Errorf("%q survived redaction: %s", key, got)
				}
			}
			if len(tc.gone) > 0 {
				if _, ok := fields["content_state"]; !ok {
					t.Errorf("something was dropped and nothing said so — an absent key and a "+
						"withheld one are different answers: %s", got)
				}
			}
			if len(tc.gone) == 0 && string(got) != tc.image {
				t.Errorf("an image needing no redaction was rewritten: %s, want %s", got, tc.image)
			}
		})
	}
}

func TestRedactionLeavesAnAbsentImageAbsent(t *testing.T) {
	// A row that carried no image must not gain a marker: "nothing was
	// recorded" and "you may not see what was recorded" are the two answers a
	// compliance reader is trying to tell apart.
	if got := redactAuditImage(nil, auditImageGovernanceKeys); got != nil {
		t.Errorf("a nil image became %s; it must stay absent", got)
	}
	if got := redactAuditImage([]byte{}, auditImageGovernanceKeys); len(got) != 0 {
		t.Errorf("an empty image became %s; it must stay absent", got)
	}
}

func TestRedactionWithholdsAnImageItCannotParse(t *testing.T) {
	// A scalar or array image cannot be partially redacted. Passing it through
	// would be the disclosure, so the unreadable case withholds.
	for _, image := range []string{`"a bare string"`, `[1,2,3]`, `{not json`} {
		got := string(redactAuditImage([]byte(image), auditImageGovernanceKeys))
		if got == image {
			t.Errorf("an unparseable image %s was passed through unredacted", image)
		}
		if got != string(auditWithheldImage) {
			t.Errorf("unparseable image %s → %s, want the withheld marker", image, got)
		}
	}
}
