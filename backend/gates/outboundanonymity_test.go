// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Every outbound HTTP request either says who is calling, or is registered as
// deliberately anonymous with the reason.
//
// The sibling gate next door (TestNoOutboundIdentityIsWrittenAtItsCallSite)
// holds the calls that DECLARE something to one spelling. It has nothing to say
// about a call that declares nothing, and it cannot: a gate over the source can
// see a wrong header and not an absent one — unless it goes looking, which is
// what this does.
//
// A request with no User-Agent is not anonymous. Go supplies
// `Go-http-client/1.1`, which names the language runtime and nothing about who
// is calling or why, and it is what 23 live paths in this tree were sending. A
// remote operator diagnosing that traffic can block a language or nothing.
//
// NOT every call needs a name, and the register is where that is said rather
// than assumed. A token-authenticated provider API already knows exactly who is
// calling — the credential says so — and a request to this product's own origin
// is talking to itself. What the register is for is the DIFFERENCE between a
// call somebody decided needs no identity and one nobody thought about, which
// is invisible in the code either way.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// anonymousOutbound registers the request builders that carry no identity, and
// why each is right to. Every entry is one FUNCTION, keyed `path:func name`,
// because a file's waived builder must not vouch for an anonymous sibling
// written beside it later.
var anonymousOutbound = gatekit.Waive(map[string]string{
	// Past this module. Neither can import internal/platform/outbound — they
	// are separate Go modules — and neither needs to.
	"../extensions/openchannel/client.go:func post": "posts a document signed with a secret the RECEIVER issued, over a nonce and a timestamp they check; the signature names the sender to them more exactly than an agent could, and to anyone else the request is unverifiable whatever it claims",
	"../cli/craft/gate/anthropic.go:func Complete":  "carries the developer's own x-api-key, which is the identity that provider bills and throttles — the same ground as the model providers below",

	// This product's own origin, and the harnesses that drive it.
	"internal/modules/agents/apps/fetch.go:func Fetch":         "fetches this product's own origin, so the server on the other end is this same process and a name would be it introducing itself to itself",
	"internal/compose/integration/apptest/appenv.go:func Call": "drives a server the test itself started, in the same process tree, for the length of one test",
	"internal/compose/integration/apptest/mcp.go:func rpc":     "drives a server the test itself started, in the same process tree, for the length of one test",
	"tools/seed-demo/apiclient.go:func delete":                 "seeds a demo estate through this product's own API, run by hand against a deployment the operator chose",
	"tools/seed-demo/apiclient.go:func get":                    "seeds a demo estate through this product's own API, run by hand against a deployment the operator chose",
	"tools/seed-demo/apiclient.go:func patch":                  "seeds a demo estate through this product's own API, run by hand against a deployment the operator chose",
	"tools/seed-demo/apiclient.go:func patchGuarded":           "seeds a demo estate through this product's own API, run by hand against a deployment the operator chose",
	"tools/seed-demo/apiclient.go:func post":                   "seeds a demo estate through this product's own API, run by hand against a deployment the operator chose",
	"tools/seed-demo/apiclient.go:func put":                    "seeds a demo estate through this product's own API, run by hand against a deployment the operator chose",
	"tools/seed-demo/documents.go:func upload":                 "seeds a demo estate through this product's own API, run by hand against a deployment the operator chose",

	// The model providers. Each call carries the customer's own API key, which
	// is the account the provider bills, rate-limits and revokes; an agent
	// beside it names software the provider has no lever over.
	"internal/modules/ai/anthropic.go:func sendOnce": "carries the customer's own provider key, which is the identity that provider bills and throttles",
	"internal/modules/ai/gemini.go:func post":        "carries the customer's own provider key, which is the identity that provider bills and throttles",
	"internal/modules/ai/ollama.go:func post":        "reaches a model runner the operator runs themselves, on a host they configured — they already know what is calling it",
	"internal/modules/ai/openai.go:func postRaw":     "carries the customer's own provider key, which is the identity that provider bills and throttles",
	"internal/modules/ai/openaicompat.go:func post":  "carries the customer's own provider key, which is the identity that provider bills and throttles",

	// The capture connectors. Every one of these is an OAuth or token session
	// the person themselves granted, so the provider knows the grant, the app
	// it was granted to, and the account it was granted on.
	"internal/modules/capture/gmail/client.go:func Watch":           "runs inside an OAuth grant the person made to this app, which names the caller to the provider more precisely than an agent could",
	"internal/modules/capture/gmail/client.go:func get":             "runs inside an OAuth grant the person made to this app, which names the caller to the provider more precisely than an agent could",
	"internal/modules/capture/gmail/send.go:func postJSON":          "runs inside an OAuth grant the person made to this app, which names the caller to the provider more precisely than an agent could",
	"internal/modules/capture/googleconn/googleconn.go:func Get":    "runs inside an OAuth grant the person made to this app, which names the caller to the provider more precisely than an agent could",
	"internal/modules/capture/graph/client.go:func GetMIME":         "runs inside an OAuth grant the person made to this app, which names the caller to the provider more precisely than an agent could",
	"internal/modules/capture/graph/transport.go:func get":          "runs inside an OAuth grant the person made to this app, which names the caller to the provider more precisely than an agent could",
	"internal/modules/capture/oauthflow/oauthflow.go:func token":    "exchanges a code against a token endpoint using this app's registered client id, which is the identity that endpoint checks",
	"internal/modules/capture/telegram/api.go:func request":         "calls a bot API under the bot's own token, and the bot IS the identity there",
	"internal/modules/capture/telegram/sendfiles.go:func SendFiles": "calls a bot API under the bot's own token, and the bot IS the identity there",
})

