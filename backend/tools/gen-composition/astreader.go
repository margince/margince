// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"sort"
	"strconv"
	"strings"

	"github.com/margince/margince/backend/pkg/extension"
)

// scannableGoFile reports whether to parse this .go file for the
// declaration scan. It excludes only what go/build ignores BY NAME — a
// name beginning with '.' or '_', and _test.go test files. It deliberately
// does NOT apply //go:build constraints or GOOS/GOARCH suffixes: the scan
// is platform-independent ON PURPOSE, so the committed manifest is the
// same on every host. A build-tag/GOOS-split New() is therefore parsed on
// all platforms and rejected by the multiple-New guard rather than
// resolved per-context (which would make the manifest platform-dependent).
func scannableGoFile(name string) bool {
	if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
		return false
	}
	return !strings.HasSuffix(name, "_test.go")
}

// deriveUnitManifest reads one unit's declaration and encodes it, which is
// what the file next to the unit holds. A caller that also needs the
// declaration ITSELF — the composed SPA registry does, for the secret scope
// its settings placement derives from — reads it through readUnitManifest and
// encodes the same value, so the bytes on disk and the data the composition
// reasons over cannot come from two different parses of one declaration.
func deriveUnitManifest(u extensionUnit, vocab map[string]string, verbs []declaredVerb, jobDecls []extension.JobDeclaration) ([]byte, error) {
	m, err := readUnitManifest(u, vocab, verbs, jobDecls)
	if err != nil {
		return nil, err
	}
	return encodeUnitManifest(m)
}

func readUnitManifest(u extensionUnit, vocab map[string]string, verbs []declaredVerb, jobDecls []extension.JobDeclaration) (unitManifest, error) {
	fset := token.NewFileSet()
	// ParseComments, because one of the declarations this reader has to judge
	// lives in a comment: //go:embed binds a pattern to the var beneath it, and
	// the Migrations field is checked against exactly that binding.
	pkgs, err := parseDirByPackage(fset, u.Dir, parser.SkipObjectResolution|parser.ParseComments)
	if err != nil {
		return unitManifest{}, fmt.Errorf("extensions/%s: %w", u.Name, err)
	}
	if len(pkgs) != 1 {
		return unitManifest{}, fmt.Errorf("extensions/%s: the unit root must hold exactly one package, found %d", u.Name, len(pkgs))
	}
	if err := rejectLiveInitializers(pkgs, fset); err != nil {
		return unitManifest{}, fmt.Errorf("extensions/%s: %w", u.Name, err)
	}
	r := &unitReader{
		fset:            fset,
		vocab:           vocab,
		verbs:           verbs,
		jobs:            jobDecls,
		hasMigrations:   u.HasMigrations,
		migrationEmbeds: migrationEmbedVars(pkgs),
		handlerAliases:  collectHandlerAliases(pkgs, extensionPkgPath),
	}
	newFn, newFile, count := findNew(pkgs)
	if count == 0 {
		return unitManifest{}, fmt.Errorf("extensions/%s: no New() in the unit root package — the declaration constructor is required", u.Name)
	}
	if count > 1 {
		// The scan is platform-independent (build tags/GOOS are not
		// applied), so a build-tag or GOOS-split New() appears as several
		// here. That is rejected by design: an extension declaration is
		// platform-independent inert data, and picking one of several
		// (unordered map iteration) would make the committed manifest
		// nondeterministic. Declare exactly one New().
		return unitManifest{}, fmt.Errorf("extensions/%s: multiple New() constructors in the unit root — declare exactly one; an extension declaration is platform-independent, so a build-tag/GOOS-split New() is unsupported", u.Name)
	}
	m, err := r.readExtension(newFn, newFile)
	if err != nil {
		return unitManifest{}, err
	}
	if m.Name != u.Name {
		return unitManifest{}, fmt.Errorf("extensions/%s: New() declares name %q — the directory name IS the unit name", u.Name, m.Name)
	}
	return m, nil
}

func findNew(pkgs map[string][]*ast.File) (fn *ast.FuncDecl, file *ast.File, count int) {
	for _, files := range pkgs {
		for _, f := range files {
			for _, decl := range f.Decls {
				if d, ok := decl.(*ast.FuncDecl); ok && d.Recv == nil && d.Name.Name == "New" {
					fn, file, count = d, f, count+1
				}
			}
		}
	}
	return fn, file, count
}

