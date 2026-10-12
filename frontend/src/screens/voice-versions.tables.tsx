// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import { Badge, Button, OverflowMenu } from "../design-system/atoms";
import type { DataTableColumn } from "../design-system/datatable";
import { formatDate, formatNumber, identifierNumber } from "../format/format";
import type { Locale, PluralTranslator, Translator } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { hasLabel } from "./voice-samples";

type VoiceProfileVersion = components["schemas"]["VoiceProfileVersion"];
type VoiceProfileDelta = components["schemas"]["VoiceProfileDelta"];

const STATUS_LABEL: Record<VoiceProfileVersion["status"], MessageKey> = {
  active: "voice.status.active",
  candidate: "voice.status.candidate",
  superseded: "voice.status.superseded",
  rejected: "voice.status.rejected",
};

const REASON_LABEL: Record<VoiceProfileVersion["reason"], MessageKey> = {
  onboarding: "voice.reason.onboarding",
  manual: "voice.reason.manual",
  automatic: "voice.reason.automatic",
  rollback: "voice.reason.rollback",
};

const CLASSIFICATION_LABEL: Record<
  VoiceProfileDelta["classification"],
  MessageKey
> = {
  routine: "voice.classification.routine",
  material: "voice.classification.material",
};

const OUTCOME_LABEL: Record<
  VoiceProfileDelta["activation_outcome"],
  MessageKey
> = {
  auto_activated: "voice.outcome.autoActivated",
  review_required: "voice.outcome.reviewRequired",
  manually_activated: "voice.outcome.manuallyActivated",
  rejected: "voice.outcome.rejected",
  rollback: "voice.outcome.rollback",
};

// A value a newer server sends renders verbatim rather than hiding the row.
function labelOf<Key extends string>(
  t: Translator,
  labels: Record<Key, MessageKey>,
  value: string,
): string {
  return hasLabel(labels, value) ? t(labels[value]) : value;
}

type Columns = Readonly<{
  t: Translator;
  plural: PluralTranslator;
  locale: Locale;
  zone: string;
}>;

// Only a version that asks something of the reader carries a badge. Superseded
// is history, and active is the normal reading.
export function versionColumns({
  t,
  plural,
  locale,
  zone,
  canEdit,
  onRestore,
}: Columns &
  Readonly<{
    canEdit: boolean;
    onRestore: (version: VoiceProfileVersion) => void;
  }>): DataTableColumn<VoiceProfileVersion>[] {
  const columns: DataTableColumn<VoiceProfileVersion>[] = [
    {
      key: "version",
      header: t("voice.history.col.version"),
      render: (version) =>
        t("voice.history.version", {
          n: identifierNumber(version.profile_version),
        }),
    },
    {
      key: "status",
      header: t("voice.history.col.status"),
      fold: "end",
      render: (version) =>
        version.status !== "superseded" &&
        version.status !== "active" && (
          <Badge tone={version.status === "rejected" ? undefined : "info"}>
            {labelOf(t, STATUS_LABEL, version.status)}
          </Badge>
        ),
    },
    {
      key: "created",
      header: t("voice.history.col.created"),
      render: (version) => formatDate(version.created_at, locale, zone),
    },
    {
      key: "reason",
      header: t("voice.history.col.reason"),
      render: (version) => labelOf(t, REASON_LABEL, version.reason),
    },
    {
      key: "samples",
      header: t("voice.history.col.samples"),
      align: "end",
      render: (version) => formatNumber(version.source_count, locale),
      folded: (version) =>
        plural("voice.history.sampleCount", version.source_count, {
          count: formatNumber(version.source_count, locale),
        }),
    },
  ];
  if (!canEdit) {
    return columns;
  }
  return [
    ...columns,
    {
      key: "actions",
      header: t("voice.history.col.actions"),
      headerHidden: true,
      fold: "end",
      render: (version) => {
        const n = identifierNumber(version.profile_version);
        return (
          version.status === "superseded" && (
            <span className="cell-actions">
              <OverflowMenu
                label={t("table.rowActions", {
                  name: t("voice.history.version", { n }),
                })}
              >
                <Button
                  aria-haspopup="dialog"
                  onClick={() => onRestore(version)}
                >
                  {t("voice.history.rollback", { n })}
                </Button>
              </OverflowMenu>
            </span>
          )
        );
      },
    },
  ];
}

export function changeColumns({
  t,
  plural,
  locale,
  zone,
}: Columns): DataTableColumn<VoiceProfileDelta>[] {
  return [
    {
      key: "change",
      header: t("voice.history.col.change"),
      render: (delta) =>
        t("voice.history.deltaRow", {
          from: identifierNumber(delta.from_version ?? 0),
          to: identifierNumber(delta.to_version),
        }),
    },
    {
      key: "kind",
      header: t("voice.history.col.kind"),
      render: (delta) => labelOf(t, CLASSIFICATION_LABEL, delta.classification),
    },
    {
      key: "outcome",
      header: t("voice.history.col.outcome"),
      render: (delta) => labelOf(t, OUTCOME_LABEL, delta.activation_outcome),
    },
    {
      key: "words",
      header: t("voice.history.col.wordsAdded"),
      align: "end",
      render: (delta) => formatNumber(delta.words_added, locale),
      folded: (delta) =>
        plural("voice.history.wordsAdded", delta.words_added, {
          count: formatNumber(delta.words_added, locale),
        }),
    },
    {
      key: "date",
      header: t("voice.history.col.date"),
      render: (delta) => formatDate(delta.created_at, locale, zone),
    },
  ];
}
