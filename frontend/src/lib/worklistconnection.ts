// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { providerName } from "../format/connectorprovider";
import type { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import type { WorklistItem } from "./worklisttypes";

// The words a capture health row owes its connection: which provider it is,
// and whether it carries mail or a calendar. A booking health row's words
// name what stops its page.

type T = ReturnType<typeof useT>;
type Connected = Pick<WorklistItem, "connection">;

export function connectionName(item: Connected, t: T): string | undefined {
  return item.connection && providerName(item.connection.provider, t);
}

// A paused booking page names why: the page's blocker is the row's kind. A
// blocker this build does not know keeps the generic title.
const BOOKING_TITLES: Readonly<Record<string, MessageKey>> = {
  no_calendar: "worklist.untitled.booking_health_no_calendar",
  calendar_read_only: "worklist.untitled.booking_health_calendar_read_only",
  calendar_unavailable: "worklist.untitled.booking_health_calendar_unavailable",
};

export function bookingTitleKey(
  item: Pick<WorklistItem, "source" | "kind">,
): MessageKey | undefined {
  if (item.source !== "booking_health" || !item.kind) return undefined;
  return BOOKING_TITLES[item.kind];
}

export function connectionTitleKey(item: Connected): MessageKey | undefined {
  return item.connection?.medium === "calendar"
    ? "worklist.untitled.capture_health_calendar"
    : undefined;
}
