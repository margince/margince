// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package policy

// The six seeded system-role documents. Every grant row, the `grid` builder
// they are assembled with and the object-name constants they repeat live in
// grants.go; this file is only the six decisions about who holds what.

import (
	"encoding/json"
	"fmt"
	"maps"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The objects whose posture is not visible from any one role's overrides,
// because every role treats them the same way or because the reason lives in a
// relationship no grant expresses.
//
// finance — the ingested accounting mirror (ADR-0083/A128). READ is broad:
// every role that opens a company page sees whether the customer pays on time.
// Connecting or disconnecting the source is destructive workspace-wide config,
// so create/update/delete are admin/ops-only, exactly like overlay_connection.
// No role holds a write on a finance RECORD, because there is no such action:
// the mirror's read-only posture is the ABSENCE of the grant (FIN-DDL-N-1),
// not a runtime refusal.
//
// contract (ADR-0109/A160) and commission carry no owner column of their own —
// visibility is inherited from the deal they hang off, so the row scope that
// reaches them is the DEAL's. A rep records and maintains the agreements they
// close and sees what their partner-sourced deal earned; approving and paying
// are the business's decisions, and an entry is written by the won-deal accrual
// rather than by hand.
//
// integrations (ADR-0101/A152) — a rep reads whether a provider is connected,
// so a dated value on a person record has an explanation; connecting one spends
// money and is admin/ops.
//
// offer_template follows product and offer rather than the pipeline-config
// posture: it is the offer's own branding input, not a locked-down schema
// surface, so reps create and work templates like any other offer-adjacent
// record. Delete stays manager/admin/ops — archiveOfferTemplate carries no
// x-agent-access gate, so any role holding delete may call it directly.
//
// overlay_connection, channel_connection and webhook_subscription share one
// posture: each binds the whole workspace to something outside it — an
// incumbent CRM, a chat bot carrying every seat's inbound traffic, an outbound
// egress of governed events — so create/update/delete are admin/ops-only while
// every role reads the binding's status. A rep needs to know whether overlay
// mode is live, or whether the channel is up, before expecting a reply to
// arrive there. (UC-E10-04 narrates a Rep registering a subscription; that
// posture question is tracked upstream, not settled here.)
//
// weekly_plan — a rep plans their own week and settles it. Their own only: the
// store resolves the plan from the caller and takes no owner argument, so the
// grant admits them to their week and to nobody else's. A lead's read of a
// rep's plan is a separate path gated on the shared-team question, not on a
// wider grant here. No delete, for the reason introduction carries: a week that
// was planned is a thing that happened, and a commitment is dropped rather than
// erased.

// managerObjects is the team lead's grid. `management` starts from it and
// departs in exactly the ways managementObjects names below; the two used to be
// one variable, and the administration objects are what separated them.
var managerObjects = grid(crud, map[string]grant{
	objAiModelRate:          none,
	objAiRouting:            none,
	"automation":            readOnly,
	objCaptureSettings:      createRead,
	objCaptureTrace:         readOnly,
	"channel_connection":    readOnly,
	objComputedField:        readOnly,
	"custom_field":          readOnly,
	objDataCoverage:         none,
	objEmbeddingReindex:     none,
	"finance":               readOnly,
	objForecast:             createRead,
	objFxRate:               none,
	objImportRun:            none,
	objInstallationSettings: readOnly,
	"integrations":          readOnly,
	objIntroduction:         writeNoDelete,
	"knowledge_corpus":      readOnly,
	"knowledge_document":    readOnly,
	objLicense:              none,
	"overlay_connection":    readOnly,
	"pipeline":              readOnly,
	objRetentionPolicy:      none,
	"tag":                   readOnly,
	"webhook_subscription":  readOnly,

	// A team lead administers records, never the installation. Every
	// administration object is spelled out rather than left to the crud base,
	// because the base is what a NEW object inherits, and a settings surface
	// added later must not arrive already granted to every team lead.
	objUserAdmin:            none,
	objRoleAdmin:            none,
	objTeamAdmin:            none,
	objPrivacyRequest:       none,
	objAuditLog:             none,
	objJobHealth:            none,
	objExtensionAccess:      none,
	objSystemReset:          none,
	objAiDiagnostics:        none,
	objConsentConfig:        none,
	objAuthenticationPolicy: none,
	objOauthApplication:     none,
	objSeatUsage:            none,
})

// managementObjects is managerObjects with the five administration reads a
// sales leader answers for: the AI spend, the consent vocabulary their team is
// bound by, the sign-in posture, which OAuth applications the workspace issued,
// and how many seats are used. Every one is a READ — management sees what it is
// accountable for and changes none of it.
//
// Derived from managerObjects by copy rather than by aliasing it: the two grids
// now differ, and one variable serving both is how a later edit to a team lead's
// records posture would silently widen the organization-scoped seat too.
var managementObjects = func() map[string]grant {
	out := maps.Clone(managerObjects)
	for _, object := range []string{
		objAiDiagnostics,
		objConsentConfig,
		objAuthenticationPolicy,
		objOauthApplication,
		objSeatUsage,
	} {
		out[object] = readOnly
	}
	return out
}()

var defaults = map[string]Document{
	// The installation's administrator: every object, every verb, except where
	// the PRODUCT offers no such verb. computed_field has no authoring surface
	// (RD-AC-7); license is issued rather than edited; capture_trace is written
	// by the pipeline and swept by its own job; the rate sheets and
	// capture_settings are append-forward because a past-dated row prices a
	// historical rollup; forecast supersedes rather than being rewritten.
	"admin": {
		Objects: grid(crud, map[string]grant{
			objAiModelRate:          writeNoDelete,
			objAiRouting:            readUpdate,
			objCaptureSettings:      writeNoDelete,
			objCaptureTrace:         readOnly,
			objComputedField:        readOnly,
			objDataCoverage:         readOnly,
			objEmbeddingReindex:     readUpdate,
			objForecast:             createRead,
			objFxRate:               writeNoDelete,
			objInstallationSettings: readUpdate,
			objIntroduction:         writeNoDelete,
			objLicense:              readOnly,

			// The administration objects. Admin holds every one, but not at crud:
			// each is spelled to the verbs its surface actually offers, so a grant
			// never names an action no endpoint implements.
			objTeamAdmin:            writeNoDelete,
			objPrivacyRequest:       readUpdate,
			objAuditLog:             readOnly,
			objJobHealth:            readOnly,
			objExtensionAccess:      readOnly,
			objSystemReset:          deleteOnly,
			objAiDiagnostics:        readOnly,
			objAuthenticationPolicy: readUpdate,
			objSeatUsage:            readOnly,
		}),
		RowScope: principal.RowScopeAll,
	},
	// management is the sales leader's seat (ADR-0110): the manager grid over
	// EVERY row in the organization, plus the five administration READS a sales
	// leader answers for, and no administration write at all. Inviting users,
	// changing roles, editing role policy, binding another user's passport and
	// issuing password links are objUserAdmin and objRoleAdmin, which this seat
	// does not hold — so an unbounded row scope widens what management sees,
	// never what it administers.
	"management": {
		Objects:  managementObjects,
		RowScope: principal.RowScopeAll,
	},
	"manager": {
		Objects: managerObjects,
		// Team scope: a Team Lead manages their team, so they read and work the
		// records of everyone sharing a live team with them without a share
		// being arranged first. This is the manager grid above, bounded to the
		// team rather than the organization — `management` is the same grid
		// unbounded.
		//
		// Membership resolves through team_membership and live teams only, so a
		// Team Lead who belongs to no team reaches exactly their own rows. A
		// record_grant naming a user or a team stays the mechanism for sharing
		// ACROSS teams, and for every seat that is not this one.
		RowScope: principal.RowScopeTeam,
	},
	// The rep's baseline is READ. Sixteen objects take it and sixteen take
	// writeNoDelete, so the tie is broken deliberately rather than by the
	// count: a base is what an object added later inherits before anybody
	// revisits this file, and inheriting "may read" is a smaller mistake than
	// inheriting "may create and edit". The overrides below are the records
	// they actually work. Those take writeNoDelete — create, read, same-day-correct, never
	// delete — because archiving a commercial record is a manager's call.
	//
	// The three departures from that rule each have a reason. `lead` is crud
	// because disqualifying IS the delete and is routine rep work.
	// `saved_view` is crud because it is the rep's own per-user view state
	// (owner-scoped in the store), so full self-service is correct.
	//
	// Nine objects take the zero grant, and not all for one reason. Most are
	// surfaces with no rep consumer: the workspace's entitlement (UC-ADMIN-03
	// F1), the retention ladder, the two rate sheets, the reindex trigger, the
	// import run, source coverage, and which vendor receives the installation's
	// text. The shared-channel capture trace is a different and stronger claim
	// — a member's own capture traffic is personal data that no grant widens —
	// and its reason is written beside its constant rather than folded in here.
	//
	// A custom field is read-only even for the rep who fills it in: the field
	// itself is admin/ops-set config, and only the VALUE on a record is theirs.
	// knowledge_corpus and knowledge_document are reads because the ask itself
	// is that read — an answer whose source is unreadable is not a citation —
	// while uploading the prose every seat then asks stays with admin/ops.
	// `introduction` and `weekly_plan` carry no delete for a shared reason: an
	// ask that was made and a week that was planned are things that happened,
	// so they are withdrawn rather than erased, and EVERY seat carries that
	// property, admin included.
	"rep": {
		Objects: grid(readOnly, map[string]grant{
			"activity":          writeNoDelete,
			objAiModelRate:      none,
			objAiRouting:        none,
			objCaptureSettings:  createRead,
			objCaptureTrace:     none,
			"contract":          writeNoDelete,
			objDataCoverage:     none,
			"deal":              writeNoDelete,
			"deal_room":         writeNoDelete,
			objEmbeddingReindex: none,
			objFxRate:           none,
			objImportRun:        none,
			objIntroduction:     writeNoDelete,
			"lead":              crud,
			objLicense:          none,
			"list":              writeNoDelete,
			"offer":             writeNoDelete,
			"offer_template":    writeNoDelete,
			"organization":      writeNoDelete,
			"person":            writeNoDelete,
			"product":           writeNoDelete,
			"project":           writeNoDelete,
			"relationship":      writeNoDelete,
			objRetentionPolicy:  none,
			objSavedView:        crud,
			"signal":            writeNoDelete,
			"voice_profile":     writeNoDelete,
			"weekly_plan":       writeNoDelete,

			// The administration objects. The base here is readOnly, so every one
			// must be named: leaving them to the base would let a rep read the
			// audit trail and the roster's privileged view on the day the object
			// was added, which is the opposite of what adding it was for.
			objUserAdmin:            none,
			objRoleAdmin:            none,
			objTeamAdmin:            none,
			objPrivacyRequest:       none,
			objAuditLog:             none,
			objJobHealth:            none,
			objExtensionAccess:      none,
			objSystemReset:          none,
			objAiDiagnostics:        none,
			objConsentConfig:        none,
			objAuthenticationPolicy: none,
			objOauthApplication:     none,
			objSeatUsage:            none,
		}),
		RowScope: principal.RowScopeOwn,
	},
	"read_only": {
		// A read-only role still owns its personal view state: saved views
		// are P1-exempt per-user prefs (runtime-config-surface.md §3), not
		// shared records, so full self-service is correct even here. The zero
		// grants are the same set the rep holds nothing on, for the same
		// reasons.
		Objects: grid(readOnly, map[string]grant{
			objAiModelRate:      none,
			objAiRouting:        none,
			objCaptureTrace:     none,
			objDataCoverage:     none,
			objEmbeddingReindex: none,
			objFxRate:           none,
			objImportRun:        none,
			objLicense:          none,
			objRetentionPolicy:  none,
			objSavedView:        crud,

			// The administration objects. The base here is readOnly, so every one
			// must be named: leaving them to the base would let a rep read the
			// audit trail and the roster's privileged view on the day the object
			// was added, which is the opposite of what adding it was for.
			objUserAdmin:            none,
			objRoleAdmin:            none,
			objTeamAdmin:            none,
			objPrivacyRequest:       none,
			objAuditLog:             none,
			objJobHealth:            none,
			objExtensionAccess:      none,
			objSystemReset:          none,
			objAiDiagnostics:        none,
			objConsentConfig:        none,
			objAuthenticationPolicy: none,
			objOauthApplication:     none,
			objSeatUsage:            none,
		}),
		RowScope: principal.RowScopeAll,
	},
	// ops administers the installation's wiring on the admin grid, and differs
	// from admin in row scope alone at this layer: what actually separates them
	// is the literal-admin gate on identity and governance routes, which no
	// grant here expresses.
	"ops": {
		Objects: grid(crud, map[string]grant{
			objAiModelRate:          writeNoDelete,
			objAiRouting:            readUpdate,
			objCaptureSettings:      writeNoDelete,
			objCaptureTrace:         readOnly,
			objComputedField:        readOnly,
			objDataCoverage:         readOnly,
			objEmbeddingReindex:     readUpdate,
			objForecast:             createRead,
			objFxRate:               writeNoDelete,
			objInstallationSettings: readUpdate,
			objIntroduction:         writeNoDelete,
			objLicense:              readOnly,

			// The administration objects, and the sharpest place admin and ops
			// differ. Ops administers the installation's WIRING: the consent
			// vocabulary, the OAuth applications, the queues, the composed units.
			// It does not administer PEOPLE — no user_admin, no team_admin — and it
			// does not read the audit trail or hold the reset, because an operator
			// is not the party those two exist to hold to account. role_admin is
			// read: answering "why can this person not see that" needs the policy
			// in front of you, and changing it does not.
			objUserAdmin:            none,
			objRoleAdmin:            readOnly,
			objTeamAdmin:            none,
			objPrivacyRequest:       none,
			objAuditLog:             none,
			objJobHealth:            readOnly,
			objExtensionAccess:      readOnly,
			objSystemReset:          none,
			objAiDiagnostics:        readOnly,
			objAuthenticationPolicy: readOnly,
			objSeatUsage:            readOnly,
		}),
		RowScope: principal.RowScopeAll,
	},
}

// MustDefaultJSON returns the seeded policy document for a system role key,
// ready for the role.permissions column. Unknown keys panic: the caller
// iterates the compiled-in role list, so a miss is a programming error.
func MustDefaultJSON(roleKey string) []byte {
	doc, ok := defaults[roleKey]
	if !ok {
		panic(fmt.Sprintf("policy: no default document for role %q", roleKey))
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		panic(err) // a compiled-in document always marshals
	}
	return raw
}
