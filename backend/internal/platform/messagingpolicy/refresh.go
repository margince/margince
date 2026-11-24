// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package messagingpolicy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"mime"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/platform/webread"
	"github.com/margince/margince/backend/internal/shared/kernel/elapsed"
)

// SourcePage is what one address answered.
type SourcePage struct {
	ContentType string
	Body        []byte
}

// FetchSource reads one address. The refresh tool passes an HTTP client, so
// this package opens no connection of its own.
type FetchSource func(ctx context.Context, address string) (SourcePage, error)

// SourceOutcome is what a refresh found for one instrument.
type SourceOutcome string

const (
	// SourceUnchanged is a text whose digest is the one the layer records.
	SourceUnchanged SourceOutcome = "unchanged"
	// SourceChanged is a text whose digest differs from the recorded one.
	SourceChanged SourceOutcome = "changed"
	// SourceFirstRead is a text the layer recorded no digest of.
	SourceFirstRead SourceOutcome = "first digest"
	// SourceUnread is an address that gave no law text. Detail says why.
	SourceUnread SourceOutcome = "not read"
	// SourceByHand is an instrument marked checked-by-hand, which is not fetched.
	SourceByHand SourceOutcome = "by hand"
	// SourceNone is an instrument that states no address to read.
	SourceNone SourceOutcome = "no source"
)

// SourceFinding is the outcome for one instrument of one layer.
type SourceFinding struct {
	Layer      string
	Instrument string
	Address    string
	Outcome    SourceOutcome
	// Detail is the size of the text read, or why none was.
	Detail string

	digest string
}

// SourceReport is one policy folder's refresh.
type SourceReport struct {
	Findings []SourceFinding
	// Stamped holds each layer the refresh changed, by its name in the folder.
	Stamped map[string][]byte
}

// minSourceText is the least text a page must hold to count as the law. A
// page that loads its text by script holds a few words, and so would match.
const minSourceText = 500

// RefreshSources reads the source of each instrument in the policy folder
// fsys and compares its text with the digest the layer records. It writes
// nothing: Stamped is what the caller may write back.
func RefreshSources(ctx context.Context, fsys fs.FS, fetch FetchSource, today time.Time) (SourceReport, error) {
	unit, err := CompileUnit(fsys)
	if err != nil {
		return SourceReport{}, err
	}
	run := sourceRefresh{fetch: fetch, today: elapsed.CalendarDay(today, time.UTC), texts: map[string]sourceText{}}
	report := SourceReport{Stamped: map[string][]byte{}}
	for _, name := range layerNames(unit) {
		raw, err := fs.ReadFile(fsys, name)
		if err != nil {
			return SourceReport{}, err
		}
		findings, stamped, err := run.layer(ctx, name, raw)
		if err != nil {
			return SourceReport{}, err
		}
		report.Findings = append(report.Findings, findings...)
		if string(stamped) != string(raw) {
			report.Stamped[name] = stamped
		}
	}
	return report, nil
}

// layerNames answers each layer the unit's packs compile, once. A shared
// layer is listed by every pack and holds one set of instruments.
func layerNames(unit Unit) []string {
	var names []string
	for _, policy := range unit.Policies {
		for _, source := range policy.Sources[1:] {
			if !slices.Contains(names, source) {
				names = append(names, source)
			}
		}
	}
	return names
}

type sourceRefresh struct {
	fetch FetchSource
	today time.Time
	// texts keeps one answer per address, so two layers citing one page agree.
	texts map[string]sourceText
}

type sourceText struct {
	digest string
	size   string
	err    error
}

