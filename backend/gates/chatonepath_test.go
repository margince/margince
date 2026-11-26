// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H3

package gates

// Every chat network receives, files, changes and sends through the same core
// functions. A later feature, such as a bot reply, then works once for all.
//
// The gate reads the type-checked module, so an alias, a method value or an
// embedded wrapper resolves to the object it names. Three places can fork the path:
//
//   - A chat-speaking file constructs a connector.NormalizedRecord or references
//     Upsert of the capture sink. capture.FileChatMessage maps a chat message to a
//     record, and compose/extingress.go routes the unit ingress through it.
//   - A chat-speaking file writes a table of the activities module in SQL, or
//     references a write method of the activities Store, outside capture and the
//     activities module.
//   - A provider's send API is reached outside its connector.MessageSender, or
//     a file outside the provider package uses the provider's host.
//
// The provider packages are the capture subpackages that declare a Provider
// constant, checked against the channel_provider seeds in the migrations. Send and
// I/O functions are found by what they reach in net/http. The activities tables
// come from tableOwners.

import (
	"go/ast"
	"go/constant"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"io/fs"
	"path"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const (
	chatRecordOwner   = "internal/modules/capture/chatmessage.go"
	sdkIngressMapping = "internal/compose/extingress.go"
	connectorDir      = "internal/shared/ports/connector"
	captureDir        = "internal/modules/capture"
	activitiesDir     = "internal/modules/activities"
	chatSendResolver  = "internal/compose/commschannel.go"
	// senderResolver holds ChatSenders.For, which resolves a provider's
	// connector.MessageSender and credential.
	// Held by: TestChatNetworksShareOneReceiveFileChangeAndSendPath (backend/gates/chatonepath_test.go)
	senderResolver = "internal/modules/capture/chatsenders.go"
)

// isSenderResolveSeam reports a file besides the resolver that may return a
// connector.MessageSender. Each hands on what the resolver answered, or answers
// for a transport no core provider owns.
func isSenderResolveSeam(file string) bool {
	switch file {
	case "internal/modules/capture/channelsend.go": // the workspace bot's lookup, which the resolver calls, and the registry's entry to the resolver
	case "internal/compose/commschannel.go": // the comms send path's entry, which calls the resolver
	case "internal/compose/extchannelsend.go": // the transport an extension unit declares
	default:
		return false
	}
	return true
}

var activityWriterPrefixes = []string{"Create", "Insert", "Update", "Delete", "Archive", "Edit", "Remove", "Set"}

var providerHost = regexp.MustCompile(`^https://[A-Za-z0-9.-]+`)

var channelProviderSeed = regexp.MustCompile(`(?s)INSERT INTO channel_provider.*?;`)

var seedQuotedName = regexp.MustCompile(`'([a-z_]+)'`)

// chatPackages is the type-checked module a run reads, with what the rules derive from it.
type chatPackages struct {
	pkgs      []typedPackage
	fset      *token.FileSet
	vocab     map[types.Object]bool
	providers map[string]string
	ioFuncs   map[types.Object]bool
	direct    map[types.Object]bool
	sends     map[types.Object]bool
	writers   map[types.Object]bool
	hosts     map[string]string
	tables    *regexp.Regexp
}

func (c *chatPackages) file(f *ast.File) string { return c.fset.Position(f.Pos()).Filename }

func pkgDir(p typedPackage) string { return strings.TrimPrefix(p.path, modulePath+"/") }

func TestChatNetworksShareOneReceiveFileChangeAndSendPath(t *testing.T) {
	t.Parallel()
	c := newChatPackages(t, typedModule(t))
	for _, f := range c.findings() {
		t.Error(f)
	}
}

// TestChatOnePathGateFailsOnAForkedPath plants one violation per rule in a
// synthetic package, checked against the real module, and expects a finding that
// names it. Two plants must stay clean: a file that uses the shared entry, and
// an unrelated type that happens to share method names.
func TestChatOnePathGateFailsOnAForkedPath(t *testing.T) {
	t.Parallel()
	module := typedModule(t)
	const imp = "import \"" + modulePath + "/internal/shared/ports/connector\"\n"
	cases := []struct {
		name, want string
		files      map[string]string
		path       string
	}{
		{
			name: "a record built through an alias", want: "NormalizedRecord", path: modulePath + "/internal/compose/forkalias",
			files: map[string]string{"a.go": "package forkalias\n" + imp + "type rec = connector.NormalizedRecord\nvar _ connector.ChatBinding\nvar _ = rec{}\n"},
		},
		{
			name: "Upsert taken as a method value", want: "Upsert", path: modulePath + "/internal/compose/forkvalue",
			files: map[string]string{"a.go": "package forkvalue\n" + imp + "var _ connector.ChatMessage\nfunc f(s connector.Sink) { w := s.Upsert; _ = w }\n"},
		},
		{
			name: "Upsert through an embedded wrapper", want: "Upsert", path: modulePath + "/internal/compose/forkembed",
			files: map[string]string{"a.go": "package forkembed\n" + imp + "var _ connector.ChatMessage\ntype wrap struct{ connector.Sink }\nfunc f(w wrap, r connector.NormalizedRecord) { w.Upsert(nil, r) }\n"},
		},
		{
			name: "SQL that writes an activities table", want: "writes the table", path: modulePath + "/internal/compose/forksql",
			files: map[string]string{"a.go": "package forksql\n" + imp + "var _ connector.ChatMessage\nconst q = `INSERT INTO activity (id) VALUES (1)`\n"},
		},
		{
			name: "a write method of the activities store", want: "SetAudience", path: modulePath + "/internal/compose/forkstore",
			files: map[string]string{"a.go": "package forkstore\n" + imp + "import \"" + modulePath + "/internal/modules/activities\"\n" +
				"var _ connector.ChatChange\nfunc f(s *activities.Store) { _ = s.SetAudience }\n"},
		},
		{
			name: "a provider's SendText called outside it", want: "SendText", path: modulePath + "/internal/compose/forksend",
			files: map[string]string{"a.go": "package forksend\nimport \"" + modulePath + "/internal/modules/capture/whatsapp\"\n" +
				"func f(c *whatsapp.Client) { _, _ = c.SendText(nil, \"\", \"\", \"\", \"\") }\n"},
		},
		{
			name: "a provider host constant used with http.Client.Do", want: "GraphBase", path: modulePath + "/internal/compose/forkhost",
			files: map[string]string{"a.go": "package forkhost\nimport (\n\"net/http\"\n\"" + modulePath + "/internal/modules/capture/whatsapp\"\n)\n" +
				"func f(c *http.Client) { _, _ = c.Get(whatsapp.GraphBase) }\n"},
		},
		{
			name: "a provider POST through its low-level do outside its sender", want: "sends to the provider", path: modulePath + "/internal/modules/capture/signal",
			files: map[string]string{
				"client.go": "package signal\nimport \"net/http\"\nconst Provider = \"signal\"\ntype C struct{}\n" +
					"func (c *C) do(r *http.Request) (*http.Response, error) { return http.DefaultClient.Do(r) }\n" +
					"func (c *C) SendText() { _, _ = c.do(nil) }\n",
				"fork.go": "package signal\nfunc (c *C) Fork() { _, _ = c.do(nil) }\n",
			},
		},
		{
			name: "a second function that resolves a MessageSender", want: "resolves a connector.MessageSender", path: modulePath + "/internal/compose/forkresolver",
			files: map[string]string{"a.go": "package forkresolver\n" + imp + "func pick() connector.MessageSender { return nil }\n"},
		},
		{
			name: "a provider's sender built by a constructor of any name", want: "builds a provider's sender", path: modulePath + "/internal/compose/forkbuilder",
			files: map[string]string{"a.go": "package forkbuilder\nimport \"" + modulePath + "/internal/modules/capture/telegram\"\n" +
				"func build() { _ = telegram.New(nil) }\n"},
		},
		{
			name: "a sender built by a struct literal", want: "literal builds a provider's sender", path: modulePath + "/internal/compose/forkliteral",
			files: map[string]string{"a.go": "package forkliteral\nimport (\n\"context\"\n\"" + modulePath + "/internal/shared/ports/connector\"\n)\n" +
				"type mine struct{}\nfunc (mine) SendMessage(context.Context, connector.Auth, connector.ChannelMessage) (connector.SendReceipt, error) { return connector.SendReceipt{}, nil }\n" +
				"var _ = mine{}\n"},
		},
		{
			name: "a factory of a concrete sender", want: "resolves a connector.MessageSender", path: modulePath + "/internal/compose/forkfactory",
			files: map[string]string{"a.go": "package forkfactory\nimport \"" + modulePath + "/internal/modules/capture/whatsapp\"\n" +
				"func factory() *whatsapp.Sender { return nil }\n"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := newChatPackages(t, append(slices.Clone(module), plantPackage(t, module, tc.path, tc.files)))
			got := c.findings()
			planted := strings.TrimPrefix(tc.path, modulePath+"/")
			if !slices.ContainsFunc(got, func(f string) bool { return strings.Contains(f, planted) && strings.Contains(f, tc.want) }) {
				t.Errorf("planting %s found nothing naming %q; findings: %v", tc.path, tc.want, got)
			}
		})
	}
	clean := map[string]map[string]string{
		"a caller of the one resolver": {"a.go": "package cleanresolve\n" +
			"import \"" + modulePath + "/internal/modules/capture\"\n" +
			"func f(c capture.ChatSenders) { _, _, _ = c.For(nil, \"whatsapp\", [16]byte{}) }\n"},
		"a file that files through FileChatMessage": {"a.go": "package cleanentry\n" + imp +
			"import \"" + modulePath + "/internal/modules/capture\"\n" +
			"func f(s connector.Sink, b connector.ChatBinding, m connector.ChatMessage) { _, _ = capture.FileChatMessage(nil, s, b, m) }\n"},
		"an unrelated type with the same method names": {"a.go": "package cleanother\n" + imp +
			"var _ connector.ChatMessage\ntype other struct{}\nfunc (other) SetAudience() {}\nfunc (other) SendText() {}\n" +
			"func f(o other) { o.SetAudience(); o.SendText() }\n"},
	}
	for name, files := range clean {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			p := modulePath + "/internal/compose/" + strings.Fields(files["a.go"])[1]
			c := newChatPackages(t, append(slices.Clone(module), plantPackage(t, module, p, files)))
			for _, f := range c.findings() {
				t.Errorf("a clean file was refused: %s", f)
			}
		})
	}
}

