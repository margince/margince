// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/margince/margince/backend/tools/internal/oasnode"
)

// fixture is a small contract with one of each case. The anchor &shared is
// defined inside the human-only operation and aliased from a kept one, and the
// anchor name &reused is defined twice, as crm.yaml does.
const fixture = `openapi: 3.1.0
info: { title: Fixture, version: '1' }
servers:
  - url: https://crm.example.com/v1
security:
  - bearerAuth: []
tags:
  - name: Contacts
  - name: Identity
x-top-level: dropped
paths:
  /passports:
    post:
      tags: [Identity]
      operationId: issuePassport
      x-agent-access: human-only   # human session only
      parameters:
        - &shared
          name: limit
          in: query
          schema: { type: integer }
      responses:
        '201': { $ref: '#/components/responses/Minted' }
  /auth/login:
    post:
      operationId: login
      x-agent-access: auth-bootstrap   # the session machinery itself
      responses:
        '200': { description: ok }
  /contacts:
    parameters:
      - $ref: '#/components/parameters/Cursor'
    get:
      tags: [Contacts]
      operationId: listContacts
      x-mcp-tool: { verb: list_records, tier: auto_execute, scope: read }
      parameters:
        - *shared
        - &reused
          name: q
          in: query
          schema: { type: string }
      responses:
        '200':
          description: ok
          content:
            application/json:
              schema: { $ref: '#/components/schemas/ContactList' }
    post:
      tags: [Contacts]
      operationId: createContact
      x-agent-access: human-only
      responses:
        '201': { description: ok }
  /digest:
    get:
      tags: [Contacts]
      operationId: getDigest
      security: [ { cookieAuth: [] } ]
      responses:
        '200': { description: ok }
  /public/preferences/{token}:
    get:
      tags: [Contacts]
      operationId: getPreferences
      security: []
      responses:
        '200': { description: ok }
  /ext/endpoints:
    put:
      tags: [Contacts]
      operationId: openEndpoint
      x-agent-access:
        access: human-only
        verb: open_endpoint
      responses:
        '200': { description: ok }
  /contacts/{id}/merge:
    post:
      tags: [Contacts]
      operationId: mergeContact
      x-mcp-tool: { verb: merge_records, tier: confirmation_required, scope: write }
      summary: Merge a contact (ADR-0042) into a target.
      description: |
        Merges this contact into the target (ADR-0042), keeping both histories.

        Second paragraph, for the contract's developers.
      responses:
        '200': { description: ok }
  /contacts/{id}/brief:
    post:
      tags: [Contacts]
      operationId: draftBrief
      x-mcp-tool: { verb: draft_brief, tier: auto_execute, scope: draft }
      x-waits-on-model: always
      summary: Selects WHERE archived_at IS NULL from contact_note.
      description: Reads the contact_note table.
      responses:
        '200': { description: ok }
  /contacts/{id}:
    get:
      tags: [Contacts]
      operationId: getContact
      parameters:
        - &reused
          name: id
          in: path
          required: true
          schema: { type: string }
        - *reused
      responses:
        '200': { description: ok }
components:
  securitySchemes:
    bearerAuth: { type: http, scheme: bearer }
    cookieAuth: { type: apiKey, in: cookie, name: crm_session }
  parameters:
    Cursor: { name: cursor, in: query, schema: { type: string } }
  responses:
    Minted: { description: minted }
  schemas:
    ContactList:
      type: object
      x-go-type: ContactList
      properties:
        data: { type: array, items: { $ref: '#/components/schemas/Contact' } }
    Contact:
      type: object
      properties:
        name: { type: string, x-extension: true }
    Unreferenced: { type: string }
`

// tables stands in for the schema catalog's table names.
var tables = []string{"contact_note"}

