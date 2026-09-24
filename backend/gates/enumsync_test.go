// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// The enum-vocabulary sync as a fitness function: where domain logic
// branches on a typed Go enum, its constant set must equal the schema's
// CHECK (col IN (...)) set for the column it mirrors. The valid set
// living only in the DB is how a typo'd Go literal compiles and
// misbehaves silently; a Go set drifting from the CHECK is how a valid
// value 500s at insert. This gate pins the two spellings together —
// registry-driven, so adding an enum means adding one line here, and
// the sets themselves are DERIVED from the migration sources and the
// Go const declarations, never restated.

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// enumBindings maps "table.column" to the Go type that mirrors it.
//
// The entity vocabulary is restated by a dozen polymorphic columns, so it
// is bound here rather than grepped: adding a record type means widening
// one Go const set and every CHECK that mirrors it, and this gate is what
// makes widening eleven of twelve a failure instead of a silent half-job.
// Two sets, because they are genuinely two vocabularies — a reference TO a
// record (datasource.RecordType) never names an activity, while a thing
// hung OFF an object (datasource.EntityType) can.
var enumBindings = map[string]struct{ pkgDir, typeName string }{
	// Which check the ingress grammar refused a record on. The core writes the
	// class and the column holds the vocabulary, so a class Go learned and the
	// schema did not must fail the insert rather than land a value nothing can
	// name. The published surface is where the type lives, because a unit reads
	// the same class off its Result.
	"extension_ingest_refusal.refusal": {"pkg/extension", "RecordRefusal"},
	"lead.status":                      {"internal/modules/contacts", "LeadStatus"},
	"deal.status":                      {"internal/modules/deals", "DealStatus"},
	"stage.semantic":                   {"internal/modules/deals", "StageSemantic"},
	"contact_consent.state":            {"internal/modules/consent", "ConsentState"},
	"offer_line_item.proposal_state":   {"internal/modules/deals", "ProposalState"},
	"stage_exit_criterion.kind":        {"internal/modules/deals", "CriterionKind"},
	"knowledge_document.ingest_status": {"internal/contracts", "KnowledgeDocumentIngestStatus"},
	// The tag palette is spelled in five places: this CHECK, the contract enum,
	// the MCP tool schemas, the frontend's TAG_TONES and its dot stylesheet.
	// Widening four of the five is SILENT — the pill drops a tone it does not
	// know, so a half-widened palette reaches a reader as a tag with no dot,
	// which is exactly how an uncoloured tag looks.
	"tag.color": {"internal/contracts", "TagColor"},
	// Where a saved draft was opened. The wire enum is what the composer sends,
	// so a value the contract admits and the CHECK refuses would be a save that
	// fails with a constraint error instead of a validation answer.
	"mail_draft.anchor_type": {"internal/contracts", "MailDraftAnchorType"},

	"activity_link.entity_type": {"internal/shared/ports/datasource", "RecordType"},
	"list.entity_type":          {"internal/shared/ports/datasource", "RecordType"},
	"list_member.entity_type":   {"internal/shared/ports/datasource", "RecordType"},
	"taggable.entity_type":      {"internal/shared/ports/datasource", "RecordType"},
	"record_grant.record_type":  {"internal/shared/ports/datasource", "RecordType"},

	"attachment.entity_type":       {"internal/shared/ports/datasource", "EntityType"},
	"embedding.entity_type":        {"internal/shared/ports/datasource", "EntityType"},
	"field_provenance.object_type": {"internal/shared/ports/datasource", "EntityType"},
	// NOT EntityType, and this is the one binding where the difference is the
	// point. A custom field hangs off a TARGET; an EntityType is something a
	// record provider can be asked about. Contract is the first member that is
	// the former without being the latter, so the CHECK mirrors
	// fieldcatalog.Target and the gate compares it against that.
	"custom_field.object": {"internal/shared/ports/fieldcatalog", "Target"},

	// The CONTRACT-BACKED columns: a column whose CHECK is mirrored by a named
	// schema in crm.yaml, which the generator turns into a Go const set.
	//
	// These are the instance #1496 names — "a contract enum wider than the
	// constraint behind it" — and what makes them different from the bindings
	// above is where the drift hurts. A Go set drifting from its CHECK 500s at
	// insert, which is loud. A CONTRACT set drifting wider is quiet and worse:
	// the document promises a value, a caller sends it, and the database
	// refuses a request the published contract said was well-formed.
	//
	// Bound by hand and deliberately not derived, because the derivation is
	// what does not work. An exact-match scan over the committed head catalog
	// finds forty-odd pairs and most are coincidence — MeetingPlanTier and
	// assurance_exception.severity share [high, low, medium] and have nothing
	// to do with each other, and a gate that bound those would fail the day one
	// of them legitimately moved. The pairs below are the ones where the schema
	// NAMES the column's concept, which is a judgement a scan cannot make.
	"activity.audience":               {"internal/contracts", "ActivityAudience"},
	"capture_exclusion.kind":          {"internal/contracts", "CaptureExclusionKind"},
	"capture_exclusion.scope":         {"internal/contracts", "CaptureExclusionScope"},
	"capture_owner_identity.kind":     {"internal/contracts", "CaptureOwnerIdentityKind"},
	"capture_owner_identity.source":   {"internal/contracts", "CaptureOwnerIdentitySource"},
	"commission_entry.status":         {"internal/contracts", "CommissionStatus"},
	"conversation_claim.kind":         {"internal/contracts", "ConversationClaimKind"},
	"deal_room.state":                 {"internal/contracts", "DealRoomState"},
	"deal_stage_evidence.author_side": {"internal/contracts", "StageEvidenceAuthorSide"},
	"deal_stage_evidence.commitment":  {"internal/contracts", "StageEvidenceCommitment"},
	"deal_stage_evidence.source_type": {"internal/contracts", "StageEvidenceSource"},
	"import_run.status":               {"internal/contracts", "ImportRunStatus"},
	"intro_request.fallback_policy":   {"internal/contracts", "IntroFallbackPolicy"},
	"intro_request.note_generated_by": {"internal/contracts", "IntroNoteOrigin"},
	"intro_request.route_type":        {"internal/contracts", "ContactGraphRouteType"},
	"intro_request.status":            {"internal/contracts", "IntroRequestStatus"},
	"lead_manual_signal.signal_kind":  {"internal/contracts", "LeadManualSignalKind"},
	"lead_source.intent":              {"internal/contracts", "LeadSourceIntent"},
	"project_health_assessment.state": {"internal/contracts", "ProjectHealthState"},
	"provider_connection.mode":        {"internal/contracts", "ProviderConnectionMode"},
	"provider_connection.status":      {"internal/contracts", "ProviderConnectionStatus"},
	"retention_policy.action":         {"internal/contracts", "RetentionAction"},
	"saved_view.resource":             {"internal/contracts", "SavedViewResource"},

	// One vocabulary, six columns. Every derived narrative row records whether a
	// human template or a model wrote it, and WrittenBy is the word the contract
	// uses for that everywhere, so widening the contract without widening all
	// six CHECKs fails here rather than at the first insert of the new value.
	//
	// A seventh narrative table needs its OWN line: this registry is keyed by
	// table.column and has no wildcard, so reusing the vocabulary is free and
	// being bound to it is not. That is the hand-kept cost #1496 names, and it
	// is worth paying here for the reason the block above gives — which pairs
	// are real is a judgement, and a wildcard would bind the next column called
	// generated_by whether or not it holds this vocabulary.
	"company_brief.generated_by":      {"internal/contracts", "WrittenBy"},
	"company_dossier.generated_by":    {"internal/contracts", "WrittenBy"},
	"company_growth_fit.generated_by": {"internal/contracts", "WrittenBy"},
	"company_scan.generated_by":       {"internal/contracts", "WrittenBy"},
	"contact_brief.generated_by":      {"internal/contracts", "WrittenBy"},
	"deal_status_card.generated_by":   {"internal/contracts", "WrittenBy"},
}

