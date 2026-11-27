// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Button, EmptyState, PendingBody } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { Panel, PanelBody } from "../../design-system/panel";
import { formatNumber } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import type { LibraryReads } from "./filterslibrary.reads";
import type { LibraryItem } from "./library";
import { LibraryTable, type LibraryTableGroup } from "./listlibrary";

const GROUP_TITLE: Record<LibraryTableGroup, MessageKey> = {
  mine: "filters.library.mine",
  shared: "filters.library.shared",
  views: "filters.library.views",
};

/**
 * One group: its rows, and what its reads have not answered yet. A deleted
 * view hands focus to the group holding every saved view.
 */
export function LibraryGroupPanel({
  group,
  rows,
  reads,
  narrowed,
  captionOf,
  folded,
  returnFocusTo,
}: Readonly<{
  group: LibraryTableGroup;
  rows: readonly LibraryItem[];
  reads: LibraryReads;
  narrowed: boolean;
  captionOf: (item: LibraryItem) => string;
  folded: boolean;
  returnFocusTo: () => HTMLElement | null;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const title = t(GROUP_TITLE[group]);
  const viewsHere = group !== "shared";
  const listsHere = group !== "views";
  const pending =
    (viewsHere && reads.viewsPending) || (listsHere && reads.listsPending);
  const failedViews = viewsHere && reads.viewsFailed;
  const failedLists = listsHere && reads.listsFailed;
  const settled = !pending && !failedViews && !failedLists;
  // Rows held from the previous archived toggle cannot say the group is empty.
  const held = listsHere && reads.listsHeld;
  const empty = settled && !held && rows.length === 0 && !narrowed;
  const counted =
    settled && !held && !reads.truncated ? (
      <span className="t-num">{formatNumber(rows.length, locale)}</span>
    ) : undefined;
  return (
    <Panel title={title} titleAction={counted}>
      <GroupFailures
        failedViews={failedViews}
        failedLists={failedLists}
        reads={reads}
      />
      {rows.length > 0 && (
        <LibraryTable
          label={title}
          items={rows}
          group={group}
          captionOf={captionOf}
          folded={folded}
          returnFocusTo={returnFocusTo}
        />
      )}
      {pending && (
        <PanelBody>
          <PendingBody label={t("filters.library.loading")} lines={6} />
        </PanelBody>
      )}
      {empty && (
        <PanelBody>
          <EmptyState>
            {t(
              group === "shared"
                ? "filters.library.sharedEmpty"
                : "filters.library.mineEmpty",
            )}
          </EmptyState>
        </PanelBody>
      )}
    </Panel>
  );
}

/** The group's failed reads, each with its Retry, in a band above the rows. */
function GroupFailures({
  failedViews,
  failedLists,
  reads,
}: Readonly<{
  failedViews: boolean;
  failedLists: boolean;
  reads: LibraryReads;
}>) {
  const t = useT();
  if (!failedViews && !failedLists) {
    return null;
  }
  return (
    <PanelBody>
      {failedViews && (
        <ErrorLine actions={<RetryButton onRetry={reads.retryViews} />}>
          {t("filters.library.viewsFailed")}
        </ErrorLine>
      )}
      {failedLists && (
        <ErrorLine actions={<RetryButton onRetry={reads.retryLists} />}>
          {t("filters.library.listsFailed")}
        </ErrorLine>
      )}
    </PanelBody>
  );
}

function RetryButton({ onRetry }: Readonly<{ onRetry: () => void }>) {
  const t = useT();
  return (
    <Button variant="link" onClick={onRetry}>
      {t("common.retry")}
    </Button>
  );
}
