// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useInfiniteQuery } from "@tanstack/react-query";
import { AppLink } from "../../design-system/applink";
import { Badge } from "../../design-system/atoms";
import { Panel, PanelBody, PanelRow } from "../../design-system/panel";
import { SurfaceState } from "../../design-system/surfacestate";
import { formatNumber } from "../../format/format";
import { viewerZone } from "../../format/timezone";
import { useLocale, usePlural, useT } from "../../i18n";
import { useRecordZone } from "../../lib/recordzone";
import { routeHash } from "../../lib/router";
import { listReadState } from "../../lib/worklist.listread";
import { itemTitle, rowHref } from "../../lib/worklistcopy";
import {
  phrasedReasons,
  reasonText,
  whenText,
} from "../../lib/worklistcopyreasons";
import { worklistDayOptions } from "../../lib/worklistday";
import type { WorklistItem } from "../../lib/worklisttypes";
import { DEFAULT_VIEW } from "../../lib/worklistviews";

// How many of the day's ranked items stand here, in the Worklist's own order.
const NAMED_WORK = 3;

/**
 * The first moves of the reader's own day, read under the key Home and the
 * Worklist read it by. Each row says what, why and by when, and opens the
 * record it is about. Doing the work, and every write it takes, stays on
 * the Worklist, so no button here can act on a stale or withdrawn item.
 */
export function DoNext() {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const day = useInfiniteQuery(
    worklistDayOptions("mine", "all", undefined, DEFAULT_VIEW),
  );
  const page = day.data?.pages[0];
  const items = page?.queue?.slice(0, NAMED_WORK) ?? [];
  // A failed or withheld source means the list may be short, so an empty
  // answer is never read as a clear day.
  const incomplete = (page?.sources_unavailable?.length ?? 0) > 0;
  const read = listReadState(day, page?.queue);
  const state = read === "empty" && incomplete ? "unavailable" : read;
  const total = page?.summary?.total ?? 0;
  const worklist = routeHash({ screen: "worklist" });
  return (
    <Panel
      title={t("analytics.doNext.title")}
      flatHead
      footer={
        state === "ready" || state === "unavailable" ? (
          <AppLink href={worklist}>
            {/* The total counts the day's items, and one card can fold several,
                so it is said whole rather than less the cards drawn here. */}
            {total > items.length
              ? plural("analytics.doNext.all", total, {
                  count: formatNumber(total, locale),
                })
              : t("analytics.sdrOpenWorklist")}
          </AppLink>
        ) : undefined
      }
    >
      {state !== "ready" && (
        <PanelBody>
          <SurfaceState
            state={state}
            emptyLabel={t("analytics.sdrNothingNeedsYou")}
            loadingLabel={t("analytics.doNext.title")}
            detail={{ onRetry: () => void day.refetch() }}
          >
            {null}
          </SurfaceState>
        </PanelBody>
      )}
      {(state === "ready" || state === "unavailable") && incomplete && (
        <PanelBody>
          <p className="t-caption">{t("analytics.sdrWorkIncomplete")}</p>
        </PanelBody>
      )}
      {state === "ready" &&
        items.map((item) => (
          <WorkRow key={`${item.source}-${item.id}`} item={item} />
        ))}
    </Panel>
  );
}

// One Worklist item in the Worklist's own words: its title, the reasons it is
// here, the clock it races, and the record it opens.
function WorkRow({ item }: Readonly<{ item: WorklistItem }>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  const zone = viewerZone();
  const when = whenText(item, t, locale, zone, recordZone, new Date());
  const reasons = phrasedReasons(item, when !== null)
    .map((reason) => reasonText(reason, t, locale, zone))
    .filter((phrase): phrase is string => phrase !== null);
  const facts = [when, ...new Set(reasons)].filter(
    (part): part is string => !!part,
  );
  const title = itemTitle(item, t, locale);
  return (
    <PanelRow className="analytics-attention-row">
      <div className="analytics-attention-text">
        <p className="analytics-attention-title">
          {title}{" "}
          {item.overdue && <Badge tone="danger">{t("worklist.overdue")}</Badge>}
        </p>
        {facts.length > 0 && <p className="t-caption">{facts.join(" · ")}</p>}
      </div>
      <AppLink
        className="btn btn-ghost"
        href={rowHref(item) ?? routeHash({ screen: "worklist" })}
        aria-label={t("analytics.doNext.openNamed", { title })}
      >
        {t("analytics.doNext.open")}
      </AppLink>
    </PanelRow>
  );
}
