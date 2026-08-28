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
// why each is right to. Every entry is a FILE, because the reason is about what
// that file talks to.
var anonymousOutbound = gatekit.Waive(map[string]string{
	"internal/modules/agents/apps/fetch.go": "fetches this product's own origin, so the server on the other end is this same process and a name would be it introducing itself to itself",

	// The model providers. Each call carries the customer's own API key, which
	// is the account the provider bills, rate-limits and revokes; an agent
	// beside it names software the provider has no lever over.
	"internal/modules/ai/anthropic.go":    "carries the customer's own provider key, which is the identity that provider bills and throttles",
	"internal/modules/ai/gemini.go":       "carries the customer's own provider key, which is the identity that provider bills and throttles",
	"internal/modules/ai/ollama.go":       "reaches a model runner the operator runs themselves, on a host they configured — they already know what is calling it",
	"internal/modules/ai/openai.go":       "carries the customer's own provider key, which is the identity that provider bills and throttles",
	"internal/modules/ai/openaicompat.go": "carries the customer's own provider key, which is the identity that provider bills and throttles",

	// The capture connectors. Every one of these is an OAuth or token session
	// the person themselves granted, so the provider knows the grant, the app
	// it was granted to, and the account it was granted on.
	"internal/modules/capture/gmail/client.go":          "runs inside an OAuth grant the person made to this app, which names the caller to the provider more precisely than an agent could",
	"internal/modules/capture/gmail/send.go":            "runs inside an OAuth grant the person made to this app, which names the caller to the provider more precisely than an agent could",
	"internal/modules/capture/googleconn/googleconn.go": "runs inside an OAuth grant the person made to this app, which names the caller to the provider more precisely than an agent could",
	"internal/modules/capture/graph/client.go":          "runs inside an OAuth grant the person made to this app, which names the caller to the provider more precisely than an agent could",
	"internal/modules/capture/graph/transport.go":       "runs inside an OAuth grant the person made to this app, which names the caller to the provider more precisely than an agent could",
	"internal/modules/capture/oauthflow/oauthflow.go":   "exchanges a code against a token endpoint using this app's registered client id, which is the identity that endpoint checks",
	"internal/modules/capture/telegram/api.go":          "calls a bot API under the bot's own token, and the bot IS the identity there",
	"internal/modules/capture/telegram/sendfiles.go":    "calls a bot API under the bot's own token, and the bot IS the identity there",

	// Test support and tooling. Both call a server this repository started, so
	// the operator on the other end is the person who ran them.
	"internal/compose/integration/apptest/appenv.go": "drives a server the test itself started, in the same process tree, for the length of one test",
	"internal/compose/integration/apptest/mcp.go":    "drives a server the test itself started, in the same process tree, for the length of one test",
	"tools/seed-demo/apiclient.go":                   "seeds a demo estate through this product's own API, run by hand against a deployment the operator chose",
	"tools/seed-demo/documents.go":                   "seeds a demo estate through this product's own API, run by hand against a deployment the operator chose",
})

func TestEveryOutboundRequestSaysWhoIsCallingOrRegistersWhyNot(t *testing.T) {
	t.Parallel()
	var findings []string
	builders := 0
	eachGoFileInTheModule(t, func(path string, file *ast.File) {
		if strings.HasSuffix(path, "_test.go") {
			// A test's outbound call goes to a server the test started.
			return
		}
		anonymous := anonymousRequestBuildersIn(file)
		builders += len(requestBuildersIn(file))
		if len(anonymous) == 0 {
			return
		}
		if anonymousOutbound.Waived(t, path) {
			return
		}
		findings = append(findings, path+": "+strings.Join(anonymous, ", "))
	})
	if builders < outboundBuilderFloor {
		t.Fatalf("the walk found only %d outbound request builder(s), and this census is pinned at "+
			"%d — a walk that stopped reaching them reports a clean tree in the same words as a "+
			"tree with nothing left to fix", builders, outboundBuilderFloor)
	}
	anonymousOutbound.AssertAllMatched(t)
	if len(findings) > 0 {
		sort.Strings(findings)
		t.Errorf("%d file(s) build an outbound request that names nobody:\n\t%s\n\n"+
			"Go then sends `Go-http-client/1.1`, which names the language and not the caller, and "+
			"an operator reading it can act on a language or on nothing. Set the User-Agent from "+
			"an identity in internal/platform/outbound, or register the file above with the "+
			"reason it needs none.",
			len(findings), strings.Join(findings, "\n\t"))
	}
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
	for _, fn := range requestBuildersIn(file) {
		if setsUserAgent(fn) {
			continue
		}
		out = append(out, "func "+fn.Name.Name)
	}
	return out
}

// requestBuildersIn names every function in the file that builds an outbound
// http.Request.
func requestBuildersIn(file *ast.File) []*ast.FuncDecl {
	var out []*ast.FuncDecl
	for _, decl := range file.Decls {
		fn, isFunc := decl.(*ast.FuncDecl)
		if !isFunc || fn.Body == nil {
			continue
		}
		if buildsARequest(fn.Body) {
			out = append(out, fn)
		}
	}
	return out
}

func buildsARequest(body *ast.BlockStmt) bool {
	found := false
	ast.Inspect(body, func(node ast.Node) bool {
		call, isCall := node.(*ast.CallExpr)
		if !isCall {
			return true
		}
		selector, isSelector := call.Fun.(*ast.SelectorExpr)
		if !isSelector {
			return true
		}
		pkg, isIdent := selector.X.(*ast.Ident)
		if !isIdent || pkg.Name != "http" {
			return true
		}
		if strings.HasPrefix(selector.Sel.Name, "NewRequest") {
			found = true
		}
		return !found
	})
	return found
}

// setsUserAgent reports whether the function names the caller on the request it
// builds — by the header, or by a transport that carries one for it.
func setsUserAgent(fn *ast.FuncDecl) bool {
	found := false
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		call, isCall := node.(*ast.CallExpr)
		if !isCall {
			return true
		}
		for _, arg := range call.Args {
			lit, isLit := arg.(*ast.BasicLit)
			if !isLit || lit.Kind != token.STRING {
				continue
			}
			if strings.EqualFold(gatekit.TextOf(lit), "User-Agent") {
				found = true
			}
		}
		return !found
	})
	return found
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
