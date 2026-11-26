// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Every chat network receives, files, changes and sends through the same core
// functions. A later feature, such as a bot reply, then works once for all of them.
//
// Three places can fork that path, and each is derived from the tree:
//
//   - A chat-speaking file builds a connector.NormalizedRecord or calls
//     Sink.Upsert. capture.FileChatMessage maps a chat message to a record, and
//     compose/extingress.go routes the unit ingress through it.
//   - A chat-speaking file writes the activities table or calls a write method of
//     the activities store, outside capture and the activities module.
//   - A call reaches a provider's send API outside its connector.MessageSender,
//     or a file outside the provider package names the provider's host.
//
// The vocabulary is read off connector/chatmessage.go and chatchange.go. The
// provider packages are the capture subpackages that use it, and their Send*
// methods are the send API. A network added tomorrow enrols itself.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path"
	"path/filepath"
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
	chatVocabularyDir = "internal/shared/ports/connector/"
	chatProviderRoot  = "internal/modules/capture/"
	chatWritersDir    = "internal/modules/capture/"
	activitiesDir     = "internal/modules/activities/"
	chatSendResolver  = "internal/compose/commschannel.go"
	backendImportBase = "github.com/margince/margince/backend/"
)

var activitiesTableWrite = regexp.MustCompile(`(?i)\b(insert\s+into|update|delete\s+from)\s+activities\b`)

var providerHost = regexp.MustCompile(`^https://[A-Za-z0-9.-]+`)

var activityWriterPrefixes = []string{"Create", "Insert", "Update", "Delete", "Archive", "Edit", "Remove", "Set"}

// chatSource is one parsed non-test Go file, named by its module-relative path.
type chatSource struct {
	path string
	file *ast.File
}

func (s chatSource) dir() string { return path.Dir(s.path) + "/" }

func TestChatNetworksShareOneReceiveFileChangeAndSendPath(t *testing.T) {
	t.Parallel()
	corpus := chatCorpus(t)
	for _, f := range chatOnePathFindings(t, corpus) {
		t.Error(f)
	}
}

