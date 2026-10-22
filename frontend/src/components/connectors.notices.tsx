// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useState } from "react";
import type { components } from "../api/schema";
import { Button } from "../design-system/atoms";
import { Callout, type CalloutTone } from "../design-system/callout";
import { ErrorLine } from "../design-system/errorline";
import {
  ACCOUNT_NAME,
  type AccountVendor,
  VENDOR_SERVICES,
  vendorOf,
} from "../format/connectorprovider";
import { type Translator, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import type { PairedCalendar } from "../lib/connect-account";

// What a page says about a consent round trip that has just come back. It
// takes the landing as props so Connections and Meetings draw the same answer.

type CaptureConnection = components["schemas"]["CaptureConnection"];

// The callback lands on .../{outcome}/{provider}. Only these outcomes are
// server-defined; any other segment is ignored rather than printed back.
// Three failures exist so one nobody can fix by retrying does not say retry.
// Either the provider refused the grant, or its API was never enabled, or it
// refused this installation's client credentials.
const OAUTH_OUTCOME_NOTE: Record<string, MessageKey> = {
  ok: "connectors.oauthOk",
  denied: "connectors.oauthDenied",
  rejected: "connectors.oauthRejected",
  misconfigured: "connectors.oauthMisconfigured",
  bad_client: "connectors.oauthBadClient",
  error: "connectors.oauthError",
};

type Reading = {
  tone: CalloutTone;
  title: string;
  body: string | null;
  /** Set when the mail grant held and only its calendar leg is missing. */
  calendar: PairedCalendar | null;
};

function liveProviders(connections: readonly CaptureConnection[]) {
  return new Set(
    connections
      .filter((c) => c.status !== "disconnected")
      .map((c) => c.provider),
  );
}

function importRunning(
  connections: readonly CaptureConnection[],
  mail: CaptureConnection["provider"],
) {
  return connections.some(
    (c) =>
      c.provider === mail &&
      (c.backfill?.state === "queued" || c.backfill?.state === "running"),
  );
}

// A vendor landing is read against the roster, because a chained consent
// lands on the calendar's outcome while the mail grant before it already holds.
function readVendorLanding(
  t: Translator,
  outcome: string,
  note: MessageKey,
  vendor: AccountVendor,
  provider: string | undefined,
  connections: readonly CaptureConnection[],
): Reading | null {
  const services = VENDOR_SERVICES[vendor];
  const live = liveProviders(connections);
  const mail = provider === services.mail || live.has(services.mail);
  if (outcome === "ok") {
    const calendar =
      provider === services.calendar || live.has(services.calendar);
    let syncing: MessageKey = "connectors.oauthCalendarOnly";
    if (mail) {
      syncing = calendar ? "connectors.oauthBoth" : "connectors.oauthMailOnly";
    }
    return {
      tone: "success",
      title: t("connectors.oauthConnectedTitle", {
        provider: t(ACCOUNT_NAME[vendor]),
      }),
      body: importRunning(connections, services.mail)
        ? `${t(syncing)} ${t("connectors.oauthImportRuns")}`
        : t(syncing),
      calendar: null,
    };
  }
  if (provider !== services.calendar || !mail) {
    return null;
  }
  const declined = outcome === "denied";
  return {
    tone: "warning",
    title: t(
      declined
        ? "connectors.oauthCalendarDeclined"
        : "connectors.oauthCalendarFailed",
    ),
    body: declined ? null : t(note),
    calendar: services.calendar,
  };
}

function readLanding(
  t: Translator,
  outcome: string,
  note: MessageKey,
  provider: string | undefined,
  connections: readonly CaptureConnection[],
): Reading {
  const vendor = vendorOf(provider);
  const vendorReading =
    vendor &&
    readVendorLanding(t, outcome, note, vendor, provider, connections);
  if (vendorReading) {
    return vendorReading;
  }
  const ok = outcome === "ok";
  return {
    tone: ok ? "success" : "danger",
    title: t(ok ? "connectors.oauthConnected" : "connectors.oauthNotConnected"),
    body: t(note),
    calendar: null,
  };
}

/**
 * The OAuth return outcome above a page's own content: it reports what the
 * reader just did, not one of the page's standing decisions.
 *
 * A calendar refused after its mailbox was granted is a partial success, so
 * it offers the one missing consent through `connectCalendar`, which the host
 * page wires. Dismissing clears it until the landing changes.
 */
export function OAuthOutcomeNote({
  outcome,
  provider,
  connections,
  connectCalendar,
}: Readonly<{
  /** The landing's outcome segment, unvalidated. */
  outcome: string | undefined;
  /** The landing's provider segment, unvalidated. */
  provider: string | undefined;
  /** The reader's connections from GET /connectors; undefined until it
   *  answers, so a chain's landing is never read against an empty roster. */
  connections: readonly CaptureConnection[] | undefined;
  connectCalendar?: Readonly<{
    onConnect: (calendar: PairedCalendar) => void;
    pending: boolean;
    error: string | null;
  }>;
}>) {
  const t = useT();
  const landing = `${outcome}/${provider}`;
  const [dismissed, setDismissed] = useState<string | null>(null);
  // Object.hasOwn, not a bare index: a segment like "constructor" would
  // otherwise resolve to an inherited member and render an empty note.
  const note =
    outcome && Object.hasOwn(OAUTH_OUTCOME_NOTE, outcome)
      ? OAUTH_OUTCOME_NOTE[outcome]
      : undefined;
  if (!outcome || !note || !connections || landing === dismissed) {
    return null;
  }
  const reading = readLanding(t, outcome, note, provider, connections);
  const calendar = reading.calendar;
  const connectError = calendar ? connectCalendar?.error : null;
  return (
    <Callout
      tone={reading.tone}
      kind="outcome"
      title={reading.title}
      actions={
        calendar &&
        connectCalendar && (
          <Button
            pending={connectCalendar.pending}
            onClick={() => connectCalendar.onConnect(calendar)}
          >
            {t("connectors.connectCalendar")}
          </Button>
        )
      }
      dismiss={{
        label: t("connectors.dismissOutcome"),
        onDismiss: () => setDismissed(landing),
      }}
    >
      {reading.body || connectError ? (
        <>
          {reading.body}
          {connectError && <ErrorLine>{connectError}</ErrorLine>}
        </>
      ) : undefined}
    </Callout>
  );
}