// plantPackage type-checks synthetic files as one package beside the real
// module. Imports of module packages resolve to the packages already checked.
func plantPackage(t *testing.T, module []typedPackage, pkgPath string, files map[string]string) typedPackage {
	t.Helper()
	fset := gatekit.SourceFileSet()
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	var parsed []*ast.File
	for _, name := range names {
		f, err := parser.ParseFile(fset, strings.TrimPrefix(pkgPath, modulePath+"/")+"/"+name, files[name], 0)
		if err != nil {
			t.Fatalf("parsing the planted file: %v", err)
		}
		parsed = append(parsed, f)
	}
	info := newTypedInfo()
	conf := types.Config{Importer: plantImporter{module: module, std: importer.ForCompiler(fset, "source", nil)}}
	checked, err := conf.Check(pkgPath, fset, parsed, info)
	if err != nil {
		t.Fatalf("type-checking the planted package %s: %v", pkgPath, err)
	}
	return typedPackage{path: pkgPath, files: parsed, info: info, types: checked}
}

type plantImporter struct {
	module []typedPackage
	std    types.Importer
}

func (p plantImporter) Import(importPath string) (*types.Package, error) {
	for _, pkg := range p.module {
		if pkg.path == importPath {
			return pkg.types, nil
		}
	}
	// A standard package a module package already imported is that one, so a
	// planted type that names context.Context is identical to the module's.
	for _, pkg := range p.module {
		for _, imported := range pkg.types.Imports() {
			if imported.Path() == importPath {
				return imported, nil
			}
		}
	}
	return p.std.Import(importPath)
}

