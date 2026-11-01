// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { Badge } from "../../design-system/atoms";
import { CellStack } from "../../design-system/cellstack";
import { DataTable, type DataTableColumn } from "../../design-system/datatable";
import { type Fact, FactList } from "../../design-system/factlist";
import {
  PanelBody,
  PanelGroupHead,
  PanelIntro,
} from "../../design-system/panel";
import { formatDateTime, formatNumber } from "../../format/format";
import { formatAge, formatInterval } from "../../format/interval";
import { viewerZone } from "../../format/timezone";
import {
  type Locale,
  type PluralTranslator,
  type Translator,
  useLocale,
  usePlural,
  useT,
} from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { useCan } from "../../lib/capability";
import { useHealthScope, useHealthVisible } from "../../lib/healthscope";
import { unwrap } from "../../lib/problem";
import { HealthCard } from "./healthcard";

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

function sweepColumns(
  health: Health,
  t: Translator,
  plural: PluralTranslator,
  locale: Locale,
  zone: string,
): DataTableColumn<Sweep>[] {
  const generatedAt = health.generated_at;
  // A column that would stand empty in every row is left out.
  const warned = health.sweeps.some((sweep) =>
    sweepWarning(sweep, generatedAt),
  );
  const columns: DataTableColumn<Sweep>[] = [
    {
      key: "pass",
      header: t("captureHealth.col.pass"),
      render: (sweep) => t(SWEEP_LABEL[sweep.sweep]),
    },
    {
      key: "lastSuccess",
      header: t("captureHealth.col.lastSuccess"),
      render: (sweep) =>
        sweep.last_succeeded_at
          ? formatDateTime(sweep.last_succeeded_at, locale, zone)
          : t("captureHealth.never"),
      folded: (sweep) =>
        sweep.last_succeeded_at
          ? t("captureHealth.lastSucceeded", {
              when: formatDateTime(sweep.last_succeeded_at, locale, zone),
            })
          : t("captureHealth.neverSucceeded"),
    },
    {
      key: "cadence",
      header: t("captureHealth.col.runsEvery"),
      render: (sweep) =>
        sweep.cadence_seconds
          ? formatInterval(sweep.cadence_seconds, locale)
          : null,
      folded: (sweep) =>
        sweep.cadence_seconds
          ? t("captureHealth.runsEvery", {
              interval: formatInterval(sweep.cadence_seconds, locale),
            })
          : undefined,
    },
    {
      key: "processed",
      header: t("captureHealth.col.processed"),
      align: "end",
      render: (sweep) =>
        sweep.last_run ? formatNumber(sweep.last_run.processed, locale) : null,
      folded: (sweep) =>
        sweep.last_run
          ? plural("captureHealth.processed", sweep.last_run.processed, {
              count: formatNumber(sweep.last_run.processed, locale),
            })
          : undefined,
    },
  ];
  if (!warned) {
    return columns;
  }
  return [
    ...columns,
    {
      key: "warning",
      header: t("captureHealth.col.state"),
      fold: "end",
      render: (sweep) => {
        const warning = sweepWarning(sweep, generatedAt);
        const errorClass = sweep.last_run?.error_class;
        // The class verbatim, as the job card shows one: the worker-log token.
        return (
          warning && (
            <CellStack>
              <Badge tone={warning.tone}>{t(warning.label)}</Badge>
              {errorClass && <code className="t-caption">{errorClass}</code>}
            </CellStack>
          )
        );
      },
    },
  ];
}

// A count above zero is the one thing in the row worth finding.
function waitingCount(count: number, shown: ReactNode): ReactNode {
  return count > 0 ? <Badge tone="warning">{shown}</Badge> : shown;
}

function oldestWait(box: Mailbox, locale: Locale): string | null {
  const ages = [
    box.oldest_contact_age_seconds,
    box.oldest_thread_age_seconds,
  ].filter((age): age is number => typeof age === "number");
  return ages.length === 0 ? null : formatAge(Math.max(...ages), locale);
}

