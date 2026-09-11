// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// What a proof row records as the thing the subject agreed to.
//
// Both confirm-link doors used to write in.MarketingWording — the sentence that
// arrived in the same request as the answer it is meant to evidence. This
// resolves the published question the link pinned instead, so the proof names a
// row the controller published and a reader can check it against.
//
// WHAT THE PIN DOES NOT SETTLE, stated because the limits are real and a reader
// should not take this for more than it is.
//
// The page renders its own copy of the question from the frontend catalog
// rather than fetching the published row. Two consequences follow, and both are
// bounded rather than closed:
//
//   - The page picks its language from the reader's browser, while the pin
//     records the language the MAIL went out in. A link mailed in English and
//     opened in a German browser records the English wording as what was seen.
//   - The pin names the version live when the link was minted. A frontend
//     redeploy between minting and clicking changes what the page shows while
//     the proof still names what was pinned.
//
// Both are narrower than what they replace: an unpinned, unpublished,
// client-supplied string that could say anything. The cross-catalog gate
// (gates/marketingquestion_test.go) keeps the two copies of the wording in step
// so the second case needs a deliberate edit to arise, and closing either
// properly means the page rendering the published row — one source, nothing to
// drift.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5"

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

// wordingForGrantTx decides what a grant through a confirm link records.
//
// ONE HELPER FOR BOTH DOORS. A record-confirmation link and a dedicated consent
// link each wrote whatever sentence arrived in the submission, and fixing one
// would have left the other — the one most double-opt-in grants come through —
// exactly as it was.
func (s *Store) wordingForGrantTx(
	ctx context.Context, tx pgx.Tx, ref ConfirmRef, submitted string,
) (GrantWording, error) {
	bound, found, err := publishedQuestionForLinkTx(ctx, tx, ref)
	if err != nil {
		return GrantWording{}, err
	}
	if !found {
		// The client's sentence, which is what this door recorded before and is
		// the honest evidence for a link that cannot name a published row.
		return GrantWording{Text: submitted}, nil
	}
	// THE PURPOSE GOES INTO THE RECORDED SENTENCE, not into the published row.
	//
	// The subscription question carries a {purpose} placeholder, and the page
	// fills it with the purpose's own label before the subject reads it. The
	// PUBLISHED row keeps the placeholder — publishing one row per purpose
	// would make a version mean something different for each — but the PROOF
	// records what was actually on the screen.
	//
	// Without this the subject's own data export hands them "Confirm that you
	// want to receive {purpose}.", a sentence with a hole in it, and the export
	// carries the purpose's KEY rather than its label, so nothing in front of
	// them fills it.
	text, err := substitutePurposeTx(ctx, tx, bound.Text, ref.PurposeID)
	if err != nil {
		return GrantWording{}, err
	}
	version := bound.Version
	return GrantWording{Text: text, Version: &version, VersionID: bound.VersionID}, nil
}

// substitutePurposeTx fills the {purpose} placeholder with the label the page
// showed, leaving any other text untouched.
//
// A MISSING LABEL LEAVES THE PLACEHOLDER rather than failing the grant. A
// purpose archived between the mail and the click has no label to read, and
// refusing somebody's consent because we can no longer name what they
// subscribed to would be the wrong way round — the proof still carries
// purpose_id, which is what identifies it.
func substitutePurposeTx(
	ctx context.Context, tx pgx.Tx, text string, purposeID ids.PurposeID,
) (string, error) {
	if !strings.Contains(text, purposePlaceholder) || purposeID.IsZero() {
		return text, nil
	}
	var label string
	err := tx.QueryRow(ctx,
		`SELECT label FROM consent_purpose WHERE id = $1`, purposeID).Scan(&label)
	if errors.Is(err, pgx.ErrNoRows) {
		return text, nil
	}
	if err != nil {
		return "", fmt.Errorf("consent: naming the purpose this question asked about: %w", err)
	}
	return strings.ReplaceAll(text, purposePlaceholder, label), nil
}

// purposePlaceholder is what the published subscription question holds where
// the purpose's own name goes. One spelling, shared with the catalog that
// writes it and the screen that fills it.
const purposePlaceholder = "{purpose}"

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
