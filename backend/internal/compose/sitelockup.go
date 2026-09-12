// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The cold start's second mark. The installation's own company is the one
// record the chrome draws at two widths — the wide lockup an expanded sidebar
// has room for, and the square badge a collapsed 56px rail draws — and one
// picture cannot serve both (contacts/companylogowrite.go). The chain in sitelogo.go
// finds the badge: it prefers the icons a site declares, which are square by
// construction. This file finds the lockup, from the pictures the page itself
// calls its logo (webread.Page.Logos), and decides which slot each picture
// fills.
//
// Only the onboarding read runs it. Every other company is drawn once, as
// a square avatar on a record card, and a wordmark letterboxed into that box
// would be the illegible row of strokes the badge exists to avoid — so the
// enrichment read keeps resolving the one square-preferring mark it always has.

import (
	"context"
	"fmt"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/imagenorm"
)

const (
	// lockupMinLongEdge is the smallest lockup worth storing. The expanded
	// rail draws it at roughly 190px wide; a source narrower than this is a
	// tracking pixel or a thumbnail, not the mark, whatever the page called it.
	lockupMinLongEdge = 64
	// lockupMaxAspect is the widest a lockup may be. A wordmark is routinely
	// four or five times wider than tall and still reads as the company at the
	// rail's width; past this a picture is a banner strip, which does not.
	// Deliberately wider than logoMaxAspect: that bound asks whether a picture
	// survives an avatar-sized SQUARE, and a lockup is never drawn in one.
	lockupMaxAspect = 6.0
)

// Outcomes the slot decision records. They read beside the chain's own
// (logoOutcomeChosen and its siblings) in the debug report, so a company whose
// face came out wrong can be diagnosed slot by slot.
const (
	lockupOutcomeServesBothWidths = "serves both widths: the site declared no lockup that resolved"
	badgeOutcomeIsTheWideMark     = "not stored as the badge: it is the wide mark already, and the collapsed rail draws that"
	badgeOutcomeLockupIsSquare    = "not stored as the badge: the lockup is square already, and the collapsed rail draws that"
	badgeOutcomeNotSquare         = "not stored as the badge: wide, and a badge is square"
)

// resolveLockup walks the pictures the page calls its logo, in the order the
// page offered them, and returns the first that decodes to a usable lockup —
// or a zero resolvedLogo when none did. Every candidate tried comes back too.
//
// No shape ranking here, unlike the badge chain: the page LABELLED these as
// its logo, and that label is the evidence. What shape still decides is
// whether the picture can be drawn at all — big enough to read, not so wide
// it is a strip.
func resolveLockup(ctx context.Context, fetch assetFetcher, declared declaredAssets) (resolvedLogo, []logoAttempt) {
	attempts := make([]logoAttempt, 0, len(declared.logos))
	for _, candidate := range declared.logos {
		logo, drop := fetchLockupCandidate(ctx, fetch, candidate)
		if drop != "" {
			attempts = append(attempts, logoAttempt{URL: candidate, Outcome: drop})
			continue
		}
		attempts = append(attempts, logoAttempt{URL: candidate, Outcome: logoOutcomeChosen})
		return logo, attempts
	}
	return resolvedLogo{}, attempts
}

// fetchLockupCandidate fetches one declared logo and normalizes it as a
// lockup — aspect kept, never letterboxed into a square — or says in plain
// words why it is not one. The stored edge is the upload path's, because the
// two writers of the wide slot must agree on what a lockup is stored as.
func fetchLockupCandidate(ctx context.Context, fetch assetFetcher, rawURL string) (logo resolvedLogo, drop string) {
	body, _, err := fetch.FetchAsset(ctx, rawURL)
	if err != nil {
		return resolvedLogo{}, fmt.Sprintf("could not be fetched: %v", err)
	}
	img, err := imagenorm.Decode(body)
	if err != nil {
		return resolvedLogo{}, fmt.Sprintf("is not a decodable image: %v", err)
	}
	bounds := img.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	short, long := min(width, height), max(width, height)
	if long < lockupMinLongEdge {
		return resolvedLogo{}, fmt.Sprintf("is %dx%d, under the %dpx minimum long edge", width, height, lockupMinLongEdge)
	}
	if float64(long)/float64(short) > lockupMaxAspect {
		return resolvedLogo{}, fmt.Sprintf("is %dx%d — a strip, not a lockup", width, height)
	}
	png, err := imagenorm.FitPNG(img, companyLogoEdge)
	if err != nil {
		return resolvedLogo{}, fmt.Sprintf("could not be normalized: %v", err)
	}
	return resolvedLogo{PNG: png, SourceURL: rawURL, SourceWidth: width, SourceHeight: height}, ""
}

