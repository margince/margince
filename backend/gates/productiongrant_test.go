// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Pages cannot promise indefinite free production seats.

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/licensecheck"
	"github.com/margince/margince/backend/internal/shared/gatekit"
)

//nolint:misspell // These patterns must match German grant wording in its original spelling.
var (
	additionalGrantLine = regexp.MustCompile(`(?ms)^Additional Use Grant:[ \t]*(.*?)^Change Date:`)
	freeProductionWords = regexp.MustCompile(`(?i)\bproduction\b|\b(?:produktiv|produktion)[a-z]*\b|\bechtbetrieb\b|sản xuất|vận hành thực tế`)
	freeGrantWords      = regexp.MustCompile(`(?i)\bfree\b|no charge|kostenlos|kostenfrei|miễn phí`)
	seatAllowanceWords  = regexp.MustCompile(`(?i)(?:\b[0-9]+|\bone|\btwo|\bthree|\bfour|\bfive|\bsix|\bseven|\beight|\bnine|\bten|\btwenty|\bhundred|\bunlimited|\bein|\bzwei|\bdrei|\bvier|\bfünf|\bmột|\bhai|\bba|\bbốn|\bnăm)[\s-]+(?:(?:free|production|kostenlose|produktive)[\s-]+)*(?:seats?\b|users?\b|members?\b|people\b|persons?\b|licen[cs]es?\b|sitze?\b|personen\b|plätze\b|lizenzen\b|(?:be)?nutzer(?:n|innen)?\b|người dùng|tài khoản)`)
	nonProductionWords  = regexp.MustCompile(`(?i)non[- ]production|nicht[- ]produktiv[a-z]*`)
)

func productionSeatPromise(paragraph string) bool {
	text := strings.Join(strings.Fields(paragraph), " ")
	text = strings.NewReplacer("`", "", "*", "", "_", "").Replace(text)
	text = nonProductionWords.ReplaceAllString(text, "")
	return freeProductionWords.MatchString(text) && freeGrantWords.MatchString(text) && seatAllowanceWords.MatchString(text)
}

func TestNoMarkdownPageGrantsProductionSeatsTheLicenseWithholds(t *testing.T) {
	t.Parallel()
	license, err := gatekit.ReadFile(filepath.Join(repoRoot, "LICENSE"))
	if err != nil {
		t.Fatal(err)
	}
	if err := requireProductionTrialGrant(string(license)); err != nil {
		t.Fatal(err)
	}
	pages := 0
	for _, file := range trackedFiles(t) {
		ext := strings.ToLower(filepath.Ext(file.path))
		if file.symlink || (ext != ".md" && ext != ".mdx" && ext != ".markdown") {
			continue
		}
		pages++
		raw, err := gatekit.ReadFile(filepath.Join(repoRoot, file.path))
		if err != nil {
			t.Fatal(err)
		}
		for paragraph := range strings.SplitSeq(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n\n") {
			if productionSeatPromise(paragraph) {
				t.Errorf("%s promises free production seats beyond the finite trial; link LICENSING.md for the terms", file.path)
			}
		}
	}
	if pages < 600 {
		t.Fatalf("Markdown census is short: read %d tracked pages", pages)
	}
}

func TestTheProductionGrantCensusReadsWrappedAndTranslatedPromises(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		text string
		want bool
	}{
		{"Run it in production for free for up to 3\nseats.", true},
		{"Three free production seats are included.", true},
		{"Free production use for 3-seat teams.", true},
		{"Run it in production free for up to 3 people.", true},
		{"Up to three licenses free in production.", true},
		{"Kostenlos im Produktionsbetrieb für bis zu 3 Nutzer.", true},
		{"Mit 3 Nutzern kostenlos produktiv arbeiten.", true},
		{"Four **free** production **seats** are included.", true},
		{"3 users may use the production system at no charge.", true},
		{"3 Nutzer dürfen es kostenlos produktiv nutzen.", true},
		{"Dùng miễn phí trong môi trường sản xuất cho 3 người dùng.", true},
		{"3 seats may evaluate it free in non-production.", false},
		{"Production use needs a commercial subscription. Read LICENSING.md.", false},
		{"The code is free to read; three examples show production deployment.", false},
	} {
		if got := productionSeatPromise(c.text); got != c.want {
			t.Errorf("%q: promise=%v, want %v", c.text, got, c.want)
		}
	}
}

func requireProductionTrialGrant(license string) error {
	grant := additionalGrantLine.FindStringSubmatch(license)
	if len(grant) != 2 {
		return errors.New("LICENSE has no readable Additional Use Grant")
	}
	trial, _, hosting := strings.Cut(strings.Join(strings.Fields(grant[1]), " "), "An Authorized Hosting Partner")
	days := licensecheck.TrialLength / (24 * time.Hour)
	if days <= 0 || licensecheck.TrialLength%(24*time.Hour) != 0 {
		return errors.New("the trial runtime must use a whole positive number of days")
	}
	expected := fmt.Sprintf("You may make production use of the Licensed Work for one initial %d-day trial per installation, starting when its first workspace is created. The trial has no seat limit. After the trial, production use requires a commercial subscription from the Licensor. Changing the recorded creation date or modifying the trial checks does not extend this permission.", days)
	if !hosting || strings.TrimSpace(trial) != expected {
		return errors.New("the Additional Use Grant changed; review the finite-trial gate and runtime together")
	}
	return nil
}

func TestAChangedProductionGrantRequiresReview(t *testing.T) {
	t.Parallel()
	license, err := gatekit.ReadFile(filepath.Join(repoRoot, "LICENSE"))
	if err != nil {
		t.Fatal(err)
	}
	original := string(license)
	for _, changed := range []string{
		strings.Replace(original, fmt.Sprintf("%d-day", licensecheck.TrialLength/(24*time.Hour)), fmt.Sprintf("%d-day", licensecheck.TrialLength/(24*time.Hour)+1), 1),
		strings.Replace(original, "initial ", "unlimited ", 1),
		strings.Replace(original, "The trial has no seat limit.", "Three free production seats are allowed forever.", 1),
		strings.Replace(original, "After the trial, production use requires a commercial", "Production use never requires a commercial", 1),
		"LICENSE without the grant heading",
	} {
		if err := requireProductionTrialGrant(changed); err == nil {
			t.Error("a changed production grant passed without review")
		}
	}
}
