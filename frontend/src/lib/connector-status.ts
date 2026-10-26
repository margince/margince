// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import {
  statusTone as kindTone,
  type StatusKind,
} from "../design-system/statusbadge";
import type { ConnectionAccount } from "../format/connectorprovider";
import type { MessageKey } from "../i18n/en";

// The shared connector-status vocabulary: Settings (the connected-inboxes
// card) and the Brief's digest both render connector health, and they must never
// describe the same state differently. Seeded by extracting the inline
// statusTone/statusLabel that shipped with connectors.tsx (RC-8) so the two
// surfaces stay on one definition rather than two copies drifting apart.

type CaptureConnection = components["schemas"]["CaptureConnection"];
type ChannelConnection = components["schemas"]["ChannelConnection"];

// The mail providers (CaptureConnection) and the messaging-channel bot
// (ChannelConnection) publish two different wire enums, but they share one
// status vocabulary from here on. `pending` survives in the channel enum for a
// generator reason the contract states, not a product one — no server produces it,
// because a bot binding is polled and a connect either commits live or writes
// nothing. It keeps its arms below because the switches are exhaustive over the
// published type, and it is rendered as "needs a look" rather than as healthy in
// case an older or foreign server ever sends one.
export type ConnectorStatus =
  | CaptureConnection["status"]
  | ChannelConnection["status"];

/** The scope each sending provider's grant must carry, keyed by provider.
 *
 *  A DELIBERATE MIRROR of the server's `comms.SendScopeFor`, which pre-flights
 *  a send against these same strings — so the badge below and the 422 it exists
 *  to pre-empt cannot disagree about which mailbox may send. A table rather
 *  than a chain of comparisons, because there are now two vendors and the way
 *  a chain fails is silent: a provider missing from it is never badged, and its
 *  owner discovers the missing grant from a refused send instead. */
const SEND_SCOPES: Readonly<Record<string, string>> = {
  gmail: "https://www.googleapis.com/auth/gmail.send",
  graph: "Mail.Send",
};

/** Whether this connection captures mail it will never be allowed to send.
 *  Neither Google nor Microsoft will widen an existing refresh token, so every
 *  mailbox connected before sending shipped for its vendor holds read-only
 *  access until its owner reconnects — a fact the connection's `status` cannot
 *  express, since it is genuinely connected and genuinely capturing. Only the
 *  granted scopes say it, which is why this reads them rather than the status.
 *
 *  A provider absent from the table above is not "cannot send" — it is not a
 *  sending mailbox in the first place (a calendar, an IMAP box), and badging it
 *  would be a refusal nobody could act on. */
export function missingSendGrant(
  connection: Pick<CaptureConnection, "provider" | "scopes">,
): boolean {
  const required = SEND_SCOPES[connection.provider];
  return required !== undefined && !connection.scopes.includes(required);
}

/** Each contract status gets its own Badge tone. Collapsing reauth_required
 *  and error into the same tone is what made a dead mailbox and a merely-
 *  stale one indistinguishable at a glance. `disconnected` gets no tone (the
 *  shipped card's neutral, undecorated row). `pending` reads as `warning`, the
 *  same "needs a look" tone as reauth_required — never `success`: a row this
 *  installation cannot produce must not be rendered as a healthy channel if an
 *  older or foreign server ever sends one. */
export function statusTone(
  status: ConnectorStatus,
): "success" | "warning" | "danger" | undefined {
  switch (status) {
    case "connected":
      return "success";
    case "pending":
    case "reauth_required":
      return "warning";
    case "error":
      return "danger";
    case "disconnected":
      return undefined;
  }
}

/** The status label shown beside the tone. */
export function statusLabel(status: ConnectorStatus): MessageKey {
  switch (status) {
    case "connected":
      return "connectors.statusConnected";
    case "pending":
      return "connectors.statusPending";
    case "reauth_required":
      return "connectors.statusReauth";
    case "error":
      return "connectors.statusError";
    case "disconnected":
      return "connectors.statusDisconnected";
  }
}

/** The contract states that only the error CLASS crosses the wire — detail
 *  lives in system_log. So each class gets one fixed sentence and we never
 *  invent more. The enum can widen server-side ahead of this client, so an
 *  unrecognized class degrades to an honest generic rather than rendering a
 *  raw identifier. */
export function errorClassKey(cls: string | null | undefined): MessageKey {
  switch (cls) {
    case "rate_limited":
      return "connectors.errRateLimited";
    case "unreachable":
      return "connectors.errUnreachable";
    case "auth":
      return "connectors.errAuth";
    case "history_gone":
      return "connectors.errHistoryGone";
    case "internal":
      return "connectors.errInternal";
    default:
      return "connectors.errUnknown";
  }
}

/** Brief surfaces a connector line only when something needs the user's
 *  attention: a healthy connector is not news, and a deliberately
 *  disconnected mailbox (the headline disconnect flow's own result) is not
 *  a fault — it is quiet on purpose, matching Settings, which filters
 *  `disconnected` rows out of its list entirely. Only a genuinely broken
 *  connection (`error` or `reauth_required`) is news. */
