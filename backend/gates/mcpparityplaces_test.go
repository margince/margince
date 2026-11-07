// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gates

// Where each operation lands in the MCP parity catalog: the one hand-kept table
// the page and the running server's work areas (internal/platform/workarea) have. The contract's tags are close to what a user would turn on or off.
// Yet Identity alone holds a user's own password, the member list and the health
// pages. So a path rule can overrule a tag. A tag in parityMixedTags has no default,
// and a new route under it fails until someone places it.

import "strings"

// A parity group is one question the later product step can ask a user.
const (
	groupBusiness   = "business"
	groupCompliance = "compliance"
	groupCapture    = "capture"
	groupWorkspace  = "workspace_settings"
	groupAdmin      = "admin_settings"
	groupOwnAccount = "own_account"
	groupGovernance = "agent_governance"
	groupOperator   = "operator"
	groupPublic     = "public"
	// groupSettingsByGrant marks an area whose operations land in groupWorkspace or
	// groupAdmin, one by one, by whether the seeded rep role passes the check.
	groupSettingsByGrant = "settings_by_grant"
)

// parityGroupInfo.Passport is the name a user chooses the group by on a
// passport. A group without one is either Free, admitted on every passport, or
// HeldAs the group whose switch also covers it.
type parityGroupInfo struct {
	Key, Title, What, Passport string
	Free                       bool
	HeldAs                     string
}

var parityGroupInfos = []parityGroupInfo{
	{groupBusiness, "Business operations", "The daily sales work: records, deals, activities, offers, reports and lists.", "Sales work", false, ""},
	{groupCompliance, "Compliance and privacy", "Consent, privacy requests, notices, retention and the audit trail.", "Compliance and privacy", false, ""},
	{groupCapture, "Capture and connectors", "What comes in from mail and calendars, and the connections it comes through.", "Email and calendar capture", false, ""},
	{groupWorkspace, "Workspace settings a non-admin can reach", "How the workspace is set up, where the seeded `rep` role passes the check. Most of these are reads.", "Company settings", false, ""},
	{groupAdmin, "Admin settings", "How the workspace is set up, where the seeded `rep` role is refused.", "Admin settings", false, ""},
	{groupOwnAccount, "The user's own account and security", "The user's profile, preferences, password, sessions and second factor.", "My account", false, ""},
	// An agent reads and answers its own approvals here, so no passport may lose them.
	{groupGovernance, "Agent governance", "What limits agents: passports, connected clients, approvals and autonomy.", "", true, ""},
	{groupOperator, "Operator and system", "Running the installation: health, repair, license and the search index.", "", false, groupAdmin},
	{groupPublic, "Public, with no signed-in user", "Sign-in steps and pages for visitors from outside the workspace. No agent acts here, because no signed-in user does.", "", false, ""},
}

type parityArea struct{ Key, Title, Group string }