func TestEveryOutboundRequestSaysWhoIsCallingOrRegistersWhyNot(t *testing.T) {
	t.Parallel()
	var findings []string
	builders := 0
	eachGoFileUnder(t, outboundSurfaceRoots, func(path string, file *ast.File) {
		if strings.HasSuffix(path, "_test.go") {
			// A test's outbound call goes to a server the test started.
			return
		}
		anonymous := anonymousRequestBuildersIn(file)
		builders += len(requestBuildersIn(file))
		if len(anonymous) == 0 {
			return
		}
		for _, fn := range anonymous {
			// Keyed on the FUNCTION, not the file. A file's waived builder must
			// not vouch for an anonymous sibling written beside it later, which
			// is the same reason the census judges per function in the first
			// place.
			if anonymousOutbound.Waived(t, path+":"+fn) {
				continue
			}
			findings = append(findings, path+": "+fn)
		}
	})
	if builders < outboundBuilderFloor {
		t.Fatalf("the walk found only %d outbound request builder(s), and this census is pinned at "+
			"%d — a walk that stopped reaching them reports a clean tree in the same words as a "+
			"tree with nothing left to fix", builders, outboundBuilderFloor)
	}
	anonymousOutbound.AssertAllMatched(t)
	if len(findings) > 0 {
		sort.Strings(findings)
		t.Errorf("%d function(s) build an outbound request that names nobody:\n\t%s\n\n"+
			"Go then sends `Go-http-client/1.1`, which names the language and not the caller, and "+
			"an operator reading it can act on a language or on nothing. Set the User-Agent from "+
			"an identity in internal/platform/outbound, or register the FUNCTION above — the "+
			"register keys on `path:func name`, so a file-shaped entry matches nothing.",
			len(findings), strings.Join(findings, "\n\t"))
	}
}

// outboundSurfaceRoots are the trees that can make an outbound call — the same
// set the sibling identity gate sweeps.
//
// Past this module on purpose. A request built in an extension, in the CLI or
// on the desktop leaves the same installation and reaches the same operator,
// and a census that stopped at the backend's edge would have claimed "every
// outbound request" over trees it never read.
var outboundSurfaceRoots = []string{
	".", "../extensions", "../cli", "../desktop", "../fixtures",
}

// outboundBuilderFloor is what the walk found when this census landed.
const outboundBuilderFloor = 30

// anonymousRequestBuildersIn names each function in the file that builds an
// outbound request and never sets a User-Agent on it.
//
// Judged per FUNCTION, because that is the unit a header is set in: a file with
// one named request and one anonymous one is a file with an anonymous request,
// and judging the file whole would let the first vouch for the second.
func anonymousRequestBuildersIn(file *ast.File) []string {
	var out []string
	client, dotImported := gatekit.ImportedAs(file, "net/http")
	if client == "" && !dotImported {
		return nil
	}
	for _, fn := range requestBuildersIn(file) {
		if setsUserAgent(fn, requestsBuiltIn(fn.Body, client)) {
			continue
		}
		out = append(out, "func "+fn.Name.Name)
	}
	return out
}

// requestBuildersIn names every function in the file that builds an outbound
// http.Request.
func requestBuildersIn(file *ast.File) []*ast.FuncDecl {
	// The file's OWN name for net/http, resolved rather than assumed. Matched
	// as the literal `http`, a file importing it under an alias had every
	// builder in it silently left out of the census — and the floor cannot
	// catch that, because a smaller count still clears it.
	client, dotImported := gatekit.ImportedAs(file, "net/http")
	if client == "" && !dotImported {
		return nil
	}
	var out []*ast.FuncDecl
	for _, decl := range file.Decls {
		fn, isFunc := decl.(*ast.FuncDecl)
		if !isFunc || fn.Body == nil {
			continue
		}
		if buildsARequest(fn.Body, client) {
			out = append(out, fn)
		}
	}
	return out
}

