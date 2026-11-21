// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package messagingpolicy

import (
	"context"
	"errors"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

var refreshDay = time.Date(2026, 3, 9, 15, 4, 0, 0, time.UTC)

// lawPage wraps text in the markup that differs between two fetches of a page.
func lawPage(text, nonce string) SourcePage {
	return SourcePage{ContentType: "text/html; charset=utf-8", Body: []byte(
		"<html><head><style>p{color:#" + nonce + "}</style><script>var token='" + nonce + "'</script></head>" +
			"<body><p>" + text + "</p>\n\n<p>" + strings.Repeat("The sender asks first. ", 40) + "</p></body></html>")}
}

// sources answers each address from a table and counts the fetches.
type sources struct {
	pages   map[string]SourcePage
	fetches map[string]int
}

func (s *sources) fetch(_ context.Context, address string) (SourcePage, error) {
	if s.fetches == nil {
		s.fetches = map[string]int{}
	}
	s.fetches[address]++
	page, served := s.pages[address]
	if !served {
		return SourcePage{}, errors.New("the host answered 403")
	}
	return page, nil
}

const blockInstruments = `instruments:
  - name: Act A
    provision: "§ 1"
    source-url: https://example.org/a
  - name: "Act B: second"
    source-url: https://example.org/b
`

func refresh(t *testing.T, fsys fstest.MapFS, served *sources) SourceReport {
	t.Helper()
	report, err := RefreshSources(t.Context(), fsys, served.fetch, refreshDay)
	if err != nil {
		t.Fatal(err)
	}
	return report
}

// applied answers the folder with the refresh's stamps written back.
func applied(fsys fstest.MapFS, report SourceReport) fstest.MapFS {
	for name, data := range report.Stamped {
		fsys[name] = &fstest.MapFile{Data: data}
	}
	return fsys
}

func outcomes(report SourceReport) []SourceOutcome {
	var out []SourceOutcome
	for _, finding := range report.Findings {
		out = append(out, finding.Outcome)
	}
	return out
}

func TestAFirstRefreshStampsEachInstrumentAndTheLayerInPlace(t *testing.T) {
	served := &sources{pages: map[string]SourcePage{
		"https://example.org/a": lawPage("Act A", "1"),
		"https://example.org/b": lawPage("Act B", "1"),
	}}
	fsys := policyFS(country + blockInstruments + "reply-window: 720h\n")
	report := refresh(t, fsys, served)

	if got := outcomes(report); len(got) != 2 || got[0] != SourceFirstRead || got[1] != SourceFirstRead {
		t.Fatalf("outcomes %v, want two first digests", got)
	}
	stamped := string(report.Stamped["aa.md"])
	digest := report.Findings[0].digest
	want := "---\njurisdiction: aa\nversion: 3\ncreated: 2026-01-02\nsource-checked: 2026-03-09\ninstruments:\n" +
		"  - name: Act A\n    provision: \"§ 1\"\n    source-url: https://example.org/a\n" +
		"    text-digest: " + digest + "\n    text-fetched: 2026-03-09\n" +
		"  - name: \"Act B: second\"\n    source-url: https://example.org/b\n" +
		"    text-digest: " + report.Findings[1].digest + "\n    text-fetched: 2026-03-09\n" +
		"reply-window: 720h\n" + pairKeys + "---\nWhy.\n"
	if stamped != want {
		t.Errorf("the stamped layer is\n%s\nwant\n%s", stamped, want)
	}
	policy, err := Compile(applied(fsys, report))
	if err != nil {
		t.Fatalf("the stamped layer does not compile: %v", err)
	}
	if in := policy.Rules.Instruments[0]; in.TextDigest != digest || day(in.TextFetchedAt) != "2026-03-09" ||
		day(policy.Rules.Provenance.SourceCheckedAt) != "2026-03-09" {
		t.Errorf("compiled instrument %+v, provenance %+v", in, policy.Rules.Provenance)
	}
}

func TestASecondRefreshSeesChangedTextAndNotChangedMarkup(t *testing.T) {
	served := &sources{pages: map[string]SourcePage{
		"https://example.org/a": lawPage("Act A", "1"),
		"https://example.org/b": lawPage("Act B", "1"),
	}}
	fsys := policyFS(country + blockInstruments)
	fsys = applied(fsys, refresh(t, fsys, served))

	served.pages["https://example.org/a"] = lawPage("Act A", "2")
	served.pages["https://example.org/b"] = lawPage("Act B, as amended", "2")
	report := refresh(t, fsys, served)

	if got := outcomes(report); got[0] != SourceUnchanged || got[1] != SourceChanged {
		t.Errorf("outcomes %v, want the first unchanged and the second changed", got)
	}
	if stamped := string(report.Stamped["aa.md"]); strings.Count(stamped, "text-digest:") != 2 ||
		!strings.Contains(stamped, report.Findings[1].digest) {
		t.Errorf("the changed digest was not written over the old one:\n%s", stamped)
	}
}

func TestALayerWithAnUnreadSourceKeepsItsCheckedDate(t *testing.T) {
	served := &sources{pages: map[string]SourcePage{
		"https://example.org/a": lawPage("Act A", "1"),
		"https://example.org/b": {ContentType: "text/html", Body: []byte("<html><script>load()</script><body>Loading</body></html>")},
	}}
	report := refresh(t, policyFS(country+"source-checked: 2026-01-05\n"+blockInstruments), served)

	if got := outcomes(report); got[0] != SourceFirstRead || got[1] != SourceUnread {
		t.Fatalf("outcomes %v, want the script page not read", got)
	}
	if detail := report.Findings[1].Detail; !strings.Contains(detail, "too few") {
		t.Errorf("the finding says %q, want it to say the page holds too little text", detail)
	}
	stamped := string(report.Stamped["aa.md"])
	if !strings.Contains(stamped, "source-checked: 2026-01-05\n") || strings.Count(stamped, "text-digest:") != 1 {
		t.Errorf("want the old date kept and the read instrument stamped:\n%s", stamped)
	}

	delete(served.pages, "https://example.org/b")
	if got := refresh(t, policyFS(country+blockInstruments), served).Findings[1]; got.Outcome != SourceUnread || got.Detail != "the host answered 403" {
		t.Errorf("a refused fetch reads %+v", got)
	}
}

func TestASourceCheckedByHandIsNotFetchedAndDatesTheLayer(t *testing.T) {
	served := &sources{pages: map[string]SourcePage{"https://example.org/a": lawPage("Act A", "1")}}
	byHand := strings.Replace(blockInstruments, "source-url: https://example.org/b\n",
		"source-url: https://example.org/b\n    checked-by-hand: 2026-02-20\n", 1)
	report := refresh(t, policyFS(country+byHand), served)

	if got := outcomes(report); got[0] != SourceFirstRead || got[1] != SourceByHand {
		t.Fatalf("outcomes %v", got)
	}
	if served.fetches["https://example.org/b"] != 0 {
		t.Error("a source checked by hand was fetched")
	}
	if stamped := string(report.Stamped["aa.md"]); !strings.Contains(stamped, "source-checked: 2026-02-20\n") {
		t.Errorf("the layer is as fresh as its oldest check, 2026-02-20:\n%s", stamped)
	}
}

func TestAReadableTextAddressIsFetchedInsteadOfTheOfficialPage(t *testing.T) {
	served := &sources{pages: map[string]SourcePage{
		"https://example.org/a.pdf": {ContentType: "application/pdf", Body: []byte("%PDF-1.7 Act A")},
	}}
	front := country + "instruments:\n  - name: Act A\n    source-url: https://example.org/a\n    text-url: https://example.org/a.pdf\n"
	report := refresh(t, policyFS(front), served)

	if got := report.Findings[0]; got.Outcome != SourceFirstRead || got.Address != "https://example.org/a.pdf" || got.Detail != "14 bytes" {
		t.Errorf("finding %+v, want the PDF read as its bytes", got)
	}
}

func TestASharedLayerIsReadOnceAndAnAddressFetchedOnce(t *testing.T) {
	served := &sources{pages: map[string]SourcePage{"https://example.org/a": lawPage("Act A", "1")}}
	shared := "instruments:\n  - name: Act A\n    source-url: https://example.org/a\n"
	own := "instruments:\n  - name: Act A again\n    source-url: https://example.org/a\n"
	fsys := fstest.MapFS{
		"regional/r.md": {Data: []byte("---\n" + shared + pairKeys + "---\nWhy.\n")},
		"fixtures/a.md": {Data: []byte(draft)},
		"fixtures/b.md": {Data: []byte(draft)},
	}
	for _, code := range []string{"aa", "bb"} {
		fsys[code+"/index.md"] = &fstest.MapFile{Data: []byte("---\nlayers: [regional/r.md, " + code + "/" + code + ".md]\n---\nThe layers.\n")}
		front := "jurisdiction: " + code + "\nversion: 1\ncreated: 2026-01-02\n"
		if code == "bb" {
			front += own
		}
		fsys[code+"/"+code+".md"] = &fstest.MapFile{Data: []byte("---\n" + front + pairKeys + "---\nWhy.\n")}
	}
	report := refresh(t, fsys, served)

	if len(report.Findings) != 2 || served.fetches["https://example.org/a"] != 1 {
		t.Errorf("%d findings and %d fetches, want one per instrument and one fetch", len(report.Findings), served.fetches["https://example.org/a"])
	}
	if _, stamped := report.Stamped["aa/aa.md"]; stamped || len(report.Stamped) != 2 {
		t.Errorf("stamped %d layers, want the shared layer and bb, and aa left alone", len(report.Stamped))
	}
}

func TestInstrumentsOnOneLineAreRefusedAStamp(t *testing.T) {
	served := &sources{pages: map[string]SourcePage{"https://example.org/a": lawPage("Act A", "1")}}
	_, err := RefreshSources(t.Context(), policyFS(country+actA), served.fetch, refreshDay)
	if err == nil || !strings.Contains(err.Error(), "block list") {
		t.Errorf("a flow-style instruments list was stamped or refused without the reason: %v", err)
	}
}

func TestAStampThatWouldLandOutsideItsInstrumentIsRefused(t *testing.T) {
	served := &sources{pages: map[string]SourcePage{"https://example.org/a": lawPage("Act A", "1")}}
	digestFirst := country + "instruments:\n  - text-digest: " + strings.Repeat("0", 64) +
		"\n    text-fetched: 2026-01-03\n    name: Act A\n    source-url: https://example.org/a\n"
	if _, err := RefreshSources(t.Context(), policyFS(digestFirst), served.fetch, refreshDay); err == nil {
		t.Error("a second text-digest key was written beside the one on the item's first line")
	}
}
