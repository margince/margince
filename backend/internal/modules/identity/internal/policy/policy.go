// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package policy owns the role permission-policy documents (B-EP03.1,
// data-model §2.4): the JSONB shape stored in role.permissions, the five
// seeded system-role defaults, the validator that keeps
// a policy honest, and the merge that resolves a user's role set into
// one effective principal.Permissions at authentication time.
package policy

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"slices"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// coreObjects is the closed set of RBAC-governed object types
// (features/04 §1). A policy naming anything else is rejected — a typo'd
// object would otherwise silently grant nothing and read as a bug in the
// role, not the document.
var coreObjects = []string{"person", "organization", "deal", "lead", "activity", "pipeline", "list", "tag", "relationship", "partner", "automation", "voice_profile", "product", "offer", "signal", "saved_view", "custom_field", "computed_field", "offer_template", "overlay_connection", "embedding_reindex", "webhook_subscription", "fx_rate", "ai_model_rate", "capture_settings", "project", "channel_connection", "import_run", "installation_settings", "finance", "integrations", "retention_policy", "capture_trace", "license", "contract", "ai_routing", "commission", "deal_room", "knowledge_corpus", "knowledge_document", "introduction", "weekly_plan", "forecast"}

// IsCoreObject reports whether an RBAC object is in the closed set a role
// document may grant. Parse enforces it on stored documents; it is also the
// answer to "could ANY principal ever hold a grant on this object", which a
// caller deriving an authority requirement needs before trusting it.
func IsCoreObject(object string) bool {
	return slices.Contains(coreObjects, object)
}

// Document is the role.permissions JSONB shape:
// {"objects": {"<object>": {"create":…,"read":…,"update":…,"delete":…}},
//
//	"row_scope": "own"|"team"|"all", "field_masks": […]}.
type Document struct {
	Objects  map[string]grant   `json:"objects"`
	RowScope principal.RowScope `json:"row_scope"`
	// FieldMasks is carried for shape-completeness; enforcement is
	// B-EP03.4 (field-level masking), not built yet.
	FieldMasks []string `json:"field_masks,omitempty"`
}

type grant struct {
	Create bool `json:"create"`
	Read   bool `json:"read"`
	Update bool `json:"update"`
	Delete bool `json:"delete"`
}

// Parse reads one STORED role.permissions document.
//
// It rejects a malformed document and an invalid row_scope, and it DROPS an
// object outside the grantable vocabulary — logged, never fatal. The asymmetry
// is the point, and it was learned the hard way.
//
// A row_scope this code cannot read is a question with no safe answer: the
// value decides how far the grants below it reach, and neither guessing nor
// defaulting is honest. An unknown OBJECT is different — dropping it grants
// nothing, which is the strictest possible reading, and the only reading that
// cannot be exploited.
//
// Rejecting the whole document was the earlier behaviour, and it made the
// vocabulary — which is a property of the COMPILED-IN code plus whichever
// extensions this process happens to compose — a precondition for reading
// STORED DATA that outlives both. Removing an extension therefore did not
// degrade its screen: it took the whole installation's authentication down,
// because `crmauth` fails the login when any of the user's roles will not
// parse. Every user in a workspace whose role still carried
// `ext_<unit>_<object>` was locked out, with no endpoint and no migration to
// clear it — found by the Task 14 UAT, on the removal leg the tier's own
// guarantee requires an operator to perform.
//
// Data outlives the code that gave it meaning. That is not a special case for
// extensions; it is what a stored document IS.
//
// What is NOT given up: a typo'd object still grants nothing, so the
// motivating case for strictness — "a typo must not read as a bug in the role"
// — is answered by the log line rather than by refusing to authenticate. When
// a role-editing endpoint lands (`/roles` CRUD is deferred), that WRITE path is
// where an unknown object must be refused: refusing input a human just typed
// costs them a correction, while refusing stored data costs them their session.
func Parse(raw []byte) (Document, error) {
	var doc Document
	if err := json.Unmarshal(raw, &doc); err != nil {
		return Document{}, fmt.Errorf("policy: malformed permissions document: %w", err)
	}
	for object := range doc.Objects {
		// The GRANTABLE vocabulary, not the core one: a composed extension
		// registers its ext_<unit>_<object> names at boot (composable.go).
		if IsGrantableObject(object) {
			continue
		}
		// Deleted from the parsed document, so the grant cannot reach
		// principal.Permissions by any later path. Mutating the map while
		// ranging it is defined in Go for the entry being visited.
		delete(doc.Objects, object)
		slog.Default().Warn("policy: dropping a grant on an object this installation does not know",
			"object", object,
			"why", "the object is neither a core object nor one a composed extension registered at boot; "+
				"most likely its unit was removed, or the name is a typo. The grant is ignored — it "+
				"authorizes nothing — and the rest of the document still applies.")
	}
	switch doc.RowScope {
	case principal.RowScopeOwn, principal.RowScopeTeam, principal.RowScopeAll:
	case "":
		// An unset scope means the narrowest, never a silent widest.
		doc.RowScope = principal.RowScopeOwn
	default:
		return Document{}, fmt.Errorf("policy: invalid row_scope %q (want own|team|all)", doc.RowScope)
	}
	return doc, nil
}

// Merge resolves a user's assigned roles into the effective permission
// set: grants union (any role allowing an action allows it), row scope
// widens to the maximum any role holds. Zero roles yield zero grants.
func Merge(byRole map[string]Document) principal.Permissions {
	extensionObjects := RegisteredObjects()
	merged := principal.Permissions{
		Objects:  make(map[string]principal.ObjectGrant, len(coreObjects)+len(extensionObjects)),
		RowScope: principal.RowScopeOwn,
	}
	// Every registered extension object is SEEDED at the zero grant, before any
	// role document is read. The seeded core role documents list all thirty core
	// objects, so /me's snapshot has always been the complete vocabulary with
	// the holder's grants filled in — a client can tell "you hold nothing on
	// this" from "no such object". An extension object arrives after those
	// documents were seeded, so without this it would be absent from the
	// snapshot for every principal who was not explicitly granted it, and the
	// unit's screen could not tell the two apart. A union follows below: a role
	// that DOES grant the object widens the zero, never the reverse.
	for _, object := range extensionObjects {
		merged.Objects[object] = principal.ObjectGrant{}
	}
	for _, key := range slices.Sorted(maps.Keys(byRole)) {
		doc := byRole[key]
		merged.RoleKeys = append(merged.RoleKeys, key)
		for object, g := range doc.Objects {
			have := merged.Objects[object]
			merged.Objects[object] = principal.ObjectGrant{
				Create: have.Create || g.Create,
				Read:   have.Read || g.Read,
				Update: have.Update || g.Update,
				Delete: have.Delete || g.Delete,
			}
		}
		if doc.RowScope.Wider(merged.RowScope) {
			merged.RowScope = doc.RowScope
		}
	}
	return merged
}
