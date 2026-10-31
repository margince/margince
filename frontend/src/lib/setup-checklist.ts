// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useState } from "react";
import type { components } from "../api/schema";
import type { ChecklistStepState } from "../design-system/checklist";
import {
  isMailbox,
  VENDOR_SERVICES,
  vendorOf,
} from "../format/connectorprovider";
import { missingCalendarWriteGrant } from "./connector-status";
import { type ConnectorsResult, useConnectors } from "./connectors";
import { useSchedulingProfile } from "./scheduling-profile-query";
import type { SettingsPageId, SettingsReach } from "./settingscatalog";
import { readStored, STORAGE_KEYS, writeStored } from "./storage";
import { useVoiceProfile } from "./voice-profile";

// The setup checklist the topbar reminder and the settings home both draw.
// Every step is read off what the product can already see, never stored, so a
// skipped calendar keeps being asked for until it is connected.

type CaptureConnection = components["schemas"]["CaptureConnection"];
type Provider = CaptureConnection["provider"];
type Readiness = ConnectorsResult["providers"][number]["reason"];
type VoiceProfile = components["schemas"]["VoiceProfile"];
type SchedulingProfile = components["schemas"]["SchedulingProfile"];

export type SetupStepId = "mail" | "calendar" | "voice" | "booking";

/** Why an unfinished step reads differently from its plain reason. */
export type SetupStepReason =
  /** Mail came from Google or Microsoft and never had a calendar row. */
  | "skipped"
  /** The connection expired, or a calendar cannot write the bookings. */
  | "needsReconnect"
  /** The mail is IMAP, which carries no calendar. */
  | "needsVendor"
  /** No usable company Google or Microsoft app yet. */
  | "waitsAdmin"
  /** The booking link offers times from a calendar. */
  | "needsCalendar";

export type SetupStep = Readonly<{
  id: SetupStepId;
  state: ChecklistStepState;
  /** The settings page the step's one action opens. */
  page: SettingsPageId;
  /** Waiting on somebody or something else: the step offers no action. */
  blocked: boolean;
  reason: SetupStepReason | null;
  /** The connection that finished the step, for a caption naming it. */
  account: Readonly<{ label: string | null; provider: Provider }> | null;
}>;

/** What the reader's reads answered; null where a read was refused or failed. */
export type SetupReads = Readonly<{
  acts: ReadonlySet<SettingsPageId>;
  connectors: ConnectorsResult | null;
  voice: Readonly<{ profile: VoiceProfile | null }> | null;
  scheduling: SchedulingProfile | null;
}>;

type Draft = Omit<SetupStep, "state"> & Readonly<{ done: boolean }>;

const VENDORS = Object.values(VENDOR_SERVICES);
const CALENDARS: ReadonlySet<Provider> = new Set<Provider>(
  VENDORS.map((services) => services.calendar),
);

function connected(
  connectors: ConnectorsResult,
  kind: (provider: Provider) => boolean,
): CaptureConnection | undefined {
  return connectors.data.find(
    (row) => row.status === "connected" && kind(row.provider),
  );
}

/** A refusal only an administrator clears, by registering or repairing the
 *  company's app. */
export type AdminWait = Extract<Readiness, "app_missing" | "app_unusable">;

// An unusable stored app is the administrator's fix as much as a missing one.
// The connect step reads this too, so the step and the checklist agree.
export function waitsOnAdmin(
  reason: Readiness | undefined,
): reason is AdminWait {
  return reason === "app_missing" || reason === "app_unusable";
}

function draft(
  id: SetupStepId,
  page: SettingsPageId,
  fields: Partial<Pick<Draft, "done" | "blocked" | "reason">> & {
    connection?: CaptureConnection;
  } = {},
): Draft {
  const { connection, ...rest } = fields;
  return {
    id,
    page,
    done: connection !== undefined,
    blocked: false,
    reason: null,
    account: connection
      ? {
          label: connection.account_label ?? null,
          provider: connection.provider,
        }
      : null,
    ...rest,
  };
}

function expired(
  connectors: ConnectorsResult,
  kind: (provider: Provider) => boolean,
): boolean {
  return connectors.data.some(
    (row) => row.status === "reauth_required" && kind(row.provider),
  );
}

function mailStep(connectors: ConnectorsResult): Draft | null {
  const mailbox = connected(connectors, isMailbox);
  if (mailbox) {
    return draft("mail", "connections", { connection: mailbox });
  }
  const reasons = connectors.providers.map((entry) => entry.reason);
  if (reasons.includes("ready")) {
    return draft("mail", "connections", {
      reason: expired(connectors, isMailbox) ? "needsReconnect" : null,
    });
  }
  // Nothing connects here and no administrator can change that: no step.
  return reasons.some(waitsOnAdmin)
    ? draft("mail", "connections", { blocked: true, reason: "waitsAdmin" })
    : null;
}

