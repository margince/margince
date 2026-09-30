// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useCan } from "../app/capability";
import { Badge, EmptyState } from "../design-system/atoms";
import { type Fact, FactList } from "../design-system/factlist";
import { SettingList, SettingRow } from "../design-system/settingrow";
import { Row } from "../design-system/stack";
import { formatDateTime, formatNumber } from "../format/format";
import { viewerZone } from "../format/timezone";
import {
  type Locale,
  type PluralBase,
  type PluralTranslator,
  type Translator,
  useLocale,
  usePlural,
  useT,
} from "../i18n";
import type { MessageKey } from "../i18n/en";
import { throwProblem } from "./common";
import { formatWaitedFor, HealthCard } from "./healthcard";

// GET /admin/capture-health — whether mailbox capture keeps up with its own
// questions, for an administrator who may not see the mail itself.
//
// Counts and ages only. The contacts it counts are visible to their mailbox's
// owner alone, so this card names a mailbox and never what waits inside it.

type Health = components["schemas"]["CaptureHealth"];
type Sweep = components["schemas"]["CaptureSweepHealth"];
type Mailbox = components["schemas"]["CaptureMailboxHealth"];

// Keyed on the contract's union, so a pass added upstream is a compile error
// here rather than a raw token on the card.
const SWEEP_LABEL: Record<Sweep["sweep"], MessageKey> = {
  settled_thread_verdicts: "captureHealth.sweep.settledThreads",
  stranded_contacts: "captureHealth.sweep.strandedContacts",
  filed_meeting_holds: "captureHealth.sweep.filedMeetings",
};

type SweepWarning = Readonly<{ label: MessageKey; tone: "warning" | "danger" }>;

// Why a pass needs attention, most specific first, or null when it keeps up.
// "Overdue" is measured against the report's own time rather than the reader's
// clock, so the card and the numbers beside it describe one instant.
export function sweepWarning(
  sweep: Sweep,
  generatedAt: string,
): SweepWarning | null {
  const run = sweep.last_run;
  if (!run) {
    return { label: "captureHealth.warn.neverRun", tone: "warning" };
  }
  if (run.outcome === "failed") {
    return { label: "captureHealth.warn.failed", tone: "danger" };
  }
  if (run.outcome === "skipped") {
    return { label: "captureHealth.warn.skipped", tone: "warning" };
  }
  const cadence = sweep.cadence_seconds;
  if (cadence !== null && cadence !== undefined) {
    const succeeded = sweep.last_succeeded_at;
    const since =
      succeeded === null || succeeded === undefined
        ? Number.POSITIVE_INFINITY
        : Date.parse(generatedAt) - Date.parse(succeeded);
    if (since > 2 * cadence * 1000) {
      return { label: "captureHealth.warn.overdue", tone: "warning" };
    }
  }
  if (run.cap_hit) {
    return { label: "captureHealth.warn.backlog", tone: "warning" };
  }
  return null;
}

function formatCadence(
  seconds: number,
  plural: PluralTranslator,
  locale: Locale,
): string {
  const [unit, count] =
    seconds % 86_400 === 0
      ? (["Days", seconds / 86_400] as const)
      : seconds % 3_600 === 0
        ? (["Hours", seconds / 3_600] as const)
        : (["Minutes", Math.max(1, Math.round(seconds / 60))] as const);
  const base: PluralBase = `captureHealth.every${unit}`;
  return plural(base, count, { count: formatNumber(count, locale) });
}

function sweepFacts(
  health: Health,
  t: Translator,
  plural: PluralTranslator,
  locale: Locale,
  zone: string,
): Fact[] {
  return health.sweeps.map((sweep) => {
    const warning = sweepWarning(sweep, health.generated_at);
    const succeeded = sweep.last_succeeded_at;
    const errorClass = sweep.last_run?.error_class;
    const cadence = sweep.cadence_seconds;
    return {
      key: sweep.sweep,
      term: <span>{t(SWEEP_LABEL[sweep.sweep])}</span>,
      value: warning ? (
        <Badge tone={warning.tone}>{t(warning.label)}</Badge>
      ) : (
        <Badge tone="success">{t("captureHealth.keepingUp")}</Badge>
      ),
      note: (
        <>
          {succeeded === null || succeeded === undefined
            ? t("captureHealth.neverSucceeded")
            : t("captureHealth.lastSucceeded", {
                when: formatDateTime(succeeded, locale, zone),
              })}
          {cadence !== null && cadence !== undefined && (
            <>
              {" · "}
              {formatCadence(cadence, plural, locale)}
            </>
          )}
          {/* Verbatim, as the job card shows a class: it is the token an
              operator greps the worker log for. */}
          {errorClass !== undefined && (
            <>
              {" · "}
              <span>{errorClass}</span>
            </>
          )}
        </>
      ),
    };
  });
}

function waitedNote(
  key: MessageKey,
  seconds: number | null | undefined,
  t: Translator,
  plural: PluralTranslator,
  locale: Locale,
): string | null {
  if (seconds === null || seconds === undefined) {
    return null;
  }
  return t(key, { waited: formatWaitedFor(seconds, plural, locale) });
}

