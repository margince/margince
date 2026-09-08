// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// A provider whose reach nobody chose is the failure mode of a map like this:
// it reads as the zero value, and the zero value has to be the strict lane for
// that to be safe. Both directions, so a removed provider leaves no entry
// claiming to guard a lane that no longer exists.
func TestEveryProviderDeclaresAnEgressClass(t *testing.T) {
	t.Parallel()

	declared := make(map[string]bool, len(providerEgress))
	for _, provider := range KnownProviders() {
		if _, ok := providerEgress[provider]; !ok {
			t.Errorf("provider %q declares no egress class, so its outbound client is guarded by a default nobody chose", provider)
		}
		declared[provider] = true
	}
	for provider := range providerEgress {
		if !declared[provider] {
			t.Errorf("providerEgress declares %q, which SelectBrain does not accept — a rule for a lane that does not exist", provider)
		}
	}
	if egressPublicOnly != 0 {
		t.Error("the zero egress class must be the strict one, or a provider added without an entry gets the permissive lane")
	}
}

// Sovereign-eligibility and the permissive lane are one idea seen twice: a
// provider is local BECAUSE it is the operator's own endpoint, which is the same
// reason it may dial the operator's own network. Derived rather than restated,
// so a new local provider cannot arrive on the vendor lane.
//
// The implication runs one way. openai_compatible is a BYOK cloud adapter that
// is nonetheless documented to reach a self-hosted gateway, so it takes the
// permissive lane too — named HERE, as the sole exception, so the next adapter
// that wants it has to say so in a diff rather than inherit it.
func TestEveryLocalProviderTakesTheOperatorLane(t *testing.T) {
	t.Parallel()

	for _, provider := range KnownProviders() {
		if provider == ProviderFake {
			continue // dials nothing, so its class binds nothing
		}
		onOperatorLane := egressFor(provider) == egressOperatorEndpoint
		if ProviderIsLocal(provider) && !onOperatorLane {
			t.Errorf("provider %q is sovereign-eligible but may not dial the operator's own network", provider)
		}
		if !ProviderIsLocal(provider) && onOperatorLane && provider != providerOpenAICompatible {
			t.Errorf("provider %q is a cloud vendor on the permissive lane, so a binding may point this installation's model key at an address inside its own network", provider)
		}
	}
}

// What each lane may reach, stated as the addresses themselves.
//
// The operator lane's whole point is the middle column: a local Ollama, a GPU
// box on the operator's own network. What neither lane may reach is the third
// group — the reserved ranges that are nobody's inference endpoint, with cloud
// metadata at the head of it.
func TestWhichAddressesEachLaneMayDial(t *testing.T) {
	t.Parallel()

	for address, want := range map[string]struct{ vendor, operator bool }{
		"8.8.8.8":         {vendor: true, operator: true},
		"2606:4700::1111": {vendor: true, operator: true},

		"127.0.0.1":   {vendor: false, operator: true},
		"::1":         {vendor: false, operator: true},
		"10.4.1.20":   {vendor: false, operator: true},
		"172.16.0.9":  {vendor: false, operator: true},
		"192.168.1.5": {vendor: false, operator: true},
		"fd00::1":     {vendor: false, operator: true},
		// An IPv4-mapped IPv6 address is judged by its IPv4 rules, so the
		// mapping smuggles nothing past either lane.
		"::ffff:10.0.0.1": {vendor: false, operator: true},

		// Cloud metadata, and its NAT64 spelling: refused on BOTH lanes. An
		// operator serves inference from an address they configured, never from
		// an autoconfiguration one.
		"169.254.169.254":    {vendor: false, operator: false},
		"fe80::1":            {vendor: false, operator: false},
		"64:ff9b::a9fe:a9fe": {vendor: false, operator: false},
		"100.64.0.1":         {vendor: false, operator: false}, // carrier-grade NAT
		"192.0.2.10":         {vendor: false, operator: false}, // documentation range
		"0.0.0.0":            {vendor: false, operator: false},
		"2002:7f00:1::1":     {vendor: false, operator: false}, // 6to4-wrapped loopback
	} {
		ip := net.ParseIP(address)
		if ip == nil {
			t.Fatalf("the table entry %q is not an address", address)
		}
		if got := addressAllowed(egressPublicOnly, ip); got != want.vendor {
			t.Errorf("the vendor lane may dial %s = %v, want %v", address, got, want.vendor)
		}
		if got := addressAllowed(egressOperatorEndpoint, ip); got != want.operator {
			t.Errorf("the operator lane may dial %s = %v, want %v", address, got, want.operator)
		}
	}
}

