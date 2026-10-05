// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/mailcopy"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type stubSignature struct {
	body    string
	err     error
	askedID ids.UUID
}

func (s *stubSignature) SignatureFor(_ context.Context, userID ids.UUID) (string, error) {
	s.askedID = userID
	return s.body, s.err
}

func humanCtx(userID ids.UUID) context.Context {
	return principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + userID.String(), UserID: userID,
	})
}

// signedBody is the plain part the send path builds: the sign-off for this
// message, beneath it.
func signedBody(ctx context.Context, t *testing.T, store *Store, body string) string {
	t.Helper()
	sign, err := store.signOff(ctx, body, "")
	if err != nil {
		t.Fatalf("resolving the sign-off failed: %v", err)
	}
	return sign.under(body)
}

// The sign-off goes under the message the rep wrote, separated by a blank line
// — which is what makes it read as theirs rather than as another paragraph.
// A sender who wrote a signature gets that signature and nothing else: no
// closing, and no name the signature did not write itself.
func TestASignatureIsAppendedBeneathTheMessage(t *testing.T) {
	store := (&Store{}).
		WithSignature(&stubSignature{body: "Marek Janetzke\nGradion"}).
		WithSenderName(&stubSenderName{name: "Marek J."})

	got := signedBody(humanCtx(ids.NewV7()), t, store, "Shall we say Tuesday at 10?")
	if got != "Shall we say Tuesday at 10?\n\nMarek Janetzke\nGradion" {
		t.Fatalf("unexpected signed body:\n%q", got)
	}
}

// The separator is a blank line, never the "-- " sig-dash. This product's own
// reply parser treats that dash as a signature boundary and cuts everything
// below it, so writing one would make our captured copy of the thread end at
// the signature we just added.
func TestTheSeparatorIsNotASigDash(t *testing.T) {
	store := (&Store{}).WithSignature(&stubSignature{body: "Marek"})

	got := signedBody(humanCtx(ids.NewV7()), t, store, "Body")
	if strings.Contains(got, "\n-- \n") || strings.Contains(got, "\n--\n") {
		t.Fatalf("the signature was introduced by a sig-dash:\n%q", got)
	}
}

// A sender who wrote no signature still signs off: a plain closing in the
// message's own language, above their name. Short notes the detector cannot
// place fall to the installation's language, then to English.
func TestASenderWithNoSignatureClosesWithTheirName(t *testing.T) {
	cases := map[string]struct {
		signature, body, base, want string
	}{
		"english message": {
			body: "Hi Anna, as discussed I am sending you the offer and the documents for the project. I look forward to your reply.",
			want: "Best regards,\nLars Jankowfsky",
		},
		"german message": {
			body: "Hallo Anna, wie besprochen schicke ich dir das Angebot und die Unterlagen für das Projekt. Ich freue mich auf deine Rückmeldung.",
			want: "Viele Grüße,\nLars Jankowfsky",
		},
		"vietnamese message": {
			body: "Chào anh, em gửi anh bản báo giá và tài liệu dự án như đã trao đổi. Mong nhận được phản hồi của anh.",
			want: "Trân trọng,\nLars Jankowfsky",
		},
		"too short to tell, german installation":      {body: "Danke!", base: "de", want: "Viele Grüße,\nLars Jankowfsky"},
		"too short to tell, no installation language": {body: "Thx", want: "Best regards,\nLars Jankowfsky"},
		"a signature of only spaces":                  {signature: "  \n ", body: "Thx", want: "Best regards,\nLars Jankowfsky"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			store := (&Store{}).
				WithSignature(&stubSignature{body: tc.signature}).
				WithSenderName(&stubSenderName{name: "Lars Jankowfsky"}).
				WithBaseLanguage(BaseLanguageFunc(func(context.Context) string { return tc.base }))

			sign, err := store.signOff(humanCtx(ids.NewV7()), tc.body, "")
			if err != nil {
				t.Fatalf("resolving the sign-off failed: %v", err)
			}
			if sign.Kind != SignOffClosing || sign.Text != tc.want {
				t.Fatalf("sign-off = %+v, want closing %q", sign, tc.want)
			}
			if got := sign.under(tc.body); got != tc.body+"\n\n"+tc.want {
				t.Fatalf("the closing is not beneath the message: %q", got)
			}
		})
	}
}

