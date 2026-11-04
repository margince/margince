// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"encoding/base64"
	"encoding/json"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The ids a sweep cannot seed exist nowhere, so a call that takes one changes nothing.
const placeholderUUID = "00000000-0000-4000-8000-000000000001"

// recordPool hands each call a fresh record the sweep's user owns. So a gate
// behind a lookup is met, and one call's archive never reads as the next call's
// refusal. Counting mode answers placeholders, to learn how many to seed.
type recordPool struct {
	fresh  map[string][]string
	wanted map[string]int
	// sweepStart anchors every date a call sends, so a run answers the same
	// way whichever day it runs.
	sweepStart time.Time
	// taken, when set, collects the records handed out by kind.
	taken map[string][]string
}

func (p *recordPool) take(kind string) string {
	if p.wanted != nil {
		p.wanted[kind]++
		return placeholderUUID
	}
	ids := p.fresh[kind]
	p.fresh[kind] = ids[1:]
	if p.taken != nil {
		p.taken[kind] = append(p.taken[kind], ids[0])
	}
	return ids[0]
}

// recordKinds names the record a sweep seeds for an argument, or for the path
// segment before a route's parameter.
//
// gatekit:fixture the record kind each argument or collection names — expected data, not a waived cost
var recordKinds = map[string]string{
	"company_id": "company", "entity_id": "company", "record_id": "company", "companies": "company", "records": "company",
	"contact_id": "contact", "contacts": "contact",
	"deal_id": "deal", "deals": "deal",
	"lead_id": "lead", "leads": "lead",
	"activity_id": "activity", "activities": "activity", "source_activity_id": "activity",
	"attachment_id": "attachment", "attachments": "attachment",
	"project_id": "project", "projects": "project",
	"tag_id": "tag", "into_tag_id": "tag", "tags": "tag",
	"users": "user", "userId": "user", "user_id": "user", "to_user_id": "user", "via_user_id": "user",
	"introducer_user_id": "user", "introduced_user_id": "user", "recipient_user_id": "user", "owner_id": "user",
	"host_user_id": "user", "to_id": "user",
	"teams": "team", "legal-entities": "legal_entity", "deal-rooms": "deal_room", "offers": "offer",
	"contracts": "contract", "lead-sources": "lead_source", "lead-origins": "lead_origin",
	"lead-disqualify-reasons": "lead_disqualify_reason", "acquisition-sources": "acquisition_source",
	"loss-reasons": "loss_reason", "record-roles": "record_role", "role_id": "record_role",
	"intro-requests": "intro_request", "voice-profiles": "voice_profile", "corpora": "corpus",
	"web-forms": "web_form", "views": "saved_view", "lists": "list", "list_id": "list",
	"initiatives": "initiative", "initiative_id": "initiative",
	"commitments": "commitment", "assignments": "assignment",
}

func recordKindOf(name string) string { return recordKinds[name] }

// pathRecordKind names the record a route's path parameter takes: by its own
// name, or by the collection segment before it.
func pathRecordKind(pattern, param string) string {
	before, _, found := strings.Cut(pattern, "/{"+param+"}")
	if !found || !isIDName(param) {
		return ""
	}
	if kind := recordKindOf(param); kind != "" {
		return kind
	}
	for _, segment := range slices.Backward(strings.Split(before, "/")) {
		if !strings.HasPrefix(segment, "{") {
			return recordKindOf(segment)
		}
	}
	return ""
}

func (c toolRoleCall) args(pool *recordPool) map[string]any {
	args := fillSchema(c.input, c.route.Pattern, pool)
	if _, takesBody := args["request_body"]; takesBody && len(c.route.Body) > 0 {
		args["request_body"] = fillSchema(c.route.Body, "", pool)
	}
	if fixture, ok := toolRoleFixtures[c.verb]; ok {
		for name, value := range fixture(pool) {
			if body, isBody := value.(map[string]any); isBody && name == "request_body" {
				merged, _ := args["request_body"].(map[string]any)
				args["request_body"] = mergedBody(merged, body)
				continue
			}
			args[name] = value
		}
	}
	return args
}

//craft:ignore naked-any a request body is decoded JSON, whatever its shape
func mergedBody(base, over map[string]any) map[string]any {
	merged := map[string]any{}
	maps.Copy(merged, base)
	maps.Copy(merged, over)
	return merged
}

// schemaProp is the part of a property's JSON schema the filler reads.
type schemaProp struct {
	Type       any                        `json:"type"`
	Format     string                     `json:"format"`
	Enum       []any                      `json:"enum"`
	Default    any                        `json:"default"`
	Examples   []any                      `json:"examples"`
	MinItems   int                        `json:"minItems"`
	Minimum    *float64                   `json:"minimum"`
	Items      json.RawMessage            `json:"items"`
	Required   []string                   `json:"required"`
	Properties map[string]json.RawMessage `json:"properties"`
}

