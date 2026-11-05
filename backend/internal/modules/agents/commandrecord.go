// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// The two commands one tool serves through TWO contract operations
// (margince/margince#928 task 7): merge_records is mergeContact and
// mergeCompany, and enrich is scrapeCompany and deepReadCompany. Both are
// where the seam has to carry meaning rather than shape.
//
// A merge is the only command here that names TWO records, and which is which
// is the whole of it — one row survives and one is archived into it. The REST
// route says so in two places at once: POST /v1/contacts/{id}/merge merges the
// ROUTED contact INTO the body's `target_id`, so the routed id is the record
// merged FROM and the approval binds to the survivor named in the body. The
// route walk this seam replaces reads the routed id and gets the wrong half.
//
// An enrich is the same tool at two depths, and the depth is the difference
// between reading one page and crawling a whole site. It is carried as a TYPE
// whose two values each decoder sets STRUCTURALLY — never parsed from a string
// on the REST door — so there is no string for a mistranslation to hide in: a
// descriptor that synthesized "site" for scrapeCompany would satisfy every
// schema and describe a whole-site crawl to a human approving a one-page read.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/margince/margince/backend/internal/platform/webread"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/baselanguage"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// MergeCommand is one merge, whichever door asked for it.
//
// SourceID is the record merged FROM — archived and redirected to the
// survivor — and TargetID is the survivor everything relinks to. Both are
// operands and both belong here: each is read separately, each carries its own
// refusal, and the summary names both, so a command holding one of them could
// only describe half of what happens.
type MergeCommand struct {
	RecordType string
	SourceID   ids.UUID
	TargetID   ids.UUID
}

// NewMergeCall binds one merge to the resolver that answers for it, reading
// both halves through the record seam the merge itself writes through.
//
//nolint:ireturn // the call IS the product: a resolver named concretely here is exactly the thing that must not leave this package
func NewMergeCall(records datasource.SystemOfRecordProvider, language baselanguage.Resolver, cmd MergeCommand) GovernedCall {
	return bind[MergeCommand](&mergeResolver{records: records, language: language}, cmd)
}

type mergeResolver struct {
	records datasource.SystemOfRecordProvider
	// survivor and source are the two rows both questions rest on, remembered
	// for the reason anchoredRecord (command.go) remembers one: Guards refuses
	// on what the reads found and Subject pins and names it, and two readings
	// are two moments those answers are free to describe differently. A pair
	// rather than two anchoredRecords because the record TYPE is the command's,
	// not the resolver's — a merge is contact-to-contact or company-to-company, and the
	// type arrives with the call.
	// seen is the command the pair was read for, so a resolver asked about a
	// second merge reads that merge — the same key archiveResolver's own memo
	// carries (command.go), for the same reason its doc gives.
	seen     MergeCommand
	survivor datasource.Record
	source   datasource.Record
	read     bool
	language baselanguage.Resolver
}

// errNothingToMerge refuses a merge of a record into itself: there is no
// survivor and no archived half, so nothing an approval could describe.
var errNothingToMerge = errors.New("source and target must differ")

// requireMergeableType refuses a record type that has no merge verb.
//
// One function for both doors — predicate and sentence together: the staging
// path asks it through Guards below and the execution path through
// mergeRecords.Handle, which the approved retry re-enters without passing
// Guards. Two copies of the membership test is how the two doors come to
// disagree about which types can be merged, or to say it differently for the
// same refusal.
func requireMergeableType(recordType string) error {
	if mergeableTypes[recordType] {
		return nil
	}
	return &BadArgsError{
		Cause:    fmt.Errorf("record_type %q cannot be merged", recordType),
		Guidance: "mergeable types are " + strings.Join(mergeableTypeNames(), ", "),
	}
}

// halves reads both records the merge touches, once, in the order their
// refusals are reported.
func (r *mergeResolver) halves(ctx context.Context, cmd MergeCommand) (survivor, source datasource.Record, err error) {
	if r.read && r.seen == cmd {
		return r.survivor, r.source, nil
	}
	if err := requireMergeableType(cmd.RecordType); err != nil {
		return datasource.Record{}, datasource.Record{}, err
	}
	if cmd.SourceID == cmd.TargetID {
		return datasource.Record{}, datasource.Record{}, &BadArgsError{Cause: errNothingToMerge}
	}
	entity := datasource.EntityType(cmd.RecordType)
	survivor, err = r.records.Read(ctx, datasource.EntityRef{Type: entity, ID: cmd.TargetID})
	if err != nil {
		return datasource.Record{}, datasource.Record{}, err
	}
	source, err = r.records.Read(ctx, datasource.EntityRef{Type: entity, ID: cmd.SourceID})
	if err != nil {
		return datasource.Record{}, datasource.Record{}, err
	}
	r.seen, r.survivor, r.source, r.read = cmd, survivor, source, true
	return survivor, source, nil
}

// Both versions bind the decision: merging retires the source and changes the survivor.
func (r *mergeResolver) Subject(ctx context.Context, cmd MergeCommand) (StageInfo, error) {
	survivor, source, err := r.halves(ctx, cmd)
	if err != nil {
		return StageInfo{}, err
	}
	said := summaryIn(ctx, r.language)
	return StageInfo{
		TargetType:    cmd.RecordType,
		TargetID:      cmd.TargetID,
		TargetVersion: &survivor.Version,
		CoTargetType:  cmd.RecordType,
		CoTargetID:    cmd.SourceID,
		Summary:       fmt.Sprintf(said.merge, said.noun(cmd.RecordType), recordLabel(source), recordLabel(survivor)),
	}, nil
}