func encodeUnitManifest(m unitManifest) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(m); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// unitReader walks one unit's declaration AST. Everything it reads is a
// LITERAL: the declaration idiom requires New() to
// return a literal so the manifest derives without compiling — a computed
// value is a hard error naming the position, never a silent gap.
type unitReader struct {
	fset  *token.FileSet
	vocab map[string]string
	// verbs are the operations this unit's contract fragments declare, read
	// from the MERGED contract before the AST is walked. The reader needs them
	// to join behavior to declaration (joinToolsToContract) and to build the
	// manifest's risk tiers, which are contract-derived, not AST-derived.
	verbs []declaredVerb
	// jobs are the scheduled jobs this unit's jobs.yaml fragment declares,
	// read from the MERGED contract for the same reason verbs are: the join
	// between behavior and declaration, and the manifest's job risk tiers.
	jobs []extension.JobDeclaration
	// hasMigrations is whether the unit ships a migrations/ layer, read from
	// the tree rather than from the AST — the other half of the join the
	// Migrations field has to satisfy.
	hasMigrations bool
	// migrationEmbeds are the package-level vars whose //go:embed directive
	// names the migrations layer, by name. The Migrations field must be one of
	// them; see readExtensionField.
	migrationEmbeds map[string]bool
	// sawMigrations records that the literal set Migrations at all, so an
	// absent field on a unit that ships SQL is caught after the walk.
	sawMigrations bool
	// description is the unit's declared sentence, held here rather than on the
	// manifest: it is validated after the walk and never emitted.
	description string
	// handlerAliases names, per published handler type ("ToolHandler",
	// "JobHandler", "InboundHandler"), every package-local type alias of it —
	// `type Handler = extension.InboundHandler` binds "Handler" under
	// "InboundHandler". isStaticallyNilHandler consults this so a nil
	// conversion spelled through the local alias (`Handler(nil)`) is
	// recognized exactly as one spelled through the published type
	// (`extension.InboundHandler(nil)`) is.
	handlerAliases map[string]map[string]bool
}

// migrationEmbedVars collects the package-level vars whose //go:embed
// directive names the migrations layer.
//
// The directive is read from a doc comment because that is where the go:embed
// contract puts it: the compiler binds the pattern to the var immediately
// below it, so the same association read here is the one that holds at build
// time. It is read from TWO places, because go/ast puts it in two:
// `var (\n //go:embed migrations\n sql embed.FS\n)` hangs it on the SPEC,
// and the ungrouped `//go:embed migrations\nvar sql embed.FS` hangs it on the
// DECL. Reading only the second rejected the grouped form, which is ordinary
// valid Go.
//
// The decl's own doc is consulted only for an UNGROUPED declaration. A comment
// above `var (` binds to no spec in Go, so treating it as one would accept a
// directive the compiler ignores — and mark every var in the group.
func migrationEmbedVars(pkgs map[string][]*ast.File) map[string]bool {
	embeds := map[string]bool{}
	for _, files := range pkgs {
		for _, f := range files {
			for _, decl := range f.Decls {
				d, ok := decl.(*ast.GenDecl)
				if !ok || d.Tok != token.VAR {
					continue
				}
				grouped := d.Lparen.IsValid()
				for _, spec := range d.Specs {
					v, ok := spec.(*ast.ValueSpec)
					if !ok {
						continue
					}
					if !embedsMigrations(v.Doc) && (grouped || !embedsMigrations(d.Doc)) {
						continue
					}
					for _, name := range v.Names {
						embeds[name.Name] = true
					}
				}
			}
		}
	}
	return embeds
}

// embedsMigrations reports whether a doc comment carries a //go:embed
// directive covering the migrations layer.
//
// It is stricter about the DIRECTIVE and looser about the PATTERN than the
// obvious prefix test, and both directions are the compiler's rule rather than
// a preference:
//
//   - The separator is a single ASCII SPACE, not "whitespace". The compiler
//     recognizes the directive by `text == "go:embed"` or the exact prefix
//     `"go:embed "` (cmd/compile/internal/noder), so BOTH
//     `//go:embedmigrations` and a tab-separated `//go:embed\tmigrations` are
//     ordinary comments: the FS beneath either stays EMPTY and the unit's
//     migrations are silently never applied. That is precisely the defect this
//     gate exists to catch, and accepting a separator the compiler does not
//     would let an empty embed.FS satisfy Migrations — the unit boots against a
//     database where its tables were never created.
//   - A pattern may be a quoted Go string literal, so `//go:embed "migrations"`
//     is valid and embeds the same directory. Comparing the raw token refused
//     it for a spelling difference.
func embedsMigrations(doc *ast.CommentGroup) bool {
	if doc == nil {
		return false
	}
	for _, c := range doc.List {
		// The trailing space is part of the prefix ON PURPOSE — it is the
		// compiler's own separator, and matching anything looser accepts a
		// comment the compiler ignores.
		rest, ok := strings.CutPrefix(c.Text, "//go:embed ")
		if !ok {
			continue
		}
		for _, pattern := range strings.Fields(rest) {
			if unquoted, err := strconv.Unquote(pattern); err == nil {
				pattern = unquoted
			}
			// `all:` is the only prefix go:embed defines, and it changes which
			// files inside the directory are taken, not which directory.
			pattern = strings.TrimPrefix(pattern, "all:")
			if first, _, _ := strings.Cut(pattern, "/"); first == migrationsLayer {
				return true
			}
		}
	}
	return false
}

