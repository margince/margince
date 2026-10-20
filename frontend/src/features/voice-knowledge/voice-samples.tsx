// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery } from "@tanstack/react-query";
import { useId, useRef, useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { ifMatch } from "../../api/version";
import { Badge, Button, OverflowMenu } from "../../design-system/atoms";
import { CellStack } from "../../design-system/cellstack";
import { ConfirmModal } from "../../design-system/confirmmodal";
import { DataTable, type DataTableColumn } from "../../design-system/datatable";
import { PanelBody } from "../../design-system/panel";
import { useLastSubject } from "../../design-system/presence";
import { QueryGate } from "../../design-system/querystates";
import { formatDate, formatNumber } from "../../format/format";
import {
  type Locale,
  type PluralTranslator,
  type Translator,
  useLocale,
  usePlural,
  useT,
} from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { problemMessageOf, unwrap } from "../../lib/problem";
import { useRecordZone } from "../../lib/recordzone";
import { VOICE_MIN_WORDS } from "../../lib/voice-intake-core";

type VoiceCorpusSource = components["schemas"]["VoiceCorpusSource"];
type VoiceCorpusSummary = components["schemas"]["VoiceCorpusSummary"];

export function useVoiceSources(profileId: string) {
  return useQuery({
    queryKey: ["voice-sources", profileId],
    queryFn: async () => {
      const data = unwrap(
        await api.GET("/voice-profiles/{id}/sources", {
          params: { path: { id: profileId } },
        }),
      );
      return { sources: data.data, summary: data.summary };
    },
  });
}

// Mirrors the server's quality bands so a removal can name the drop before it
// happens; the server remains the authority.
function bandFor(totalWords: number): VoiceCorpusSummary["quality_band"] {
  if (totalWords < 8000) {
    return "thin";
  }
  if (totalWords < 20000) {
    return "good";
  }
  if (totalWords < 30000) {
    return "rich";
  }
  return "sharp";
}

const BAND_LABEL: Record<VoiceCorpusSummary["quality_band"], MessageKey> = {
  thin: "settings.voice.bandThin",
  good: "settings.voice.bandGood",
  rich: "settings.voice.bandRich",
  sharp: "settings.voice.bandSharp",
};

// Whether a wire value has words in this build; a newer server may send more.
export function hasLabel<Key extends string>(
  labels: Record<Key, MessageKey>,
  value: string,
): value is Key {
  return Object.hasOwn(labels, value);
}

// An unknown value (a newer server) renders verbatim rather than breaking.
export function bandLabel(t: Translator, band: string | undefined): string {
  if (band === undefined) {
    return "";
  }
  return hasLabel(BAND_LABEL, band) ? t(BAND_LABEL[band]) : band;
}

const REGISTER_LABEL: Record<VoiceCorpusSource["register"], MessageKey> = {
  email: "settings.voice.register.email",
  social: "settings.voice.register.social",
  long_form: "settings.voice.register.long_form",
  spoken: "settings.voice.register.spoken",
  general: "settings.voice.register.general",
};

function registerLabel(t: Translator, register: string): string {
  return hasLabel(REGISTER_LABEL, register)
    ? t(REGISTER_LABEL[register])
    : register;
}

const KIND_LABEL: Record<VoiceCorpusSource["kind"], MessageKey> = {
  email: "settings.voice.kind.email",
  linkedin: "settings.voice.kind.linkedin",
  proposal: "settings.voice.kind.proposal",
  transcript: "settings.voice.kind.transcript",
  document: "settings.voice.kind.document",
  other: "settings.voice.kind.other",
};

const EXCLUSION_LABEL: Record<
  "owner_excluded" | "owner_removed" | "corpus_cleared",
  MessageKey
> = {
  owner_excluded: "settings.voice.exclusion.ownerExcluded",
  owner_removed: "settings.voice.exclusion.ownerRemoved",
  corpus_cleared: "settings.voice.exclusion.corpusCleared",
};

// A reason this build has no words for stays visible as its code.
function ExclusionReason({ reason }: Readonly<{ reason: string | null }>) {
  const t = useT();
  if (reason === null) {
    return null;
  }
  return hasLabel(EXCLUSION_LABEL, reason) ? (
    <span className="t-caption">{t(EXCLUSION_LABEL[reason])}</span>
  ) : (
    <code className="t-caption">{reason}</code>
  );
}

// Where the corpus words come from; spoken samples are the gap worth naming.
function RegisterMix({ summary }: Readonly<{ summary: VoiceCorpusSummary }>) {
  const t = useT();
  const entries = Object.entries(summary.register_words).filter(
    ([, words]) => words > 0,
  );
  if (entries.length === 0 || summary.total_words === 0) {
    return null;
  }
  return (
    <p className="t-caption vdna-regmix">
      {entries
        .map(
          ([register, words]) =>
            `${registerLabel(t, register)} ${Math.round((words / summary.total_words) * 100)}%`,
        )
        .join(" · ")}
    </p>
  );
}

// Below the first-build floor, the distance to it is what the reader asks.
function FloorMeter({ words }: Readonly<{ words: number }>) {
  const t = useT();
  const { locale } = useLocale();
  return (
    <p className="t-caption vdna-floor">
      <progress
        value={Math.min(words, VOICE_MIN_WORDS)}
        max={VOICE_MIN_WORDS}
        aria-label={t("settings.voice.floorLabel", {
          min: formatNumber(VOICE_MIN_WORDS, locale),
        })}
      />
      <span>
        {t("settings.voice.floorProgress", {
          words: formatNumber(words, locale),
          min: formatNumber(VOICE_MIN_WORDS, locale),
        })}
      </span>
    </p>
  );
}

function sampleColumns({
  t,
  plural,
  locale,
  zone,
  canEdit,
  onRemove,
}: Readonly<{
  t: Translator;
  plural: PluralTranslator;
  locale: Locale;
  zone: string;
  canEdit: boolean;
  onRemove: (source: VoiceCorpusSource) => void;
}>): DataTableColumn<VoiceCorpusSource>[] {
  const columns: DataTableColumn<VoiceCorpusSource>[] = [
    {
      key: "sample",
      header: t("settings.voice.col.sample"),
      grow: true,
      render: (source) => (
        <CellStack>
          <span>{source.source_label}</span>
          <span className="t-caption">
            {hasLabel(KIND_LABEL, source.kind)
              ? t(KIND_LABEL[source.kind])
              : source.kind}
          </span>
        </CellStack>
      ),
    },
    {
      key: "register",
      header: t("settings.voice.col.register"),
      render: (source) => registerLabel(t, source.register),
    },
    {
      key: "words",
      header: t("settings.voice.col.words"),
      align: "end",
      render: (source) => formatNumber(source.word_count, locale),
      folded: (source) =>
        plural("settings.voice.wordCount", source.word_count, {
          count: formatNumber(source.word_count, locale),
        }),
    },
    {
      key: "added",
      header: t("settings.voice.col.added"),
      render: (source) => formatDate(source.created_at, locale, zone),
    },
    {
      key: "excluded",
      header: t("settings.voice.col.state"),
      fold: "end",
      render: (source) =>
        !source.included && (
          <CellStack>
            <Badge tone="warning">{t("settings.voice.excluded")}</Badge>
            <ExclusionReason reason={source.exclusion_reason} />
          </CellStack>
        ),
    },
  ];
  if (!canEdit) {
    return columns;
  }
  return [
    ...columns,
    {
      key: "actions",
      header: t("settings.voice.col.actions"),
      headerHidden: true,
      fold: "end",
      render: (source) => (
        <span className="cell-actions">
          <OverflowMenu
            label={t("table.rowActions", { name: source.source_label })}
          >
            <Button aria-haspopup="dialog" onClick={() => onRemove(source)}>
              {t("settings.voice.removeSource")}
            </Button>
          </OverflowMenu>
        </span>
      ),
    },
  ];
}

/** Stands straight in the corpus panel, so the samples table bleeds. */
export function VoiceSamples({
  profileId,
  canEdit,
  onChanged,
}: Readonly<{
  profileId: string;
  canEdit: boolean;
  onChanged: () => void;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const zone = useRecordZone();
  const sources = useVoiceSources(profileId);
  const [removing, setRemoving] = useState<VoiceCorpusSource | null>(null);
  const asked = useLastSubject(removing);
  // A removed sample takes its row's menu with it; focus lands on the new total.
  const removed = useRef(false);
  const meterId = useId();
  const remove = useMutation({
    mutationFn: async (sample: Readonly<{ id: string; version: number }>) => {
      unwrap(
        await api.DELETE("/voice-profiles/{id}/sources/{sourceId}", {
          params: {
            path: { id: profileId, sourceId: sample.id },
            ...ifMatch(sample.version),
          },
        }),
      );
    },
    onSuccess: () => {
      removed.current = true;
      setRemoving(null);
      onChanged();
    },
  });
  const columns = sampleColumns({
    t,
    plural,
    locale,
    zone,
    canEdit,
    onRemove: (source) => {
      remove.reset();
      removed.current = false;
      setRemoving(source);
    },
  });

  return (
    <QueryGate query={sources} pendingLabel={t("settings.voice.corpusLabel")}>
      {({ sources: rows, summary }) => {
        const bandAfter =
          asked && bandFor(Math.max(0, summary.total_words - asked.word_count));
        const drops =
          asked?.included === true && bandAfter !== summary.quality_band;
        return (
          <>
            <PanelBody>
              <p className="t-caption" id={meterId} tabIndex={-1}>
                {t("settings.voice.meter", {
                  count: formatNumber(summary.total_words, locale),
                  target: formatNumber(summary.target_words, locale),
                })}
              </p>
              {summary.total_words < VOICE_MIN_WORDS && (
                <FloorMeter words={summary.total_words} />
              )}
              <RegisterMix summary={summary} />
              {rows.length === 0 && <p>{t("settings.voice.corpusEmpty")}</p>}
            </PanelBody>
            {rows.length > 0 && (
              <DataTable
                bleed
                fold
                label={t("settings.voice.corpusRowLabel")}
                columns={columns}
                rows={rows}
                rowKey={(source) => source.id}
              />
            )}
            <ConfirmModal
              open={removing !== null}
              title={t("settings.voice.removeConfirm.title", {
                name: asked?.source_label ?? "",
              })}
              confirmLabel={t("settings.voice.removeSource")}
              confirmVariant="danger"
              pending={remove.isPending}
              error={remove.isError ? problemMessageOf(remove.error, t) : null}
              onClose={() => setRemoving(null)}
              returnFocusTo={() =>
                removed.current ? document.getElementById(meterId) : null
              }
              onConfirm={() =>
                removing &&
                remove.mutate({ id: removing.id, version: removing.version })
              }
            >
              <p>
                {drops
                  ? t("settings.voice.bandDrop", {
                      from: bandLabel(t, summary.quality_band),
                      to: bandLabel(t, bandAfter ?? undefined),
                    })
                  : t("settings.voice.removeConfirm.body")}
              </p>
            </ConfirmModal>
          </>
        );
      }}
    </QueryGate>
  );
}