func newChatPackages(t *testing.T, pkgs []typedPackage) *chatPackages {
	t.Helper()
	c := &chatPackages{
		pkgs: pkgs, fset: gatekit.SourceFileSet(),
		vocab: map[types.Object]bool{}, providers: map[string]string{}, ioFuncs: map[types.Object]bool{},
		direct: map[types.Object]bool{}, sends: map[types.Object]bool{}, writers: map[types.Object]bool{}, hosts: map[string]string{},
	}
	c.deriveVocabulary()
	c.deriveProviders()
	c.deriveIO()
	c.deriveWriters()
	c.deriveTables(t)
	c.checkCensus(t)
	return c
}

func (c *chatPackages) pkgAt(dir string) *typedPackage {
	for i := range c.pkgs {
		if pkgDir(c.pkgs[i]) == dir {
			return &c.pkgs[i]
		}
	}
	return nil
}

// deriveVocabulary reads the exported types the connector declares in its chat
// files, and the channel identity every chat counterparty carries.
func (c *chatPackages) deriveVocabulary() {
	p := c.pkgAt(connectorDir)
	if p == nil {
		return
	}
	if obj := p.types.Scope().Lookup("ChannelIdentity"); obj != nil {
		c.vocab[obj] = true
	}
	for _, name := range p.types.Scope().Names() {
		obj := p.types.Scope().Lookup(name)
		if _, ok := obj.(*types.TypeName); ok && obj.Exported() &&
			strings.HasPrefix(path.Base(c.fset.Position(obj.Pos()).Filename), "chat") {
			c.vocab[obj] = true
		}
	}
}

