// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import { approvalHref } from "../features/automations/approvalhref";
import {
  healthSectionForSource,
  settingsHealthHref,
} from "../features/settings/systemhealthnavigation";
import type { MessageKey } from "../i18n/en";
import { type EntityKind, isEntityKind, recordRoute } from "../lib/entity";
import { routeHash } from "../lib/router";

type NoticeTarget = NonNullable<
  components["schemas"]["NotificationItem"]["target"]
>;

/** Where a notice sends its reader, and the verb that names the trip. */
export type NoticeAction = { href: string; label: MessageKey };

// Keyed by every record kind, so a kind the registry gains fails to compile
// here until its notices can name their verb.
const OPEN_RECORD: Readonly<Record<EntityKind, MessageKey>> = {
  contact: "notifications.action.openContact",
  company: "notifications.action.openCompany",
  deal: "notifications.action.openDeal",
  lead: "notifications.action.openLead",
  project: "notifications.action.openProject",
};

/**
 * The one move a notice offers besides settling it, or undefined for a notice
 * that names nothing the app can open.
 */
export function noticeAction(
  target: NoticeTarget | undefined,
): NoticeAction | undefined {
  if (!target) return undefined;
  if (target.type === "approval") {
    return {
      href: approvalHref(target.id),
      label: "notifications.action.review",
    };
  }
  if (target.type === "data_task") {
    return {
      href: settingsHealthHref(healthSectionForSource("updates")),
      label: "notifications.action.systemHealth",
    };
  }
  const route = recordRoute(target.type, target.id);
  if (!route || !isEntityKind(target.type)) return undefined;
  return { href: routeHash(route), label: OPEN_RECORD[target.type] };
}