// parityAreas are the things a user could enable or disable together.
var parityAreas = []parityArea{
	{"contacts", "Contacts and relationships", groupBusiness},
	{"duplicates", "Duplicates", groupBusiness},
	{"companies", "Companies", groupBusiness},
	{"partners", "Partners", groupBusiness},
	{"deals", "Deals", groupBusiness},
	{"deal_rooms", "Deal rooms", groupBusiness},
	{"legal_reviews", "Legal reviews", groupBusiness},
	{"leads", "Leads", groupBusiness},
	{"activities", "Activities, mail and tasks", groupBusiness},
	{"scheduling", "Scheduling and bookings", groupBusiness},
	{"offers", "Offers", groupBusiness},
	{"contracts", "Contracts", groupBusiness},
	{"projects", "Projects", groupBusiness},
	{"reports", "Reports and analytics", groupBusiness},
	{"forecast", "Forecast", groupBusiness},
	{"commissions", "Commissions", groupBusiness},
	{"lists", "Lists and saved views", groupBusiness},
	{"tags", "Tags", groupBusiness},
	{"bulk", "Bulk changes", groupBusiness},
	{"search", "Search and record context", groupBusiness},
	{"worklist", "Brief, worklist, weekly plan and notices", groupBusiness},
	{"signals", "Signals", groupBusiness},
	{"documents", "Documents", groupBusiness},
	{"imports", "Imports", groupBusiness},
	{"knowledge", "Knowledge library", groupBusiness},
	{"voice", "Voice profiles", groupBusiness},
	{"sharing", "Sharing, assignment and ownership", groupBusiness},
	{"ai_help", "AI help on records", groupBusiness},

	{"consent", "Consent and communication review", groupCompliance},
	{"privacy_requests", "Data subject requests", groupCompliance},
	{"privacy_notices", "Privacy notices and controller details", groupCompliance},
	{"retention", "Retention and legal holds", groupCompliance},
	{"audit", "Audit log and record history", groupCompliance},
	{"exports", "Data exports", groupCompliance},

	{"capture_triage", "Capture review", groupCapture},
	{"connectors", "Connectors and provider connections", groupCapture},
	{"finance_source", "Accounting source", groupCapture},

	{"pipelines", "Pipelines and stages", groupSettingsByGrant},
	{"sales_lists", "Sales lists and reasons", groupSettingsByGrant},
	{"sales_rules", "Sales rules and templates", groupSettingsByGrant},
	{"company_profile", "Your company and legal entities", groupSettingsByGrant},
	{"custom_fields", "Custom fields", groupSettingsByGrant},
	{"catalog", "Products and offer templates", groupSettingsByGrant},
	{"rates", "Exchange, tax and model rates", groupSettingsByGrant},
	{"automations", "Automations", groupSettingsByGrant},
	{"members", "Members, teams and roles", groupSettingsByGrant},
	{"ai_models", "AI models and budget", groupSettingsByGrant},
	{"capture_rules", "Capture and integration rules", groupSettingsByGrant},
	{"web_forms", "Web forms", groupSettingsByGrant},
	{"webhooks", "Webhooks", groupSettingsByGrant},
	{"extensions", "Extensions", groupSettingsByGrant},
	{"installation", "Installation settings", groupSettingsByGrant},

	{"own_profile", "Profile and preferences", groupOwnAccount},
	{"own_security", "Password, sessions and second factor", groupOwnAccount},

	{"passports", "Passports", groupGovernance},
	{"agent_clients", "Connected AI clients", groupGovernance},
	{"agent_limits", "Approvals, autonomy and agent grants", groupGovernance},

	{"system_health", "Health, data tasks and repair", groupOperator},
	{"system_facts", "License, seats and configuration", groupOperator},
	{"search_index", "Search index", groupOperator},

	{"sign_in", "Sign-in and password reset", groupPublic},
	{"oauth_callbacks", "Provider sign-in returns", groupPublic},
	{"buyer_rooms", "Buyer deal rooms", groupPublic},
	{"booking_pages", "Booking and meeting pages", groupPublic},
	{"public_links", "Forms, confirmations and preference links", groupPublic},
}

// parityTagAreas is the default area of an operation's first tag.
//
// gatekit:fixture the area each contract tag defaults to
var parityTagAreas = map[string]string{
	"Access":        "sharing",
	"Activities":    "activities",
	"Agents":        "agent_limits",
	"Approvals":     "agent_limits",
	"Assignments":   "sharing",
	"Attachments":   "documents",
	"Automations":   "automations",
	"Brief":         "worklist",
	"Bulk":          "bulk",
	"Capture":       "capture_triage",
	"Commissions":   "commissions",
	"Communication": "consent",
	"Companies":     "companies",
	"Connectors":    "connectors",
	"Contacts":      "contacts",
	"Contracts":     "contracts",
	"CustomFields":  "custom_fields",
	"Deals":         "deals",
	"Finance":       "finance_source",
	"Import":        "imports",
	"Knowledge":     "knowledge",
	"Leads":         "leads",
	"Lists":         "lists",
	"Notices":       "worklist",
	"Offers":        "offers",
	"Partners":      "partners",
	"Pipelines":     "pipelines",
	"Projects":      "projects",
	"Rates":         "rates",
	"Relationships": "contacts",
	"Reports":       "reports",
	"Search":        "search",
	"Signals":       "signals",
	"Tags":          "tags",
	"Views":         "lists",
	"Voice":         "voice",
	"WebForms":      "web_forms",
	"Webhooks":      "webhooks",
}

// parityMixedTags span several groups, so no route under one is placed by its tag.
var parityMixedTags = map[string]bool{
	"AI": true, "Compliance": true, "Extensions": true, "Identity": true,
	"Integrations": true, "Provenance": true, "Public": true,
}

