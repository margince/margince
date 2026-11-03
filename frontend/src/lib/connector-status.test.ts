// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import type { components } from "../api/schema";
import {
  accountHealth,
  type ConnectorHealth,
  type ConnectorStatus,
  errorClassKey,
  healthBadge,
  isConnectedAtAll,
  isUnhealthy,
  missingSendGrant,
  needsReconnect,
  serviceBadge,
  serviceHealth,
  serviceReason,
  statusLabel,
  statusTone,
} from "./connector-status";

const GMAIL_SEND = "https://www.googleapis.com/auth/gmail.send";

describe("statusTone", () => {
  it("gives each status its own tone so four states never collapse to two", () => {
    expect(statusTone("connected")).toBe("success");
    expect(statusTone("reauth_required")).toBe("warning");
    expect(statusTone("error")).toBe("danger");
    expect(statusTone("disconnected")).toBe(undefined);
  });

  // A channel connection whose webhook registration hasn't landed yet is a
  // half-registration, not a live channel — it must read as "needs a look"
  // (the same tone as reauth_required), never as the "success" tone a
  // healthy connected row gets (§9.1).
  it("renders pending as a warning tone, distinct from connected", () => {
    expect(statusTone("pending")).toBe("warning");
    expect(statusLabel("pending")).toBe("connectors.statusPending");
    expect(statusLabel("pending")).not.toBe(statusLabel("connected"));
  });
});

describe("errorClassKey", () => {
  it("maps every contract error class to its own sentence", () => {
    const keys = [
      "rate_limited",
      "unreachable",
      "auth",
      "history_gone",
      "internal",
    ].map(errorClassKey);
    expect(new Set(keys).size).toBe(5);
  });

  it("falls back for a class the server added ahead of this client", () => {
    expect(errorClassKey("quota_exhausted")).toBe("connectors.errUnknown");
  });

  it("falls back for null rather than throwing", () => {
    expect(errorClassKey(null)).toBe("connectors.errUnknown");
  });
});

describe("isUnhealthy", () => {
  it("flags only a genuinely broken connection — not a deliberate disconnect, not a healthy one", () => {
    expect(isUnhealthy("error")).toBe(true);
    expect(isUnhealthy("reauth_required")).toBe(true);
    expect(isUnhealthy("connected")).toBe(false);
    // A deliberately disconnected mailbox is quiet on the Brief, matching
    // Settings, which filters `disconnected` rows out of its list entirely.
    expect(isUnhealthy("disconnected")).toBe(false);
  });
});

describe("missingSendGrant", () => {
  it("flags a Gmail mailbox connected before the send scope existed", () => {
    expect(
      missingSendGrant({
        provider: "gmail",
        scopes: ["https://www.googleapis.com/auth/gmail.readonly"],
      }),
    ).toBe(true);
  });

  it("stays quiet once the send scope is actually granted", () => {
    expect(
      missingSendGrant({
        provider: "gmail",
        scopes: ["https://www.googleapis.com/auth/gmail.readonly", GMAIL_SEND],
      }),
    ).toBe(false);
  });

  // A provider that cannot send at all is not "cannot send" — it was never a
  // sending mailbox, and its scope vocabulary is neither vendor's, so no absent
  // scope says anything about it.
  it("never badges a provider that does not send in the first place", () => {
    expect(missingSendGrant({ provider: "imap", scopes: [] })).toBe(false);
    expect(missingSendGrant({ provider: "gcal", scopes: [] })).toBe(false);
  });

  // Outlook sends too, and its grant carries Microsoft's own permission name —
  // so a mailbox connected before sending shipped for that vendor is badged on
  // the SAME rule, read against a different string.
  it("badges an Outlook mailbox whose grant carries no send permission", () => {
    expect(
      missingSendGrant({
        provider: "graph",
        scopes: ["offline_access", "User.Read", "Mail.Read"],
      }),
    ).toBe(true);
    expect(
      missingSendGrant({
        provider: "graph",
        scopes: ["offline_access", "User.Read", "Mail.Read", "Mail.Send"],
      }),
    ).toBe(false);
  });
});

type CaptureConnection = components["schemas"]["CaptureConnection"];

// Every grant a service can carry, so a test narrows the one it is about.
const FULL_GRANT = [
  "https://www.googleapis.com/auth/gmail.send",
  "https://www.googleapis.com/auth/calendar.events",
  "Mail.Send",
  "Calendars.ReadWrite",
];

function service(
  provider: CaptureConnection["provider"],
  facts: Partial<CaptureConnection> = {},
): CaptureConnection {
  return {
    id: provider,
    provider,
    can_backfill: true,
    status: "connected",
    scopes: FULL_GRANT,
    ...facts,
  };
}

describe("healthBadge", () => {
  const every: readonly ConnectorHealth[] = [
    "connected",
    "syncing",
    "up_to_date",
    "needs_reconnect",
    "needs_attention",
    "not_connected",
  ];

  // The Telegram badge renders statusLabel; renaming a Connections word must
  // never reach it, so the two vocabularies share no key.
  it("uses no key the shared status vocabulary uses", () => {
    const shared: readonly ConnectorStatus[] = [
      "connected",
      "pending",
      "reauth_required",
      "error",
      "disconnected",
    ];
    const sharedKeys = new Set(shared.map(statusLabel));
    for (const health of every) {
      expect(sharedKeys.has(healthBadge(health).label)).toBe(false);
    }
  });

  it("tones a running import grey and live, and anything to fix amber", () => {
    expect(healthBadge("syncing").tone).toBe("default");
    expect(healthBadge("syncing").live).toBe(true);
    expect(healthBadge("needs_attention").tone).toBe("warning");
    expect(healthBadge("needs_reconnect").tone).toBe("warning");
    expect(healthBadge("connected").tone).toBe("success");
    expect(healthBadge("up_to_date").tone).toBe("success");
    expect(healthBadge("not_connected").tone).toBe("default");
  });
});