// A member with no display name on file still gets the closing, without a
// blank line where the name would be.
func TestAClosingWithNoNameOnFileIsTheClosingAlone(t *testing.T) {
	store := (&Store{}).WithSignature(&stubSignature{}).WithSenderName(&stubSenderName{})

	if got := signedBody(humanCtx(ids.NewV7()), t, store, "Thx"); got != "Thx\n\nBest regards," {
		t.Fatalf("unexpected signed body: %q", got)
	}
}

// A role wired without the seam sends unsigned rather than refusing to send.
func TestNoSignatureReaderSendsUnsigned(t *testing.T) {
	store := (&Store{}).WithSenderName(&stubSenderName{name: "Lars Jankowfsky"})

	if got := signedBody(humanCtx(ids.NewV7()), t, store, "Body"); got != "Body" {
		t.Fatalf("the body changed with no reader wired: %q", got)
	}
}

// An agent acts under a human's authority but is not that human. A tool-written
// message arriving under somebody's personal sign-off — or under their name
// beneath a closing — claims a hand that never touched it, so the agent path
// asks for no signature and writes no closing.
func TestAnAgentSendSignsNothing(t *testing.T) {
	reader := &stubSignature{}
	store := (&Store{}).WithSignature(reader).WithSenderName(&stubSenderName{name: "Lars Jankowfsky"})
	ctx := principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:assistant", UserID: ids.NewV7(),
	})

	sign, err := store.signOff(ctx, "Body", "")
	if err != nil {
		t.Fatalf("resolving the sign-off failed: %v", err)
	}
	if sign.Kind != SignOffNone || sign.under("Body") != "Body" {
		t.Fatalf("an agent send was signed: %+v", sign)
	}
	if reader.askedID != ids.Nil {
		t.Fatal("an agent send asked for a signature it may not use")
	}
}

// The signature is read for the AUTHENTICATED sender and nobody else, which is
// what keeps one member's sign-off off another member's mail.
func TestTheSignatureIsReadForTheAuthenticatedSender(t *testing.T) {
	user := ids.NewV7()
	reader := &stubSignature{body: "Marek"}
	store := (&Store{}).WithSignature(reader)

	signedBody(humanCtx(user), t, store, "Body")
	if reader.askedID != user {
		t.Fatalf("asked for %s, expected the sender %s", reader.askedID, user)
	}
}

// A read that fails is not silently swallowed: sending a message the sender
// believes is signed, unsigned, is a change to what they put their name to.
// The same holds for the name beneath a closing.
func TestAFailedSignOffReadRefusesTheSend(t *testing.T) {
	boom := errors.New("database is down")
	for name, store := range map[string]*Store{
		"signature": (&Store{}).WithSignature(&stubSignature{err: boom}),
		"name":      (&Store{}).WithSignature(&stubSignature{}).WithSenderName(&stubSenderName{err: boom}),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := store.signOff(humanCtx(ids.NewV7()), "Body", ""); !errors.Is(err, boom) {
				t.Fatalf("expected the read error to surface, got %v", err)
			}
		})
	}
}

// The markup alternative carries the SAME sign-off as the plain one. Two parts
// of a message that disagreed would be two messages, and which one a recipient
// reads is their client's decision rather than ours.
func TestTheMarkupAlternativeCarriesTheSameSignOff(t *testing.T) {
	sign := SignOff{Text: "Marek Janetzke\nGradion", Kind: SignOffSignature}

	got := signedHTML("<p>Shall we say Tuesday?</p>", sign, sendDeliverability{})
	if !strings.Contains(got, "Marek Janetzke<br>Gradion") {
		t.Fatalf("the markup lost the sign-off or its line break: %q", got)
	}
}

