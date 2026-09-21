// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"bytes"
	"context"
	"image/png"
	"reflect"
	"testing"

	"github.com/margince/margince/backend/internal/platform/webread"
)

const (
	lockupURL   = logoSeed + "brand/lockup.png"
	touchURL    = logoSeed + "touch.png"
	ldLogoURL   = "https://cdn.acme.example/lockup.svg"
	shareURL    = logoSeed + "share.png"
	badLogoURL  = logoSeed + "broken.png"
	stripURL    = logoSeed + "strip.png"
	pixelURL    = logoSeed + "pixel.png"
	wideMarkURL = logoSeed + "wide-icon.png"
)

func declaringLockup(logos ...string) declaredAssets {
	return declaredAssets{
		icons: []webread.IconRef{{URL: touchURL, Rel: webread.RelAppleTouchIcon, Sizes: "180x180"}},
		logos: logos,
	}
}

func TestTheLockupIsTheFirstDeclaredLogoThatDraws(t *testing.T) {
	// The page labelled these; the label is the evidence. What still decides is
	// whether the picture can be drawn at all: one that will not decode and one
	// too small to read are passed over for the wordmark behind them.
	site := &assetSite{assets: map[string][]byte{
		badLogoURL: []byte("<html>not an image</html>"),
		pixelURL:   logoFixture(t, 20, 20),
		lockupURL:  logoFixture(t, 600, 150),
	}}
	lockup, attempts := resolveLockup(context.Background(), site, declaringLockup(badLogoURL, pixelURL, lockupURL))
	if lockup.SourceURL != lockupURL {
		t.Fatalf("lockup resolved to %q, want %s: %+v", lockup.SourceURL, lockupURL, attempts)
	}
	if len(attempts) != 3 || attempts[2].Outcome != logoOutcomeChosen {
		t.Fatalf("attempts = %+v, want the two drops named and the third chosen", attempts)
	}
	// Normalized as a lockup: the aspect is kept, no square canvas is added,
	// and the long edge is the upload path's ceiling, so both writers of the
	// wide slot store the same shape.
	img, err := png.Decode(bytes.NewReader(lockup.PNG))
	if err != nil {
		t.Fatalf("the stored lockup is not a PNG: %v", err)
	}
	if w, h := img.Bounds().Dx(), img.Bounds().Dy(); w != companyLogoEdge || h != companyLogoEdge/4 {
		t.Fatalf("stored %dx%d, want %dx%d — aspect kept, long edge at the upload ceiling", w, h, companyLogoEdge, companyLogoEdge/4)
	}
}

func TestAStripIsNotALockupWhateverThePageCalledIt(t *testing.T) {
	site := &assetSite{assets: map[string][]byte{stripURL: logoFixture(t, 1400, 100)}}
	lockup, attempts := resolveLockup(context.Background(), site, declaringLockup(stripURL))
	if lockup.PNG != nil {
		t.Fatalf("a 14:1 strip was taken as the lockup: %+v", attempts)
	}
	if len(attempts) != 1 || attempts[0].Outcome == logoOutcomeChosen {
		t.Fatalf("attempts = %+v, want the strip refused by name", attempts)
	}
}

func TestTheBadgeChainRunsBeforeTheLockupSpendsAnything(t *testing.T) {
	// The face every company got before the lockup existed is resolved first,
	// so a deadline spent on the new half never costs the old one.
	site := &assetSite{assets: map[string][]byte{
		touchURL:  logoFixture(t, 180, 180),
		lockupURL: logoFixture(t, 600, 150),
	}}
	resolveCompanyMarks(context.Background(), site, logoSeed, declaringLockup(lockupURL))
	if len(site.asked) < 2 || site.asked[0] != touchURL || site.asked[len(site.asked)-1] != lockupURL {
		t.Fatalf("fetch order %v, want the declared icon before the declared lockup", site.asked)
	}
}

func TestALockupAndABadgeFillTheirOwnSlots(t *testing.T) {
	site := &assetSite{assets: map[string][]byte{
		touchURL:  logoFixture(t, 180, 180),
		lockupURL: logoFixture(t, 600, 150),
	}}
	marks := resolveCompanyMarks(context.Background(), site, logoSeed, declaringLockup(lockupURL))
	if marks.Wide.SourceURL != lockupURL {
		t.Fatalf("wide slot = %q, want the lockup at %s", marks.Wide.SourceURL, lockupURL)
	}
	if marks.Icon.SourceURL != touchURL {
		t.Fatalf("icon slot = %q, want the badge at %s", marks.Icon.SourceURL, touchURL)
	}
	// Each candidate is listed once, under the slot it decided.
	if len(marks.WideAttempts) != 1 || marks.WideAttempts[0].URL != lockupURL {
		t.Fatalf("wide attempts = %+v, want the lockup alone", marks.WideAttempts)
	}
	if len(marks.IconAttempts) != 1 || marks.IconAttempts[0].URL != touchURL {
		t.Fatalf("icon attempts = %+v, want the badge chain alone", marks.IconAttempts)
	}
}

