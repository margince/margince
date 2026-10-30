// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQueries, useQueryClient } from "@tanstack/react-query";
import { type ReactNode, useState } from "react";
import {
  ENTITY_NAME_KEY,
  EntityRef,
  entityNameQuery,
  useEntityName,
} from "../../components/entityref";
import { usableName } from "../../components/entityref.queries";
import { EmptyState } from "../../design-system/atoms";
import { Disclosure } from "../../design-system/disclosure";
import { Heading } from "../../design-system/heading";
import { PanelRow } from "../../design-system/panel";
import { QueryGate } from "../../design-system/querystates";
import { SurfaceState } from "../../design-system/surfacestate";
import { formatDateTime } from "../../format/format";
import { viewerZone } from "../../format/timezone";
import { useLocale, useT } from "../../i18n";
import {
  type MorningBrief,
  type MorningBriefItem,
  useMorningBrief,
} from "./brief.queries";

export function BriefSuggestions() {
  const t = useT();
  const [open, setOpen] = useState(false);
  return (
    <Disclosure
      summary={t("brief.suggestions.review")}
      open={open}
      onToggle={setOpen}
    >
      {open && <StoredSuggestions />}
    </Disclosure>
  );
}

function StoredSuggestions() {
  const query = useMorningBrief();
  const t = useT();
  const { locale } = useLocale();
  return (
    <QueryGate query={query} pendingLabel={t("brief.feed.loading")}>
      {(run) =>
        !run?.items.length ? (
          <EmptyState>{t("brief.suggestions.empty")}</EmptyState>
        ) : (
          <>
            <p className="t-caption">
              {formatDateTime(run.generated_at, locale, viewerZone())}
            </p>
            <RunSuggestions run={run} />
          </>
        )
      }
    </QueryGate>
  );
}

/**
 * The run's deals in rank order. A deal with a finding is a suggestion. The
 * rest are listed once under one line read off `annotated_at`: no pass has run
 * yet, or the pass left nothing this reader can read. That covers a quiet pass
 * and a finding withheld because its evidence became unreadable.
 */
function RunSuggestions({ run }: Readonly<{ run: MorningBrief }>) {
  const withFinding = run.items.filter((item) => findingOf(item) !== null);
  const without = run.items.filter((item) => findingOf(item) === null);
  return (
    <>
      {withFinding.map((item) => (
        <StoredSuggestion key={item.id} item={item} run={run} />
      ))}
      {without.length > 0 && <QuietDeals run={run} items={without} />}
    </>
  );
}

/** The finding as prose a reader can read, or null for none or a blank one. */
function findingOf(item: MorningBriefItem): string | null {
  const text = item.finding?.trim();
  return text ? text : null;
}

/**
 * The deals with no readable assessment, under the line that says so. The
 * line names "these deals", so it is drawn only while one can still be named.
 * A name still loading or failed keeps its row, because the row offers retry.
 */
function QuietDeals({
  run,
  items,
}: Readonly<{ run: MorningBrief; items: readonly MorningBriefItem[] }>) {
  const t = useT();
  const names = useQueries({
    queries: items.map((item) => entityNameQuery("deal", item.deal_id)),
  });
  // A read that answered with no name is a deal this reader may not open; it
  // leaves the list. One still loading or failed stays, for its retry.
  const listed = items.filter((_, index) => {
    const query = names[index];
    return !(query?.isSuccess && usableName(query.data) === null);
  });
  if (listed.length === 0) return null;
  const captionId = `brief-quiet-${run.id}`;
  return (
    <PanelRow>
      <p className="t-caption" id={captionId}>
        {run.annotated_at
          ? t("brief.suggestions.noAssessment")
          : t("brief.suggestions.notAnnotated")}
      </p>
      <ul className="brief-suggestion-deals" aria-labelledby={captionId}>
        {listed.map((item) => (
          <li key={item.id}>
            <NamedDeal dealId={item.deal_id}>
              {(name) => (
                <EntityRef kind="deal" id={item.deal_id} name={name} />
              )}
            </NamedDeal>
          </li>
        ))}
      </ul>
    </PanelRow>
  );
}

/**
 * Renders its children only once the deal's name is readable. A deal the
 * reader may no longer open renders nothing. A finding about it is then never
 * shown beside a name they cannot reach.
 */
function NamedDeal({
  dealId,
  children,
}: Readonly<{ dealId: string; children: (name: string) => ReactNode }>) {
  const client = useQueryClient();
  const { name, reading } = useEntityName("deal", dealId);
  const t = useT();
  if (reading === "failed" || reading === "pending")
    return (
      <SurfaceState
        state={reading === "failed" ? "failed" : "loading"}
        emptyLabel={t("common.empty")}
        loadingLabel={t("common.loading")}
        loadingLines={1}
        detail={{
          onRetry: () =>
            void client.invalidateQueries({
              queryKey: ["deal", ENTITY_NAME_KEY, dealId],
              exact: true,
            }),
        }}
      >
        {null}
      </SurfaceState>
    );
  if (!name) return null;
  return <>{children(name)}</>;
}

function StoredSuggestion({
  item,
  run,
}: Readonly<{ item: MorningBriefItem; run: MorningBrief }>) {
  const t = useT();
  const { locale } = useLocale();
  return (
    <NamedDeal dealId={item.deal_id}>
      {(name) => (
        <PanelRow>
          <Heading as="h3" size="small">
            <EntityRef kind="deal" id={item.deal_id} name={name} />
          </Heading>
          <p>
            {t("worklist.verdict.believes")} {findingOf(item)}
          </p>
          {run.annotated_at && (
            <p className="t-caption">
              {formatDateTime(run.annotated_at, locale, viewerZone())}
            </p>
          )}
        </PanelRow>
      )}
    </NamedDeal>
  );
}