// parityPathAreas overrule the tag. The longest prefix that matches wins.
//
// gatekit:fixture the area each contract path prefix lands in
var parityPathAreas = map[string]string{
	"/acquisition-sources":                "sales_lists",
	"/activities/follow-up-settings":      "sales_rules",
	"/activity-review-templates":          "sales_rules",
	"/admin":                              "system_health",
	"/agent-bundle":                       "passports",
	"/ai":                                 "ai_models",
	"/ai/agent-usage":                     "agent_limits",
	"/ai/feedback":                        "ai_help",
	"/assistant/profile":                  "sign_in",
	"/audit-log":                          "audit",
	"/auth":                               "sign_in",
	"/auth/change-password":               "own_security",
	"/auth/logout":                        "own_security",
	"/availability":                       "scheduling",
	"/bookings":                           "scheduling",
	"/capture/blocked-domains":            "capture_rules",
	"/capture/consumer-mail-baseline":     "capture_rules",
	"/capture/consumer-mail-domains":      "capture_rules",
	"/capture/email-domains":              "capture_rules",
	"/capture/exclusions":                 "capture_rules",
	"/capture/owner-identities":           "capture_rules",
	"/capture/settings":                   "capture_rules",
	"/channel-connections":                "connectors",
	"/channel-providers":                  "connectors",
	"/coldstart":                          "ai_help",
	"/company":                            "company_profile",
	"/confirm-submissions":                "consent",
	"/connectors":                         "connectors",
	"/connectors/{provider}/callback":     "oauth_callbacks",
	"/consent-purposes":                   "consent",
	"/contacts/{id}/acquisition-request":  "consent",
	"/contacts/{id}/consent":              "consent",
	"/contacts/{id}/enrichment-runs":      "contacts",
	"/data-subject-requests":              "privacy_requests",
	"/deal-rooms":                         "deal_rooms",
	"/deals/{id}/legal-reviews":           "legal_reviews",
	"/dedupe":                             "duplicates",
	"/dedupe/settings":                    "sales_rules",
	"/email-signature-template":           "sales_rules",
	"/embeddings":                         "search_index",
	"/existing-customer-notices":          "consent",
	"/exports":                            "exports",
	"/ext-oauth/{provider}/callback":      "oauth_callbacks",
	"/ext-oauth/{provider}/connect":       "extensions",
	"/extensions":                         "extensions",
	"/field-history":                      "audit",
	"/forecast":                           "forecast",
	"/installation":                       "installation",
	"/installation/agent-clients":         "agent_clients",
	"/installation/configuration":         "system_facts",
	"/installation/license":               "system_facts",
	"/installation/seat-usage":            "system_facts",
	"/installation/setup":                 "system_facts",
	"/integrations/settings":              "capture_rules",
	"/lead-disqualify-reasons":            "sales_lists",
	"/lead-origins":                       "sales_lists",
	"/lead-signal-catalog":                "sales_lists",
	"/lead-sources":                       "sales_lists",
	"/leads/{id}/consent":                 "consent",
	"/leads/settings":                     "sales_rules",
	"/legal-entities":                     "company_profile",
	"/legal-reviews":                      "legal_reviews",
	"/legal-reviews/settings":             "sales_rules",
	"/loss-reasons":                       "sales_lists",
	"/me":                                 "own_profile",
	"/me/agent-grants":                    "agent_limits",
	"/me/approval-preferences":            "agent_limits",
	"/me/mfa":                             "own_security",
	"/me/sessions":                        "own_security",
	"/oauth/consent-request":              "agent_clients",
	"/offer-templates":                    "catalog",
	"/onboarding/company":                 "company_profile",
	"/onboarding/state":                   "own_profile",
	"/openapi.json":                       "system_facts",
	"/partner-stages":                     "pipelines",
	"/passports":                          "passports",
	"/privacy":                            "privacy_notices",
	"/products":                           "catalog",
	"/provider-connections":               "connectors",
	"/public/booking":                     "booking_pages",
	"/public/confirm":                     "public_links",
	"/public/forms":                       "public_links",
	"/public/meeting":                     "booking_pages",
	"/public/preferences":                 "public_links",
	"/public/proposal":                    "booking_pages",
	"/public/record-previews":             "public_links",
	"/public/rooms":                       "buyer_rooms",
	"/record-roles":                       "members",
	"/records/{entity_type}/{id}/history": "audit",
	"/records/{record_type}":              "sharing",
	"/retention":                          "retention",
	"/retention-policies":                 "retention",
	"/roles":                              "members",
	"/scheduling":                         "scheduling",
	"/status":                             "sign_in",
	"/teams":                              "members",
	"/users":                              "members",
	"/whoami":                             "own_profile",
}

// parityAreaOf places one operation, and answers empty when nothing places it.
func parityAreaOf(path string, tags []string) string {
	best := ""
	for prefix := range parityPathAreas {
		if len(prefix) > len(best) && pathUnderPrefix(path, prefix) {
			best = prefix
		}
	}
	if best != "" {
		return parityPathAreas[best]
	}
	if len(tags) == 0 {
		return ""
	}
	return parityTagAreas[tags[0]]
}

// pathUnderPrefix matches whole segments, so /company never claims /companies. A
// contract path may add a custom verb to a segment, as /emails:preview does.
func pathUnderPrefix(path, prefix string) bool {
	rest, found := strings.CutPrefix(path, prefix)
	return found && (rest == "" || rest[0] == '/' || rest[0] == ':')
}