func TestWithoutALockupTheBadgeServesBothWidthsAsItAlwaysDid(t *testing.T) {
	// Nothing a site used to get is lost: the wide slot takes the mark the
	// chain always resolved, the icon slot stays empty, and the collapsed rail
	// falls back to the wide mark on its own.
	site := &assetSite{assets: map[string][]byte{touchURL: logoFixture(t, 180, 180)}}
	marks := resolveCompanyMarks(context.Background(), site, logoSeed, declaringLockup(badLogoURL))
	if marks.Wide.SourceURL != touchURL || marks.Icon.PNG != nil {
		t.Fatalf("wide %q icon %q, want the badge wide and no icon", marks.Wide.SourceURL, marks.Icon.SourceURL)
	}
	// The wide slot's story names the lockup that failed and then the chain.
	if len(marks.WideAttempts) < 2 || marks.WideAttempts[0].URL != badLogoURL {
		t.Fatalf("wide attempts = %+v, want the failed lockup first, then the chain", marks.WideAttempts)
	}
	want := []logoAttempt{{URL: touchURL, Outcome: badgeOutcomeIsTheWideMark}}
	if !reflect.DeepEqual(marks.IconAttempts, want) {
		t.Fatalf("icon attempts = %+v, want %+v", marks.IconAttempts, want)
	}
	// And a page that never declared a lockup tells the same story with no
	// failed candidate in front of it — the chain's own attempts are the whole
	// account, exactly as before the lockup existed.
	site = &assetSite{assets: map[string][]byte{touchURL: logoFixture(t, 180, 180)}}
	marks = resolveCompanyMarks(context.Background(), site, logoSeed, declaringLockup())
	if len(marks.WideAttempts) != 1 || marks.WideAttempts[0].Outcome != logoOutcomeChosen {
		t.Fatalf("wide attempts = %+v, want the chain's chosen icon alone", marks.WideAttempts)
	}
}

func TestABadgeThatWouldDuplicateTheWideMarkIsNotStored(t *testing.T) {
	// A square lockup already serves the collapsed rail, which falls back to
	// the wide mark; a second copy of a square is bytes stored for nothing.
	site := &assetSite{assets: map[string][]byte{
		touchURL:  logoFixture(t, 180, 180),
		ldLogoURL: logoFixture(t, 512, 512),
	}}
	marks := resolveCompanyMarks(context.Background(), site, logoSeed, declaringLockup(ldLogoURL))
	if marks.Wide.SourceURL != ldLogoURL || marks.Icon.PNG != nil {
		t.Fatalf("wide %q icon %q, want the square lockup wide and no badge", marks.Wide.SourceURL, marks.Icon.SourceURL)
	}
	last := marks.IconAttempts[len(marks.IconAttempts)-1]
	if last.URL != touchURL || last.Outcome != badgeOutcomeLockupIsSquare {
		t.Fatalf("icon attempts = %+v, want the badge declined because the lockup is square", marks.IconAttempts)
	}
}

func TestAWideFallbackMarkIsNotABadge(t *testing.T) {
	// The chain kept a 2:1 picture only as a fallback for a company drawn once;
	// beside a real lockup it is neither the lockup nor a square badge.
	site := &assetSite{assets: map[string][]byte{
		shareURL:  logoFixture(t, 1200, 630),
		lockupURL: logoFixture(t, 600, 150),
	}}
	declared := declaredAssets{ogImage: shareURL, logos: []string{lockupURL}}
	marks := resolveCompanyMarks(context.Background(), site, logoSeed, declared)
	if marks.Wide.SourceURL != lockupURL || marks.Icon.PNG != nil {
		t.Fatalf("wide %q icon %q, want the lockup wide and no badge", marks.Wide.SourceURL, marks.Icon.SourceURL)
	}
	last := marks.IconAttempts[len(marks.IconAttempts)-1]
	if last.URL != shareURL || last.Outcome != badgeOutcomeNotSquare {
		t.Fatalf("icon attempts = %+v, want the sharing card declined as not square", marks.IconAttempts)
	}
}