// deriveProviders finds the capture subpackages that declare a Provider constant.
func (c *chatPackages) deriveProviders() {
	for _, p := range c.pkgs {
		rest, ok := strings.CutPrefix(pkgDir(p), captureDir+"/")
		if !ok || strings.Contains(rest, "/") {
			continue
		}
		if k, ok := p.types.Scope().Lookup("Provider").(*types.Const); ok && k.Val().Kind() == constant.String {
			c.providers[pkgDir(p)] = constant.StringVal(k.Val())
		}
	}
}

// isNetHTTPCall reports a reference to a net/http function that sends a request.
func isNetHTTPCall(fn *types.Func) bool {
	if fn.Pkg() == nil || fn.Pkg().Path() != "net/http" {
		return false
	}
	switch fn.Name() {
	case "Do", "Get", "Post", "PostForm", "Head", "RoundTrip":
		return true
	}
	return false
}

func funcOf(obj types.Object) *types.Func {
	fn, ok := obj.(*types.Func)
	if !ok {
		return nil
	}
	return fn.Origin()
}

func recvNamed(fn *types.Func) *types.Named {
	recv := fn.Type().(*types.Signature).Recv()
	if recv == nil {
		return nil
	}
	t := recv.Type()
	if ptr, ok := t.(*types.Pointer); ok {
		t = ptr.Elem()
	}
	n, _ := t.(*types.Named)
	return n
}

// deriveIO computes, across every loaded package, the functions that reach
// net/http (ioFuncs). A provider that sends through a shared helper still counts.
// In each provider package, direct are the functions that call net/http or an
// outside function that reaches it. Sends are the Send* functions among ioFuncs.
// An interface method counts when a method of that name in its package does.
func (c *chatPackages) deriveIO() {
	refs := map[types.Object]map[*types.Func]bool{}
	names := map[string][]types.Object{}
	for _, p := range c.pkgs {
		for _, f := range p.files {
			for _, d := range f.Decls {
				fd, ok := d.(*ast.FuncDecl)
				if !ok || fd.Body == nil {
					continue
				}
				self := p.info.Defs[fd.Name]
				refs[self] = map[*types.Func]bool{}
				names[p.path+"."+fd.Name.Name] = append(names[p.path+"."+fd.Name.Name], self)
				ast.Inspect(fd.Body, func(n ast.Node) bool {
					if id, ok := n.(*ast.Ident); ok {
						if fn := funcOf(p.info.Uses[id]); fn != nil {
							refs[self][fn] = true
						}
					}
					return true
				})
			}
		}
	}
	for self, used := range refs {
		for fn := range used {
			if isNetHTTPCall(fn) {
				c.ioFuncs[self] = true
			}
		}
	}
	for changed := true; changed; {
		changed = false
		for self, used := range refs {
			for fn := range used {
				if !c.ioFuncs[self] && c.reachesIO(fn, names) {
					c.ioFuncs[self] = true
					changed = true
				}
			}
		}
	}
	for dir := range c.providers {
		p := c.pkgAt(dir)
		for self, used := range refs {
			if self.Pkg() != p.types {
				continue
			}
			for fn := range used {
				if isNetHTTPCall(fn) || (fn.Pkg() != p.types && c.reachesIO(fn, names)) {
					c.direct[self] = true
				}
			}
			if c.ioFuncs[self] && strings.HasPrefix(strings.ToLower(self.Name()), "send") && !isMessageSenderFunc(self.(*types.Func)) {
				c.sends[self] = true
			}
		}
		c.markInterfaceSends(p, names)
	}
}

