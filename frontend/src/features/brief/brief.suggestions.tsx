// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
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
            {run.items.map((item) => (
              <StoredSuggestion key={item.id} item={item} run={run} />
            ))}
          </>
        )
      }
    </QueryGate>
  );
}

function StoredSuggestion({
  item,
  run,
}: Readonly<{ item: MorningBriefItem; run: MorningBrief }>) {
  const client = useQueryClient();
  const { name, reading } = useEntityName("deal", item.deal_id);
  const t = useT();
  const { locale } = useLocale();
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
              queryKey: ["deal", ENTITY_NAME_KEY, item.deal_id],
              exact: true,
            }),
        }}
      >
        {null}
      </SurfaceState>
    );
  if (!name) return null;
  return (
    <PanelRow>
      <Heading as="h3" size="small">
        <EntityRef kind="deal" id={item.deal_id} name={name} />
      </Heading>
      {item.finding && (
        <p>
          {t("worklist.verdict.believes")} {item.finding}
        </p>
      )}
      {item.finding && run.annotated_at && (
        <p className="t-caption">
          {formatDateTime(run.annotated_at, locale, viewerZone())}
        </p>
      )}
    </PanelRow>
  );
}