// checkInList captures CHECK (col IN ('a','b',…)) allowing an optional
// "col IS NULL OR" prefix; applied to a table block's accumulated text.
var checkInList = regexp.MustCompile(`(?is)CHECK\s*\(\s*(?:([a-z_]+)\s+IS\s+NULL\s+OR\s+)?([a-z_]+)\s+IN\s*\(([^)]*)\)`)

// alterTableStmt keys an ALTER statement's CHECK lists to their table.
var alterTableStmt = regexp.MustCompile(`(?is)ALTER\s+TABLE\s+([a-z_]+)`)

// singleQuoted pulls the 'value' literals out of a captured IN-list.
var singleQuoted = regexp.MustCompile(`'([^']*)'`)

// migrationPrefix is the numeric-prefix name shape last-wins ordering needs.
// Widths differ — custom/ stamps 14 digits, core/ ten since it started naming
// a migration for the unix second it was written, and four before that — and
// last-wins does not need them to agree, only to sort
// lexically-equals-chronologically within one namespace, which is all
// tableCheckSets' per-root walk relies on. core/'s closed four-digit sequence
// is zero-padded and so still sorts below its later stamps.
var migrationPrefix = regexp.MustCompile(`^\d{4,}_`)

// recordChecks derives every CHECK (col IN (…)) set in text and records
// it under table.col — the one spelling shared by the CREATE-block and
// ALTER-statement passes.
func recordChecks(sets map[string][]string, table, text string) {
	for _, m := range checkInList.FindAllStringSubmatch(text, -1) {
		var vals []string
		for _, q := range singleQuoted.FindAllStringSubmatch(m[3], -1) {
			vals = append(vals, q[1])
		}
		sort.Strings(vals)
		sets[table+"."+m[2]] = vals
	}
}

