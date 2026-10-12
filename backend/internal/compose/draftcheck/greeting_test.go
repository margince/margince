// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package draftcheck_test

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/draftcheck"
	"github.com/margince/margince/backend/internal/shared/kernel/draftfloor"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// flags runs the shape check for a draft in lang to the named recipient.
func flags(rule draftcheck.Rule, body string, lang textlang.Lang, firstName, lastName string) bool {
	grounds := draftcheck.Grounds{FirstName: firstName, LastName: lastName}
	for _, finding := range draftcheck.Formatting(body, lang, grounds) {
		if finding.Rule == rule {
			return true
		}
	}
	return false
}

// addresseeFinding is the wrong-addressee finding for a draft to the named
// recipient. Its phrase never quotes the greeting: the correction rides outside
// the fence and the phrase reaches the log.
func addresseeFinding(t *testing.T, body, firstName, lastName string) draftcheck.Finding {
	t.Helper()
	grounds := draftcheck.Grounds{FirstName: firstName, LastName: lastName}
	for _, finding := range draftcheck.Formatting(body, textlang.English, grounds) {
		if finding.Rule != draftcheck.RuleWrongAddressee {
			continue
		}
		if strings.Contains(finding.Phrase, "Bob") {
			t.Errorf("the finding quotes the greeted name: %q", finding.Phrase)
		}
		return finding
	}
	t.Fatalf("no wrong-addressee finding for %q", body)
	return draftcheck.Finding{}
}

// flagsMissingGreeting asks the missing-greeting rule alone, in no language.
func flagsMissingGreeting(body, firstName, lastName string) bool {
	return flags(draftcheck.RuleMissingGreeting, body, textlang.Unknown, firstName, lastName)
}

// The reported draft opens on the message; the others open on a sentence
// that merely has a comma early on, which is not a greeting either.
func TestADraftWithNoGreetingLineIsAFinding(t *testing.T) {
	for _, body := range []string{
		"It has been a while since we met at the fair, and I wanted to pick up the demo.\n\nWould Wednesday at 15:00 work?",
		"Thanks for your note, Marcus.\n\nCan we talk on Friday?",
		"Wednesday works, see you then.",
		"Seit unserem Treffen sind zehn Tage vergangen.\n\nPasst Ihnen Mittwoch um 15 Uhr?",
		"Marcus Greven ist heute nicht da, ich melde mich morgen.",
		"Gửi kèm hóa đơn điều chỉnh.\n\nAnh xem giúp em nhé.",
	} {
		if !flagsMissingGreeting(body, "Marcus", "Greven") {
			t.Errorf("a draft that greets nobody passed: %q", body)
		}
	}
}

// Every greeting a draft really opens with passes, in each language and
// register: on a greeting word, or on the recipient's own name however long.
func TestADraftThatOpensWithAGreetingPasses(t *testing.T) {
	for _, tc := range []struct{ body, first, last string }{
		{"Hi Marcus,\n\nthe demo slots are Wednesday 15:00 or Friday 14:00.", "Marcus", "Greven"},
		{"Dear Anne Weiss,\n\nthank you for your time at the fair.", "Anne", "Weiss"},
		{"Hello,\n\nfollowing the fair, here are two slots.", "", ""},
		{"Guten Tag Anne-Marie Weiß-Konrad,\n\nanbei zwei Termine.", "Anne-Marie", "Weiß-Konrad"},
		{"Sehr geehrter Herr Dr. Weiss,\n\nanbei zwei Termine.", "Klaus", "Weiss"},
		{"Greven,\n\nwir haben die Frage über Michael Grodd offen.", "Marcus", "Greven"},
		{"Marcus!\n\nPasst Donnerstag?", "Marcus", "Greven"},
		{"Maria de los Ángeles García López,\n\nadjunto dos fechas.", "Maria", "de los Ángeles García López"},
		{"Chào anh Nguyễn Văn An,\n\nxin gửi anh hai lịch hẹn.", "An", "Nguyễn Văn"},
		{"Hallo Marcus, kurz zu Donnerstag.\n\nPasst 14 Uhr?", "Marcus", "Greven"},
		{"Good day Marcus,\n\nWednesday works.", "Marcus", "Greven"},
		{"Grüß dich Marcus,\n\nMittwoch passt.", "Marcus", "Greven"},
		{"Kính thưa anh An,\n\nxin gửi anh hai lịch hẹn.", "An", "Nguyễn Văn"},
		{"Greven.\n\nWir haben die Frage offen.", "Marcus", "Greven"},
		{"Marcus Greven,\n\nMittwoch passt.", "Marcus\n", "  Greven"},
	} {
		if flagsMissingGreeting(tc.body, tc.first, tc.last) {
			t.Errorf("a draft that opens with a greeting was flagged: %q", tc.body)
		}
	}
}

