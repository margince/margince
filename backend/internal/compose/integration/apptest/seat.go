// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package apptest

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// SessionUserID is who this session is signed in as, answered by /me rather
// than resolved from the table. A fixture that derives production's answer
// itself may derive a different seat than the one making the requests.
func SessionUserID(t *testing.T, e *AppEnv) string {
	t.Helper()
	var me struct {
		User struct {
			ID string `json:"id"`
		} `json:"user"`
	}
	if status := e.Call(t, "GET", "/v1/me", nil, nil, &me); status != http.StatusOK {
		t.Fatalf("reading the signed-in seat → %d", status)
	}
	if me.User.ID == "" {
		t.Fatal("/me answered no user id — there is no seat to demote, and demoting an arbitrary one would leave this session an admin")
	}
	return me.User.ID
}

// SeededRole is one system role as the database holds it: the key a role
// matrix reports, and the grant document the expectation is read from.
type SeededRole struct {
	Key      string
	Objects  map[string]map[string]bool
	RowScope string
}

// Grants reports whether the role may take verb on object.
func (r SeededRole) Grants(object, verb string) bool { return r.Objects[object][verb] }

// AgentPrincipal is an agent on a passport of a human holding the role's
// grants, as a passport minted for a seat on the role is.
func (r SeededRole) AgentPrincipal() principal.Principal {
	objects := map[string]principal.ObjectGrant{}
	for object, verbs := range r.Objects {
		objects[object] = principal.ObjectGrant{
			Create: verbs["create"], Read: verbs["read"], Update: verbs["update"], Delete: verbs["delete"],
		}
	}
	return principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:" + r.Key, PassportID: ids.NewV7(), OnBehalfOf: ids.NewV7(),
		Permissions: principal.Permissions{RoleKeys: []string{r.Key}, Objects: objects, RowScope: principal.RowScope(r.RowScope)},
	}
}

// SeededRoles reads the system roles the bootstrap laid down. A test's rows and
// expectations then come from the policy it checks, not a second copy.
func SeededRoles(t *testing.T, e *AppEnv) []SeededRole {
	t.Helper()
	rows, err := e.Owner.Query(t.Context(),
		`SELECT key, permissions FROM role WHERE is_system ORDER BY key`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []SeededRole
	for rows.Next() {
		var key string
		var raw []byte
		if err := rows.Scan(&key, &raw); err != nil {
			t.Fatal(err)
		}
		var doc struct {
			Objects  map[string]map[string]bool `json:"objects"`
			RowScope string                     `json:"row_scope"`
		}
		if err := json.Unmarshal(raw, &doc); err != nil {
			t.Fatalf("role %q carries an unreadable permission document: %v", key, err)
		}
		out = append(out, SeededRole{Key: key, Objects: doc.Objects, RowScope: doc.RowScope})
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// AssignRoleTo makes keys the only workspace-wide roles of the seat with
// email. A passport reads its user's live grants, so the next call is answered
// by these roles alone.
func AssignRoleTo(t *testing.T, e *AppEnv, email string, keys ...string) {
	t.Helper()
	user := `(SELECT id FROM app_user WHERE email = $1)`
	if _, err := e.Owner.Exec(t.Context(), `DELETE FROM role_assignment WHERE team_id IS NULL AND user_id = `+user, email); err != nil {
		t.Fatalf("clearing the roles of %s: %v", email, err)
	}
	tag, err := e.Owner.Exec(t.Context(), `
		INSERT INTO role_assignment (user_id, role_id) SELECT `+user+`, id FROM role WHERE key = ANY($2)`, email, keys)
	if err != nil {
		t.Fatalf("giving %s the roles %v: %v", email, keys, err)
	}
	if int(tag.RowsAffected()) != len(keys) {
		t.Fatalf("giving %s the roles %v assigned %d; a key names no role or the seat is missing", email, keys, tag.RowsAffected())
	}
}
