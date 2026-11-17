// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"log/slog"
	"slices"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/ports/companytypes"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
	"github.com/margince/margince/backend/pkg/extension"
)

// Extensions is the extension state one composition owns. It holds the unit set
// RegisterExtensions validated and what it built from that set. It also holds
// the runtime dependencies the role binds once its pool exists.
//
// A role holds the one RegisterExtensions returns and hands it to everything
// that serves or runs a unit. Every "what did this unit declare" question reads
// it. So the answer is the set the boot validated, never a second copy.
//
// A nil *Extensions is a composition with no units: every read answers empty
// and the runtime refuses as unwired.
type Extensions struct {
	units []extension.Extension
	verbs []extension.Verb
	tools []mcp.Tool
	subs  []ComposedSubscription
	jobs  []composedJob
	rbac  identity.RbacObjects
	// reportSet is the units' counts and breakdowns, ordered by name.
	reportSet []registeredReport

	// mu orders the runtime binding and the channel vocabulary, which the boot
	// writes after the pool exists and every call reads.
	mu       sync.RWMutex
	runtime  extensionRuntimeBinding
	channels channelVocabulary
}

// Units returns the composition's declared extension units.
func (x *Extensions) Units() []extension.Extension {
	if x == nil {
		return nil
	}
	return slices.Clone(x.units)
}

// Verbs returns the composition's contract-declared extension operations.
// The composition root mounts their routes from it.
func (x *Extensions) Verbs() []extension.Verb {
	if x == nil {
		return nil
	}
	return slices.Clone(x.verbs)
}

// Subscriptions returns the composition's registered listeners, each carrying
// the unit identity its deliveries are attributed to. The worker role starts
// one consumer per listener.
func (x *Extensions) Subscriptions() []ComposedSubscription {
	if x == nil {
		return nil
	}
	return slices.Clone(x.subs)
}

// RbacObjects returns the RBAC object vocabulary the composition's units extend
// core's with, and the starting grants they declare.
func (x *Extensions) RbacObjects() identity.RbacObjects {
	if x == nil {
		return identity.RbacObjects{}
	}
	return x.rbac
}

// CompanyTypes returns the company types the composition's units declare.
func (x *Extensions) CompanyTypes() companytypes.Registry {
	if x == nil {
		return companytypes.Registry{}
	}
	return companytypes.Of(x.units)
}

func (x *Extensions) servedJobs() []composedJob {
	if x == nil {
		return nil
	}
	return x.jobs
}

// reports answers the units' counts and breakdowns, ordered by name.
func (x *Extensions) reports() []registeredReport {
	if x == nil {
		return nil
	}
	return x.reportSet
}

// unit finds the named unit in the composition.
func (x *Extensions) unit(name string) (extension.Extension, bool) {
	if x == nil {
		return extension.Extension{}, false
	}
	for _, ext := range x.units {
		if string(ext.Name) == name {
			return ext, true
		}
	}
	return extension.Extension{}, false
}

// BindRuntime records what a unit's per-call Runtime reaches the installation
// through. Every role that serves or runs agent tools calls it once at boot,
// after the pool and the custodian exist.
//
// vault may be nil on a deployment that configured no keyvault: the secret
// namespace then refuses by name (extsecrets.ErrNoCustodian). A nil pool leaves
// the whole capability surface refusing with errExtensionRuntimeUnwired.
//
// A second bind to a different non-nil pool is a wiring fault: half the
// extension calls would run on the wrong pool. It is logged, because this layer
// does not end a boot.
func (x *Extensions) BindRuntime(pool *pgxpool.Pool, vault keyvault.Vault) {
	x.mu.Lock()
	defer x.mu.Unlock()
	if prev := x.runtime.pool; prev != nil && pool != nil && prev != pool {
		slog.Default().Warn("compose: the extension runtime was rebound to a different pool; " +
			"every extension capability from now on runs against the new one")
	}
	x.runtime.pool, x.runtime.vault = pool, vault
}

// BindCapture records the capture pipeline a unit's ingress lands through. A
// role that runs unattended extension work calls it once at boot with the
// deployment capture config its own connectors run on.
//
// It takes the config and assembles the sink here. newCaptureSink is the one
// spelling that attaches the file keeper, merge stager and counterparty ensurer. A role that never calls it leaves ingress refusing with
// errIngressUnwired.
func (x *Extensions) BindCapture(pool *pgxpool.Pool, cfg CaptureConfig) {
	x.mu.Lock()
	defer x.mu.Unlock()
	x.runtime.captureSink = newCaptureSink(pool, cfg)
}

// binding reads what the role bound, per call, so the order between binding
// and building a registry cannot matter.
func (x *Extensions) binding() extensionRuntimeBinding {
	if x == nil {
		return extensionRuntimeBinding{}
	}
	x.mu.RLock()
	defer x.mu.RUnlock()
	bound := x.runtime
	bound.composed = x
	return bound
}

// bindExtensions hands the composition to every handler set that serves a unit.
// It runs after the options, because an option may replace a handler set whole.
func (s *Server) bindExtensions() {
	s.extensionsHandlers.extensions = s.extensions
	s.extensionSettingsHandlers.extensions = s.extensions
	s.channelProvidersHandlers.extensions = s.extensions
	s.connectorHandlers.extensions = s.extensions
	s.connectorAppHandlers.extensions = s.extensions
	s.installationSettingsHandlers.extensions = s.extensions
	s.dataTaskHandlers.extensions = s.extensions
	s.analyticsQueryHandlers.extensions = s.extensions
}

// ExtensionsWithSubscriptionsForTest is a composition holding only listeners.
// The worker's lane starter reads a composition's listeners, and its tests need
// a set containing one it cannot resolve.
func ExtensionsWithSubscriptionsForTest(subs []ComposedSubscription) *Extensions {
	composed := &Extensions{subs: slices.Clone(subs)}
	for i := range composed.subs {
		composed.subs[i].composed = composed
	}
	return composed
}

// ForkForTest is a composition serving the same validated set with a runtime
// binding of its own. Parallel tests that share one registration each bind
// their own pool to a fork.
func (x *Extensions) ForkForTest() *Extensions {
	fork := &Extensions{units: x.units, verbs: x.verbs, jobs: x.jobs, rbac: x.rbac, reportSet: x.reportSet}
	for _, tool := range x.tools {
		if adapted, ok := tool.(extensionTool); ok {
			adapted.composed = fork
			tool = adapted
		}
		fork.tools = append(fork.tools, tool)
	}
	for _, sub := range x.subs {
		sub.composed = fork
		fork.subs = append(fork.subs, sub)
	}
	return fork
}
