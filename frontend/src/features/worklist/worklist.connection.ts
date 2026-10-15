// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { providerName } from "../../format/connectorprovider";
import type { useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import type { WorklistItem } from "./worklist.queries";

// The words a capture health row owes its connection: which provider it is,
// and whether it carries mail or a calendar.

type T = ReturnType<typeof useT>;
type Connected = Pick<WorklistItem, "connection">;

export function connectionName(item: Connected, t: T): string | undefined {
  return item.connection && providerName(item.connection.provider, t);
}

export function connectionTitleKey(item: Connected): MessageKey | undefined {
  return item.connection?.medium === "calendar"
    ? "worklist.untitled.capture_health_calendar"
    : undefined;
}

// What happens if the reader does nothing. The wire says mailbox for every
// capture connection, so the medium decides, and an unknown one says neither.
export function consequenceText(item: WorklistItem, t: T): string | null {
  if (item.consequence === "none") {
    return null;
  }
  if (
    item.consequence === "mailbox_blind" &&
    item.connection?.medium !== "mailbox"
  ) {
    return t(
      item.connection
        ? "worklist.consequence.calendar_blind"
        : "worklist.consequence.connection_blind",
    );
  }
  return t(`worklist.consequence.${item.consequence}` as const);
}
