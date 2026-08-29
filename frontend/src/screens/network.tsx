// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useRecordZone } from "../app/recordzone";
import { Badge, Card, EmptyState, Skeleton } from "../design-system/atoms";
import { formatDateTime, formatNumber } from "../format/format";
import { useLocale, useT } from "../i18n";
import "./network.css";
import {
  OverlayUnavailable,
  problemMessageOf,
  throwProblem,
  useSorMode,
} from "./common";

// The relationship-graph card (ADR-0078): "who here knows them", the question a
// rep asks before they write a cold email. It was server-only until it landed —
// the endpoint shipped and nothing rendered it, so the interaction projection
// was a fact the product held and never showed anybody.
//
// The deal COVERAGE card was the other half and is gone. Its seats moved to the
// deal rail (deal360/dealseats.tsx) when the readings band started counting
// them, and its findings became chips (dealsignals.tsx) — leaving a card nothing
// rendered, still carrying its own overlay handling, its own withheld-before-
// empty ordering and its own copy. Two spellings of one card, and the dead one
// had passing tests, which is what made it look maintained.
//
// Ordering is the answer here and is NOT re-sorted. The server ranks colleagues
// by a strength it computes at read; a client that re-ordered them would be a
// second implementation of the decay formula, and the two would disagree the
// moment either changed.

type PersonNetworkColleague = components["schemas"]["PersonNetworkColleague"];

// The per-user band vocabulary is PO-F-3b's — none/weak/moderate/strong — and
// deliberately NOT the workspace-wide card's dormant/weak/warm/strong. The two
// measure different things, and giving them one set of words on screen would
// invite a reader to compare numbers that are not comparable.
const COLLEAGUE_TONE: Record<
  PersonNetworkColleague["strength_bucket"],
  "success" | "accent" | "warn" | undefined
> = {
  strong: "success",
  moderate: "accent",
  weak: "warn",
  none: undefined,
};

async function fetchPersonNetwork(
  id: string,
): Promise<components["schemas"]["PersonNetwork"]> {
  const { data, error } = await api.GET("/people/{id}/network", {
    params: { path: { id } },
  });
  if (error) {
    throwProblem(error);
  }
  return data;
}

/** The colleagues who know this contact, warmest first. */
export function PersonNetworkCard({ id }: Readonly<{ id: string }>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  // The projection is folded from natively captured participants, which the
  // incumbent mirror does not hold. Show the honest unavailable state rather
  // than a doomed fetch that renders as "nobody knows them".
  const overlay = useSorMode() === "overlay";
  const query = useQuery({
    queryKey: ["person-network", id],
    queryFn: () => fetchPersonNetwork(id),
    enabled: !overlay,
  });
  const colleagues = query.data?.colleagues ?? [];

  return (
    <Card className="net-card" title={t("network.title")}>
      {overlay && <OverlayUnavailable />}
      {!overlay && query.isPending && <Skeleton width="80%" />}
      {!overlay && query.isError && (
        <EmptyState>{problemMessageOf(query.error, t)}</EmptyState>
      )}
      {!overlay && query.isSuccess && colleagues.length === 0 && (
        <EmptyState>{t("network.empty")}</EmptyState>
      )}
      {!overlay && colleagues.length > 0 && (
        <ul className="net-colleagues">
          {colleagues.map((colleague) => (
            <li key={colleague.user_id}>
              <span className="net-name">{colleague.display_name}</span>
              <Badge tone={COLLEAGUE_TONE[colleague.strength_bucket]}>
                {t(`network.bucket.${colleague.strength_bucket}`)}
              </Badge>
              {/* A `none` band carries NO count, for the same reason it
                  carries no score: never spoken and spoken-then-cold are
                  different facts, and a zero renders them identically. */}
              {colleague.strength_bucket !== "none" && (
                <span className="t-caption">
                  {t("network.interactions", {
                    count: formatNumber(colleague.interactions_90d, locale),
                  })}
                </span>
              )}
              <span className="t-caption">
                {colleague.last_at
                  ? formatDateTime(colleague.last_at, locale, recordZone)
                  : t("network.neverSpoken")}
              </span>
            </li>
          ))}
        </ul>
      )}
    </Card>
  );
}

/** What is wrong with how this deal is covered, and why. */
