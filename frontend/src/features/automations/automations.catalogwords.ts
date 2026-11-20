// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import type { Translator } from "../../i18n";
import type { MessageKey } from "../../i18n/en";

type CatalogEntry = components["schemas"]["AutomationCatalogEntry"];
type Automation = components["schemas"]["Automation"];
type TemplateWords = Readonly<{ name: MessageKey; description: MessageKey }>;

// The server words its closed catalog in English. A key newer than this client
// keeps the server's words.
export const TEMPLATE_WORDS: Readonly<Record<string, TemplateWords>> = {
  no_activity_reminder: {
    name: "auto.template.no_activity_reminder.name",
    description: "auto.template.no_activity_reminder.description",
  },
  renewal_reminder: {
    name: "auto.template.renewal_reminder.name",
    description: "auto.template.renewal_reminder.description",
  },
  stage_change_notify: {
    name: "auto.template.stage_change_notify.name",
    description: "auto.template.stage_change_notify.description",
  },
  route_lead: {
    name: "auto.template.route_lead.name",
    description: "auto.template.route_lead.description",
  },
  check_in_cadence: {
    name: "auto.template.check_in_cadence.name",
    description: "auto.template.check_in_cadence.description",
  },
  post_meeting_recap: {
    name: "auto.template.post_meeting_recap.name",
    description: "auto.template.post_meeting_recap.description",
  },
  website_inquiry: {
    name: "auto.template.website_inquiry.name",
    description: "auto.template.website_inquiry.description",
  },
  assign_lead_owner: {
    name: "auto.template.assign_lead_owner.name",
    description: "auto.template.assign_lead_owner.description",
  },
  stage_change_create_task: {
    name: "auto.template.stage_change_create_task.name",
    description: "auto.template.stage_change_create_task.description",
  },
  list_membership_task: {
    name: "auto.template.list_membership_task.name",
    description: "auto.template.list_membership_task.description",
  },
  list_membership_notify: {
    name: "auto.template.list_membership_notify.name",
    description: "auto.template.list_membership_notify.description",
  },
  list_membership_shortlist: {
    name: "auto.template.list_membership_shortlist.name",
    description: "auto.template.list_membership_shortlist.description",
  },
};

export function templateName(entry: CatalogEntry, t: Translator): string {
  const words = TEMPLATE_WORDS[entry.key];
  return words ? t(words.name) : entry.name;
}

export function templateDescription(
  entry: CatalogEntry,
  t: Translator,
): string | undefined {
  const words = TEMPLATE_WORDS[entry.key];
  return words ? t(words.description) : entry.description;
}

// A seeded rule carries its template's server name until someone renames it,
// so that name reads in the reader's language too.
export function ruleName(
  automation: Automation,
  entry: CatalogEntry | undefined,
  t: Translator,
): string {
  return entry && automation.name === entry.name
    ? templateName(entry, t)
    : automation.name;
}
