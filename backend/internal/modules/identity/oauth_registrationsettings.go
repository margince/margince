// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"

	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// agentClientObject gates the AI tool sign-in clients and the two switches
// below. Admitting a new app is admin authority, so ops, which writes
// installation_settings, cannot turn a method back on.
const agentClientObject = "agent_client"

// The switches for the two ways an AI tool can get a client ID without an
// admin. Off refuses new sign-ins through that method, a returning user
// included. A grant already made keeps renewing, because refresh never reads
// them. Both default to on; the deployment file can hold either off for good.
// They survive a data reset: which apps may connect is not customer data.
var (
	DynamicClientRegistration = config.DefineStored(config.Spec[bool]{
		Key:     "identity.oauth_dynamic_registration",
		Env:     "MARGINCE_IDENTITY_OAUTH_DYNAMIC_REGISTRATION",
		Type:    config.Bool,
		Default: config.Some(true),
		Doc:     "AI tools may register their own OAuth client (DCR) for new sign-ins.",
	}, config.Store{RBACObject: agentClientObject, AuditVerb: installationAuditVerb, SurvivesDataReset: true})
	MetadataDocumentClients = config.DefineStored(config.Spec[bool]{
		Key:     "identity.oauth_metadata_documents",
		Env:     "MARGINCE_IDENTITY_OAUTH_METADATA_DOCUMENTS",
		Type:    config.Bool,
		Default: config.Some(true),
		Doc:     "AI tools may sign in with a client ID that is the URL of their metadata document (CIMD).",
	}, config.Store{RBACObject: agentClientObject, AuditVerb: installationAuditVerb, SurvivesDataReset: true})
)

// McpClientCheck turns on the admin-only check_client tool. The tool asks an AI
// tool's user to fill in a form. Off, no tool list or call shows that it exists.
var McpClientCheck = config.DefineStored(config.Spec[bool]{
	Key:     "identity.mcp_client_check",
	Env:     "MARGINCE_IDENTITY_MCP_CLIENT_CHECK",
	Type:    config.Bool,
	Default: config.Some(false),
	Doc:     "Admins may run the check_client tool from an AI tool to test whether it can answer a form.",
}, config.Store{RBACObject: agentClientObject, AuditVerb: installationAuditVerb, SurvivesDataReset: true})

// ClientRegistrationReadActor names a switch read in the audit trail: no human
// asked for it, a sign-in or a tool list did.
const ClientRegistrationReadActor = "system:client_registration_read"

// readClientCheck reads the switch as the system. Every role reads the settings
// answer, and only an admin may read agent_client, so a seat's own read would
// refuse the whole answer.
func readClientCheck(ctx context.Context, cfg *config.Config) (bool, error) {
	return config.StoredVal(principal.SystemActing(ctx, ClientRegistrationReadActor), cfg, McpClientCheck)
}

func (p InstallationPatch) clientCheckWrites() ([]pendingWrite, error) {
	write, err := encodeStoredField(McpClientCheck, p.McpClientCheck)
	if err != nil {
		return nil, err
	}
	return []pendingWrite{write}, nil
}

// AdminRegistrationSwitches reads the two switches above, as the system. The
// user a tool signs in cannot read agent_client, and must not need to.
type AdminRegistrationSwitches func(ctx context.Context) (RegistrationMethods, error)

// RegistrationMethods says, per automatic method, whether it is allowed.
type RegistrationMethods struct {
	Dynamic          bool
	MetadataDocument bool
}

func (m RegistrationMethods) and(o RegistrationMethods) RegistrationMethods {
	return RegistrationMethods{Dynamic: m.Dynamic && o.Dynamic, MetadataDocument: m.MetadataDocument && o.MetadataDocument}
}

// WithClientRegistration injects the deployment file's limit and the admin's
// switches. Handlers built without it, such as a unit test, read both methods
// as on, the behaviour before the switches existed.
func (h Handlers) WithClientRegistration(deployment RegistrationMethods, switches AdminRegistrationSwitches) Handlers {
	h.registrationDeployment = &deployment
	h.registrationSwitches = switches
	return h
}

// registrationState is each method's deployment limit and admin switch.
type registrationState struct{ deployment, switches RegistrationMethods }

func (st registrationState) effective() RegistrationMethods { return st.deployment.and(st.switches) }

func (h Handlers) registrationState(ctx context.Context) (registrationState, error) {
	allOn := RegistrationMethods{Dynamic: true, MetadataDocument: true}
	if h.registrationDeployment == nil || h.registrationSwitches == nil {
		return registrationState{deployment: allOn, switches: allOn}, nil
	}
	switches, err := h.registrationSwitches(ctx)
	if err != nil {
		return registrationState{}, err
	}
	return registrationState{deployment: *h.registrationDeployment, switches: switches}, nil
}

func (h Handlers) registrationMethods(ctx context.Context) (RegistrationMethods, error) {
	st, err := h.registrationState(ctx)
	if err != nil {
		return RegistrationMethods{}, err
	}
	return st.effective(), nil
}

// admits reports whether a client that registered this way may start a sign-in.
// A client an admin registered is always admitted.
func (m RegistrationMethods) admits(createdVia string) bool {
	switch createdVia {
	case "dcr":
		return m.Dynamic
	case "cimd":
		return m.MetadataDocument
	}
	return true
}
