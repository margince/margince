// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQueryClient } from "@tanstack/react-query";
import { type ReactNode, useState } from "react";
import {
  ENTITY_NAME_KEY,
  EntityRef,
  useEntityName,
} from "../../components/entityref";
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
 * The run's deals, in rank order, split by whether the overnight pass said
 * anything about them.
 *
 * A deal with a finding is a suggestion: its name, the assessment and when it
 * was written. A deal without one is not, and drawing it as a heading with
 * nothing under it read as a finished recommendation that had lost its text.
 * Those deals are listed once, under one line that says which of two things
 * is true, read off `annotated_at`: no pass has annotated this run yet, or a
 * pass ran and had nothing to add. The list never says why a pass did not run;
 * the run carries no answer to that.
 */
function RunSuggestions({ run }: Readonly<{ run: MorningBrief }>) {
  const t = useT();
  const withFinding = run.items.filter((item) => item.finding);
  const without = run.items.filter((item) => !item.finding);
  return (
    <>
      {withFinding.map((item) => (
        <StoredSuggestion key={item.id} item={item} run={run} />
      ))}
      {without.length > 0 && (
        <PanelRow>
          <p className="t-caption">
            {run.annotated_at
              ? t("brief.suggestions.nothingToAdd")
              : t("brief.suggestions.notAnnotated")}
          </p>
          <ul className="brief-suggestion-deals">
            {without.map((item) => (
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
      )}
    </>
  );
}

/**
 * Renders its children only once the deal's name is readable. A deal the
 * reader may no longer open renders nothing at all, so a stored finding about
 * it is never shown beside a name they cannot reach.
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
            {t("worklist.verdict.believes")} {item.finding}
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
