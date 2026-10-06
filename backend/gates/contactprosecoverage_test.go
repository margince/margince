// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H3

package gates

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// Every table with a foreign key to contact and a column prose fits in is
// registered or carries a reason.
//
// piiTables (piicoverage_test.go) is the registry that makes a table PII-bearing,
// and the censuses then prove erasure and SAR reach it. What nothing asked was
// whether a table SHOULD be in it: a new table with a contact_id and a note
// column joined the schema unexamined, and the coverage gates went on reporting
// full coverage of the registry they were handed.
//
// So the corpus is derived from the catalog — a foreign key to contact(id), plus
// a column that can hold a sentence — and every member is either registered or
// carries a reason here. Both directions fail: an unaccounted table fails
// below, and a reason for a table that has since been registered, lost its
// prose or lost its contact key fails as a stale subject.
//
// WHAT THIS CANNOT SEE, stated because a census that quietly reads short is
// worse than none: a table holding subject prose with no foreign key to
// contact. Three registered tables are exactly that shape — attachment,
// raw_capture and embedding carry a subject's data with no contact column — so
// the shape is real and this corpus would miss a fourth. A polymorphic
// entity_id pair is the same blind spot (#6548 tracks those), as is a contact
// named only inside a jsonb payload. Registration remains the act that declares
// a table PII-bearing; this gate narrows how many can go unconsidered, and does
// not close the question.
var contactProseWithoutSubjectData = gatekit.Waive(map[string]string{
	"relationship":                   "the edge itself, whose prose columns are vocabulary rather than prose about anybody: kind, role, employment_status, the two precisions, source and the captured_by principal. Held under Art. 5 accountability, which is why the registry's own comment names it as deliberately out",
	"contact_consent":                "the consent state machine: lawful_basis, policy_version, source and state are enums and a version string. The subject's identity lives on the contact row this points at, and the proof of what they agreed to is the point of keeping it",
	"consent_event":                  "the consent proof log. confirm_ip and confirm_user_agent ARE the subject's personal data and are retained deliberately — they are the evidence that this human, at this address, gave this consent, which is the Art. 5 record a controller must be able to produce. Erasing it would destroy the lawfulness proof for the processing that preceded it",
	"consent_qualifying_event":       "the same proof log one step earlier, recording what qualified the subject for a consent request; note is the operator's reason for that qualification. Retained on the same Art. 5 basis as consent_event",
	"consent_doi_token":              "double-opt-in tokens: token_hash is a hash, and the addresses the token was sent to live in the consent event beside it",
	"activity_link":                  "the join between an activity and what it is about; entity_type is a discriminator naming a table, not a word about anybody",
	"contact_moment_dismissal":       "which moment card a reader dismissed: claim_key is a derived key and evidence_fingerprint a hash of what the card cited. Neither reconstructs the citation, and the row is the reader's own choice rather than a record about the subject",
	"relationship_nudge_dismissal":   "the same shape for a relationship nudge; set_by names the colleague who dismissed it, not the subject",
	"provider_employment_resolution": "the provider resolver's own bookkeeping: episode_key is a derived key and state its machine's position",
	"signal":                         "the warm-room signal substrate. Its evidence carries SNIPPETS of activity bodies, which is subject prose — and it is retained for the reason the timeline's own bodies are: what was said is the seller's record of a conversation, erased only where the body it was copied from is. A snippet outliving its source would be the defect; the timeline redaction is what must reach it, so this is a declaration about WHERE the obligation sits, not that none exists",
	"dedupe_candidate":               "erased already, and by name: scrubDedupeEvidence empties the evidence snapshot on both privacy acts (dedupeevidencescrub.go). Unregistered because the row that survives is the detector's memory that this pair was judged, which holds no subject data once the snapshot is gone",
	"intro_request":                  "erased already: redactIntroductionRequests clears forwardable_note, value_for_target, internal_reason and decision_reason and cancels what is still open (erasureintroductions.go). Registering it would oblige an Art. 15 section for an intro ask, which is a question about derived artifacts rather than a gap in the erasure",
	"contact_brief":                  "erased already: purgeSubjectBriefCache deletes every reader's cached brief on both acts (erasurebriefcache.go). Unregistered for the same reason as intro_request — whether a regenerable cache belongs in an Art. 15 package is a product question, and the cache is destroyed either way",
})

var (
	contactForeignKeyLine = regexp.MustCompile(`^public\.([a-z_0-9]+)\.[a-z_0-9]+ FOREIGN KEY \([^)]*\) REFERENCES contact\(id\)`)
	// Types that can hold a sentence. `vector` is left out as the registry
	// leaves it out of SAR: an embedding is opaque and hands nothing back.
	proseColumnLine = regexp.MustCompile(`^public\.([a-z_0-9]+)\.[a-z_0-9]+ (text|jsonb|character|tsvector)(\[\])?(\s|$)`)
)

// contactProseTables derives the corpus: tables that name a contact by foreign
// key AND carry a column prose can be stored in.
func contactProseTables(t *testing.T) []string {
	t.Helper()
	raw, err := os.ReadFile("migrations/testdata/head_catalog.txt")
	if err != nil {
		t.Fatalf("reading the head catalog: %v", err)
	}
	namesContact, hasProse := map[string]bool{}, map[string]bool{}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if m := contactForeignKeyLine.FindStringSubmatch(line); m != nil {
			namesContact[m[1]] = true
		}
		if m := proseColumnLine.FindStringSubmatch(line); m != nil {
			hasProse[m[1]] = true
		}
	}
	var corpus []string
	for table := range namesContact {
		if hasProse[table] {
			corpus = append(corpus, table)
		}
	}
	sort.Strings(corpus)
	return corpus
}

func TestEveryContactReferencingProseTableIsRegisteredOrAccountedFor(t *testing.T) {
	t.Parallel()
	corpus := contactProseTables(t)
	// The corpus is the whole set, which does not shrink as tables are
	// registered: a floor on the UNACCOUNTED set would fall as the work
	// succeeded and report the census broken.
	if len(corpus) < 30 {
		t.Fatalf("the corpus is %d contact-referencing prose tables, which is too few to be the "+
			"schema — the catalog's line shape has probably changed under %s",
			len(corpus), "contactForeignKeyLine/proseColumnLine")
	}
	defer contactProseWithoutSubjectData.AssertAllMatched(t)
	for _, table := range corpus {
		if _, registered := piiTables[table]; registered {
			continue
		}
		if contactProseWithoutSubjectData.Waived(t, table) {
			continue
		}
		t.Errorf("%s names a contact and holds a column prose fits in, and nothing says what that "+
			"means for a subject's erasure. Register it in piiTables (piicoverage_test.go), which "+
			"obliges erasure and SAR to reach it, or give it a reason in "+
			"contactProseWithoutSubjectData naming what the columns actually hold — and, where a "+
			"privacy act already clears them, the function that does it.", table)
	}
	for _, table := range contactProseWithoutSubjectData.Subjects() {
		if _, registered := piiTables[table]; registered {
			t.Errorf("contactProseWithoutSubjectData explains %q, which piiTables now registers — "+
				"the registry's censuses answer for it, so delete the reason here", table)
		}
	}
}
