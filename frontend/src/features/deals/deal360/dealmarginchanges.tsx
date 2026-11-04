// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { ReceiptUndo } from "../../../components/receiptundo";
import { Button } from "../../../design-system/atoms";
import {
  OverviewCard,
  OverviewList,
  OverviewRow,
} from "../../../design-system/overviewcard";
import { formatDayMonth } from "../../../format/format";
import { useLocale, useT } from "../../../i18n";
import { useDealMarginceChanges } from "../../../lib/marginchanges";
import { useViewerZone } from "../../../lib/viewerzone";
import {
  receiptBasis,
  receiptSummary,
} from "../../worklist/worklist.receiptcopy";

/**
 * Every change Margince made to this deal, newest first, with its reason and
 * an Undo while it is not undone. It stays when the change leaves Home, so an
 * owner who dismissed it there can still take it back here.
 */
export function DealMarginceChangesCard({
  dealId,
}: Readonly<{ dealId: string }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useViewerZone();
  const read = useDealMarginceChanges(dealId);
  const changes = read.data?.pages.flatMap((page) => page.changes ?? []) ?? [];
  if (!read.isSuccess || changes.length === 0) {
    return null;
  }
  return (
    <OverviewCard
      title={t("changes.deal.title")}
      foot={
        read.hasNextPage ? (
          <Button
            variant="link"
            pending={read.isFetchingNextPage}
            busyLabel={t("changes.deal.loading")}
            onClick={() => void read.fetchNextPage()}
          >
            {t("worklist.handled.showMore")}
          </Button>
        ) : undefined
      }
    >
      <OverviewList>
        {changes.map((change) => (
          <OverviewRow
            key={change.id}
            title={receiptSummary(change, t, locale, zone)}
            sub={[
              receiptBasis(change, t, locale),
              formatDayMonth(change.occurred_at, locale, zone),
            ]
              .filter(Boolean)
              .join(" · ")}
            right={<ReceiptUndo receipt={change} />}
          />
        ))}
      </OverviewList>
    </OverviewCard>
  );
}