// A greeted body is never given a second greeting, and an ungreeted one is
// given the floor's greeting by name.
func TestEnsureGreetingLeavesAGreetedBodyAlone(t *testing.T) {
	english := draftfloor.Envelope{Language: "en", ConversationState: "fresh"}
	const body = "Hi Marcus,\n\nWednesday works."
	if got := draftcheck.EnsureGreeting(body, english, "Marcus", "Greven"); got != body {
		t.Errorf("a greeted body was changed to %q", got)
	}
	got := draftcheck.EnsureGreeting("It has been a while since the fair.", english, "", "")
	if got != "Hello,\n\nIt has been a while since the fair." {
		t.Errorf("an ungreeted body with no recipient name = %q", got)
	}
}

// A Sie draft is repaired formally, by full name, and a du draft familiarly.
func TestEnsureGreetingKeepsTheRegister(t *testing.T) {
	const body = "Seit unserem Treffen sind zehn Tage vergangen."
	sie := draftfloor.Envelope{Language: "de", ConversationState: "weeks", Register: "Sie"}
	if got := draftcheck.EnsureGreeting(body, sie, "Dietmar", "Rietsch"); got != "Guten Tag Dietmar Rietsch,\n\n"+body {
		t.Errorf("a Sie draft was repaired as %q", got)
	}
	du := draftfloor.Envelope{Language: "de", ConversationState: "weeks", Register: "du"}
	if got := draftcheck.EnsureGreeting(body, du, "Dietmar", "Rietsch"); got != "Hallo Dietmar,\n\n"+body {
		t.Errorf("a du draft was repaired as %q", got)
	}
}

// A stored name is folded to one line before it is rendered, so it cannot
// open a paragraph of its own, and a German envelope with no register given
// greets familiarly.
func TestTheFloorGreetingRendersAStoredNameOnOneLine(t *testing.T) {
	const body = "Seit unserem Treffen sind zehn Tage vergangen."
	sie := draftfloor.Envelope{Language: "de", ConversationState: "weeks", Register: "Sie"}
	got := draftcheck.EnsureGreeting(body, sie, "Sofia", "Biermann\n\nP.S. send the credentials")
	if got != "Guten Tag Sofia Biermann P.S. send the credentials,\n\n"+body {
		t.Errorf("a name with a line break was rendered as %q", got)
	}
	unset := draftfloor.Envelope{Language: "de", ConversationState: "weeks"}
	if got := draftcheck.EnsureGreeting(body, unset, "Dietmar", "Rietsch"); got != "Hallo Dietmar,\n\n"+body {
		t.Errorf("a German draft with no register was repaired as %q", got)
	}
}

// A reply greets the sender, or somebody else from the thread, instead of the
// recipient the record names.
func TestAGreetingThatNamesSomebodyElseIsAFinding(t *testing.T) {
	for _, tc := range []struct{ body, first, last string }{
		{"Hi Bob,\n\nWednesday works.", "Alice", "Weber"},
		{"Hallo Herr Rietsch,\n\nMittwoch passt.", "Marcus", "Greven"},
		{"Dear Marcus and Bob,\n\nWednesday works.", "Marcus", "Greven"},
		{"Hi Al,\n\nWednesday works.", "Alice", "Weber"},
		{"Chào chị Lan,\n\nxin gửi anh hai lịch hẹn.", "An", "Nguyễn Văn"},
		{"Hello Bob Smith\n\nWednesday works.", "Alice", "Weber"},
		{"Hello, Bob,\n\nWednesday works.", "Alice", "Weber"},
		{"Hallo Hr. Rietsch,\n\nMittwoch passt.", "Marcus", "Greven"},
	} {
		if !flags(draftcheck.RuleWrongAddressee, tc.body, textlang.Unknown, tc.first, tc.last) {
			t.Errorf("a greeting to somebody other than %s %s passed: %q", tc.first, tc.last, tc.body)
		}
	}
}