// companyMarks is what the cold-start read resolved for each slot, with the
// story of how each slot was decided. A slot whose PNG is nil stays empty on
// the record, and its attempts say why.
type companyMarks struct {
	Wide         resolvedLogo
	WideAttempts []logoAttempt
	Icon         resolvedLogo
	IconAttempts []logoAttempt
}

// resolveCompanyMarks resolves both of the anchor's marks from one page's
// declarations.
//
// The badge chain runs FIRST, and the order is load-bearing: it is the face
// every company got before the lockup existed, and the lane runs under one
// deadline. A slow lockup fetch that ran first could spend the budget the badge
// chain needed and leave the company faceless where it used to have a face;
// run second, a deadline mid-lockup loses only the second picture.
func resolveCompanyMarks(ctx context.Context, fetch assetFetcher, seedURL string, declared declaredAssets) companyMarks {
	mark, markAttempts := resolveCompanyLogo(ctx, fetch, seedURL, declared)
	lockup, lockupAttempts := resolveLockup(ctx, fetch, declared)
	return chooseSlots(lockup, lockupAttempts, mark, markAttempts)
}

// chooseSlots decides which picture fills which slot.
//
// The lockup takes the wide slot when it resolved; the badge chain's mark takes
// the icon slot beside it, but only when it would show something the wide slot
// does not — the collapsed rail falls back to the wide mark on its own, so a
// badge that duplicates a square lockup, or a wide mark that is not a badge,
// is bytes stored for nothing. When no lockup resolved the mark serves both
// widths from the wide slot, which is exactly what every installation had
// before the lockup existed: nothing a site used to get is lost.
//
// Each candidate is listed once, under the slot it decided.
func chooseSlots(lockup resolvedLogo, lockupAttempts []logoAttempt, mark resolvedLogo, markAttempts []logoAttempt) companyMarks {
	if lockup.PNG == nil {
		marks := companyMarks{Wide: mark, WideAttempts: append(lockupAttempts, markAttempts...)}
		if mark.PNG != nil {
			marks.IconAttempts = []logoAttempt{{URL: mark.SourceURL, Outcome: badgeOutcomeIsTheWideMark}}
		}
		return marks
	}
	marks := companyMarks{Wide: lockup, WideAttempts: lockupAttempts, IconAttempts: markAttempts}
	switch {
	case mark.PNG == nil:
		// The chain's own attempts already say why nothing resolved.
	case !mark.square():
		marks.IconAttempts = append(marks.IconAttempts, logoAttempt{URL: mark.SourceURL, Outcome: badgeOutcomeNotSquare})
	case lockup.square():
		marks.IconAttempts = append(marks.IconAttempts, logoAttempt{URL: mark.SourceURL, Outcome: badgeOutcomeLockupIsSquare})
	default:
		marks.Icon = mark
	}
	return marks
}

// resolveDossierMarks resolves the anchor's two marks and parks each on the
// dossier for the confirmation to bind, on the terms resolveLogo states.
func (w *siteDeepReadWorker) resolveDossierMarks(ctx context.Context, args SiteDeepReadArgs, claim contacts.SiteReadClaim, crawl siteCrawl) {
	// claim.SeedURL is the spelling that ANSWERED — the deep read replaces it
	// with the crawl's own once the crawl returns, so /favicon.ico is never
	// guessed under a host that served nothing.
	marks := resolveCompanyMarks(ctx, w.fetch, claim.SeedURL, crawl.SeedAssets)
	w.parkDossierMark(ctx, args, claim, contacts.LogoWide, marks.Wide, marks.WideAttempts)
	w.parkDossierMark(ctx, args, claim, contacts.LogoIcon, marks.Icon, marks.IconAttempts)
}

// parkDossierMark stores one slot's mark and points the dossier at it, or
// says which slot stays empty and why.
func (w *siteDeepReadWorker) parkDossierMark(ctx context.Context, args SiteDeepReadArgs, claim contacts.SiteReadClaim, slot contacts.LogoSlot, logo resolvedLogo, attempts []logoAttempt) {
	if logo.PNG == nil {
		w.log.InfoContext(ctx, "site read resolved no mark for the slot",
			"read", args.SiteReadID.String(), "slot", slot.String(), "seed", claim.SeedURL,
			"candidates", logoAttemptSummary(attempts))
		return
	}
	// Bytes first, row second: the other order would point a row at bytes that
	// are not there, which is the one outcome a user would see.
	key := w.storeResolvedLogo(ctx, args, claim, logo)
	if key == "" {
		return
	}
	w.recordDossierLogo(ctx, args.SiteReadID, claim, slot, key, logo, attempts)
}