func buildsARequest(body *ast.BlockStmt, client string) bool {
	found := false
	ast.Inspect(body, func(node ast.Node) bool {
		if call, isCall := node.(*ast.CallExpr); isCall && isRequestConstructor(call, client) {
			found = true
		}
		return !found
	})
	return found
}

// setsUserAgent reports whether the function writes the caller's name onto the
// REQUEST IT BUILT.
//
// Three things have to line up, and each was a way to read a clean tree over an
// anonymous call.
//
// The WRITE, not the words: reading any call that mentions "User-Agent" made
// `log.Printf("User-Agent missing")` enough — a census passing because the code
// complains about the very thing it is doing.
//
// A HEADER, not any receiver: a `Set` on a map keyed "User-Agent" writes no
// header at all.
//
// And THIS request's header. A function can build an anonymous `req` and set
// the agent on something else it holds — a response, an outgoing second
// request, a header it is composing for a different call — and the request it
// actually sends still carries Go's default.
func setsUserAgent(fn *ast.FuncDecl, built map[string]bool) bool {
	found := false
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		call, isCall := node.(*ast.CallExpr)
		if !isCall || len(call.Args) < 2 {
			return true
		}
		method, isMethod := call.Fun.(*ast.SelectorExpr)
		if !isMethod || (method.Sel.Name != "Set" && method.Sel.Name != "Add") {
			return true
		}
		header, onHeader := method.X.(*ast.SelectorExpr)
		if !onHeader || header.Sel.Name != "Header" {
			return true
		}
		owner, named := header.X.(*ast.Ident)
		if !named || !built[owner.Name] {
			return true
		}
		name, isLit := call.Args[0].(*ast.BasicLit)
		if isLit && name.Kind == token.STRING && strings.EqualFold(gatekit.TextOf(name), "User-Agent") {
			found = true
		}
		return !found
	})
	return found
}

// requestsBuiltIn names the identifiers this function assigns an
// http.NewRequest* result to — the requests whose headers are its own.
func requestsBuiltIn(body *ast.BlockStmt, client string) map[string]bool {
	built := map[string]bool{}
	ast.Inspect(body, func(node ast.Node) bool {
		assign, isAssign := node.(*ast.AssignStmt)
		if !isAssign || len(assign.Rhs) != 1 || len(assign.Lhs) == 0 {
			return true
		}
		call, isCall := assign.Rhs[0].(*ast.CallExpr)
		if !isCall || !isRequestConstructor(call, client) {
			return true
		}
		if name, isIdent := assign.Lhs[0].(*ast.Ident); isIdent {
			built[name.Name] = true
		}
		return true
	})
	return built
}

// isRequestConstructor matches the ways net/http starts an outbound request,
// under whatever name the file imports it by.
func isRequestConstructor(call *ast.CallExpr, client string) bool {
	switch fn := call.Fun.(type) {
	case *ast.SelectorExpr:
		pkg, isIdent := fn.X.(*ast.Ident)
		return isIdent && pkg.Name == client && startsARequest(fn.Sel.Name)
	case *ast.Ident:
		// A dot import spells it unqualified.
		return client == "" && startsARequest(fn.Name)
	}
	return false
}

// startsARequest names the net/http functions that put a request on the wire.
//
// The CONVENIENCE forms are here beside NewRequest, and they are the sharper
// half: http.Get and its siblings build and send in one call, so there is no
// request to set a header on and the call is anonymous by construction. A
// census that only knew NewRequest would report a clean tree over one, and its
// floor could not catch that — a smaller count still clears it.
func startsARequest(name string) bool {
	switch {
	case strings.HasPrefix(name, "NewRequest"):
		return true
	case name == "Get", name == "Post", name == "PostForm", name == "Head":
		return true
	}
	return false
}