func (c *chatPackages) reachesIO(fn *types.Func, names map[string][]types.Object) bool {
	if c.ioFuncs[fn] {
		return true
	}
	n := recvNamed(fn)
	if n == nil || fn.Pkg() == nil {
		return false
	}
	if _, isIface := n.Underlying().(*types.Interface); !isIface {
		return false
	}
	return slices.ContainsFunc(names[fn.Pkg().Path()+"."+fn.Name()], func(o types.Object) bool { return c.ioFuncs[o] })
}

// markInterfaceSends adds the Send* methods of the package's interfaces whose
// concrete methods reach net/http.
func (c *chatPackages) markInterfaceSends(p *typedPackage, names map[string][]types.Object) {
	for _, name := range p.types.Scope().Names() {
		tn, ok := p.types.Scope().Lookup(name).(*types.TypeName)
		if !ok {
			continue
		}
		iface, ok := tn.Type().Underlying().(*types.Interface)
		if !ok {
			continue
		}
		for m := range iface.ExplicitMethods() {
			if strings.HasPrefix(strings.ToLower(m.Name()), "send") && !takesChannelMessage(m) &&
				slices.ContainsFunc(names[p.path+"."+m.Name()], func(o types.Object) bool { return c.ioFuncs[o] }) {
				c.sends[m] = true
			}
		}
	}
}

func takesChannelMessage(fn *types.Func) bool {
	params := fn.Type().(*types.Signature).Params()
	for param := range params.Variables() {
		if n, ok := param.Type().(*types.Named); ok && n.Obj().Name() == "ChannelMessage" {
			return true
		}
	}
	return false
}

// isMessageSenderFunc is a SendMessage method taking a ChannelMessage.
func isMessageSenderFunc(fn *types.Func) bool {
	return fn.Name() == "SendMessage" && recvNamed(fn) != nil && takesChannelMessage(fn)
}

// deriveWriters finds the write methods of the activities Store by type.
func (c *chatPackages) deriveWriters() {
	p := c.pkgAt(activitiesDir)
	if p == nil {
		return
	}
	store, ok := p.types.Scope().Lookup("Store").(*types.TypeName)
	if !ok {
		return
	}
	ms := types.NewMethodSet(types.NewPointer(store.Type()))
	for sel := range ms.Methods() {
		fn, ok := sel.Obj().(*types.Func)
		if ok && fn.Exported() && slices.ContainsFunc(activityWriterPrefixes, func(pre string) bool { return strings.HasPrefix(fn.Name(), pre) }) {
			c.writers[fn.Origin()] = true
		}
	}
}

// deriveTables builds the SQL pattern from the tables tableOwners gives the
// activities module.
func (c *chatPackages) deriveTables(t *testing.T) {
	t.Helper()
	var tables []string
	for table, owner := range tableOwners {
		if owner == activitiesDir {
			tables = append(tables, regexp.QuoteMeta(table))
		}
	}
	if len(tables) == 0 {
		t.Fatal("tableOwners gives the activities module no table, so the SQL rule would read nothing")
	}
	sort.Strings(tables)
	c.tables = regexp.MustCompile(`(?i)\b(insert\s+into|update|delete\s+from)\s+(` + strings.Join(tables, "|") + `)\b`)
}