// TestChatOnePathGateFailsOnAForkedPath plants one violation per rule in a
// synthetic file and runs the real census over the tree plus that file. A rule
// that reads a smaller tree than it should passes the real run and fails here.
func TestChatOnePathGateFailsOnAForkedPath(t *testing.T) {
	t.Parallel()
	corpus := chatCorpus(t)
	const connectorImport = "import \"github.com/margince/margince/backend/internal/shared/ports/connector\"\n"
	cases := []struct {
		name, at, src, want string
	}{
		{
			name: "a record built beside FileChatMessage", at: "internal/compose/forkrecord.go", want: "NormalizedRecord",
			src: "package compose\n" + connectorImport + "var _ connector.ChatBinding\nvar _ = connector.NormalizedRecord{}\n",
		},
		{
			name: "a chat message upserted straight into the sink", at: "internal/compose/forkupsert.go", want: "Upsert",
			src: "package compose\n" + connectorImport + "func f(s connector.Sink, m connector.ChatMessage) { s.Upsert(nil, m) }\n",
		},
		{
			name: "an activities insert for a chat message", at: "internal/compose/forkactivity.go", want: "activities",
			src: "package compose\n" + connectorImport + "var _ connector.ChatMessage\nconst q = `INSERT INTO activities (id) VALUES (1)`\n",
		},
		{
			name: "an activities store write for a chat change", at: "internal/compose/forkstorewrite.go", want: "activities store",
			src: "package compose\n" + connectorImport + "var _ connector.ChatChange\nfunc f(s interface{ UpdateActivity() }) { s.UpdateActivity() }\n",
		},
		{
			name: "a provider's SendText called by a compose file", at: "internal/compose/forksend.go", want: "SendText",
			src: "package compose\nfunc f(c interface{ SendText() }) { c.SendText() }\n",
		},
		{
			name: "a provider's SendMessage called after importing its package", at: "internal/compose/forktelegram.go", want: "SendMessage",
			src: "package compose\nimport \"github.com/margince/margince/backend/internal/modules/capture/telegram\"\n" +
				"var _ telegram.Bot\nfunc f(c interface{ SendMessage() }) { c.SendMessage() }\n",
		},
		{
			name: "a send inside a provider package but outside its MessageSender", at: "internal/modules/capture/whatsapp/forksend.go", want: "SendText",
			src: "package whatsapp\nfunc f(c *Client) { c.SendText(nil, \"\", \"\", \"\", \"\") }\n",
		},
		{
			name: "a provider host named outside its package", at: "internal/compose/forkhost.go", want: "graph.facebook.com",
			src: "package compose\nconst u = \"https://graph.facebook.com/v1/messages\"\n",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			planted, err := parser.ParseFile(token.NewFileSet(), c.at, c.src, 0)
			if err != nil {
				t.Fatalf("parsing the planted file: %v", err)
			}
			got := chatOnePathFindings(t, append(slices.Clone(corpus), chatSource{path: c.at, file: planted}))
			if !slices.ContainsFunc(got, func(f string) bool {
				return strings.HasPrefix(f, c.at) && strings.Contains(f, c.want)
			}) {
				t.Errorf("planting %s found nothing naming %q; findings: %v", c.at, c.want, got)
			}
		})
	}
	t.Run("a file that files through FileChatMessage is clean", func(t *testing.T) {
		t.Parallel()
		at := "internal/compose/cleanadapter.go"
		src := "package compose\nimport (\n\"github.com/margince/margince/backend/internal/modules/capture\"\n" +
			"\"github.com/margince/margince/backend/internal/shared/ports/connector\"\n)\n" +
			"func f(s connector.Sink, b connector.ChatBinding, m connector.ChatMessage) { capture.FileChatMessage(nil, s, b, m) }\n"
		planted, err := parser.ParseFile(token.NewFileSet(), at, src, 0)
		if err != nil {
			t.Fatalf("parsing the clean file: %v", err)
		}
		for _, f := range chatOnePathFindings(t, append(slices.Clone(corpus), chatSource{path: at, file: planted})) {
			t.Errorf("a file that uses the shared entry was refused: %s", f)
		}
	})
}

// chatCorpus parses every hand-written non-test Go file under internal/.
func chatCorpus(t *testing.T) []chatSource {
	t.Helper()
	var corpus []chatSource
	for _, p := range handWrittenGoSources(t) {
		slash := filepath.ToSlash(p)
		if strings.HasSuffix(slash, "_test.go") || !strings.HasPrefix(slash, "internal/") {
			continue
		}
		file, err := gatekit.ParseFile(p, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", slash, err)
		}
		corpus = append(corpus, chatSource{path: slash, file: file})
	}
	return corpus
}

// chatOnePathFindings runs the three rules over a corpus. It fails the test when
// a derived set comes up short. A short set reads less of the tree and reports a
// pass.
func chatOnePathFindings(t *testing.T, corpus []chatSource) []string {
	t.Helper()
	vocabulary := chatVocabulary(corpus)
	providers := chatProviderDirs(corpus, vocabulary)
	sends := providerSendNames(corpus, providers)
	hosts := providerHosts(corpus, providers)
	writers := activityStoreWriters(corpus)
	for _, floor := range []struct {
		what string
		got  int
		min  int
	}{
		{"chat vocabulary types", len(vocabulary), 6},
		{"provider packages", len(providers), 3},
		{"provider send methods", len(sends), 3},
		{"provider hosts", len(hosts), 3},
		{"activities store write methods", len(writers), 2},
	} {
		if floor.got < floor.min {
			t.Fatalf("found %d %s, fewer than the %d the tree holds: the derivation has stopped reading it", floor.got, floor.what, floor.min)
		}
	}
	if !slices.ContainsFunc(corpus, func(s chatSource) bool { return s.path == chatSendResolver }) {
		t.Fatalf("%s is not in the corpus: the send path has moved, and this gate names where sends resolve", chatSendResolver)
	}
	var findings []string
	for _, src := range corpus {
		findings = append(findings, chatRecordFindings(src, vocabulary)...)
		findings = append(findings, chatActivityFindings(src, vocabulary, writers)...)
		findings = append(findings, chatSendFindings(src, providers, sends, hosts)...)
	}
	sort.Strings(findings)
	return findings
}