// layer checks one layer's instruments and answers the file with its stamps.
// The stamped front matter must decode to the layer plus the stamps alone.
func (r *sourceRefresh) layer(ctx context.Context, name string, raw []byte) ([]SourceFinding, []byte, error) {
	front, body, err := splitFrontMatter(name, raw)
	if err != nil {
		return nil, nil, err
	}
	var layer layerFile
	if err := decodeStrict(name, front, &layer); err != nil {
		return nil, nil, err
	}
	want := layer
	want.Instruments = slices.Clone(layer.Instruments)
	lines := strings.Split(strings.TrimSuffix(string(front), "\n"), "\n")
	findings := make([]SourceFinding, 0, len(layer.Instruments))
	checked, compared, unread := r.today, 0, 0
	for i, instrument := range layer.Instruments {
		finding := r.instrument(ctx, name, instrument)
		findings = append(findings, finding)
		switch finding.Outcome {
		case SourceNone:
		case SourceUnread:
			unread++
		case SourceByHand:
			compared++
			checked = earliest(checked, instrument.CheckedByHand, false)
		case SourceUnchanged, SourceChanged, SourceFirstRead:
			compared++
			want.Instruments[i].TextDigest, want.Instruments[i].TextFetched = finding.digest, r.today
			if lines, err = stampInstrument(lines, i, finding.digest, day(r.today)); err != nil {
				return nil, nil, fmt.Errorf("%s: %w", name, err)
			}
		}
	}
	// A layer with a source nobody read keeps its old date, so the release
	// check goes on counting from the last full comparison.
	if compared > 0 && unread == 0 {
		want.SourceChecked = &checked
		lines = setTopKey(lines, "source-checked", day(checked))
	}
	stamped := strings.Join(lines, "\n") + "\n"
	var got layerFile
	if err := decodeStrict(name, []byte(stamped), &got); err != nil {
		return nil, nil, fmt.Errorf("the stamped front matter no longer reads: %w", err)
	}
	if !reflect.DeepEqual(got, want) {
		return nil, nil, fmt.Errorf("%s: a stamp changed more than the refresh keys; write each instrument as a block with name first", name)
	}
	return findings, []byte(fence + "\n" + stamped + fence + "\n" + string(body)), nil
}

func (r *sourceRefresh) instrument(ctx context.Context, layer string, in instrumentEntry) SourceFinding {
	finding := SourceFinding{Layer: layer, Instrument: in.Name, Address: in.readFrom()}
	switch {
	case !in.CheckedByHand.IsZero():
		finding.Outcome, finding.Detail = SourceByHand, "checked "+day(in.CheckedByHand)
		return finding
	case finding.Address == "":
		finding.Outcome = SourceNone
		return finding
	}
	text := r.read(ctx, finding.Address)
	if text.err != nil {
		finding.Outcome, finding.Detail = SourceUnread, text.err.Error()
		return finding
	}
	finding.digest, finding.Detail = text.digest, text.size
	switch in.TextDigest {
	case "":
		finding.Outcome = SourceFirstRead
	case text.digest:
		finding.Outcome = SourceUnchanged
	default:
		finding.Outcome = SourceChanged
	}
	return finding
}

func (r *sourceRefresh) read(ctx context.Context, address string) sourceText {
	if text, read := r.texts[address]; read {
		return text
	}
	var text sourceText
	page, err := r.fetch(ctx, address)
	if err != nil {
		text.err = err
	} else {
		text.digest, text.size, text.err = digestText(page)
	}
	r.texts[address] = text
	return text
}

// digestText answers the SHA-256 of the text a page holds. Markup is reduced
// to its text, because two fetches of one page differ in scripts and tokens.
// Another type, such as a PDF, is digested as the bytes it is.
func digestText(page SourcePage) (digest, size string, err error) {
	content := page.Body
	size = fmt.Sprintf("%d bytes", len(content))
	if isMarkup(page.ContentType) {
		text := webread.StripTags(string(page.Body))
		if len([]rune(text)) < minSourceText {
			return "", "", fmt.Errorf("the page holds %d characters of text, too few to be the law", len([]rune(text)))
		}
		content, size = []byte(text), fmt.Sprintf("%d characters", len([]rune(text)))
	}
	if len(content) == 0 {
		return "", "", errors.New("the answer is empty")
	}
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:]), size, nil
}

// isMarkup reports a type whose text sits between tags. An answer that names
// no type is read as a page.
func isMarkup(contentType string) bool {
	media, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return true
	}
	return strings.HasPrefix(media, "text/") || strings.HasSuffix(media, "xml")
}

// day writes an instant as the date a front-matter key holds.
func day(t time.Time) string { return t.UTC().Format(time.DateOnly) }