// checkCensus fails when a derived set is smaller than a second source says.
func (c *chatPackages) checkCensus(t *testing.T) {
	t.Helper()
	if len(c.vocab) == 0 {
		t.Fatal("the connector declares no chat type, so no file would count as chat-speaking")
	}
	seeded := seededProviders(t)
	subdirs := c.captureSubdirs()
	want := 0
	for name := range seeded {
		if slices.ContainsFunc(subdirs, func(d string) bool { return strings.HasPrefix(name, d) }) {
			want++
		}
	}
	if want == 0 || len(c.providers) < want {
		t.Fatalf("found %d provider packages, but the migrations seed %d providers that have a capture package: %v", len(c.providers), want, c.providers)
	}
	for dir := range c.providers {
		found := false
		for o := range c.sends {
			found = found || (o.Pkg() != nil && o.Pkg().Path() == modulePath+"/"+dir)
		}
		if !found {
			t.Errorf("%s has no send function found: the derivation reads less than it holds", dir)
		}
	}
	if len(c.writers) == 0 {
		t.Fatal("the activities Store has no write method, so the store rule would read nothing")
	}
	c.hostsFromProviders()
	if len(c.hosts) < len(c.providers) {
		t.Fatalf("found %d provider hosts for %d providers", len(c.hosts), len(c.providers))
	}
	resolver := c.pkgAt(path.Dir(chatSendResolver))
	if resolver == nil || !slices.ContainsFunc(resolver.files, func(f *ast.File) bool { return c.file(f) == chatSendResolver }) {
		t.Fatalf("%s is not in the module: the send path moved", chatSendResolver)
	}
}

func (c *chatPackages) captureSubdirs() []string {
	var out []string
	for _, p := range c.pkgs {
		if rest, ok := strings.CutPrefix(pkgDir(p), captureDir+"/"); ok && !strings.Contains(rest, "/") {
			out = append(out, rest)
		}
	}
	return out
}

func seededProviders(t *testing.T) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	err := gatekit.WalkDir("migrations/core", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".up.sql") {
			return err
		}
		for _, stmt := range channelProviderSeed.FindAllString(readGateFile(t, p), -1) {
			for _, m := range seedQuotedName.FindAllStringSubmatch(stmt, -1) {
				out[m[1]] = true
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("reading the channel_provider seeds: %v", err)
	}
	return out
}

func (c *chatPackages) hostsFromProviders() {
	for dir := range c.providers {
		p := c.pkgAt(dir)
		for _, name := range p.types.Scope().Names() {
			k, ok := p.types.Scope().Lookup(name).(*types.Const)
			if !ok || k.Val().Kind() != constant.String {
				continue
			}
			if host := providerHost.FindString(constant.StringVal(k.Val())); host != "" {
				c.hosts[strings.TrimPrefix(host, "https://")] = dir
			}
		}
	}
}

// speaksChat reports whether a file uses chat vocabulary or a provider package.
func (c *chatPackages) speaksChat(p typedPackage, f *ast.File) bool {
	if _, ok := c.providers[pkgDir(p)]; ok {
		return true
	}
	found := false
	ast.Inspect(f, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok {
			if obj := p.info.Uses[id]; obj != nil && (c.vocab[obj] || c.providerObject(obj)) {
				found = true
			}
		}
		return !found
	})
	return found
}

func (c *chatPackages) providerObject(obj types.Object) bool {
	if obj.Pkg() == nil {
		return false
	}
	_, ok := c.providers[strings.TrimPrefix(obj.Pkg().Path(), modulePath+"/")]
	return ok
}

func (c *chatPackages) findings() []string {
	var out []string
	for _, p := range c.pkgs {
		for _, f := range p.files {
			if !c.speaksChat(p, f) {
				continue
			}
			out = append(out, c.recordFindings(p, f)...)
			out = append(out, c.activityFindings(p, f)...)
			out = append(out, c.sendFindings(p, f)...)
		}
		out = append(out, c.resolverFindings(p)...)
	}
	sort.Strings(out)
	return slices.Compact(out)
}

func isNormalizedRecord(p typedPackage, e ast.Expr) bool {
	tv, ok := p.info.Types[e]
	if !ok {
		return false
	}
	n, ok := types.Unalias(tv.Type).(*types.Named)
	return ok && n.Obj().Name() == "NormalizedRecord" && n.Obj().Pkg() != nil && n.Obj().Pkg().Path() == modulePath+"/"+connectorDir
}