function mailboxFacts(
  mailboxes: readonly Mailbox[],
  t: Translator,
  plural: PluralTranslator,
  locale: Locale,
): Fact[] {
  return mailboxes.map((box) => {
    const notes = [
      waitedNote(
        "captureHealth.oldestContact",
        box.oldest_contact_age_seconds,
        t,
        plural,
        locale,
      ),
      waitedNote(
        "captureHealth.oldestThread",
        box.oldest_thread_age_seconds,
        t,
        plural,
        locale,
      ),
    ].filter((note): note is string => note !== null);
    return {
      key: box.user_id,
      term: (
        <span>{box.display_name ?? t("captureHealth.unnamedMailbox")}</span>
      ),
      value: (
        <Row>
          <Badge
            tone={box.contacts_awaiting_decision > 0 ? "warning" : undefined}
          >
            {plural(
              "captureHealth.contactsWaiting",
              box.contacts_awaiting_decision,
              { count: formatNumber(box.contacts_awaiting_decision, locale) },
            )}
          </Badge>
          <Badge
            tone={box.threads_awaiting_verdict > 0 ? "warning" : undefined}
          >
            {plural(
              "captureHealth.threadsWaiting",
              box.threads_awaiting_verdict,
              { count: formatNumber(box.threads_awaiting_verdict, locale) },
            )}
          </Badge>
        </Row>
      ),
      note: notes.length > 0 ? notes.join(" · ") : undefined,
    };
  });
}

function installationFacts(
  health: Health,
  t: Translator,
  plural: PluralTranslator,
  locale: Locale,
): Fact[] {
  const { classifier, held_meetings: held } = health;
  const shown = (value: number) => formatNumber(value, locale);
  return [
    {
      key: "classifier",
      term: <span>{t("captureHealth.classifier")}</span>,
      value: (
        <Row>
          <Badge>
            {plural("captureHealth.pending", classifier.pending, {
              count: shown(classifier.pending),
            })}
          </Badge>
          <Badge tone={classifier.unsure > 0 ? "warning" : undefined}>
            {plural("captureHealth.unsure", classifier.unsure, {
              count: shown(classifier.unsure),
            })}
          </Badge>
          <Badge tone={classifier.exhausted > 0 ? "warning" : undefined}>
            {plural("captureHealth.exhausted", classifier.exhausted, {
              count: shown(classifier.exhausted),
            })}
          </Badge>
        </Row>
      ),
      note:
        waitedNote(
          "captureHealth.oldestPending",
          classifier.oldest_pending_age_seconds,
          t,
          plural,
          locale,
        ) ?? undefined,
    },
    {
      key: "held-meetings",
      term: <span>{t("captureHealth.heldMeetings")}</span>,
      value: (
        <Badge tone={held.count > 0 ? "warning" : undefined}>
          {plural("captureHealth.meetingsHeld", held.count, {
            count: shown(held.count),
          })}
        </Badge>
      ),
      note:
        waitedNote(
          "captureHealth.oldestMeeting",
          held.oldest_age_seconds,
          t,
          plural,
          locale,
        ) ?? t("captureHealth.heldMeetingsSub"),
    },
  ];
}

function CaptureHealthBody({
  health,
  zone,
}: Readonly<{ health: Health; zone: string }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  return (
    <SettingList>
      <SettingRow
        label={t("captureHealth.sweeps")}
        description={t("captureHealth.sweepsSub")}
        layout="stack"
        control={
          <div className="settingrow-measure">
            <FactList facts={sweepFacts(health, t, plural, locale, zone)} />
          </div>
        }
      />
      <SettingRow
        label={t("captureHealth.mailboxes")}
        description={t("captureHealth.noDetail")}
        layout="stack"
        control={
          <div className="settingrow-measure">
            {health.mailboxes.length === 0 ? (
              <EmptyState>{t("captureHealth.mailboxesEmpty")}</EmptyState>
            ) : (
              <FactList
                numeric
                facts={mailboxFacts(health.mailboxes, t, plural, locale)}
              />
            )}
          </div>
        }
      />
      <SettingRow
        label={t("captureHealth.installation")}
        layout="stack"
        control={
          <div className="settingrow-measure">
            <FactList
              numeric
              facts={installationFacts(health, t, plural, locale)}
            />
          </div>
        }
      />
    </SettingList>
  );
}

// A payload missing a part is reported rather than defaulted: `?? []` would
// draw a clean installation, which is the reassurance this card must not give
// on a response it could not read.
function isCaptureHealth(data: Health | undefined): data is Health {
  return (
    data !== undefined &&
    typeof data.generated_at === "string" &&
    Array.isArray(data.mailboxes) &&
    Array.isArray(data.sweeps) &&
    typeof data.classifier === "object" &&
    data.classifier !== null &&
    typeof data.held_meetings === "object" &&
    data.held_meetings !== null
  );
}

export function CaptureHealthCard() {
  const t = useT();
  const { locale } = useLocale();
  const zone = viewerZone();
  // `job_health:read`, what the endpoint asks for (compose/capturehealth.go).
  const canSee = useCan("job_health", "read");
  const query = useQuery({
    queryKey: ["capture-health"],
    enabled: canSee,
    queryFn: async () => {
      const { data, error } = await api.GET("/admin/capture-health");
      if (error) {
        throwProblem(error);
      }
      if (!isCaptureHealth(data)) {
        throw new Error("malformed capture-health response");
      }
      return data;
    },
  });

  return (
    <HealthCard
      title={t("settings.captureHealth")}
      sub={t("settings.captureHealthSub")}
      withheld={t("captureHealth.adminOnly")}
      canSee={canSee}
      query={query}
      footer={(report) =>
        t("captureHealth.generatedAt", {
          time: formatDateTime(report.generated_at, locale, zone),
        })
      }
    >
      {(health) => <CaptureHealthBody health={health} zone={zone} />}
    </HealthCard>
  );
}
