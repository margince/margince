// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { CellStack } from "../../design-system/cellstack";
import { DataTable, type DataTableColumn } from "../../design-system/datatable";
import { RowOpen } from "../../design-system/rowopen";
import { StatusBadge, type StatusKind } from "../../design-system/statusbadge";
import { formatDateTime } from "../../format/format";
import { viewerZone } from "../../format/timezone";
import { useLocale, useT } from "../../i18n";
import { useProviderLabel } from "../../lib/channelproviders";

// Settings › Capture activity: one row per message in the window, each opening
// the trace drawer that shows every step the pipeline took with it.

type TraceEntry = components["schemas"]["CaptureTraceEntry"];

// The reasons this screen can render, as a closed set. The catalog falls back
// to the key itself, so a reason outside the set renders nothing.
const REASONS = [
  "internal_only",
  "deferral_capped",
  "noise_prior",
  "decided_prior",
  "no_granting_human",
  "invisible_incumbent",
  "derivation_failed",
  "no_counterparty",
  "role_mailbox",
  "private_thread",
  "transactional_infra",
  "transactional_prefix",
] as const;

type KnownReason = (typeof REASONS)[number];

function knownReason(reason: string | null | undefined): KnownReason | null {
  return REASONS.find((known) => known === reason) ?? null;
}

// Closed for the same reason REASONS is: a status a newer binary writes would
// otherwise render as its catalog key.
const RESOLUTIONS = [
  "pending",
  "unsure",
  "real",
  "noise",
  "rejected",
  "suppressed",
] as const;

type KnownResolution = (typeof RESOLUTIONS)[number];

function knownResolution(status: string | undefined): KnownResolution | null {
  return RESOLUTIONS.find((known) => known === status) ?? null;
}

// Waiting on a verdict is neutral like every other settled outcome; only a
// fault means something broke.
const OUTCOME_KIND: Record<TraceEntry["outcome"], StatusKind> = {
  captured: "done",
  internal: "neutral",
  suppressed: "neutral",
  deferred: "neutral",
  fault: "failure",
};

export function CaptureMessages({
  entries,
  payloads,
  onOpen,
}: Readonly<{
  entries: TraceEntry[];
  /** Whether the installation stores senders and subjects at all. */
  payloads: boolean;
  onOpen: (entry: TraceEntry) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const providerLabel = useProviderLabel();
  // The reader's own zone: a trace is read against "I sent that at 9:04".
  const zone = viewerZone();
  const columns: DataTableColumn<TraceEntry>[] = [
    {
      key: "when",
      header: t("captureActivity.col.when"),
      render: (entry) => (
        <span className="capture-activity__nowrap">
          {formatDateTime(entry.occurred_at, locale, zone)}
        </span>
      ),
    },
    {
      key: "message",
      header: t("captureActivity.col.message"),
      fold: "title",
      render: (entry) => <EntryContent entry={entry} />,
    },
    {
      key: "outcome",
      header: t("captureActivity.col.outcome"),
      render: (entry) => <EntryOutcome entry={entry} />,
    },
    {
      key: "via",
      header: t("captureActivity.col.via"),
      render: (entry) => (
        <span className="capture-activity__nowrap">
          {providerLabel(entry.connector)}
        </span>
      ),
    },
    {
      key: "open",
      header: t("table.actions"),
      headerHidden: true,
      align: "end",
      fold: "end",
      render: (entry) => {
        const at = formatDateTime(entry.occurred_at, locale, zone);
        return (
          <RowOpen
            label={
              entry.counterparty
                ? t("captureActivity.openTraceFrom", {
                    sender: entry.counterparty,
                    at,
                  })
                : t("captureActivity.openTraceAt", { at })
            }
            opensDialog
            onOpen={() => onOpen(entry)}
          />
        );
      },
    },
  ];
  return (
    <DataTable
      bleed
      fold
      label={t("captureActivity.messages")}
      // Without stored payloads the message column has nothing to say, and the
      // note above the table says why once.
      columns={
        payloads ? columns : columns.filter((col) => col.key !== "message")
      }
      rows={entries}
      rowKey={(entry) => entry.id}
      rowTestId={(entry) => `capture-entry-${entry.id}`}
      onRowClick={onOpen}
    />
  );
}

function EntryContent({ entry }: Readonly<{ entry: TraceEntry }>) {
  const t = useT();
  if (!entry.counterparty && !entry.subject) {
    // Payload capture is on, so the absence is about this message.
    return (
      <span className="capture-activity__absent">
        {t("captureActivity.contentNone")}
      </span>
    );
  }
  return (
    <span className="capture-activity__content">
      <CellStack>
        {entry.counterparty && (
          <span className="capture-activity__line">{entry.counterparty}</span>
        )}
        {entry.subject && (
          <span className="capture-activity__line t-caption">
            {entry.subject}
          </span>
        )}
      </CellStack>
    </span>
  );
}

// The outcome and what qualifies it. A captured message needs no badge; every
// other outcome is the reason the reader opened this page.
function EntryOutcome({ entry }: Readonly<{ entry: TraceEntry }>) {
  const t = useT();
  const reason = knownReason(entry.reason);
  const resolution = knownResolution(entry.resolution?.status);
  const words = t(`captureActivity.outcome.${outcomeLabel(entry, reason)}`);
  const caption = reason
    ? t(`captureActivity.reason.${reason}`)
    : resolution && t(`captureActivity.resolution.${resolution}`);
  return (
    <CellStack>
      {entry.outcome_now === "captured" ? (
        <span>{words}</span>
      ) : (
        <StatusBadge kind={OUTCOME_KIND[entry.outcome]} label={words} />
      )}
      {caption && <span className="t-caption">{caption}</span>}
    </CellStack>
  );
}

// Only `pending` leaves a question open: a capped deferral never asked, and an
// unknown status is a newer binary's answer, so both read as not waiting.
function outcomeLabel(
  entry: TraceEntry,
  reason: KnownReason | null,
): TraceEntry["outcome"] | "deferred_capped" | "deferred_sent" {
  if (entry.outcome !== "deferred") {
    return entry.outcome;
  }
  if (reason === "deferral_capped") {
    return "deferred_capped";
  }
  if (!entry.resolution || entry.resolution.status === "pending") {
    return "deferred";
  }
  return "deferred_sent";
}