// isSinkUpsert reports a reference to Upsert of the connector's Sink or the capture Sink.
func isSinkUpsert(fn *types.Func) bool {
	if fn.Name() != "Upsert" {
		return false
	}
	n := recvNamed(fn)
	if n == nil || n.Obj().Pkg() == nil || n.Obj().Name() != "Sink" {
		return false
	}
	pkg := strings.TrimPrefix(n.Obj().Pkg().Path(), modulePath+"/")
	return pkg == connectorDir || pkg == captureDir
}

func (c *chatPackages) recordFindings(p typedPackage, f *ast.File) []string {
	name := c.file(f)
	if name == chatRecordOwner || name == sdkIngressMapping || pkgDir(p) == connectorDir {
		return nil
	}
	var out []string
	ast.Inspect(f, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.CompositeLit:
			if isNormalizedRecord(p, x) {
				out = append(out, name+": constructs a NormalizedRecord for a chat message; file it through capture.FileChatMessage")
			}
		case *ast.Ident:
			if fn := funcOf(p.info.Uses[x]); fn != nil && isSinkUpsert(fn) {
				out = append(out, name+": references Sink.Upsert for a chat message; file it through capture.FileChatMessage")
			}
		}
		return true
	})
	return out
}

func (c *chatPackages) activityFindings(p typedPackage, f *ast.File) []string {
	dir := pkgDir(p)
	if dir == captureDir || dir == activitiesDir || dir == connectorDir {
		return nil
	}
	name := c.file(f)
	var out []string
	ast.Inspect(f, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok {
			if fn := funcOf(p.info.Uses[id]); fn != nil && c.writers[fn] {
				out = append(out, name+": references "+fn.Name()+", a write method of the activities store, for a chat message")
			}
		}
		return true
	})
	for _, lit := range chatLiterals(p, f) {
		if c.tables.MatchString(lit) {
			out = append(out, name+": writes the table of an activity row for a chat message; capture owns those rows")
		}
	}
	return out
}

func (c *chatPackages) sendFindings(p typedPackage, f *ast.File) []string {
	dir := pkgDir(p)
	_, inProvider := c.providers[dir]
	name := c.file(f)
	importsProvider := false
	for _, imp := range f.Imports {
		if ip, err := strconv.Unquote(imp.Path.Value); err == nil {
			_, ok := c.providers[strings.TrimPrefix(ip, modulePath+"/")]
			importsProvider = importsProvider || ok
		}
	}
	clientFile := inProvider && c.holdsDirectIO(p, f)
	var out []string
	for _, d := range f.Decls {
		fd, ok := d.(*ast.FuncDecl)
		if !ok || fd.Body == nil {
			continue
		}
		self, _ := p.info.Defs[fd.Name].(*types.Func)
		sender := self != nil && isMessageSenderFunc(self)
		ast.Inspect(fd.Body, func(n ast.Node) bool {
			id, ok := n.(*ast.Ident)
			if !ok {
				return true
			}
			obj := p.info.Uses[id]
			fn := funcOf(obj)
			switch {
			case fn != nil && inProvider && c.providerObject(fn) && (c.direct[fn] || c.sends[fn]):
				if !sender && !clientFile {
					out = append(out, name+": reaches "+fn.Name()+", which sends to the provider, outside its MessageSender and its client files")
				}
			case fn != nil && !inProvider && c.sends[fn] && (importsProvider || fn.Name() != "SendMessage"):
				out = append(out, name+": reaches the provider send method "+fn.Name()+" outside a connector.MessageSender; send through the sender compose/commschannel.go resolves")
			}
			if k, ok := obj.(*types.Const); ok && !inProvider && c.providerObject(k) && k.Val().Kind() == constant.String && providerHost.MatchString(constant.StringVal(k.Val())) {
				out = append(out, name+": uses the provider host constant "+k.Name()+" outside its package")
			}
			return true
		})
	}
	if !inProvider {
		for _, lit := range chatLiterals(p, f) {
			for host, owner := range c.hosts {
				if strings.Contains(lit, host) {
					out = append(out, name+": names the provider host "+host+", which only "+owner+" calls")
				}
			}
		}
	}
	return out
}

