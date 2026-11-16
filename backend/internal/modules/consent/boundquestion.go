// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// What a proof row records as the thing the subject agreed to.
//
// A confirm link pins one published question: its key, version and language.
// The page's GET serves it with the purpose filled in, and the page shows those
// words. The proof records the same served text. The client's sentence is only
// a check. A grant with other words came from a page older than the question.
// It is refused with "reload the page" and never recorded. A link that pinned
// nothing keeps the client's sentence, the weaker evidence it always had.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/mailcopy"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// GrantWording is what one proof row says about what was agreed.
type GrantWording struct {
	// Text is the sentence recorded on the proof.
	Text string
	// Version names which published version it is, nil when nothing was bound
	// and the engine's own default applies.
	Version *string
	// VersionID is the published row, zero when nothing was bound.
	VersionID ids.UUID
}

// ServedQuestion is a pinned question as a confirm page serves it and a proof
// records it. Text is the published body with the purpose filled in: the ask on
// its first line, the answers the page offers on the lines after.
type ServedQuestion struct {
	Version string
	Text    string
}

// Parts splits Text into the sentence the reader answers and the answers' labels.
func (q ServedQuestion) Parts() (string, []string) {
	lines := strings.Split(q.Text, "\n")
	return lines[0], lines[1:]
}

// errPageOlderThanQuestion refuses a grant whose words are not the question the
// link served. Nothing is recorded and the link stays unspent.
var errPageOlderThanQuestion = &apperrors.VersionSkewError{
	Message: "The question on this page changed after it was opened. Reload the page and answer again.",
}

// servedQuestionTx resolves the question this link pinned, in the link's
// language, with its fields filled.
func (s *Store) servedQuestionTx(ctx context.Context, tx pgx.Tx, ref ConfirmRef) (ServedQuestion, boundQuestion, bool, error) {
	bound, found, err := publishedQuestionForLinkTx(ctx, tx, ref)
	if err != nil || !found {
		return ServedQuestion{}, boundQuestion{}, false, err
	}
	fields, err := s.questionFieldsTx(ctx, tx, ref)
	if err != nil {
		return ServedQuestion{}, boundQuestion{}, false, err
	}
	return ServedQuestion{Version: bound.Version, Text: fillQuestion(bound.Text, fields)}, bound, true, nil
}

// servedQuestionFor is the question a confirm page shows for this link, absent
// when the link pinned none.
func (s *Store) servedQuestionFor(ctx context.Context, ref ConfirmRef) (*ServedQuestion, error) {
	var out *ServedQuestion
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		served, _, found, err := s.servedQuestionTx(ctx, tx, ref)
		if found {
			out = &served
		}
		return err
	})
	return out, err
}

// wordingForGrantTx decides what an answer through a confirm link records: the
// served question where the link pinned one, else the client's sentence. Both
// confirm-link doors call it. A withdrawal is never refused over its words.
func (s *Store) wordingForGrantTx(
	ctx context.Context, tx pgx.Tx, ref ConfirmRef, in ConfirmSubmission,
) (GrantWording, error) {
	served, bound, found, err := s.servedQuestionTx(ctx, tx, ref)
	if err != nil {
		return GrantWording{}, err
	}
	if !found {
		return GrantWording{Text: in.MarketingWording}, nil
	}
	if in.MarketingChoice == string(StateGranted) && in.MarketingWording != served.Text {
		return GrantWording{}, errPageOlderThanQuestion
	}
	version := bound.Version
	return GrantWording{Text: served.Text, Version: &version, VersionID: bound.VersionID}, nil
}

// questionFields are what a published question leaves open, in the
// question's language.
type questionFields struct {
	purpose, controller, frequency, privacyPage string
	words                                       mailcopy.Copy
}

// questionFieldsTx reads the fields for one link: the installation's legal
// name and privacy page, and the name and frequency of the purpose it names.
//
// A purpose archived between the mail and the click leaves its two fields
// empty rather than failing the grant: the proof still carries purpose_id.
func (s *Store) questionFieldsTx(ctx context.Context, tx pgx.Tx, ref ConfirmRef) (questionFields, error) {
	out := questionFields{words: mailcopy.For(ref.QuestionLocale)}
	var held PrivacyInformation
	if err := readControllerTx(ctx, tx, &held); err != nil {
		return out, err
	}
	out.controller = held.Controller.LegalName
	out.privacyPage = privacyPageOf(held.Controller)
	if ref.PurposeID.IsZero() {
		return out, nil
	}
	var key, label, frequency string
	err := tx.QueryRow(ctx,
		`SELECT key, label, frequency FROM consent_purpose WHERE id = $1`, ref.PurposeID).Scan(&key, &label, &frequency)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, nil
	}
	if err != nil {
		return out, fmt.Errorf("consent: naming the purpose this question asked about: %w", err)
	}
	out.purpose = purposeName(key, label, out.words)
	out.frequency = frequencyName(frequency, out.words)
	return out, nil
}

