// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  reportingStoryCatalog,
  reportingStoryEvaluation,
  reportingStoryFramework,
} from "../../src/screens/reporting.fixtures";
import * as ai from "./ai";
import * as booking from "./booking";
import * as capture from "./capture";
import {
  aiCalls,
  aiHealth,
  aiProviderHealth,
  aiProviderKeys,
  aiRouting,
  aiUsage,
  anna,
  approval,
  automationCatalog,
  brandt,
  captureActivity,
  jobHealth,
  knowledgeCorpora,
  passports,
  seats,
  seededLead,
} from "./fixtures";
import * as insight from "./insight";
import * as lists from "./lists";
import * as records from "./records";
import { type MockRoute, notFound, page, reply } from "./server";
import * as session from "./session";
import * as work from "./work";

const empty = reply(page([]));

// Many routes are answered only so that the catch-all's list envelope never
// reaches a screen that reads an entity: that envelope takes the screen down.
//
// Order is behaviour. The first row that answers wins, so an exact row sits
// above any prefix row that would also match it.
export const routes: readonly MockRoute[] = [
  { path: "/me", handle: session.me },
  { path: "/me/working-hours", handle: reply(session.workingHours) },
  {
    method: "GET",
    path: "/me/notification-preferences",
    handle: reply(session.notificationPreferences),
  },
  {
    method: "GET",
    path: (path) => path.startsWith("/installation/oauth-apps/"),
    handle: session.oauthApp,
  },
  {
    method: "GET",
    path: "/installation/settings",
    handle: reply(session.installationSettings),
  },
  // The anonymous reads the signed-out surface makes, by design.
  { path: "/embeddings/reindex/status", handle: session.reindexStatus },
  { path: "/auth/capabilities", handle: session.authCapabilities },
  { path: "/assistant/profile", handle: reply(session.assistantProfile) },
  { path: () => true, handle: records.projects },
  {
    method: "PATCH",
    path: (path) => path.startsWith("/deals/"),
    handle: records.patchDeal,
  },
  {
    path: "/company/context/capabilities",
    handle: reply(session.companyContextCapabilities),
  },
  {
    method: "GET",
    path: "/installation/setup",
    handle: session.installationSetup,
  },
  // An undescribed installation has no company, which sends the shell into
  // onboarding.
  {
    path: "/company",
    when: (state) => state.undescribed,
    handle: reply(notFound, 404),
  },
  { method: "GET", path: "/onboarding/state", handle: session.onboardingState },
  { path: "/company", handle: reply(session.company) },
  { method: "GET", path: "/contacts", handle: reply(page([anna])) },
  { method: "POST", path: "/contacts", handle: records.createContact },
  { path: "/contacts/p-new", handle: reply(records.newContact) },
  { path: "/contacts/p-jonas", handle: reply(records.jonas) },
  { method: "POST", path: "/companies", handle: records.createCompany },
  { path: "/companies/o-new", handle: reply(records.newCompany) },
  { method: "POST", path: "/leads", handle: records.createLead },
  { path: "/leads/l-new", handle: reply(records.createdLead) },
  { method: "POST", path: "/deals", handle: records.createDeal },
  { path: "/contacts/p-anna", handle: reply(anna) },
  {
    method: "GET",
    path: /^\/contacts\/[^/]+\/360$/,
    handle: reply(records.contact360),
  },
  {
    method: "GET",
    path: /^\/activities\/[^/]+\/meeting-brief$/,
    handle: records.meetingBrief,
  },
  {
    method: "GET",
    path: /^\/contacts\/[^/]+\/brief$/,
    handle: reply(records.contactBrief),
  },
  {
    method: "GET",
    path: /^\/contacts\/[^/]+\/consent\/guard$/,
    handle: reply({ entries: [] }),
  },
  {
    method: "GET",
    path: "/contacts/p-anna/consent",
    handle: reply({ state: [], events: [] }),
  },
  { method: "GET", path: "/companies", handle: reply(page([brandt])) },
  {
    method: "GET",
    path: /^\/companies\/[^/]+\/360$/,
    handle: reply(records.company360),
  },
  { path: "/companies/o-brandt", handle: reply(brandt) },
  { method: "GET", path: "/leads", handle: reply(page([seededLead])) },
  { method: "GET", path: "/leads/l-1", handle: reply(seededLead) },
  { method: "GET", path: "/lead-sources", handle: reply(records.leadSources) },
  {
    method: "GET",
    path: "/lead-disqualify-reasons",
    handle: reply(records.disqualifyReasons),
  },
  {
    method: "GET",
    path: "/leads/settings",
    handle: reply({
      first_response_enabled: false,
      first_response_target_minutes: 240,
    }),
  },
  { method: "PATCH", path: "/leads/l-1", handle: records.patchLead },
  {
    path: "/leads/l-1/score",
    handle: reply({ score: seededLead.score, explained: false }),
  },
  { path: "/leads/l-1/promote-preview", handle: reply({ outcome: "create" }) },
  {
    method: "POST",
    path: "/leads/l-1/promote",
    handle: reply(records.promotedLead),
  },
  { path: "/users", handle: reply(page(seats)) },
  { path: "/users/names", handle: records.userNames },
  { path: "/pipelines", handle: reply(records.pipelines) },
  {
    method: "GET",
    path: "/filters/vocabulary",
    handle: lists.filterVocabulary,
  },
  { method: "POST", path: "/filters/preview", handle: lists.filterPreview },
  { method: "GET", path: "/lists", handle: empty },
  { method: "GET", path: "/views", handle: lists.listViews },
  { method: "POST", path: "/views", handle: lists.createView },
  { path: /^\/views\/([^/]+)$/, handle: lists.viewById },
  { method: "GET", path: "/deals", handle: records.listDeals },
  {
    method: "POST",
    path: /^\/deals\/([^/]+)\/advance$/,
    handle: records.advanceDeal,
  },
  {
    path: (path) =>
      path.startsWith("/deals/") && path.endsWith("/stakeholders"),
    handle: empty,
  },
  { path: (path) => path.startsWith("/deals/"), handle: records.getDeal },
  { method: "GET", path: "/magic", handle: reply(work.magicReceipt) },
  { method: "GET", path: "/brief", handle: work.getBrief },
  { method: "POST", path: "/brief", handle: work.runBrief },
  {
    method: "POST",
    path: /^\/brief\/items\/([^/]+)\/(act|dismiss)$/,
    handle: work.markBriefItem,
  },
  { path: "/approvals", handle: reply(page([approval])) },
  { method: "GET", path: "/worklist", handle: reply(work.worklist) },
  { method: "GET", path: work.isOneApproval, handle: reply(approval) },
  {
    method: "POST",
    path: (path) => path.startsWith("/approvals/"),
    handle: work.decideApproval,
  },
  { method: "POST", path: "/activities", handle: work.logActivity },
  {
    method: "POST",
    path: /^\/activities\/([^/]+)\/relink$/,
    handle: work.relinkActivity,
  },
  { path: "/activities", handle: work.listActivities },
  { path: "/consent-purposes", handle: reply(work.consentPurposes) },
  { path: "/data-subject-requests", handle: empty },
  { method: "GET", path: "/passports", handle: reply({ data: passports }) },
  { path: "/audit-log", handle: work.auditLog },
  { path: "/automations/catalog", handle: reply({ data: automationCatalog }) },
  { method: "GET", path: "/automations", handle: work.listAutomations },
  { method: "POST", path: "/automations", handle: work.createAutomation },
  {
    path: (path) => path.startsWith("/automations/"),
    handle: work.automationById,
  },
  { path: "/scheduling/profile", handle: reply(booking.schedulingProfile) },
  {
    path: "/public/booking/host-1/profile",
    handle: reply(booking.schedulingProfile),
  },
  { path: "/scheduling/calendars", handle: reply(booking.calendars) },
  {
    path: "/public/meeting/guest-booking",
    handle: reply(booking.guestBooking),
  },
  {
    path: "/public/booking/host-1/availability",
    handle: reply(booking.hostAvailability),
  },
  { method: "POST", path: "/public/booking/host-1", handle: booking.bookSlot },
  { path: "/availability", handle: reply(booking.availability) },
  { path: "/search", handle: insight.search },
  { path: "/analytics/context", handle: reply(insight.analyticsContext) },
  { path: "/forecast", handle: reply(insight.forecast) },
  { path: "/analytics/framework", handle: reply(reportingStoryFramework) },
  { path: "/analytics/metrics", handle: reply(reportingStoryCatalog) },
  { path: "/analytics/evaluate", handle: reply(reportingStoryEvaluation) },
  {
    path: (path) =>
      path.startsWith("/reports/") && path.endsWith("/derivation"),
    handle: insight.reportDerivation,
  },
  {
    path: (path) =>
      path.startsWith("/reports/") && !path.includes("/derivation"),
    handle: insight.report,
  },
  // Entity reads the 360 fires.
  {
    method: "GET",
    path: (path) => path.endsWith("/partner"),
    handle: reply({ code: "not_found", title: "no partner" }, 404),
  },
  {
    path: (path) => path.endsWith("/hierarchy-rollup"),
    handle: reply(insight.hierarchyRollup),
  },
  // The context panel reads `{sections: []}`. Matched by substring, so any
  // later `/context` route needs its own row above this one.
  {
    path: (path) => path.includes("/context"),
    handle: reply({ anchor: { type: "contact", id: "x" }, sections: [] }),
  },
  { path: "/digest", handle: reply(insight.digest) },
  { path: "/capture/activity", handle: reply(captureActivity) },
  { path: "/capture/activity/workspace", handle: reply(captureActivity) },
  {
    method: "GET",
    path: "/knowledge/corpora",
    handle: reply(knowledgeCorpora),
  },
  { path: "/installation/license", handle: ai.license },
  {
    path: "/me/ai-activity",
    when: (state) => state.options.agentRunning === true,
    handle: ai.runningActivity,
  },
  { method: "GET", path: "/ai/routing", handle: reply(aiRouting) },
  { path: "/ai/status", handle: ai.aiStatus },
  { path: "/ai/budget", handle: ai.aiBudget },
  { path: "/ai/health", handle: reply(aiHealth) },
  { path: "/ai/usage", handle: reply(aiUsage) },
  { method: "GET", path: "/ai/provider-keys", handle: reply(aiProviderKeys) },
  { path: "/ai/provider-health", handle: reply(aiProviderHealth) },
  { method: "GET", path: "/ai/calls", handle: reply(aiCalls) },
  { path: "/ai/call-stats", handle: ai.callStats },
  { path: "/ai/call-stats/flow", handle: ai.callStatsFlow },
  { method: "GET", path: "/ai/task-overrides", handle: reply({}) },
  { path: "/ai/routing/schema", handle: reply({}) },
  { path: "/admin/job-health", handle: reply(jobHealth) },
  { path: "/capture/settings", handle: capture.captureSettings },
  { path: "/capture/senders", handle: capture.senders },
  {
    path: (path) =>
      path.startsWith("/capture/senders/") && path.endsWith("/decision"),
    handle: capture.senderDecision,
  },
  { path: "/capture/counterparty-holds", handle: capture.counterpartyHolds },
  {
    path: (path) => path.startsWith("/capture/counterparty-holds/"),
    handle: capture.releaseHold,
  },
  {
    path: (path) =>
      path.startsWith("/connectors/") && path.endsWith("/mail-posture"),
    handle: capture.mailPosture,
  },
  { path: "/connectors", handle: capture.connectors },
  { path: "/agent-tools", handle: reply(capture.agentTools) },
];
