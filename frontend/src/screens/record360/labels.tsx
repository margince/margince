// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Wire values turned into words, for every record page.
//
// Each map here shares one rule: the value arrives as an OPEN string even when
// the contract calls it an enum, so a value nobody mapped renders as its own
// words rather than vanishing or throwing. A role somebody typed is still a
// fact about that contact, and a signal kind added upstream must not make the
// tile disappear.
//
// The lookups are own-property checks, never a bare index. A wire value named
// `toString` or `constructor` finds something on Object's prototype, passes a
// truthy check, and renders as an empty badge — which reads exactly like a
// record with no role at all.

import type { MessageKey } from "../../i18n/en";

// What each signal kind is, in words. Keyed by plain string, matching how the
// value arrives: the strip's signal kind is an open wire string.
const SIGNAL_KIND_LABELS: Record<string, MessageKey> = {
  stalled_deal: "signal.kind.stalled_deal",
  champion_left: "signal.kind.champion_left",
  reengagement: "signal.kind.reengagement",
  buying_intent: "signal.kind.buying_intent",
  risk: "signal.kind.risk",
  other: "signal.kind.other",
  contract_ended: "signal.kind.contract_ended",
  new_opportunity: "signal.kind.new_opportunity",
  commitment_made: "signal.kind.commitment_made",
  ghosted_thread: "signal.kind.ghosted_thread",
  project_gone_quiet: "signal.kind.project_gone_quiet",
  funding: "signal.kind.funding",
  leadership_change: "signal.kind.leadership_change",
  expansion: "signal.kind.expansion",
  product_launch: "signal.kind.product_launch",
  technical_change: "signal.kind.technical_change",
};

/** signalKindLabel names a signal, degrading to its own words when unmapped. */
export function signalKindLabel(
  kind: string,
  t: (key: MessageKey) => string,
): string {
  const key = Object.hasOwn(SIGNAL_KIND_LABELS, kind)
    ? SIGNAL_KIND_LABELS[kind]
    : undefined;
  return key ? t(key) : kind.replaceAll("_", " ");
}

// How serious a signal is, in the strip's own vocabulary of tones. `info` is
// deliberately untoned: a record whose worst news is a commitment somebody
// made is a record with no bad news — colouring that would cry wolf on every
// healthy record.
const SIGNAL_TONE: Record<string, "warn" | "danger" | undefined> = {
  info: undefined,
  warn: "warn",
  urgent: "danger",
};

/** signalTone colours a signal by severity; an unknown severity is untoned. */
export function signalTone(severity: string): "warn" | "danger" | undefined {
  return Object.hasOwn(SIGNAL_TONE, severity)
    ? SIGNAL_TONE[severity]
    : undefined;
}

// The deal-stakeholder roles worth a word. `role` is free text on the wire
// (the enum is an unminted contract extension, DEAL-EXT-5).
const DEAL_ROLE_LABELS: Record<string, MessageKey> = {
  champion: "co.role.champion",
  economic_buyer: "co.role.economic_buyer",
  blocker: "co.role.blocker",
  influencer: "co.role.influencer",
  user: "co.role.user",
};

/**
 * dealRoleLabel names a stakeholder's role on a deal.
 *
 * It lives in the kit rather than on the company page because a deal role is
 * not a company's fact: person360, the coverage card and the project sections
 * all name the same roles, and all three used to import this from a screen
 * about companies.
 */
export function dealRoleLabel(role: string, t: (key: MessageKey) => string) {
  const key = Object.hasOwn(DEAL_ROLE_LABELS, role)
    ? DEAL_ROLE_LABELS[role]
    : undefined;
  return key ? t(key) : role.replace(/_/g, " ");
}

// The delivery roles a project adds to the deal-stakeholder vocabulary. The
// five deal roles fall through to their own labels below, so one word names a
// champion on a deal and on the project it became.
const PROJECT_ROLE_LABELS: Record<string, MessageKey> = {
  sponsor: "project.role.sponsor",
  project_lead: "project.role.project_lead",
  delivery_lead: "project.role.delivery_lead",
  subject_matter_expert: "project.role.subject_matter_expert",
};

/**
 * projectRoleLabel names a stakeholder's seat on a project.
 *
 * Beside dealRoleLabel because it IS dealRoleLabel plus four words: the project
 * vocabulary is the deal one with the delivery roles added, and the card that
 * draws a seat and the dialog that offers one must not each keep their own copy
 * of that list.
 */
export function projectRoleLabel(
  role: string,
  t: (key: MessageKey) => string,
): string {
  const key = Object.hasOwn(PROJECT_ROLE_LABELS, role)
    ? PROJECT_ROLE_LABELS[role]
    : undefined;
  return key ? t(key) : dealRoleLabel(role, t);
}