function calendarStep(connectors: ConnectorsResult): Draft | null {
  const isCalendar = (provider: Provider) => CALENDARS.has(provider);
  const calendar = connected(connectors, isCalendar);
  // A read-only grant blocks no time for bookings it cannot write: not done.
  if (calendar && !missingCalendarWriteGrant(calendar)) {
    return draft("calendar", "connections", { connection: calendar });
  }
  // A vendor's calendar runs on the same company app as its mail.
  const readiness = VENDORS.map(
    (services) =>
      connectors.providers.find((entry) => entry.provider === services.mail)
        ?.reason,
  );
  if (!readiness.includes("ready")) {
    return readiness.some(waitsOnAdmin)
      ? draft("calendar", "connections", {
          blocked: true,
          reason: "waitsAdmin",
        })
      : null;
  }
  if (calendar || expired(connectors, isCalendar)) {
    return draft("calendar", "connections", { reason: "needsReconnect" });
  }
  const mailbox = connected(connectors, isMailbox);
  if (!mailbox) {
    return draft("calendar", "connections");
  }
  const vendor = vendorOf(mailbox.provider);
  if (!vendor) {
    return draft("calendar", "connections", { reason: "needsVendor" });
  }
  // A calendar disconnected later was not skipped; only a never-had one was.
  const hadCalendar = connectors.data.some(
    (row) => row.provider === VENDOR_SERVICES[vendor].calendar,
  );
  return draft("calendar", "connections", {
    reason: hadCalendar ? null : "skipped",
  });
}

function bookingStep(
  scheduling: SchedulingProfile,
  calendar: Draft | null,
): Draft | null {
  if (scheduling.enabled) {
    return draft("booking", "meetings", { done: true });
  }
  if (calendar === null) {
    return null;
  }
  return calendar.done
    ? draft("booking", "meetings")
    : draft("booking", "meetings", { blocked: true, reason: "needsCalendar" });
}

/** The steps this reader is asked to finish, in the order they are shown. */
export function deriveSetupSteps(reads: SetupReads): readonly SetupStep[] {
  const { acts, connectors, voice, scheduling } = reads;
  const connections =
    acts.has("connections") && connectors !== null && !connectors.notConfigured
      ? connectors
      : null;
  const calendar = connections && calendarStep(connections);
  const drafts = [
    connections && mailStep(connections),
    calendar,
    acts.has("voice") && voice
      ? draft("voice", "voice", {
          // Zero is the schema's "no active artifact yet".
          done: (voice.profile?.profile_version ?? 0) > 0,
        })
      : null,
    acts.has("meetings") && scheduling
      ? bookingStep(scheduling, calendar)
      : null,
  ].filter((step): step is Draft => step !== null);

  // The step to do next is the first one the reader can act on now.
  const next = drafts.find((step) => !step.done && !step.blocked);
  return drafts.map(({ done, ...step }) => ({
    ...step,
    state: done ? "done" : step.id === next?.id ? "current" : "open",
  }));
}

export type SetupChecklist = Readonly<{
  /** False until every read the steps depend on has answered. */
  settled: boolean;
  steps: readonly SetupStep[];
  done: number;
  total: number;
  /** The reader hid the reminder for the rest of this sign-in. */
  hidden: boolean;
  hide: () => void;
}>;

export function useSetupChecklist(reach: SettingsReach): SetupChecklist {
  const acts = new Set(reach.acts.map((page) => page.id));
  // Each read waits for a page that could show its step.
  const connectors = useConnectors({ enabled: acts.has("connections") });
  const voice = useVoiceProfile(acts.has("voice"));
  const scheduling = useSchedulingProfile({ enabled: acts.has("meetings") });
  const [hidden, setHidden] = useState(
    () => readStored(STORAGE_KEYS.setupReminderHidden) === "1",
  );

  const asked = [
    acts.has("connections") && connectors,
    acts.has("voice") && voice,
    acts.has("meetings") && scheduling,
  ];
  // A refused or failed read drops the steps it feeds rather than claiming
  // them open. The reader is never sent to something the product cannot see.
  const steps = deriveSetupSteps({
    acts,
    connectors: connectors.data ?? null,
    voice: voice.data === undefined ? null : { profile: voice.data },
    scheduling: scheduling.data ?? null,
  });
  return {
    settled: asked.every((query) => query === false || !query.isPending),
    steps,
    done: steps.filter((step) => step.state === "done").length,
    total: steps.length,
    hidden,
    hide: () => {
      writeStored(STORAGE_KEYS.setupReminderHidden, "1");
      setHidden(true);
    },
  };
}