func (r *unitReader) readExtension(fn *ast.FuncDecl, file *ast.File) (unitManifest, error) {
	expr, err := r.singleReturn(fn)
	if err != nil {
		return unitManifest{}, err
	}
	lit, ok := expr.(*ast.CompositeLit)
	if !ok || !isSelector(lit.Type, importAlias(file, extensionPkgPath), "Extension") {
		return unitManifest{}, r.errAt(expr, "New must return an extension.Extension literal")
	}
	tiers, err := toolRequests(r.verbs)
	if err != nil {
		return unitManifest{}, err
	}
	jobTiers, err := jobRequests(r.jobs)
	if err != nil {
		return unitManifest{}, err
	}
	m := unitManifest{Schema: 1, RiskTiers: append(tiers, jobTiers...)}
	for _, elt := range lit.Elts {
		if err := r.readExtensionField(elt, file, &m); err != nil {
			return unitManifest{}, err
		}
	}
	if r.hasMigrations && !r.sawMigrations {
		return unitManifest{}, r.errPos(lit, "the unit ships %s/ but New() declares no Migrations field — the directory is validated and gated, and applied by nothing; embed it and name it here", migrationsLayer)
	}
	// Validate identity through the published grammar the boot preflight
	// runs, so gen-time acceptance cannot diverge from boot-time: an empty,
	// whitespace-framed, or non-printable Version passes neither. These are
	// SEMANTIC errors — the value is a literal, just an invalid one — so
	// they carry position but not the "declare literal values" prescription.
	if err := extension.Name(m.Name).Validate(); err != nil {
		return unitManifest{}, r.errPos(lit, "%v", err)
	}
	if err := extension.Version(m.Version).Validate(); err != nil {
		return unitManifest{}, r.errPos(lit, "%v", err)
	}
	// Absent reads as empty here, which is exactly what Validate refuses — so
	// a unit that declares no Description at all fails the generator the same
	// way one declaring an empty string does, and for the same reason.
	if err := extension.Description(r.description).Validate(); err != nil {
		return unitManifest{}, r.errPos(lit, "%v", err)
	}
	sort.Slice(m.RiskTiers, func(i, j int) bool { return m.RiskTiers[i].ID < m.RiskTiers[j].ID })
	sort.Slice(m.Secrets, func(i, j int) bool {
		if m.Secrets[i].Key != m.Secrets[j].Key {
			return m.Secrets[i].Key < m.Secrets[j].Key
		}
		return m.Secrets[i].Scope < m.Secrets[j].Scope
	})
	sort.Slice(m.Subscriptions, func(i, j int) bool { return m.Subscriptions[i].Name < m.Subscriptions[j].Name })
	sort.Slice(m.Ingress, func(i, j int) bool { return m.Ingress[i].System < m.Ingress[j].System })
	return m, nil
}

