// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  useInfiniteQuery,
  useQueries,
  useQueryClient,
} from "@tanstack/react-query";
import { useEffect } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { AppLink } from "../design-system/applink";
import { Button } from "../design-system/atoms";
import { ChoiceList } from "../design-system/choicelist";
import { ErrorLine } from "../design-system/errorline";
import { activityDay } from "../format/activitywhen";
import { formatDate } from "../format/format";
import { useLocale, useT } from "../i18n";
import {
  activityUnavailableQuery,
  markActivitiesReadable,
} from "../lib/activityread";
import { ENTITY } from "../lib/entity";
import { useSourceOpener } from "../lib/opensource";
import { unwrap } from "../lib/problem";
import { routeHash } from "../lib/router";
import { useViewerZone } from "../lib/viewerzone";
import { useInteractionLabel } from "./interactionchrome";

type Evidence = components["schemas"]["NoticeSourceEvidence"];
export type SourceEvidenceRef =
  components["schemas"]["NoticeSourceEvidenceReference"];

function referenceKey(ref: SourceEvidenceRef): string {
  return `${ref.kind}:${ref.id}`;
}

export function SourceReviewEvidence({
  caseId,
  originalKind,
  selected,
  onSelect,
}: Readonly<{
  caseId: string;
  originalKind: string | undefined;
  selected: SourceEvidenceRef | null;
  onSelect: (ref: SourceEvidenceRef | null) => void;
}>) {
  const t = useT();
  const opener = useSourceOpener();
  const client = useQueryClient();
  const read = useInfiniteQuery({
    queryKey: ["notice-source-evidence", caseId],
    initialPageParam: "",
    retry: false,
    queryFn: async ({ pageParam, signal }) => {
      const page = unwrap(
        await api.GET("/privacy/notice-case-reviews/{id}/source", {
          params: {
            path: { id: caseId },
            query: pageParam ? { cursor: pageParam } : {},
          },
          signal,
        }),
      );
      markActivitiesReadable(
        client,
        [page.original_activity, ...page.items]
          .filter((entry) => entry?.record_type === "activity")
          .map((entry) => entry?.record_id ?? ""),
      );
      return page;
    },
    getNextPageParam: (page) => page.next_cursor || undefined,
  });
  const candidates = useEvidenceCandidates(
    read.isError ? undefined : read.data?.pages,
    selected,
    onSelect,
  );
  const active = selected ? candidates.get(referenceKey(selected)) : undefined;
  const open =
    active?.kind !== "acquisition" && active
      ? opener({
          id: active.record_id,
          kind: active.kind,
          contentState: "available",
        })
      : undefined;
  return (
    <>
      {read.isPending && <p role="status">{t("common.loading")}</p>}
      <ErrorLine error={read.isError ? read.error : null} />
      <EvidenceAnswers
        candidates={candidates}
        originalKind={originalKind}
        selected={selected}
        onSelect={onSelect}
      />
      {active?.record_type === "contact" && (
        <AppLink href={routeHash(ENTITY.contact.route(active.record_id))}>
          {t("notice.source.openRecord")}
        </AppLink>
      )}
      {open && (
        <Button variant="link" onClick={open}>
          {t("notice.source.openRecord")}
        </Button>
      )}
      {read.hasNextPage && (
        <Button
          variant="link"
          onClick={() => read.fetchNextPage()}
          disabled={read.isFetchingNextPage}
        >
          {t("notice.source.moreEvidence")}
        </Button>
      )}
      {read.isError && (
        <Button variant="link" onClick={() => read.refetch()}>
          {t("common.retry")}
        </Button>
      )}
    </>
  );
}

function EvidenceAnswers({
  candidates,
  originalKind,
  selected,
  onSelect,
}: Readonly<{
  candidates: ReadonlyMap<string, Evidence>;
  originalKind: string | undefined;
  selected: SourceEvidenceRef | null;
  onSelect: (ref: SourceEvidenceRef | null) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useViewerZone();
  const interactionLabel = useInteractionLabel();
  return (
    <ChoiceList
      legend={t("notice.source.evidenceOnFile")}
      value={selected ? referenceKey(selected) : ""}
      onChange={(key) => onSelect(candidates.get(key)?.reference ?? null)}
      choices={[
        ...[...candidates.values()].map((candidate) => ({
          value: referenceKey(candidate.reference),
          label:
            candidate.kind === "acquisition"
              ? t("notice.source.original")
              : candidate.title || interactionLabel(candidate.kind),
          description: [
            t(
              candidate.occurred_at
                ? "notice.source.happenedOn"
                : "notice.source.recordedOn",
              {
                date: candidate.occurred_at
                  ? activityDay(
                      candidate.occurred_at,
                      candidate.occurred_precision,
                      locale,
                      zone,
                    )
                  : formatDate(candidate.recorded_at, locale, zone),
              },
            ),
            candidate.kind === "acquisition" &&
            (!originalKind || originalKind === "unknown_legacy")
              ? t("notice.source.originalUnknown")
              : "",
          ]
            .filter(Boolean)
            .join(" · "),
          disabled:
            candidate.kind === "acquisition" &&
            (!originalKind || originalKind === "unknown_legacy"),
        })),
        { value: "", label: t("notice.source.describeEvidence") },
      ]}
    />
  );
}

function useEvidenceCandidates(
  pages:
    | readonly components["schemas"]["NoticeSourceEvidencePage"][]
    | undefined,
  selected: SourceEvidenceRef | null,
  onSelect: (ref: SourceEvidenceRef | null) => void,
): Map<string, Evidence> {
  const offered = new Map<string, Evidence>(
    (pages ?? [])
      .flatMap((page) => [
        page.original,
        page.original_activity,
        ...(page.items ?? []),
      ])
      .filter((candidate): candidate is Evidence => candidate != null)
      .map((candidate) => [referenceKey(candidate.reference), candidate]),
  );
  const entries = [...offered.values()];
  const access = useQueries({
    queries: entries.map((entry) =>
      activityUnavailableQuery(
        entry.record_type === "activity" ? entry.record_id : undefined,
      ),
    ),
  });
  const denied = entries.filter((_, index) => access[index]?.data === true);
  const candidates = new Map(
    [...offered].filter(([, entry]) => !denied.includes(entry)),
  );
  const selectionDenied =
    selected !== null &&
    denied.some(
      (entry) => referenceKey(entry.reference) === referenceKey(selected),
    );
  useEffect(() => {
    if (selectionDenied) onSelect(null);
  }, [selectionDenied, onSelect]);
  return candidates;
}