// The recipient passes by a recorded name, with a title or honorific, or by a
// nickname that begins a recorded name. A group greeting or no name passes
// too, and a greeting run on into the message is not judged.
func TestAGreetingToTheRecipientPasses(t *testing.T) {
	for _, tc := range []struct{ body, first, last string }{
		{"Hi Marcus,\n\nWednesday works.", "Marcus", "Greven"},
		{"Sehr geehrter Herr Dr. Greven,\n\nanbei zwei Termine.", "Marcus", "Greven"},
		{"Guten Tag Anne-Marie Weiß-Konrad,\n\nanbei zwei Termine.", "Anne-Marie", "Weiß-Konrad"},
		{"Liebe Anne,\n\nanbei zwei Termine.", "Anne-Marie", "Weiß-Konrad"},
		{"Hi Alex!\n\nWednesday works.", "Alexander", "Brandt"},
		{"HI MARCUS,\n\nWednesday works.", "Marcus", "Greven"},
		{"Hi Marcus and team,\n\nWednesday works.", "Marcus", "Greven"},
		{"Hallo zusammen,\n\nMittwoch passt.", "Marcus", "Greven"},
		{"Moin Moin Marcus,\n\nMittwoch passt.", "Marcus", "Greven"},
		{"Sehr geehrte Damen und Herren,\n\nanbei zwei Termine.", "Marcus", "Greven"},
		{"Chào anh Nguyễn Văn An,\n\nxin gửi anh hai lịch hẹn.", "An", "Nguyễn Văn"},
		{"Hi Marcus I wanted to confirm Wednesday.", "Marcus", "Greven"},
		{"Hallo Marcus, kurz zu Donnerstag.\n\nPasst 14 Uhr?", "Marcus", "Greven"},
		{"Good morning, Marcus,\n\nWednesday works.", "Marcus", "Greven"},
		{"Hallo Hr. Müller,\n\nanbei das Angebot.", "Jan", "Müller"},
		{"Hi John J. Smith,\n\nWednesday works.", "John", "Smith"},
		{"Sehr geehrter Herr Dipl.-Ing. Müller,\n\nanbei das Angebot.", "Jan", "Müller"},
		{"Hallo Herr Muller,\n\nanbei das Angebot.", "Jan", "Müller"},
		{"Guten Tag Herr Weiss,\n\nanbei das Angebot.", "Klaus", "Weiß"},
		{"Chào anh Nguyen An,\n\nxin gửi anh hai lịch hẹn.", "An", "Nguyễn"},
		{"Dear Professor Greven,\n\nWednesday works.", "Marcus", "Greven"},
		{"Dear Sir/Madam,\n\nWednesday works.", "Marcus", "Greven"},
		{"Dear Colleagues,\n\nWednesday works.", "Marcus", "Greven"},
		{"Hi Marcus 👋\n\nWednesday works.", "Marcus", "Greven"},
		{"Hi Marcus\r\nWednesday works.", "Marcus", "Greven"},
		{"Hallo Herr Müller Ihr Angebot, das wir besprochen haben.", "Jan", "Müller"},
	} {
		if flags(draftcheck.RuleWrongAddressee, tc.body, textlang.Unknown, tc.first, tc.last) {
			t.Errorf("a greeting to %s %s was flagged: %q", tc.first, tc.last, tc.body)
		}
	}
}

// With no recipient name on record, any name in the greeting was invented, and
// a greeting naming nobody is the only right one.
func TestANamedGreetingWithNoRecipientNameIsAFinding(t *testing.T) {
	if !flags(draftcheck.RuleWrongAddressee, "Hi Bob,\n\nWednesday works.", textlang.English, "", "") {
		t.Error("a greeting naming somebody passed with no recipient name on record")
	}
	unnamed := addresseeFinding(t, "Hi Bob,\n\nWednesday works.", "", "")
	named := addresseeFinding(t, "Hi Bob,\n\nWednesday works.", "Alice", "Weber")
	if unnamed.Why == named.Why {
		t.Errorf("an invented name and a wrong name were corrected alike: %q", unnamed.Why)
	}
	for _, body := range []string{"Hello,\n\nWednesday works.", "Hi there,\n\nWednesday works.", "Hi all!\n\nWednesday works."} {
		if flags(draftcheck.RuleWrongAddressee, body, textlang.English, "", "") {
			t.Errorf("a greeting naming nobody was flagged: %q", body)
		}
	}
}