// The write-time rule and the dialer must answer the same question the same way,
// or a binding is accepted at the door and refused by the first call — the exact
// failure ValidateTierBinding's own comment says it exists to avoid. This is
// what makes addressAllowed's "single spelling" a fact rather than a hope: a
// second copy of the rule in either caller fails here the moment it drifts.
func TestTheWriteRuleAndTheDialerAgree(t *testing.T) {
	t.Parallel()

	for _, provider := range KnownProviders() {
		guard := dialGuard(egressFor(provider))
		for _, address := range []string{
			"8.8.8.8", "2606:4700::1111",
			"127.0.0.1", "::1", "10.4.1.20", "192.168.1.5", "fd00::1", "::ffff:10.0.0.1",
			"169.254.169.254", "fe80::1", "64:ff9b::a9fe:a9fe", "100.64.0.1", "192.0.2.10",
			"0.0.0.0", "2002:7f00:1::1", "172.32.0.9",
		} {
			hostPort := net.JoinHostPort(address, "8080")
			atTheWrite := requireDialableEndpoint("tier premium", provider, "http://"+hostPort) == nil
			atTheSocket := guard("tcp", hostPort, nil) == nil
			if atTheWrite != atTheSocket {
				t.Errorf("provider %q, address %s: the write rule says allowed=%v and the dialer says allowed=%v",
					provider, address, atTheWrite, atTheSocket)
			}
		}
	}
}

// The guard is only as good as its coverage: an adapter handed a client built
// anywhere else dials unguarded, and nothing about the call site would look
// wrong. So the package is allowed exactly one call to newOutboundClient, and it
// is the one in SelectBrain — read off the syntax tree rather than grepped, so a
// call spelled across a line break cannot hide from it.
func TestSelectBrainIsTheOnlyBuilderOfAnOutboundClient(t *testing.T) {
	t.Parallel()

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("reading the package directory: %v", err)
	}
	var callers []string
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), name, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", name, err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			ast.Inspect(fn, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				if ident, ok := call.Fun.(*ast.Ident); ok && ident.Name == "newOutboundClient" {
					callers = append(callers, name+":"+fn.Name.Name)
				}
				return true
			})
		}
	}
	if len(callers) != 1 || callers[0] != "selectbrain.go:SelectBrain" {
		t.Errorf("newOutboundClient is called from %v, want only selectbrain.go:SelectBrain — every other builder dials unguarded", callers)
	}
}

// The Control hook is what the socket actually consults, and it must refuse the
// shapes it cannot judge as well as the ones it judges badly — an address it
// cannot parse is not an address it may dial.
func TestTheDialGuardRefusesWhatItCannotJudge(t *testing.T) {
	t.Parallel()

	guard := dialGuard(egressOperatorEndpoint)
	if err := guard("tcp", "127.0.0.1:11434", nil); err != nil {
		t.Fatalf("the operator lane must dial its own loopback endpoint, got %v", err)
	}
	for _, address := range []string{
		"169.254.169.254:80",    // the address this guard exists for
		"not-a-host-port",       // unparseable
		"ollama.internal:11434", // a name: the hook sees resolved addresses only
	} {
		if err := guard("tcp", address, nil); err == nil {
			t.Errorf("the guard dialed %q", address)
		}
	}
}

// The production wiring, through the exported constructor rather than around
// it: a client built by SelectBrain carries its lane's guard.
//
// Both directions, because a refusal-only test also passes when the whole
// adapter is broken — the local lane must still reach the endpoint it is for.
func TestSelectBrainWiresTheEgressGuard(t *testing.T) {
	t.Parallel()

	t.Run("a vendor binding may not be pointed at this host", func(t *testing.T) {
		t.Parallel()
		// Bound to an address the loopback lane owns: a BYOK adapter carries
		// this installation's model key, and the API host's own loopback is
		// where a stored base_url would send it.
		client, err := SelectBrain(
			ProviderConfig{Provider: providerAnthropic, BaseURL: "http://127.0.0.1:11434", Model: "m"},
			cloudKeyFor(providerAnthropic, "stands-in-for-a-key"),
		)
		if err != nil {
			t.Fatal(err)
		}
		_, err = client.(model.Lister).ListModels(context.Background())
		if err == nil || !strings.Contains(err.Error(), "netguard") {
			t.Fatalf("the vendor lane dialed loopback, or refused for another reason: %v", err)
		}
	})

	t.Run("a local binding may not be pointed at cloud metadata", func(t *testing.T) {
		t.Parallel()
		client, err := SelectBrain(
			ProviderConfig{Provider: providerOllama, BaseURL: "http://169.254.169.254", Model: "m"},
			noCloudKeys(),
		)
		if err != nil {
			t.Fatal(err)
		}
		_, err = client.(model.Lister).ListModels(context.Background())
		if err == nil || !strings.Contains(err.Error(), "netguard") {
			t.Fatalf("the operator lane dialed the metadata service, or refused for another reason: %v", err)
		}
	})

	t.Run("a local binding still reaches its own endpoint", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if _, err := w.Write([]byte(`{"models":[{"name":"gemma3"}]}`)); err != nil {
				t.Errorf("writing the ollama tag list: %v", err)
			}
		}))
		t.Cleanup(srv.Close)

		client, err := SelectBrain(ProviderConfig{Provider: providerOllama, BaseURL: srv.URL, Model: "gemma3"}, noCloudKeys())
		if err != nil {
			t.Fatal(err)
		}
		models, err := client.(model.Lister).ListModels(context.Background())
		if err != nil {
			t.Fatalf("the guard blocked the local endpoint it exists to allow: %v", err)
		}
		if len(models) != 1 || models[0].ID != "gemma3" {
			t.Fatalf("the local lane answered %v, want the one model the endpoint serves", models)
		}
	})
}