// The signature is stored as plain text and reaches a markup document, so it is
// escaped. A member whose sign-off contains "Weiß & Konrad <Recht>" must not
// have it become a broken tag, and one who typed a script tag must not have it
// run in the recipient's client.
func TestASignatureCannotInjectMarkup(t *testing.T) {
	sign := SignOff{Text: `Weiß & Konrad <Recht><script>alert(1)</script>`, Kind: SignOffSignature}

	got := signedHTML("<p>Body</p>", sign, sendDeliverability{})
	if strings.Contains(got, "<script>") {
		t.Fatalf("a signature injected live markup: %q", got)
	}
	if !strings.Contains(got, "Wei&#223; &amp; Konrad") && !strings.Contains(got, "Weiß &amp; Konrad") {
		t.Fatalf("the signature was not escaped as text: %q", got)
	}
}

// A message with no markup stays single-part. Manufacturing an HTML alternative
// would make every plain send multipart for no reader's benefit.
func TestNoMarkupBodyProducesNoMarkupAlternative(t *testing.T) {
	if got := signedHTML("", SignOff{Text: "Marek", Kind: SignOffSignature}, sendDeliverability{}); got != "" {
		t.Fatalf("a plain-text send gained a markup part: %q", got)
	}
}

// Both parts must offer the unsubscribe link, from the SAME token: whether a
// recipient can unsubscribe must not depend on which alternative their client
// chose to render.
func TestBothPartsCarryTheUnsubscribeSurface(t *testing.T) {
	derived := sendDeliverability{
		links: unsubscribeLinks{
			unsubscribe: "https://app.test/#/unsubscribe/tok/newsletter?lang=en",
			manage:      "https://app.test/#/preferences/tok?lang=en",
		},
		words: mailcopy.For("en"),
	}

	got := signedHTML("<p>Body</p>", SignOff{Kind: SignOffNone}, derived)
	if !strings.Contains(got, derived.links.unsubscribe) || !strings.Contains(got, derived.links.manage) {
		t.Fatalf("the markup part carries no unsubscribe surface: %q", got)
	}
}

type stubSenderName struct {
	name string
	err  error
}

func (s *stubSenderName) ActorIdentity(context.Context) (string, string, error) {
	return s.name, "", s.err
}

// The name reaches the send when identity knows it.
func TestTheSenderNameIsResolvedForTheSend(t *testing.T) {
	store := (&Store{}).WithSenderName(&stubSenderName{name: "Lars Jankowfsky"})

	got, err := store.senderDisplayName(humanCtx(ids.NewV7()))
	if err != nil {
		t.Fatalf("resolving the sender name failed: %v", err)
	}
	if got != "Lars Jankowfsky" {
		t.Fatalf("expected the sender's name, got %q", got)
	}
}

// A role wired without the seam sends a bare address rather than refusing.
func TestNoSenderNameReaderSendsUnnamed(t *testing.T) {
	got, err := (&Store{}).senderDisplayName(humanCtx(ids.NewV7()))
	if err != nil {
		t.Fatalf("resolving the sender name failed: %v", err)
	}
	if got != "" {
		t.Fatalf("a store with no reader produced a name: %q", got)
	}
}

// A name that cannot be read is not a reason to refuse a send: the message is
// correct without it, and trading a cosmetic gap for a delivery failure is the
// worse answer. The error still surfaces rather than being swallowed.
func TestAFailedSenderNameReadSurfaces(t *testing.T) {
	boom := errors.New("identity is unreachable")
	store := (&Store{}).WithSenderName(&stubSenderName{err: boom})

	if _, err := store.senderDisplayName(humanCtx(ids.NewV7())); !errors.Is(err, boom) {
		t.Fatalf("expected the read error to surface, got %v", err)
	}
}