describe("serviceHealth", () => {
  it("reads a healthy service as up to date", () => {
    expect(serviceHealth(service("gmail"))).toBe("up_to_date");
  });

  it("reads a connected service in a failure streak as needing attention", () => {
    expect(
      serviceHealth(service("gmail", { last_sync_error_class: "unreachable" })),
    ).toBe("needs_attention");
  });

  it("asks for a reconnect when the grant expired, and attention on an error", () => {
    expect(serviceHealth(service("gcal", { status: "reauth_required" }))).toBe(
      "needs_reconnect",
    );
    expect(serviceHealth(service("gcal", { status: "error" }))).toBe(
      "needs_attention",
    );
  });
});

describe("isConnectedAtAll", () => {
  it("counts a service still set up, whatever its sync is doing", () => {
    expect(isConnectedAtAll(service("gmail"))).toBe(true);
    expect(
      isConnectedAtAll(
        service("gmail", { last_sync_error_class: "unreachable" }),
      ),
    ).toBe(true);
    expect(isConnectedAtAll(service("gcal", { status: "error" }))).toBe(true);
  });

  it("does not count one removed or expired", () => {
    expect(isConnectedAtAll(service("gcal", { status: "disconnected" }))).toBe(
      false,
    );
    expect(
      isConnectedAtAll(service("gcal", { status: "reauth_required" })),
    ).toBe(false);
  });
});

describe("accountHealth", () => {
  const running: Partial<CaptureConnection> = {
    backfill: { state: "running" },
  };

  it("reads an account with healthy services as connected", () => {
    expect(
      accountHealth({ mail: service("gmail"), calendar: service("gcal") }),
    ).toBe("connected");
  });

  it("reads an account as syncing while its import is queued or running", () => {
    expect(
      accountHealth({ mail: service("gmail", running), calendar: null }),
    ).toBe("syncing");
    expect(
      accountHealth({
        mail: service("gmail", { backfill: { state: "queued" } }),
        calendar: null,
      }),
    ).toBe("syncing");
    expect(
      accountHealth({
        mail: service("gmail", { backfill: { state: "done" } }),
        calendar: null,
      }),
    ).toBe("connected");
  });

  it("puts a broken calendar ahead of a running import", () => {
    expect(
      accountHealth({
        mail: service("gmail", running),
        calendar: service("gcal", { status: "reauth_required" }),
      }),
    ).toBe("needs_attention");
  });

  it("needs attention when a grant cannot send mail or write bookings", () => {
    expect(
      accountHealth({
        mail: service("gmail", { scopes: ["read"] }),
        calendar: service("gcal"),
      }),
    ).toBe("needs_attention");
    expect(
      accountHealth({
        mail: service("graph"),
        calendar: service("graphcal", { scopes: ["Calendars.Read"] }),
      }),
    ).toBe("needs_attention");
  });

  it("reads an account with no live service as not connected", () => {
    expect(accountHealth({ mail: null, calendar: null })).toBe("not_connected");
    expect(
      accountHealth({
        mail: service("graph", { status: "disconnected" }),
        calendar: null,
      }),
    ).toBe("not_connected");
  });
});

describe("a service's one badge and one reason", () => {
  it("names a grant too narrow to send by what it cannot do", () => {
    const mail = service("gmail", { scopes: ["read"] });
    expect(serviceBadge(mail)).toMatchObject({
      tone: "warning",
      label: "connectors.cannotSend",
    });
    expect(serviceReason(mail)).toBe("connectors.reconnectToSend");
    expect(needsReconnect(mail)).toBe(true);
  });

  it("names a calendar it may only read", () => {
    const calendar = service("gcal", { scopes: ["read"] });
    expect(serviceBadge(calendar).label).toBe("scheduling.readOnlyBadge");
    expect(serviceReason(calendar)).toBe("scheduling.readOnlyCalendar");
  });

  it("lets a refused grant outrank a narrow one, and says why", () => {
    const mail = service("gmail", {
      status: "reauth_required",
      scopes: ["read"],
      last_sync_error_class: "auth",
    });
    expect(serviceBadge(mail).label).toBe("connectors.health.reconnect");
    expect(serviceReason(mail)).toBe("connectors.errAuth");
  });

  it("keeps a failure streak off the reconnect, which would not fix it", () => {
    const mail = service("gmail", { last_sync_error_class: "rate_limited" });
    expect(serviceBadge(mail).label).toBe("connectors.health.attention");
    expect(serviceReason(mail)).toBe("connectors.errRateLimited");
    expect(needsReconnect(mail)).toBe(false);
  });

  it("has nothing to explain on a healthy service", () => {
    expect(serviceReason(service("gmail"))).toBeNull();
    expect(needsReconnect(service("gmail"))).toBe(false);
  });
});