// fillQuestion puts the fields into a published question. A purpose or
// frequency with no value keeps its placeholder. The controller is "the sender"
// when the installation has stated no name, and the privacy page is empty.
func fillQuestion(text string, f questionFields) string {
	controller := f.controller
	if controller == "" {
		controller = f.words.ControllerFallback
		if strings.HasPrefix(text, controllerPlaceholder) {
			controller = capitalized(controller)
		}
	}
	pairs := []string{controllerPlaceholder, controller, privacyPagePlaceholder, f.privacyPage}
	for placeholder, value := range map[string]string{
		purposePlaceholder: f.purpose, frequencyPlaceholder: f.frequency,
	} {
		if value != "" {
			pairs = append(pairs, placeholder, value)
		}
	}
	return strings.NewReplacer(pairs...).Replace(text)
}

// frequencyName is a purpose's frequency as the question's language says it.
func frequencyName(frequency string, words mailcopy.Copy) string {
	switch crmcontracts.ConsentPurposeFrequency(frequency) {
	case crmcontracts.ConsentPurposeFrequencyWeekly:
		return words.FrequencyWeekly
	case crmcontracts.ConsentPurposeFrequencyQuarterly:
		return words.FrequencyQuarterly
	}
	return words.FrequencyMonthly
}

// privacyPageFields is where a subscription question points its reader for
// their rights, first stated wins: the privacy page, then the privacy contact,
// then nothing. frontend/src/features/privacy/privacypage.ts mirrors it.
//
// Held by: TestThePrivacyPageFallbackIsOneOnBothSides
// (backend/gates/privacypagefallback_test.go)
var privacyPageFields = []string{"privacy_page", "privacy_contact"}

func privacyPageOf(p ControllerParticulars) string {
	stated := map[string]string{"privacy_page": p.PrivacyPage, "privacy_contact": p.PrivacyContact}
	for _, field := range privacyPageFields {
		if value := strings.TrimSpace(stated[field]); value != "" {
			return value
		}
	}
	return ""
}

func capitalized(word string) string {
	for i, r := range word {
		return string(unicode.ToUpper(r)) + word[i+utf8.RuneLen(r):]
	}
	return word
}

// The placeholders a published question holds, one spelling each, shared with
// the catalog that writes them and the screen that fills them.
const (
	purposePlaceholder     = "{purpose}"
	controllerPlaceholder  = "{controller}"
	frequencyPlaceholder   = "{frequency}"
	privacyPagePlaceholder = "{privacy_page}"
)

// boundQuestion is a published question a grant can point at.
type boundQuestion struct {
	VersionID ids.UUID
	Text      string
	Version   string
}

// publishedQuestionForLinkTx resolves the question this link pinned.
//
// NOT FOUND IS NOT AN ERROR, and it covers one expected case and one that is
// not. A link minted before the question was pinned names nothing, and there is
// no honest way to say which of three published rows somebody read — those
// grants keep the weaker evidence they already had, because refusing somebody's
// consent over our own missing bookkeeping would be the wrong way round.
//
// A link that DOES name a version and locale with nothing published for them is
// the other case: this installation asked a question it never published, or
// publishing has not run. The grant still stands — the subject clicked a real
// link and answered a real question — but it is logged rather than folded in
// with the expected case.
func publishedQuestionForLinkTx(
	ctx context.Context, tx pgx.Tx, ref ConfirmRef,
) (boundQuestion, bool, error) {
	if ref.QuestionKey == "" || ref.QuestionLocale == "" || ref.QuestionVersion == 0 {
		return boundQuestion{}, false, nil
	}
	version := fmt.Sprintf("v%d", ref.QuestionVersion)
	var out boundQuestion
	err := tx.QueryRow(ctx, `
		SELECT id, body, version
		  FROM consent_text_version
		 WHERE key = $1 AND version = $2 AND locale = $3 AND published_at IS NOT NULL`,
		ref.QuestionKey, version, ref.QuestionLocale).
		Scan(&out.VersionID, &out.Text, &out.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		slog.WarnContext(ctx, "a confirm link pinned a question this installation has not published",
			"key", ref.QuestionKey, "version", version, "locale", ref.QuestionLocale)
		return boundQuestion{}, false, nil
	}
	if err != nil {
		return boundQuestion{}, false, fmt.Errorf(
			"consent: reading the question this link asked: %w", err)
	}
	return out, true, nil
}