// chatVocabulary is the exported type names of the connector's chat files, plus
// the channel identity every chat counterparty carries.
func chatVocabulary(corpus []chatSource) map[string]bool {
	names := map[string]bool{"ChannelIdentity": true}
	for _, src := range corpus {
		base := path.Base(src.path)
		if src.dir() != chatVocabularyDir || (base != "chatmessage.go" && base != "chatchange.go") {
			continue
		}
		for _, decl := range src.file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok {
				continue
			}
			for _, spec := range gen.Specs {
				if ts, ok := spec.(*ast.TypeSpec); ok && ts.Name.IsExported() {
					names[ts.Name.Name] = true
				}
			}
		}
	}
	return names
}

// speaksChat reports whether a file names any chat vocabulary.
func speaksChat(file *ast.File, vocabulary map[string]bool) bool {
	found := false
	ast.Inspect(file, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok && vocabulary[id.Name] {
			found = true
		}
		return !found
	})
	return found
}

// chatProviderDirs are the capture subpackages that speak the chat vocabulary,
// the places a network's own code lives.
func chatProviderDirs(corpus []chatSource, vocabulary map[string]bool) map[string]bool {
	dirs := map[string]bool{}
	for _, src := range corpus {
		rest, ok := strings.CutPrefix(src.dir(), chatProviderRoot)
		if !ok || rest == "" || strings.Contains(strings.TrimSuffix(rest, "/"), "/") {
			continue
		}
		if speaksChat(src.file, vocabulary) {
			dirs[src.dir()] = true
		}
	}
	return dirs
}

// isMessageSenderImpl is the SendMessage method taking a connector.ChannelMessage,
// the one place a provider may transmit.
func isMessageSenderImpl(fn *ast.FuncDecl) bool {
	return fn.Recv != nil && fn.Name.Name == "SendMessage" && takesChannelMessage(fn.Type)
}

func takesChannelMessage(ft *ast.FuncType) bool {
	for _, p := range ft.Params.List {
		if sel, ok := p.Type.(*ast.SelectorExpr); ok && sel.Sel.Name == "ChannelMessage" {
			return true
		}
	}
	return false
}

// providerSendNames are the Send* methods and interface methods the provider
// packages declare, apart from the MessageSender implementations themselves.
func providerSendNames(corpus []chatSource, providers map[string]bool) map[string]bool {
	names := map[string]bool{}
	for _, src := range corpus {
		if !providers[src.dir()] {
			continue
		}
		ast.Inspect(src.file, func(n ast.Node) bool {
			switch d := n.(type) {
			case *ast.FuncDecl:
				if d.Recv != nil && strings.HasPrefix(d.Name.Name, "Send") && !isMessageSenderImpl(d) {
					names[d.Name.Name] = true
				}
			case *ast.InterfaceType:
				for _, m := range d.Methods.List {
					ft, ok := m.Type.(*ast.FuncType)
					if !ok || len(m.Names) == 0 || !strings.HasPrefix(m.Names[0].Name, "Send") || takesChannelMessage(ft) {
						continue
					}
					names[m.Names[0].Name] = true
				}
			}
			return true
		})
	}
	return names
}

// providerHosts are the https hosts the provider packages name as their API.
func providerHosts(corpus []chatSource, providers map[string]bool) map[string]string {
	hosts := map[string]string{}
	for _, src := range corpus {
		if !providers[src.dir()] {
			continue
		}
		for _, lit := range chatStringLiterals(src.file) {
			if host := providerHost.FindString(lit); host != "" {
				hosts[strings.TrimPrefix(host, "https://")] = src.dir()
			}
		}
	}
	return hosts
}

func chatStringLiterals(file *ast.File) []string {
	var out []string
	ast.Inspect(file, func(n ast.Node) bool {
		if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING {
			if v, err := strconv.Unquote(lit.Value); err == nil {
				out = append(out, v)
			}
		}
		return true
	})
	return out
}