func generate(t *testing.T) (*yaml.Node, string) {
	t.Helper()
	out, _, operations, err := agentContract([]byte(fixture), tables)
	if err != nil {
		t.Fatalf("agentContract: %v", err)
	}
	if operations != 4 {
		t.Errorf("operations = %d, want 4 (listContacts, mergeContact, draftBrief, getContact)", operations)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(out, &doc); err != nil {
		t.Fatalf("the output is not YAML: %v\n%s", err, out)
	}
	return oasnode.Root(&doc), string(out)
}

func at(node *yaml.Node, keys ...string) (*yaml.Node, bool) {
	for _, key := range keys {
		next, ok := oasnode.Lookup(node, key)
		if !ok {
			return nil, false
		}
		node = next
	}
	return node, true
}

func TestAHumanOnlyOperationIsDropped(t *testing.T) {
	root, _ := generate(t)
	if _, ok := at(root, "paths", "/passports"); ok {
		t.Error("/passports is still listed; its only operation is human-only, so the path must go with it")
	}
	if _, ok := at(root, "paths", "/contacts", "post"); ok {
		t.Error("createContact is human-only and is still listed")
	}
	if _, ok := at(root, "paths", "/contacts", "get"); !ok {
		t.Error("listContacts lost: an operation beside a human-only one must stay")
	}
}

func TestAnAuthBootstrapOperationIsDropped(t *testing.T) {
	root, _ := generate(t)
	if _, ok := at(root, "paths", "/auth/login"); ok {
		t.Error("/auth/login is auth-bootstrap and is still listed")
	}
}

func TestAnOperationWhoseSecurityRefusesAPassportIsDropped(t *testing.T) {
	root, _ := generate(t)
	if _, ok := at(root, "paths", "/digest"); ok {
		t.Error("/digest declares cookieAuth only, so a passport cannot call it, and it is still listed")
	}
	if _, ok := at(root, "paths", "/public/preferences/{token}"); ok {
		t.Error("/public/preferences/{token} declares security: [], which no passport authenticates, and it is still listed")
	}
}

func TestAMappingAgentAccessIsReadAsItsAccess(t *testing.T) {
	root, _ := generate(t)
	if _, ok := at(root, "paths", "/ext/endpoints"); ok {
		t.Error("an extension's x-agent-access mapping says human-only under access, and the operation is still listed")
	}
}

func TestAnOperationSaysWhatTheGateWillDoWithIt(t *testing.T) {
	root, _ := generate(t)
	merge, _ := at(root, "paths", "/contacts/{id}/merge", "post")
	want := "Merges this contact into the target, keeping both histories.\n\n" +
		"Needs the passport permission Change records.\n\nWaits for a human to approve."
	if got := oasnode.Scalar(merge, "description"); got != want {
		t.Errorf("mergeContact description = %q, want %q", got, want)
	}
	if got := oasnode.Scalar(merge, "summary"); got != "Merge a contact into a target." {
		t.Errorf("mergeContact summary = %q; the decision citation must go", got)
	}
	brief, _ := at(root, "paths", "/contacts/{id}/brief", "post")
	want = "Needs the passport permission Draft messages.\n\nRuns at once.\n\n" +
		"Holds the request open while an AI model answers."
	if got := oasnode.Scalar(brief, "description"); got != want {
		t.Errorf("draftBrief description = %q, want only the gate's sentences, its prose being a developer note", got)
	}
	if _, ok := at(brief, "summary"); ok {
		t.Error("draftBrief keeps a summary that names storage")
	}
}

func TestTheIndexListsEveryKeptOperationOnce(t *testing.T) {
	_, index, _, err := agentContract([]byte(fixture), tables)
	if err != nil {
		t.Fatalf("agentContract: %v", err)
	}
	text := string(index)
	for _, row := range []string{
		"| `GET /contacts` | listContacts |  | any | at once |",
		"| `POST /contacts/{id}/merge` | mergeContact | Merge a contact into a target. | Change records | waits for approval |",
		"| `GET /contacts/{id}` | getContact |",
	} {
		if strings.Count(text, row) != 1 {
			t.Errorf("the index does not carry the row %q once:\n%s", row, text)
		}
	}
	for _, dropped := range []string{"issuePassport", "getDigest", "openEndpoint"} {
		if strings.Contains(text, dropped) {
			t.Errorf("the index lists %s, which a passport cannot call", dropped)
		}
	}
}

func TestAnAliasIsExpandedWhereItsAnchorWasDropped(t *testing.T) {
	root, out := generate(t)
	if strings.Contains(out, "*shared") || strings.Contains(out, "&shared") || strings.Contains(out, "*reused") {
		t.Fatalf("the output still carries an anchor or alias:\n%s", out)
	}
	params, ok := at(root, "paths", "/contacts", "get", "parameters")
	if !ok || len(params.Content) != 2 {
		t.Fatalf("listContacts parameters = %v, want the two the source lists", params)
	}
	if got := oasnode.Scalar(params.Content[0], "name"); got != "limit" {
		t.Errorf("the alias of a parameter anchored in a dropped operation reads name %q, want limit", got)
	}
	byID, _ := at(root, "paths", "/contacts/{id}", "get", "parameters")
	for i, param := range byID.Content {
		if got := oasnode.Scalar(param, "name"); got != "id" {
			t.Errorf("getContact parameter %d is %q; the reused anchor name must resolve to its nearer definition, id", i, got)
		}
	}
}

func TestAnUnreferencedComponentIsDropped(t *testing.T) {
	root, _ := generate(t)
	if _, ok := at(root, "components", "schemas", "Unreferenced"); ok {
		t.Error("Unreferenced is kept, though no kept operation reaches it")
	}
	if _, ok := at(root, "components", "responses"); ok {
		t.Error("components.responses is kept; its one entry is used only by a human-only operation")
	}
	if _, ok := at(root, "components", "securitySchemes", "cookieAuth"); ok {
		t.Error("cookieAuth is kept, though no kept operation names it")
	}
}

func TestATransitivelyReferencedComponentIsKept(t *testing.T) {
	root, _ := generate(t)
	for _, path := range [][]string{
		{"components", "schemas", "ContactList"},
		{"components", "schemas", "Contact"},
		{"components", "parameters", "Cursor"},
		{"components", "securitySchemes", "bearerAuth"},
	} {
		if _, ok := at(root, path...); !ok {
			t.Errorf("%s is missing, though a kept operation reaches it", strings.Join(path, "."))
		}
	}
}

func TestVendorKeysServersAndUnusedTagsAreStripped(t *testing.T) {
	root, out := generate(t)
	if strings.Contains(out, "x-") {
		t.Errorf("the output still carries an x-* key:\n%s", out)
	}
	if _, ok := at(root, "servers"); ok {
		t.Error("servers is kept; only the running install knows its address")
	}
	tags, _ := at(root, "tags")
	if len(tags.Content) != 1 || oasnode.Scalar(tags.Content[0], "name") != "Contacts" {
		t.Errorf("tags = %d entries, want only Contacts, the one tag a kept operation uses", len(tags.Content))
	}
	if strings.Contains(out, "human session only") {
		t.Error("a source comment survived; comments are dropped with the operations they describe")
	}
}

func TestADanglingRefRefusesToGenerate(t *testing.T) {
	broken := strings.Replace(fixture, "#/components/schemas/Contact'", "#/components/schemas/Missing'", 1)
	if _, _, _, err := agentContract([]byte(broken), tables); err == nil {
		t.Error("a ref to a component the contract does not declare generated anyway")
	}
}