export function isUnhealthy(status: ConnectorStatus): boolean {
  return status === "error" || status === "reauth_required";
}

// Mirror of capture.calendarWriteGranted; its Go parity test reads this table.
const CALENDAR_WRITE_SCOPES: Readonly<Record<string, readonly string[]>> = {
  gcal: [
    "https://www.googleapis.com/auth/calendar.events.owned",
    "https://www.googleapis.com/auth/calendar.events",
    "https://www.googleapis.com/auth/calendar",
  ],
  graphcal: ["Calendars.ReadWrite"],
};

export function missingCalendarWriteGrant(
  connection: Pick<CaptureConnection, "provider" | "scopes">,
): boolean {
  const grants = CALENDAR_WRITE_SCOPES[connection.provider];
  return (
    grants !== undefined &&
    !grants.some((scope) => connection.scopes.includes(scope))
  );
}

/** The words the Connections page puts on an account and on each service.
 *  Their own keys, never statusLabel's: the Telegram badge reads those. */
export type ConnectorHealth =
  | "connected"
  | "syncing"
  | "up_to_date"
  | "needs_reconnect"
  | "needs_attention"
  | "not_connected";

type HealthBadge = Readonly<{
  tone: ReturnType<typeof kindTone>;
  label: MessageKey;
  live?: boolean;
}>;

function badgeOf(kind: StatusKind, label: MessageKey, live?: boolean) {
  return { tone: kindTone(kind), label, live };
}

const HEALTH_BADGE: Readonly<Record<ConnectorHealth, HealthBadge>> = {
  connected: badgeOf("done", "connectors.health.connected"),
  syncing: badgeOf("neutral", "connectors.health.syncing", true),
  up_to_date: badgeOf("done", "connectors.health.upToDate"),
  needs_reconnect: badgeOf("attention", "connectors.health.reconnect"),
  needs_attention: badgeOf("attention", "connectors.health.attention"),
  not_connected: badgeOf("neutral", "connectors.health.notConnected"),
};

export function healthBadge(health: ConnectorHealth): HealthBadge {
  return HEALTH_BADGE[health];
}

/** Whether a service needs its owner's consent again: the vendor refused the
 *  grant, or the grant is too narrow to send or to book. */
export function needsReconnect(service: CaptureConnection): boolean {
  return (
    service.status === "reauth_required" ||
    missingSendGrant(service) ||
    missingCalendarWriteGrant(service)
  );
}

/** One service's badge. A grant too narrow to send or to book is named by
 *  what it cannot do, so the row needs no second badge to say it. */
export function serviceBadge(service: CaptureConnection): HealthBadge {
  const health = serviceHealth(service);
  if (health === "up_to_date" && missingSendGrant(service)) {
    return badgeOf("attention", "connectors.cannotSend");
  }
  if (health === "up_to_date" && missingCalendarWriteGrant(service)) {
    return badgeOf("attention", "scheduling.readOnlyBadge");
  }
  return healthBadge(health);
}

/** The one sentence that says why a service is not up to date, or null. */
export function serviceReason(service: CaptureConnection): MessageKey | null {
  const health = serviceHealth(service);
  if (health === "needs_reconnect" || health === "needs_attention") {
    return errorClassKey(service.last_sync_error_class);
  }
  if (missingSendGrant(service)) return "connectors.reconnectToSend";
  if (missingCalendarWriteGrant(service)) return "scheduling.readOnlyCalendar";
  return null;
}

type ServiceFacts = Pick<CaptureConnection, "status" | "last_sync_error_class">;

/** One service's word. A connected row still carrying an error class is in a
 *  failure streak the sweep has not given up on, so it is not up to date. */
export function serviceHealth(service: ServiceFacts): ConnectorHealth {
  switch (service.status) {
    case "reauth_required":
      return "needs_reconnect";
    case "error":
      return "needs_attention";
    case "disconnected":
      return "not_connected";
    case "connected":
      return service.last_sync_error_class ? "needs_attention" : "up_to_date";
  }
}

/** The account's word. A broken service outranks a running import: the import
 *  shows its own progress, and the reconnect is what the member has to do. A
 *  grant too narrow to send or to book counts as broken, as its row says. */
export function accountHealth(
  account: Pick<ConnectionAccount, "mail" | "calendar">,
): ConnectorHealth {
  const services = [account.mail, account.calendar]
    .filter((service) => service !== null)
    .filter((service) => service.status !== "disconnected");
  if (services.length === 0) return "not_connected";
  const broken = services.some((service) => {
    const health = serviceHealth(service);
    return (
      health === "needs_reconnect" ||
      health === "needs_attention" ||
      missingSendGrant(service) ||
      missingCalendarWriteGrant(service)
    );
  });
  if (broken) return "needs_attention";
  const importing = services.some(
    (service) =>
      service.backfill?.state === "queued" ||
      service.backfill?.state === "running",
  );
  return importing ? "syncing" : "connected";
}