func (r *unitReader) readExtensionField(elt ast.Expr, file *ast.File, m *unitManifest) error {
	kv, ok := elt.(*ast.KeyValueExpr)
	if !ok {
		return r.errAt(elt, "Extension fields must be keyed")
	}
	key, ok := kv.Key.(*ast.Ident)
	if !ok {
		return r.errAt(kv.Key, "Extension fields must be keyed by name")
	}
	var err error
	switch key.Name {
	case "Name":
		m.Name, err = r.stringLit(kv.Value, "Name")
	case "Version":
		m.Version, err = r.stringLit(kv.Value, "Version")
	case "Description":
		// Kept on the reader, not on the manifest: the manifest records what an
		// OPERATOR must resolve, and a sentence describing the unit asks
		// nothing of them. It is still READ and VALIDATED, because gen-time
		// acceptance may not diverge from boot-time — a unit the generator
		// admitted and the boot then refused would pass the composition lane
		// and fail the deploy.
		r.description, err = r.stringLit(kv.Value, "Description")
	case "Tools":
		// The manifest's risk tiers are already set, from the merged contract.
		// What the Go slice contributes is the join: behavior for a verb the
		// contract does not declare is a defect, reported at its own line.
		var tools []declaredTool
		tools, err = r.readTools(kv.Value, file)
		if err == nil {
			err = r.joinToolsToContract(served(tools), r.verbs)
		}
	case "Jobs":
		// Symmetric with Tools: the manifest's job risk tiers are already set,
		// from the merged contract. What the Go slice contributes is the join —
		// behavior for a job the contract does not declare is a defect,
		// reported at its own line.
		var declared []declaredTool
		declared, err = r.readJobs(kv.Value, file)
		if err == nil {
			err = r.joinJobsToContract(served(declared), r.jobs)
		}
	case "Jurisdictions":
		// Recognized and deliberately skipped: a jurisdiction pack is
		// passive policy the core consults, never a governed operation an
		// operator resolves, so it contributes no manifest entry.
	case "FailureClasses":
		// Recognized and deliberately skipped, for the reason a jurisdiction
		// pack is: a failure vocabulary is inert operator-facing text — the
		// names a unit gives the ways its own jobs fail — and not a capability
		// anybody grants or resolves. There is no tier to set and no secret to
		// provision, so a manifest entry would record a request nobody answers.
		//
		// SKIPPED HERE IS NOT UNCHECKED, and the distinction matters because
		// these strings are published into river_job.errors, which has no
		// workspace and which every workspace's admin reads. What holds them is
		// nearer the use: extension.ValidateFailureClasses refuses a malformed
		// set at boot before a single class is registered, and the composed
		// registration additionally refuses one that would impersonate a core
		// class. Both see the VALUES, which a static reader of this declaration
		// cannot — the field conventionally names a package-level slice so the
		// declared set and the set the unit's code returns stay one list.
		//
		// Nothing needs emitting either way: the generated composition calls the
		// unit's New(), so the runtime value carries this field already.
	case "Migrations":
		// It contributes no MANIFEST entry, for the same reason a jurisdiction
		// pack does not: what an operator resolves are risk tiers and secret
		// requests, and a schema is neither. But the field is not unchecked,
		// because it is the ONLY thing that connects the SQL on disk to the SQL
		// that runs. collectUnitTables reads extensions/<unit>/migrations/ and
		// extmigrategate applies it as the restricted ext_<name> role; both
		// address the DIRECTORY. cmd/migrate applies this FIELD. A unit that
		// ships a validated, gated migrations/ tree and leaves the field unset,
		// or points it at some other embedded FS, passes every one of those
		// checks and then boots against a database where its tables were never
		// created — and the first symptom is a handler answering `relation
		// does not exist` in production.
		//
		// So the field must name a package-level var whose //go:embed directive
		// covers that directory. That is a shape check, not a proof: the var
		// could embed migrations/ AND more, and an fs.FS assembled at runtime
		// is outside what a static reader can follow at all. What it does close
		// is the whole population of accidents — the unset field, the typo, the
		// var that embeds a different layer.
		r.sawMigrations = true
		name, ok := kv.Value.(*ast.Ident)
		if !ok || !r.migrationEmbeds[name.Name] {
			err = r.errAt(kv.Value, "Migrations must name a package-level var whose //go:embed directive covers %s/ — the field is what cmd/migrate applies, so one pointing anywhere else leaves the unit's tables uncreated at boot", migrationsLayer)
		}
	case "Secrets":
		var secrets []secretsRequest
		secrets, err = r.readSecrets(kv.Value, file)
		if err == nil {
			m.Secrets = append(m.Secrets, secrets...)
		}
	case "Subscriptions":
		var subs []subscriptionRequest
		subs, err = r.readSubscriptions(kv.Value, file)
		if err == nil {
			m.Subscriptions = append(m.Subscriptions, subs...)
		}
	case "Ingress":
		var sources []ingressSource
		sources, err = r.readIngress(kv.Value, file)
		if err == nil {
			m.Ingress = append(m.Ingress, sources...)
		}
	case "Channels":
		var channels []declaredChannel
		channels, err = r.readChannels(kv.Value, file)
		if err == nil {
			m.Channels = append(m.Channels, channels...)
		}
	case "Inbound":
		var endpoints []inboundEndpoint
		endpoints, err = r.readInbound(kv.Value, file)
		if err == nil {
			m.Inbound = append(m.Inbound, endpoints...)
		}
	default:
		// Fail closed: a field this generator does not recognize could be a
		// future governed capability, and a manifest that silently omitted
		// it would hide a request from the operator.
		err = r.errAt(kv, "Extension field %s is not derivable by this generator — teach the manifest reader before declaring it", key.Name)
	}
	return err
}
