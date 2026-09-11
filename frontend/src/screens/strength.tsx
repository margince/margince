// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useRecordZone } from "../app/recordzone";
import { ActivityReferenceList } from "../design-system/activityreferencelist";
import {
  Badge,
  Disclosure,
  EmptyState,
  Skeleton,
} from "../design-system/atoms";
import { Panel, PanelBody } from "../design-system/panel";
import { Meter } from "../design-system/readings";
import { formatDateTime, formatNumber } from "../format/format";
import { useLocale, useT } from "../i18n";
import {
  OverlayUnavailable,
  problemMessageOf,
  throwProblem,
  useSorMode,
} from "./common";

// The relationship-strength card (Phase 3, P-4): "no mystery number" — the
// composite score NEVER renders alone. It always carries its bucket badge
// and the full recency/frequency/reciprocity/direction factor breakdown that
// explains it (spec ai-operational-spec.md), plus the receipts (last
// interaction, 90d in/out counts, contributing-activity count). A record
// with no qualifying interactions is bucket:none, score:0 — that's
// rendered plainly (0% bars, an honest "no interactions yet" caption), never
// hidden or dressed up as an error.

type RelationshipStrength = components["schemas"]["RelationshipStrength"];

const BUCKET_TONE: Record<
  RelationshipStrength["bucket"],
  "success" | "accent" | "warn" | undefined
> = {
  strong: "success",
  moderate: "accent",
  weak: "warn",
  none: undefined,
};

async function fetchStrength(
  kind: "person" | "company",
  id: string,
): Promise<RelationshipStrength> {
  if (kind === "person") {
    const { data, error } = await api.GET("/people/{id}/strength", {
      params: { path: { id } },
    });
    if (error) {
      throwProblem(error);
    }
    return data;
  }
  const { data, error } = await api.GET("/companies/{id}/strength", {
    params: { path: { id } },
  });
  if (error) {
    throwProblem(error);
  }
  return data;
}

function factorPercent(value: number): number {
  return Math.round(value * 100);
}

export function StrengthPanel({
  kind,
  id,
  onOpenEmail,
}: Readonly<{
  kind: "person" | "company";
  id: string;
  // Opens one cited message in the host's own drawer. A host that mounts none
  // passes nothing, and the receipts render without an opener.
  onOpenEmail?: (activityId: string) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  // Relationship strength is computed over the native people graph, which the
  // incumbent mirror does not hold (the endpoint 404s in overlay). Show the
  // honest unavailable state and skip the doomed fetch.
  const overlay = useSorMode() === "overlay";
  const query = useQuery({
    queryKey: ["strength", kind, id],
    queryFn: () => fetchStrength(kind, id),
    enabled: !overlay,
  });

  return (
    <Panel title={t("strength.title")}>
      <PanelBody>
        {overlay && <OverlayUnavailable />}
        {!overlay && query.isPending && (
          <div
            style={{
              display: "flex",
              flexDirection: "column",
              gap: "var(--space-3)",
            }}
          >
            <Skeleton width="40%" />
            <Skeleton width="90%" />
          </div>
        )}
        {!overlay && query.isError && (
          <EmptyState>{problemMessageOf(query.error, t)}</EmptyState>
        )}
        {!overlay && query.isSuccess && (
          <StrengthBody
            strength={query.data}
            locale={locale}
            onOpenEmail={onOpenEmail}
          />
        )}
      </PanelBody>
    </Panel>
  );
}

function StrengthBody({
  strength,
  locale,
  onOpenEmail,
}: Readonly<{
  strength: RelationshipStrength;
  locale: ReturnType<typeof useLocale>["locale"];
  onOpenEmail?: (activityId: string) => void;
}>) {
  const t = useT();
  const recordZone = useRecordZone();
  // The contract guarantees factors/bucket/score, but a single data-driven
  // card must never crash the whole 360 if a response arrives malformed —
  // degrade to the honest zero/none reading instead (craft T7).
  const factors = strength.factors ?? {
    recency: 0,
    frequency: 0,
    reciprocity: 0,
    direction: 0,
  };
  const bucket = strength.bucket ?? "none";
  const score = strength.score ?? 0;
  const factorRows: Array<{
    key: "recency" | "frequency" | "reciprocity" | "direction";
    value: number;
  }> = [
    { key: "recency", value: factors.recency },
    { key: "frequency", value: factors.frequency },
    { key: "reciprocity", value: factors.reciprocity },
    { key: "direction", value: factors.direction },
  ];
  const contributingCount = strength.contributing_activity_ids?.length ?? 0;
  const named = strength.contributing_activities ?? [];

  return (
    <div>
      <div
        style={{
          display: "flex",
          alignItems: "center",
          gap: "var(--space-2)",
          flexWrap: "wrap",
          marginBottom: 12,
        }}
      >
        <Badge tone={BUCKET_TONE[bucket]}>
          {t(`strength.bucket.${bucket}`)}
        </Badge>
        <span className="t-mono">
          {t("strength.score", { score: formatNumber(score, locale) })}
        </span>
      </div>
      <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
        {factorRows.map((row) => {
          const pct = factorPercent(row.value);
          return (
            <div key={row.key}>
              <div
                style={{
                  display: "flex",
                  justifyContent: "space-between",
                  fontSize: "var(--fs-sm)",
                }}
              >
                <span>{t(`strength.factor.${row.key}`)}</span>
                <span className="t-mono">{formatNumber(pct, locale)}%</span>
              </div>
              <Meter
                value={pct}
                max={100}
                label={t(`strength.factor.${row.key}`)}
              />
            </div>
          );
        })}
      </div>
      <p className="t-caption" style={{ marginTop: "var(--space-2)" }}>
        {strength.last_interaction
          ? t("strength.lastInteraction", {
              when: formatDateTime(
                strength.last_interaction,
                locale,
                recordZone,
              ),
            })
          : t("strength.none")}
      </p>
      {(strength.inbound_90d != null || strength.outbound_90d != null) && (
        <p className="t-caption">
          {t("strength.inout", {
            in: formatNumber(strength.inbound_90d ?? 0, locale),
            out: formatNumber(strength.outbound_90d ?? 0, locale),
          })}
        </p>
      )}
      {contributingCount > 0 &&
        (named.length > 0 ? (
          // The receipts, foldable. The count alone is a claim a reader cannot
          // check: they cannot tell whether it counts the exchange they
          // remember, and they cannot open any of it.
          //
          // The SUMMARY keeps the count from the id array, never from the
          // named list. The two can differ — a row this reader cannot discover
          // is omitted — and a number that shrank to what one reader may open
          // would tell two readers different things about one score.
          <Disclosure
            summary={t("strength.computedFrom", {
              count: formatNumber(contributingCount, locale),
            })}
          >
            <ActivityReferenceList
              references={named}
              onOpenEmail={onOpenEmail}
              formatWhen={(when) => formatDateTime(when, locale, recordZone)}
            />
          </Disclosure>
        ) : (
          // An older server names none, and so does a reader whose seat holds
          // no activity grant. The line reads as it always did.
          <p className="t-caption">
            {t("strength.computedFrom", {
              count: formatNumber(contributingCount, locale),
            })}
          </p>
        ))}
    </div>
  );
}
