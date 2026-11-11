// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { ReceiptDetail } from "../../../components/receiptdetail";
import { ReceiptReview } from "../../../components/receiptreview";
import { Button } from "../../../design-system/atoms";
import {
  OverviewCard,
  OverviewList,
  OverviewRow,
} from "../../../design-system/overviewcard";
import { formatDayMonth } from "../../../format/format";
import { type Translator, useLocale, useT } from "../../../i18n";
import {
  type DealMarginceCheck,
  useDealMarginceChanges,
} from "../../../lib/marginchanges";
import { receiptBasis, receiptSummary } from "../../../lib/receiptcopy";
import { useViewerZone } from "../../../lib/viewerzone";

/**
 * Every change Margince made to this deal, newest first, with its reason and
 * an Undo while it is not undone. It stays when the change leaves Home, so an
 * owner who dismissed it there can still take it back here. A latest check
 * that changed nothing leads the list.
 */
export function DealMarginceChangesCard({
  dealId,
}: Readonly<{ dealId: string }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useViewerZone();
  const read = useDealMarginceChanges(dealId);
  const changes = read.data?.pages.flatMap((page) => page.changes ?? []) ?? [];
  const check = read.data?.pages[0]?.last_check;
  if (!read.isSuccess || (changes.length === 0 && !check)) {
    return null;
  }
  const day = (at: string) => formatDayMonth(at, locale, zone);
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
        {check && (
          <OverviewRow
            title={t("changes.check.title", { date: day(check.checked_at) })}
            sub={checkLines(check, t, day).join(" · ")}
          />
        )}
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
            right={<ReceiptReview receipt={change} />}
            detail={<ReceiptDetail receipt={change} />}
          />
        ))}
      </OverviewList>
    </OverviewCard>
  );
}

/**
 * Why a check changed nothing, which sources it read, and when it runs next.
 * A reader the reasons were not made for gets only the next check: the
 * server sends them nothing else.
 */
export function checkLines(
  check: DealMarginceCheck,
  t: Translator,
  day: (at: string) => string,
): string[] {
  const next = check.next_check_at
    ? t("changes.check.next", { date: day(check.next_check_at) })
    : t("changes.check.nextOnNews");
  if (!check.explained) {
    return [t("changes.check.ownerOnly"), next];
  }
  const lines: string[] = [];
  if (check.skipped) {
    lines.push(t(`changes.check.skipped.${check.skipped}`));
  }
  for (const outcome of check.outcomes) {
    lines.push(
      t("changes.check.outcome", {
        kind: t(`changes.check.kind.${outcome.kind}`),
        why: t(`changes.check.why.${outcome.why}`),
      }),
    );
  }
  if (check.sources.length > 0) {
    const sources = check.sources.map((source) =>
      t(`changes.check.state.${source.state}`, {
        source: t(`changes.check.source.${source.source}`),
      }),
    );
    lines.push(t("changes.check.sources", { list: sources.join(", ") }));
  }
  lines.push(next);
  return lines;
}