// activityStoreWriters are the exported write methods of the activities Store.
func activityStoreWriters(corpus []chatSource) map[string]bool {
	names := map[string]bool{}
	for _, src := range corpus {
		if src.dir() != activitiesDir {
			continue
		}
		for _, decl := range src.file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || !fn.Name.IsExported() || !chatReceiverIs(fn, "Store") {
				continue
			}
			if slices.ContainsFunc(activityWriterPrefixes, func(p string) bool { return strings.HasPrefix(fn.Name.Name, p) }) {
				names[fn.Name.Name] = true
			}
		}
	}
	return names
}

func chatReceiverIs(fn *ast.FuncDecl, name string) bool {
	typ := fn.Recv.List[0].Type
	if star, ok := typ.(*ast.StarExpr); ok {
		typ = star.X
	}
	id, ok := typ.(*ast.Ident)
	return ok && id.Name == name
}

// chatRecordFindings is rule one: a chat-speaking file outside the two shared
// owners builds a record or upserts.
func chatRecordFindings(src chatSource, vocabulary map[string]bool) []string {
	if src.path == chatRecordOwner || src.path == sdkIngressMapping || src.dir() == chatVocabularyDir || !speaksChat(src.file, vocabulary) {
		return nil
	}
	var out []string
	ast.Inspect(src.file, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.CompositeLit:
			if chatTypeName(x.Type) == "NormalizedRecord" {
				out = append(out, src.path+": builds a NormalizedRecord for a chat message; file it through capture.FileChatMessage")
			}
		case *ast.CallExpr:
			if sel, ok := x.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Upsert" {
				out = append(out, src.path+": calls Sink.Upsert for a chat message; file it through capture.FileChatMessage")
			}
		}
		return true
	})
	return out
}

func chatTypeName(e ast.Expr) string {
	switch x := e.(type) {
	case *ast.Ident:
		return x.Name
	case *ast.SelectorExpr:
		return x.Sel.Name
	}
	return ""
}

// chatActivityFindings is rule two: a chat-speaking file outside capture and the
// activities module writes activity rows itself.
func chatActivityFindings(src chatSource, vocabulary, writers map[string]bool) []string {
	if src.dir() == chatWritersDir || src.dir() == activitiesDir || src.dir() == chatVocabularyDir || !speaksChat(src.file, vocabulary) {
		return nil
	}
	var out []string
	for _, lit := range chatStringLiterals(src.file) {
		if activitiesTableWrite.MatchString(lit) {
			out = append(out, src.path+": writes the activities table for a chat message; capture.FileChatMessage and ChangeChatMessage own those rows")
		}
	}
	ast.Inspect(src.file, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok && writers[sel.Sel.Name] {
				out = append(out, src.path+": calls "+sel.Sel.Name+", a write of the activities store, for a chat message")
			}
		}
		return true
	})
	return out
}

// chatSendFindings is rule three: a provider's send API is reached only from
// its own MessageSender, and no file outside the provider package names its host.
func chatSendFindings(src chatSource, providers, sends map[string]bool, hosts map[string]string) []string {
	var out []string
	inProvider := providers[src.dir()]
	importsProvider := false
	for _, imp := range src.file.Imports {
		if p, err := strconv.Unquote(imp.Path.Value); err == nil && providers[strings.TrimPrefix(p, backendImportBase)+"/"] {
			importsProvider = true
		}
	}
	for _, decl := range src.file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		allowed := inProvider && isMessageSenderImpl(fn)
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok || !sends[sel.Sel.Name] || allowed {
				return true
			}
			// SendMessage is also the MessageSender method every sender is called by,
			// so outside a provider package it counts only when the file imports one.
			if sel.Sel.Name == "SendMessage" && !inProvider && !importsProvider {
				return true
			}
			out = append(out, src.path+": reaches the provider send method "+sel.Sel.Name+
				" outside a connector.MessageSender; send through the sender compose/commschannel.go resolves")
			return true
		})
	}
	if !inProvider {
		for _, lit := range chatStringLiterals(src.file) {
			for host, owner := range hosts {
				if strings.Contains(lit, host) {
					out = append(out, src.path+": names the provider host "+host+", which only "+owner+" calls")
				}
			}
		}
	}
	return out
}