// holdsDirectIO reports whether the file declares a function that calls net/http
// itself or a send function. Such a file is the provider's client code.
func (c *chatPackages) holdsDirectIO(p typedPackage, f *ast.File) bool {
	for _, d := range f.Decls {
		if fd, ok := d.(*ast.FuncDecl); ok && (c.direct[p.info.Defs[fd.Name]] || c.sends[p.info.Defs[fd.Name]]) {
			return true
		}
	}
	return false
}

func chatLiterals(p typedPackage, f *ast.File) []string {
	var out []string
	ast.Inspect(f, func(n ast.Node) bool {
		if e, ok := n.(ast.Expr); ok {
			if tv, ok := p.info.Types[e]; ok && tv.Value != nil && tv.Value.Kind() == constant.String {
				out = append(out, constant.StringVal(tv.Value))
			}
		}
		return true
	})
	return out
}

// senderInterface is the connector's MessageSender interface.
func (c *chatPackages) senderInterface() *types.Interface {
	p := c.pkgAt(connectorDir)
	if p == nil {
		return nil
	}
	obj, ok := p.types.Scope().Lookup("MessageSender").(*types.TypeName)
	if !ok {
		return nil
	}
	iface, _ := obj.Type().Underlying().(*types.Interface)
	return iface
}

// isSenderType reports a type that is a connector.MessageSender or implements it,
// itself or through a pointer.
func (c *chatPackages) isSenderType(t types.Type) bool {
	iface := c.senderInterface()
	if iface == nil || t == nil {
		return false
	}
	return types.Implements(t, iface) || types.Implements(types.NewPointer(t), iface)
}

// isConcreteSender reports a built value of a type that implements MessageSender.
// An interface value is a sender somebody resolved, which the other rule judges.
func (c *chatPackages) isConcreteSender(t types.Type) bool {
	if tuple, ok := t.(*types.Tuple); ok {
		if tuple.Len() == 0 {
			return false
		}
		t = tuple.At(0).Type()
	}
	return t != nil && !types.IsInterface(t) && c.isSenderType(t)
}

// resolverFindings fails a second place that resolves or builds a
// connector.MessageSender. It judges by type, so a constructor of any name, a
// struct literal and a factory all count. A function that returns a sender is a
// second resolver, and so is an expression that builds one. Neither counts in
// the resolver, a listed seam or a provider package. A call to a function the
// resolver file declares is the resolver answering.
func (c *chatPackages) resolverFindings(p typedPackage) []string {
	if _, provider := c.providers[pkgDir(p)]; provider {
		return nil
	}
	var out []string
	for _, f := range p.files {
		name := c.file(f)
		if name == senderResolver || isSenderResolveSeam(name) {
			continue
		}
		for _, d := range f.Decls {
			if fd, ok := d.(*ast.FuncDecl); ok && c.returnsSender(p, fd) {
				out = append(out, name+": "+fd.Name.Name+" resolves a connector.MessageSender; resolve it through capture.ChatSenders.For")
			}
		}
		ast.Inspect(f, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.CompositeLit:
				if c.isConcreteSender(p.info.Types[x].Type) {
					out = append(out, name+": a literal builds a provider's sender, which only capture.ChatSenders may do")
				}
			case *ast.CallExpr:
				if c.isConcreteSender(p.info.Types[x].Type) && !c.callsResolverFile(p, x) {
					out = append(out, name+": a call builds a provider's sender, which only capture.ChatSenders may do")
				}
			}
			return true
		})
	}
	return out
}

// callsResolverFile reports a call to a function the resolver file declares.
func (c *chatPackages) callsResolverFile(p typedPackage, call *ast.CallExpr) bool {
	var id *ast.Ident
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		id = fun
	case *ast.SelectorExpr:
		id = fun.Sel
	}
	if id == nil {
		return false
	}
	fn := funcOf(p.info.Uses[id])
	return fn != nil && c.fset.Position(fn.Pos()).Filename == senderResolver
}

func (c *chatPackages) returnsSender(p typedPackage, fd *ast.FuncDecl) bool {
	obj, ok := p.info.Defs[fd.Name].(*types.Func)
	if !ok {
		return false
	}
	results := obj.Type().(*types.Signature).Results()
	for i := 0; i < results.Len(); i++ {
		if c.isSenderType(results.At(i).Type()) {
			return true
		}
	}
	return false
}