// The write-time half. Before this rule the endpoint was checked under the
// sovereign profile ALONE, so any string that parsed was persisted on the other
// two and only the first call found out — as an unreachable vendor, with the
// reason in a log nobody reads.
func TestABaseURLIsCheckedOnEveryProfile(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct{ routing, names string }{
		"cloud metadata on a local tier": {
			routing: `
profile: eu_hosted
tiers:
  local_large: { provider: ollama, base_url: "http://169.254.169.254/latest/meta-data", model: m }
embeddings: { provider: ollama, model: bge-m3 }
`,
			names: "169.254.169.254",
		},
		"this host's own network on a vendor tier": {
			routing: `
profile: cloud_frontier
tiers:
  premium: { provider: anthropic, base_url: "http://10.0.0.5:6379", model: claude-x }
embeddings: { provider: ollama, model: bge-m3 }
`,
			names: "public host",
		},
		"cloud metadata on the embeddings lane": {
			routing: `
profile: eu_hosted
tiers:
  local_large: { provider: ollama, model: m }
embeddings: { provider: ollama, base_url: "http://169.254.169.254", model: bge-m3 }
`,
			names: "169.254.169.254",
		},
		"a credential smuggled in as userinfo": {
			routing: `
profile: eu_hosted
tiers:
  local_large: { provider: ollama, base_url: "http://user:stands-in-for-a-token@10.4.1.20:11434", model: m }
embeddings: { provider: ollama, model: bge-m3 }
`,
			names: "userinfo",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := ParseRouting([]byte(tc.routing))
			if err == nil {
				t.Fatal("the binding was accepted, so the first call is what finds out")
			}
			if !strings.Contains(err.Error(), tc.names) {
				t.Errorf("the refusal %q does not say %q, so an operator cannot tell what to change", err, tc.names)
			}
			if strings.Contains(err.Error(), "stands-in-for-a-token") {
				t.Errorf("the refusal echoes the credential it was given: %v", err)
			}
		})
	}
}

// The mirror, and the reason the rule is two lanes rather than one: the ordinary
// deployments must still boot. A local model on the operator's own network, a
// self-hosted gateway behind a BYOK key, and a vendor's own public host.
func TestTheEndpointRuleLetsTheOrdinaryDeploymentsBoot(t *testing.T) {
	t.Parallel()

	for name, routing := range map[string]string{
		"a local model on the operator's network": `
profile: eu_hosted
tiers:
  local_large: { provider: ollama, base_url: "http://10.4.1.20:11434", model: m }
embeddings: { provider: ollama, base_url: "http://127.0.0.1:11434", model: bge-m3 }
`,
		"a self-hosted gateway on the OpenAI wire": `
profile: eu_hosted
tiers:
  premium: { provider: openai_compatible, base_url: "http://192.168.1.5:8080", model: m }
embeddings: { provider: ollama, model: bge-m3 }
`,
		"a vendor's own host, and a name this cannot judge": `
profile: cloud_frontier
tiers:
  premium: { provider: anthropic, base_url: "https://api.anthropic.com", model: claude-x }
  local_large: { provider: ollama, base_url: "https://ollama.internal:11434", model: m }
embeddings: { provider: ollama, model: bge-m3 }
`,
		"no base_url at all, which is the provider default": `
profile: cloud_frontier
tiers:
  premium: { provider: anthropic, model: claude-x }
embeddings: { provider: ollama, model: bge-m3 }
`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := ParseRouting([]byte(routing)); err != nil {
				t.Fatalf("an ordinary deployment must boot, got %v", err)
			}
		})
	}
}