// An agent send carries no name, for the same reason it carries no signature:
// the approval authorizes the sending, it does not make the approver the author.
// ActorIdentity resolves an agent to the human it acts for — right for a draft,
// wrong for an envelope — so the refusal has to be made at this seam.
func TestAnAgentSendCarriesNoSenderName(t *testing.T) {
	reader := &stubSenderName{name: "Lars Jankowfsky"}
	store := (&Store{}).WithSenderName(reader)
	ctx := principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:assistant", UserID: ids.NewV7(),
	})

	got, err := store.senderDisplayName(ctx)
	if err != nil {
		t.Fatalf("resolving the sender name failed: %v", err)
	}
	if got != "" {
		t.Fatalf("an agent send was named %q", got)
	}
}

// The header and the sign-off must agree about authorship. A message naming a
// human on the envelope while deliberately withholding their signature below
// would be the same claim told louder, in the line the inbox shows first.
func TestTheEnvelopeAndTheSignOffAgreeAboutAuthorship(t *testing.T) {
	store := (&Store{}).
		WithSenderName(&stubSenderName{name: "Lars Jankowfsky"}).
		WithSignature(&stubSignature{body: "Lars Jankowfsky"})

	for name, ctx := range map[string]context.Context{
		"human": humanCtx(ids.NewV7()),
		"agent": principal.WithActor(context.Background(), principal.Principal{
			Type: principal.PrincipalAgent, ID: "agent:assistant", UserID: ids.NewV7(),
		}),
	} {
		t.Run(name, func(t *testing.T) {
			envelope, err := store.senderDisplayName(ctx)
			if err != nil {
				t.Fatalf("resolving the sender name failed: %v", err)
			}
			signed := signedBody(ctx, t, store, "Body")
			named := envelope != ""
			if signedOff := signed != "Body"; named != signedOff {
				t.Fatalf("envelope named=%v but signed=%v — the two disagree", named, signedOff)
			}
		})
	}
}

// A display name typed with a line break or a run of spaces stays one line
// under the closing, in both the plain and the markup part.
func TestTheClosingNamesTheSenderOnOneLine(t *testing.T) {
	store := (&Store{}).WithSignature(&stubSignature{}).
		WithSenderName(&stubSenderName{name: " Lars \n  Jankowfsky "})

	if got := signedBody(humanCtx(ids.NewV7()), t, store, "Thx"); got != "Thx\n\nBest regards,\nLars Jankowfsky" {
		t.Fatalf("unexpected signed body: %q", got)
	}
}

// The send reads the name once and hands it to the closing: the closing uses
// the name it is given, not a second read.
func TestTheClosingCarriesTheNameItIsGiven(t *testing.T) {
	store := (&Store{}).WithSignature(&stubSignature{}).
		WithSenderName(&stubSenderName{name: "A later edit"})

	sign, err := store.signOffAs(humanCtx(ids.NewV7()), "Thx", "", "Lars Jankowfsky")
	if err != nil {
		t.Fatalf("resolving the sign-off failed: %v", err)
	}
	if sign.Text != "Best regards,\nLars Jankowfsky" {
		t.Fatalf("sign-off = %q, want the name the send already read", sign.Text)
	}
}

// The contract requires `body`; a request without the key is refused rather
// than answered as though the composer were blank.
func TestASignOffPreviewWithoutABodyIsRefused(t *testing.T) {
	h := Handlers{store: (&Store{}).WithSignature(&stubSignature{body: "Marek"})}
	for name, payload := range map[string]string{"absent": `{}`, "present": `{"body":""}`} {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/v1/emails:sign-off", strings.NewReader(payload)).
				WithContext(humanCtx(ids.NewV7()))
			rec := httptest.NewRecorder()
			h.PreviewEmailSignOff(rec, req)
			want := http.StatusOK
			if name == "absent" {
				want = http.StatusUnprocessableEntity
			}
			if rec.Code != want {
				t.Fatalf("answered %d, want %d: %s", rec.Code, want, rec.Body.String())
			}
		})
	}
}