function mailboxColumns(
  t: Translator,
  plural: PluralTranslator,
  locale: Locale,
): DataTableColumn<Mailbox>[] {
  const counted = (
    key: string,
    header: MessageKey,
    phrase: "captureHealth.contactsWaiting" | "captureHealth.threadsWaiting",
    count: (box: Mailbox) => number,
  ): DataTableColumn<Mailbox> => ({
    key,
    header: t(header),
    align: "end",
    render: (box) => waitingCount(count(box), formatNumber(count(box), locale)),
    folded: (box) =>
      waitingCount(
        count(box),
        plural(phrase, count(box), {
          count: formatNumber(count(box), locale),
        }),
      ),
  });
  return [
    {
      key: "owner",
      header: t("captureHealth.col.owner"),
      render: (box) => box.display_name ?? t("captureHealth.unnamedMailbox"),
    },
    counted(
      "contacts",
      "captureHealth.col.contacts",
      "captureHealth.contactsWaiting",
      (box) => box.contacts_awaiting_decision,
    ),
    counted(
      "threads",
      "captureHealth.col.threads",
      "captureHealth.threadsWaiting",
      (box) => box.threads_awaiting_verdict,
    ),
    {
      key: "oldest",
      header: t("captureHealth.col.oldestWait"),
      align: "end",
      render: (box) => oldestWait(box, locale),
      folded: (box) => {
        const age = oldestWait(box, locale);
        return age === null
          ? undefined
          : t("captureHealth.oldestWaited", { age });
      },
    },
    {
      key: "unread",
      header: t("captureHealth.col.unread"),
      // Dates and not a duration. The owner's next move is to ask the backfill
      // for this period, and a span of days is not something to type into it.
      render: (box) =>
        box.unread_period
          ? t("captureHealth.unreadPeriod", {
              from: formatDateTime(
                box.unread_period.from,
                locale,
                viewerZone(),
              ),
              to: formatDateTime(box.unread_period.to, locale, viewerZone()),
            })
          : "",
      folded: (box) =>
        box.unread_period
          ? t("captureHealth.unreadPeriodFolded", {
              from: formatDateTime(
                box.unread_period.from,
                locale,
                viewerZone(),
              ),
              to: formatDateTime(box.unread_period.to, locale, viewerZone()),
            })
          : undefined,
    },
  ];
}

function installationFacts(
  health: Health,
  t: Translator,
  locale: Locale,
): Fact[] {
  const { classifier, held_meetings: held } = health;
  const waiting = health.mailboxes.length;
  const waited = (seconds: number | null | undefined) =>
    typeof seconds === "number"
      ? t("captureHealth.oldestWaited", { age: formatAge(seconds, locale) })
      : undefined;
  return [
    {
      key: "pending",
      term: t("captureHealth.fact.pending"),
      value: formatNumber(classifier.pending, locale),
      note: waited(classifier.oldest_pending_age_seconds),
    },
    {
      key: "unsure",
      term: t("captureHealth.fact.unsure"),
      value: waitingCount(
        classifier.unsure,
        formatNumber(classifier.unsure, locale),
      ),
    },
    {
      key: "exhausted",
      term: t("captureHealth.fact.exhausted"),
      value: waitingCount(
        classifier.exhausted,
        formatNumber(classifier.exhausted, locale),
      ),
    },
    {
      key: "mailboxes",
      term: t("captureHealth.fact.mailboxes"),
      value: waitingCount(waiting, formatNumber(waiting, locale)),
    },
    {
      key: "held-meetings",
      term: t("captureHealth.heldMeetings"),
      value: waitingCount(held.count, formatNumber(held.count, locale)),
      note:
        waited(held.oldest_age_seconds) ?? t("captureHealth.heldMeetingsSub"),
    },
  ];
}

// The mailbox table only when one has something waiting; the installation's
// counts above already say how many do.
function CaptureHealthBody({
  health,
  zone,
}: Readonly<{ health: Health; zone: string }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  return (
    <>
      <PanelGroupHead title={t("captureHealth.sweeps")} level="h3" />
      <PanelBody>
        <PanelIntro>{t("captureHealth.sweepsSub")}</PanelIntro>
      </PanelBody>
      <DataTable
        bleed
        fold
        label={t("captureHealth.sweeps")}
        columns={sweepColumns(health, t, plural, locale, zone)}
        rows={[...health.sweeps]}
        rowKey={(sweep) => sweep.sweep}
      />
      {health.mailboxes.length > 0 && (
        <>
          <PanelGroupHead title={t("captureHealth.mailboxes")} level="h3" />
          <PanelBody>
            <PanelIntro>{t("captureHealth.noDetail")}</PanelIntro>
          </PanelBody>
          <DataTable
            bleed
            fold
            label={t("captureHealth.mailboxes")}
            columns={mailboxColumns(t, plural, locale)}
            rows={[...health.mailboxes]}
            rowKey={(box) => box.user_id}
          />
        </>
      )}
    </>
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

// `job_health:read`, what the endpoint asks for (compose/capturehealth.go).
export function useCaptureHealth() {
  const canSee = useCan("job_health", "read");
  const scope = useHealthScope();
  const visible = useHealthVisible();
  const query = useQuery({
    queryKey: ["capture-health", scope],
    enabled: canSee && visible && scope !== undefined,
    queryFn: async ({ signal }) => {
      const data = unwrap(await api.GET("/admin/capture-health", { signal }));
      if (!isCaptureHealth(data)) {
        throw new Error("malformed capture-health response");
      }
      return data;
    },
  });
  return { canSee, query };
}

export function CaptureHealthCard() {
  const t = useT();
  const { locale } = useLocale();
  const zone = viewerZone();
  const { canSee, query } = useCaptureHealth();

  return (
    <HealthCard
      title={t("settings.captureHealth")}
      sub={t("settings.captureHealthSub")}
      withheld={t("captureHealth.adminOnly")}
      canSee={canSee}
      query={query}
      lead={(health) => (
        <FactList numeric facts={installationFacts(health, t, locale)} />
      )}
    >
      {(health) => <CaptureHealthBody health={health} zone={zone} />}
    </HealthCard>
  );
}