// The detector, from the other end. A census over an ABSENCE is the easiest
// kind to have stop working: it reports a clean tree by finding nothing, which
// is also what it does when it has stopped looking.
func TestTheAnonymityCensusSeesEachShapeARequestIsBuiltIn(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		source string
		want   int
	}{
		{
			name: "a request built and sent with no agent",
			source: "package p\nimport \"net/http\"\nfunc call() {\n" +
				"\treq, _ := http.NewRequest(\"GET\", \"https://x.test\", nil)\n\t_ = req\n}\n",
			want: 1,
		},
		{
			name: "the context form is the same request",
			source: "package p\nimport (\n\t\"context\"\n\t\"net/http\"\n)\nfunc call(ctx context.Context) {\n" +
				"\treq, _ := http.NewRequestWithContext(ctx, \"GET\", \"https://x.test\", nil)\n\t_ = req\n}\n",
			want: 1,
		},
		{
			name: "a request that names the caller",
			source: "package p\nimport \"net/http\"\nfunc call() {\n" +
				"\treq, _ := http.NewRequest(\"GET\", \"https://x.test\", nil)\n" +
				"\treq.Header.Set(\"User-Agent\", \"margince-x/1.0\")\n}\n",
			want: 0,
		},
		{
			// The header's canonical spelling is not the only one Go accepts,
			// and a census that insisted on it would report a named request.
			name: "the header is matched however it is cased",
			source: "package p\nimport \"net/http\"\nfunc call() {\n" +
				"\treq, _ := http.NewRequest(\"GET\", \"https://x.test\", nil)\n" +
				"\treq.Header.Set(\"user-agent\", \"margince-x/1.0\")\n}\n",
			want: 0,
		},
		{
			// The WORDS are not the write. A builder that complains about
			// anonymity would otherwise mark itself identified, and the census
			// would report a clean tree over the very call it exists to find.
			name: "naming the header without setting it is not naming the caller",
			source: "package p\nimport (\n\t\"log\"\n\t\"net/http\"\n)\nfunc call() {\n" +
				"\treq, _ := http.NewRequest(\"GET\", \"https://x.test\", nil)\n" +
				"\tlog.Printf(\"User-Agent missing on %v\", req)\n}\n",
			want: 1,
		},
		{
			// A map keyed "User-Agent" writes no header at all.
			name: "a header set on something that is not a request header",
			source: "package p\nimport \"net/http\"\nfunc call(m map[string]string) {\n" +
				"\treq, _ := http.NewRequest(\"GET\", \"https://x.test\", nil)\n" +
				"\t_ = req\n\tm[\"User-Agent\"] = \"x\"\n}\n",
			want: 1,
		},
		{
			name: "an aliased net/http builds the same request",
			source: "package p\nimport nethttp \"net/http\"\nfunc call() {\n" +
				"\treq, _ := nethttp.NewRequest(\"GET\", \"https://x.test\", nil)\n\t_ = req\n}\n",
			want: 1,
		},
		{
			// THIS request's header, not any header the function holds. A
			// builder that names the caller on something else it is composing
			// still sends the request it built with Go's default agent.
			name: "the agent set on a different header is not this request's",
			source: "package p\nimport \"net/http\"\nfunc call(out *http.Request) {\n" +
				"\treq, _ := http.NewRequest(\"GET\", \"https://x.test\", nil)\n" +
				"\tout.Header.Set(\"User-Agent\", \"margince-x/1.0\")\n\t_ = req\n}\n",
			want: 1,
		},
		{
			// Built and sent in one call, so there is no request to name the
			// caller on: anonymous by construction, and the census has to say
			// so rather than not see it.
			name: "a convenience call is a request with nowhere to put a name",
			source: "package p\nimport \"net/http\"\nfunc call() {\n" +
				"\tresp, _ := http.Get(\"https://x.test\")\n\t_ = resp\n}\n",
			want: 1,
		},
		{
			name:   "a function that builds no request",
			source: "package p\nfunc call() string { return \"https://x.test\" }\n",
			want:   0,
		},
		{
			// Per FUNCTION: a file's named request must not vouch for its
			// anonymous one.
			name: "a named request does not cover an anonymous sibling",
			source: "package p\nimport \"net/http\"\nfunc named() {\n" +
				"\treq, _ := http.NewRequest(\"GET\", \"https://x.test\", nil)\n" +
				"\treq.Header.Set(\"User-Agent\", \"margince-x/1.0\")\n}\n" +
				"func bare() {\n\treq, _ := http.NewRequest(\"GET\", \"https://y.test\", nil)\n\t_ = req\n}\n",
			want: 1,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			parsed, err := parser.ParseFile(token.NewFileSet(), "probe.go", tc.source, 0)
			if err != nil {
				t.Fatalf("parsing the probe: %v", err)
			}
			if got := anonymousRequestBuildersIn(parsed); len(got) != tc.want {
				t.Errorf("the detector found %d anonymous builder(s), want %d: %v", len(got), tc.want, got)
			}
		})
	}
}