// Guards refuses a record type with no merge verb, a merge of a record into
// itself, either half the caller cannot see, and either half whose authority
// lives in another system of record.
//
// EVERY record this change touches, not just the pinned one, and named so a
// human reading the refusal knows which half blocked it. Validating only the
// survivor leaves the other half unguarded: the merge archives and relinks the
// source, and an externally-held source under a locally-authoritative survivor
// is still a change no approval could release.
func (r *mergeResolver) Guards(ctx context.Context, cmd MergeCommand) error {
	survivor, source, err := r.halves(ctx, cmd)
	if err != nil {
		return err
	}
	if err := refuseStagingElsewhere(survivor); err != nil {
		return fmt.Errorf("the record being merged INTO: %w", err)
	}
	if err := refuseStagingElsewhere(source); err != nil {
		return fmt.Errorf("the record being merged FROM: %w", err)
	}
	return nil
}

// EnrichCommand is one site read, whichever door asked for it.
//
// Depth is EnrichDepth rather than a string on purpose: the two contract
// operations behind this one verb differ ONLY in it, and each door sets it
// structurally — the tool from its own `depth` argument's closed vocabulary,
// the REST door from which of its two routes was taken. A string here would be
// a place for scrapeCompany to be described to a human as a whole-site crawl.
//
// URL is the caller's override for the company's own domain, empty when
// they named none.
type EnrichCommand struct {
	CompanyID ids.UUID    `json:"company_id"`
	URL       string      `json:"url"`
	Depth     EnrichDepth `json:"depth"`
}

// NewEnrichCall binds one site read to the resolver that answers for it,
// reading the company through the record seam.
//
//nolint:ireturn // the call IS the product: a resolver named concretely here is exactly the thing that must not leave this package
func NewEnrichCall(records datasource.SystemOfRecordProvider, language baselanguage.Resolver, cmd EnrichCommand) GovernedCall {
	return bind[EnrichCommand](&enrichResolver{
		language: language,
		company:  anchoredRecord{records: records, entityType: datasource.EntityCompany},
	}, cmd)
}

type enrichResolver struct {
	company  anchoredRecord
	language baselanguage.Resolver
}

// Subject names the COMPANY the approval binds to, pins its version, and
// says WHICH read is being released — a human approving a whole-site crawl
// must not read a line that says "page".
func (r *enrichResolver) Subject(ctx context.Context, cmd EnrichCommand) (StageInfo, error) {
	rec, err := r.company.row(ctx, cmd.CompanyID)
	if err != nil {
		return StageInfo{}, err
	}
	said := summaryIn(ctx, r.language)
	target := said.ownDomain
	if cmd.URL != "" {
		target = cmd.URL
	}
	return StageInfo{
		TargetType:    string(datasource.EntityCompany),
		TargetID:      cmd.CompanyID,
		TargetVersion: &rec.Version,
		Summary: fmt.Sprintf(said.enrich,
			cmd.Depth, target, recordLabel(rec)),
	}, nil
}

// Guards refuses an override URL the fetch could never take, then the
// company itself. It does not admit the DEPTH: the field's type is the
// admission, so there is no invalid value for a check to answer.
func (r *enrichResolver) Guards(ctx context.Context, cmd EnrichCommand) error {
	if err := requireEnrichURL(cmd.URL); err != nil {
		return err
	}
	if err := r.company.refuse(ctx, cmd.CompanyID); err != nil {
		return err
	}
	return r.requireReadTarget(ctx, cmd)
}

// requireReadTarget refuses a read with nothing to read: no url and no company
// domain, or a technical read, which only ever reads the domain. Staged, such a
// call is an approval nobody can accept, because its review cannot show a target.
func (r *enrichResolver) requireReadTarget(ctx context.Context, cmd EnrichCommand) error {
	if cmd.URL != "" && cmd.Depth != EnrichDepthTechnical {
		return nil
	}
	rec, err := r.company.row(ctx, cmd.CompanyID)
	if err != nil {
		return err
	}
	var company struct {
		Domains []struct {
			Domain string `json:"domain"`
		} `json:"domains"`
	}
	if err := json.Unmarshal(rec.Fields, &company); err != nil {
		return fmt.Errorf("crmagents: reading the company's domains: %w", err)
	}
	if len(company.Domains) > 0 {
		return nil
	}
	guidance := "pass `url` with the page to read, or add a domain to the company first"
	if cmd.Depth == EnrichDepthTechnical {
		guidance = "a technical read reads the company's domain; add a domain to the company first"
	}
	return &BadArgsError{Cause: errors.New("the company has no domain to read"), Guidance: guidance}
}

// Staging and execution share URL admission. An empty override uses the
// company's domain; a supplied override must be an absolute HTTP(S) URL.
func requireEnrichURL(raw string) error {
	if raw == "" {
		return nil
	}
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return &BadArgsError{Cause: fmt.Errorf("url %q must be an absolute http(s) URL", raw)}
	}
	// The fetch policy, asked HERE as well as at the fetcher.
	//
	// Not belt and braces: the fetcher's refusal is a failed run the caller
	// reads as "that did not work", and this one is a refused ARGUMENT the
	// caller reads as "do not ask for that". An agent handed the second can
	// correct itself; handed the first it retries. The fetcher stays the
	// enforcing gate — every other path into it is covered there and not here.
	if decision := webread.MayFetch(raw); !decision.Allowed {
		return &BadArgsError{Cause: fmt.Errorf("url %q cannot be read: %s", raw, decision.Reason)}
	}
	return nil
}