// typeName is the schema's one non-null type, as a nullable property lists two.
func (p schemaProp) typeName() string {
	switch typed := p.Type.(type) {
	case string:
		return typed
	case []any:
		for _, each := range typed {
			if name, ok := each.(string); ok && name != "null" {
				return name
			}
		}
	}
	return ""
}

// fillSchema fills a schema's required arguments, so the call passes the schema
// and meets the gate behind it.
//
//craft:ignore naked-any a JSON schema is walked as decoded JSON, whatever its shape
func fillSchema(raw json.RawMessage, pattern string, pool *recordPool) map[string]any {
	args := map[string]any{}
	var shape schemaProp
	if err := json.Unmarshal(raw, &shape); err != nil {
		return args
	}
	for _, name := range shape.Required {
		args[name] = fillValue(name, shape.Properties[name], pattern, pool)
	}
	// An id beside a record type names a record of that type, filled as a company.
	if args["record_type"] == "company" && args["id"] == placeholderUUID {
		args["id"] = pool.take("company")
	}
	if _, takesBody := shape.Properties["request_body"]; takesBody {
		args["request_body"] = map[string]any{}
	}
	return args
}

// namedValues are the strings a field's name settles, whatever the schema says.
//
// gatekit:fixture a valid value for each well-known field name — expected data, not a waived cost
var namedValues = map[string]string{
	"country": "de", "currency": "EUR", "locale": "en", "content_type": "application/pdf",
	"content_base64": base64.StdEncoding.EncodeToString([]byte("%PDF-1.4 role sweep")), "filename": "role-sweep.pdf",
	"scheduled_tz": "Europe/Berlin", "timezone": "Europe/Berlin", "source": "manual",
	"text_hash": strings.Repeat("0", 32), "url": "https://example.test/role-sweep",
	"role": "rep", "group_key": "commercial", "name": "Role Sweep",
}

//craft:ignore naked-any a JSON schema's value takes whichever type the schema names
func fillValue(name string, raw json.RawMessage, pattern string, pool *recordPool) any {
	if name == "entity_type" || name == "record_type" {
		return "company"
	}
	if kind := pathRecordKind(pattern, name); kind != "" {
		return pool.take(kind)
	}
	if kind := recordKindOf(name); kind != "" {
		return pool.take(kind)
	}
	if value, named := namedValues[name]; named {
		return value
	}
	var prop schemaProp
	if err := json.Unmarshal(raw, &prop); err != nil {
		return placeholderUUID
	}
	switch {
	case len(prop.Examples) > 0:
		return prop.Examples[0]
	case prop.Default != nil:
		return prop.Default
	case len(prop.Enum) > 0:
		return prop.Enum[0]
	}
	return fillTyped(name, prop, raw, pool)
}

//craft:ignore naked-any a JSON schema's value takes whichever type the schema names
func fillTyped(name string, prop schemaProp, raw json.RawMessage, pool *recordPool) any {
	switch prop.typeName() {
	case "integer", "number":
		if prop.Minimum != nil && *prop.Minimum > 1 {
			return *prop.Minimum
		}
		return 1
	case "boolean":
		return false
	case "array":
		items := []any{}
		for range prop.MinItems {
			items = append(items, fillValue("", prop.Items, "", pool))
		}
		return items
	case "object":
		return fillSchema(raw, "", pool)
	}
	return fillString(name, prop.Format, pool)
}

// fillString answers a string by its format. A date is after the sweep began,
// and one whose name ends a span comes a day after one that starts it.
func fillString(name, format string, pool *recordPool) string {
	day := pool.sweepStart.Add(30 * 24 * time.Hour)
	if endsASpan(name) {
		day = day.Add(24 * time.Hour)
	}
	switch format {
	case "date-time":
		return day.Format(time.RFC3339)
	case "date":
		return day.Format(time.DateOnly)
	case "email":
		return "sweep-" + ids.NewV7().String() + "@role-sweep.test"
	case "uri":
		return namedValues["url"]
	case "uuid":
		return placeholderUUID
	}
	switch {
	case strings.Contains(name, "email"):
		return fillString(name, "email", pool)
	case isIDName(name):
		return placeholderUUID
	}
	return "rolesweep"
}

func isIDName(name string) bool {
	return name == "id" || strings.HasSuffix(name, "_id") || strings.HasSuffix(name, "Id")
}

func endsASpan(name string) bool {
	return name == "to" || strings.HasPrefix(name, "end") || strings.Contains(name, "_end") ||
		strings.Contains(name, "effective") || strings.Contains(name, "until") || strings.Contains(name, "expires")
}