// tableCheckSets derives table.column → allowed set from the migration
// sources, using the same line-based CREATE TABLE scan as updateguard
// (column definitions nest parens beyond what a block regex can pair),
// plus a per-statement ALTER TABLE pass: vocabularies grow additively
// (drop CHECK + re-add wider), so the LAST migration to state a column's
// IN-list wins — walk order is lexical, which is migration order.
func tableCheckSets(t *testing.T) map[string][]string {
	t.Helper()
	sets := map[string][]string{}
	for _, root := range []string{"migrations/core", "migrations/custom"} {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".up.sql") {
				return err
			}
			// Last-wins depends on lexical order being migration order:
			// every name must carry the numeric version prefix.
			if !migrationPrefix.MatchString(d.Name()) {
				return fmt.Errorf("%s: migration name lacks the numeric version prefix the lexical-order derivation relies on", path)
			}
			raw, err := os.ReadFile(path) // #nosec G304 G122 -- path is a *.up.sql file from walking the trusted migrations tree
			if err != nil {
				return err
			}
			// Read through the renames the migrations themselves declare: a CHECK
			// against a table since renamed would otherwise be filed under a
			// name no caller uses, and the vocabulary derived for the current
			// one comes back empty.
			sql := withCurrentNames(string(raw))
			current, block := "", strings.Builder{}
			flush := func() {
				if current == "" {
					return
				}
				recordChecks(sets, current, block.String())
				current = ""
				block.Reset()
			}
			for _, line := range strings.Split(sql, "\n") {
				if m := createTableLine.FindStringSubmatch(line); m != nil {
					flush()
					current = m[1]
					continue
				}
				if strings.HasPrefix(line, ");") {
					flush()
					continue
				}
				if current != "" {
					block.WriteString(line)
					block.WriteString("\n")
				}
			}
			flush()
			for _, stmt := range strings.Split(sql, ";") {
				if alter := alterTableStmt.FindStringSubmatch(stmt); alter != nil {
					recordChecks(sets, alter[1], stmt)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return sets
}

// goConstSet derives the string values of every constant declared with
// the given type in the package directory.
func goConstSet(t *testing.T, pkgDir, typeName string) []string {
	t.Helper()
	var vals []string
	fset := token.NewFileSet()
	entries, err := os.ReadDir(pkgDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(pkgDir, e.Name()), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range file.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.CONST {
				continue
			}
			for _, spec := range gd.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				id, ok := vs.Type.(*ast.Ident)
				if !ok || id.Name != typeName {
					continue
				}
				for _, v := range vs.Values {
					lit, ok := v.(*ast.BasicLit)
					if !ok || lit.Kind != token.STRING {
						continue
					}
					s, err := strconv.Unquote(lit.Value)
					if err != nil {
						t.Fatal(err)
					}
					vals = append(vals, s)
				}
			}
		}
	}
	sort.Strings(vals)
	return vals
}

func TestEveryDomainEnumMatchesItsSchemaCheck(t *testing.T) {
	t.Parallel()
	checks := tableCheckSets(t)
	for col, binding := range enumBindings {
		want, ok := checks[col]
		if !ok {
			t.Errorf("enumBindings[%s]: no CHECK (… IN (…)) found in the migrations — stale binding or broken derivation", col)
			continue
		}
		got := goConstSet(t, binding.pkgDir, binding.typeName)
		if len(got) == 0 {
			t.Errorf("enumBindings[%s]: no %s constants found in %s", col, binding.typeName, binding.pkgDir)
			continue
		}
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Errorf("%s: Go %s set %v != schema CHECK set %v — change both together", col, binding.typeName, got, want)
		}
	}
}