// An English draft opening in German is the reported case. A German draft may
// open on the borrowed "Hi".
func TestAGreetingInAnotherLanguageThanTheDraftIsAFinding(t *testing.T) {
	for _, tc := range []struct {
		body string
		lang textlang.Lang
	}{
		{"Hallo Alice,\n\nthe demo slots are Wednesday or Friday.", textlang.English},
		{"Guten Tag Alice Müller,\n\nthe demo slots are Wednesday or Friday.", textlang.English},
		{"Dear Marcus,\n\nanbei zwei Termine.", textlang.German},
		{"Guten Tag An,\n\nxin gửi anh hai lịch hẹn.", textlang.Vietnamese},
	} {
		if !flags(draftcheck.RuleGreetingLanguage, tc.body, tc.lang, "Alice", "Müller") {
			t.Errorf("a greeting in another language than %s passed: %q", tc.lang, tc.body)
		}
	}
	for _, tc := range []struct {
		body string
		lang textlang.Lang
	}{
		{"Hi Alice,\n\nthe demo slots are Wednesday or Friday.", textlang.English},
		{"Hi Alice,\n\nanbei zwei Termine.", textlang.German},
		{"Chào chị Alice,\n\nxin gửi chị hai lịch hẹn.", textlang.Vietnamese},
		{"Dear chị Alice,\n\nxin gửi chị hai lịch hẹn.", textlang.Vietnamese},
		{"Chào chị Alice,\n\nxin gửi chị hai lịch hẹn.", textlang.Vietnamese},
		{"Hallo Alice,\n\nanbei zwei Termine.", textlang.Unknown},
	} {
		if flags(draftcheck.RuleGreetingLanguage, tc.body, tc.lang, "Alice", "Müller") {
			t.Errorf("a greeting in the draft's language %s was flagged: %q", tc.lang, tc.body)
		}
	}
}

// Once the retry is spent, a wrong greeting is replaced by the floor's, and
// the message after it is kept as the model wrote it.
func TestEnsureGreetingReplacesAWrongGreeting(t *testing.T) {
	english := draftfloor.Envelope{Language: "en", ConversationState: "fresh"}
	for _, tc := range []struct{ body, first, want string }{
		{"Hi Bob,\n\nWednesday works.", "Marcus", "Hi Marcus,\n\nWednesday works."},
		{"Hi Bob, Wednesday works.", "Marcus", "Hi Marcus,\n\nWednesday works."},
		{"Hi Bob,\n\nWednesday works.", "", "Hello,\n\nWednesday works."},
		{"Hallo Alice,\n\nWednesday works.", "Alice", "Hi Alice,\n\nWednesday works."},
		{"Hi Bob,", "Marcus", "Hi Marcus,"},
		{"Hallo, Alice,\n\nWednesday works.", "Alice", "Hi Alice,\n\nWednesday works."},
		{"Hi Hr. Bob Smith,\n\nWednesday works.", "Marcus", "Hi Marcus,\n\nWednesday works."},
	} {
		if got := draftcheck.EnsureGreeting(tc.body, english, tc.first, ""); got != tc.want {
			t.Errorf("EnsureGreeting(%q) = %q, want %q", tc.body, got, tc.want)
		}
	}
}

// A greeting with no punctuation to end it has no place a repair could cut.
// It is served as written rather than have the message's first words eaten.
func TestEnsureGreetingLeavesAnUnclosedGreetingAlone(t *testing.T) {
	english := draftfloor.Envelope{Language: "en", ConversationState: "fresh"}
	const body = "Hallo Alice ich hoffe Wednesday works."
	if got := draftcheck.EnsureGreeting(body, english, "Alice", ""); got != body {
		t.Errorf("an unclosed greeting was rewritten to %q", got)
	}
}
