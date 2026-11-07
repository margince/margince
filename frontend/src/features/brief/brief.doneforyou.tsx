// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// What Margince finished for the reader, as one section on Home. The deals it
// changed come first, each change with what moved, when and why, and the verbs
// that review or take it back. Other finished work follows in `completed`.
//
// The first five deals show here and every one shows in the review drawer.
// Both draw the same rows, so a sixth deal loses nothing a third one had.

import { type ReactNode, useId, useState } from "react";
import { rowsOf } from "../../api/rows";
import { EntityRef } from "../../components/entityref";
import { BundleUndo } from "../../components/marginchanges";
import { ReceiptDetail } from "../../components/receiptdetail";
import { ReceiptReview } from "../../components/receiptreview";
import { ReceiptsMore } from "../../components/receiptsmore";
import { AppLink } from "../../design-system/applink";
import { Button, Modal } from "../../design-system/atoms";
import {
  DrawerBody,
  DrawerFoot,
  DrawerHead,
} from "../../design-system/drawerbands";
import { Eyebrow } from "../../design-system/eyebrow";
import { Heading } from "../../design-system/heading";
import { PanelBody, PanelGroupHead } from "../../design-system/panel";
import { SurfaceState } from "../../design-system/surfacestate";
import { sameCalendarDay } from "../../format/calendarday";
import {
  formatDateTime,
  formatDayMonth,
  formatNumber,
  formatTimeOfDay,
} from "../../format/format";
import { type Locale, useLocale, usePlural, useT } from "../../i18n";
import { type Receipt, useHandledForYou } from "../../lib/handled";
import { useMagic } from "../../lib/magic.queries";
import {
  type MarginceChangeBundle,
  type MarginceChangeSummary,
  usePendingMarginceChanges,
} from "../../lib/marginchanges";
import { receiptHeadline, receiptReason } from "../../lib/receiptcopy";
import { isReviewableReceipt } from "../../lib/receiptreview";
import { useViewerZone } from "../../lib/viewerzone";
import { listReadState } from "../../lib/worklist.listread";

/** How many changed deals Home shows before "Review all". */
export const DONE_PREVIEW_DEALS = 5;

/** One deal and the changes on it that wait for the reader, newest first. */
export type DealChanges = Readonly<{
  dealId: string;
  name: string | undefined;
  receipts: readonly Receipt[];
}>;

/**
 * The receipts grouped by deal, in the order their newest change arrived. A
 * receipt the reader settled on this visit stays with its answer, so a row
 * never leaves under the pointer that pressed it.
 */
export function dealChanges(
  receipts: readonly Receipt[] | undefined,
  settled: ReadonlySet<string>,
): DealChanges[] {
  const groups = new Map<string, Receipt[]>();
  for (const receipt of receipts ?? []) {
    const subject = receipt.subject;
    if (subject?.type !== "deal") continue;
    if (!isReviewableReceipt(receipt) && !settled.has(receipt.id)) continue;
    const group = groups.get(subject.id) ?? [];
    group.push(receipt);
    groups.set(subject.id, group);
  }
  return [...groups].map(([dealId, group]) => ({
    dealId,
    name: group[0]?.subject?.label ?? undefined,
    receipts: group,
  }));
}

export function DoneForYou({ completed }: Readonly<{ completed: ReactNode }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const titleId = useId();
  const handled = useHandledForYou();
  const pending = usePendingMarginceChanges().data;
  const magic = useMagic();
  const [settled, setSettled] = useState<ReadonlySet<string>>(new Set());
  const settle = (id: string) => setSettled((last) => new Set([...last, id]));
  const receipts = handled.data?.receipts;
  const groups = dealChanges(receipts, settled);
  // A response with no list is unreadable, not empty.
  const state = listReadState(handled, receipts ? groups : undefined);
  // The server counts every waiting deal; the list holds those read so far.
  const waiting = Math.max(pending?.deals ?? 0, groups.length);
  const doneLines = rowsOf(magic.data?.done).length > 0;
  if (
    state === "loading" ||
    (state === "empty" && !doneLines && !magic.isError)
  ) {
    return null;
  }
  return (
    <section className="brief-section" aria-labelledby={titleId}>
      <Eyebrow as="h2" id={titleId} className="brief-section-label">
        {t("brief.done.title")}
      </Eyebrow>
      {state !== "empty" && (
        <>
          <PanelGroupHead
            title={plural("brief.done.waiting", waiting, {
              count: formatNumber(waiting, locale),
            })}
            level="h3"
          />
          <PanelBody>
            <SurfaceState
              state={state}
              emptyLabel=""
              loadingLabel={t("worklist.handled.loading")}
              detail={{ onRetry: () => void handled.refetch() }}
            >
              <DealChangeList
                groups={groups.slice(0, DONE_PREVIEW_DEALS)}
                pending={pending}
                asOf={handled.data?.as_of}
                onSettled={settle}
              />
              <div className="brief-done-foot">
                <ReviewAll
                  count={waiting}
                  groups={groups}
                  pending={pending}
                  asOf={handled.data?.as_of}
                  handled={handled}
                  onSettled={settle}
                />
                <AppLink href="#/worklist">{t("brief.done.history")}</AppLink>
              </div>
            </SurfaceState>
          </PanelBody>
        </>
      )}
      {completed}
    </section>
  );
}

/** The drawer with every waiting deal, and the Undo of all of them. */
function ReviewAll({
  count,
  groups,
  pending,
  asOf,
  handled,
  onSettled,
}: Readonly<{
  count: number;
  groups: readonly DealChanges[];
  pending: MarginceChangeSummary | undefined;
  asOf: string | undefined;
  handled: ReturnType<typeof useHandledForYou>;
  onSettled: (id: string) => void;
}>) {
  const plural = usePlural();
  const { locale } = useLocale();
  const titleId = useId();
  const triggerId = useId();
  const [open, setOpen] = useState(false);
  // The deals on screen when the drawer opened. A deal that arrives later is
  // drawn but never taken back by the Undo below unless it is opened again.
  const [chosen, setChosen] = useState<readonly MarginceChangeBundle[]>([]);
  const shown = new Set(groups.map((group) => group.dealId));
  const bundles = (pending?.bundles ?? []).filter((bundle) =>
    shown.has(bundle.deal_id),
  );
  if (count === 0) return null;
  const n = formatNumber(count, locale);
  return (
    <>
      <Button
        id={triggerId}
        variant="link"
        onClick={() => {
          setChosen(bundles);
          setOpen(true);
        }}
      >
        {plural("brief.done.reviewAll", count, { count: n })}
      </Button>
      {open && (
        <Modal
          open
          intent="drawer"
          labelledBy={titleId}
          returnFocusTo={() => document.getElementById(triggerId)}
          onClose={() => setOpen(false)}
        >
          <DrawerHead>
            <Heading id={titleId} size="medium" as="h2" className="modal-title">
              {plural("brief.done.drawerTitle", count, { count: n })}
            </Heading>
          </DrawerHead>
          <DrawerBody>
            <DealChangeList
              groups={groups}
              pending={pending}
              asOf={asOf}
              onSettled={onSettled}
            />
            <ReceiptsMore handled={handled} />
          </DrawerBody>
          {chosen.length > 0 && (
            <DrawerFoot>
              <BundleUndo
                bundles={chosen}
                label={plural("changes.undoDeals", chosen.length, {
                  count: formatNumber(chosen.length, locale),
                })}
              />
            </DrawerFoot>
          )}
        </Modal>
      )}
    </>
  );
}

function DealChangeList({
  groups,
  pending,
  asOf,
  onSettled,
}: Readonly<{
  groups: readonly DealChanges[];
  pending: MarginceChangeSummary | undefined;
  asOf: string | undefined;
  onSettled: (id: string) => void;
}>) {
  const t = useT();
  return (
    <ul className="brief-done-deals" aria-label={t("brief.done.dealsLabel")}>
      {groups.map((group) => (
        <li className="brief-done-deal" key={group.dealId}>
          <p className="brief-done-name">
            <EntityRef kind="deal" id={group.dealId} name={group.name} />
          </p>
          <ul className="brief-done-changes">
            {group.receipts.map((receipt) => (
              <ChangeRow
                key={receipt.id}
                receipt={receipt}
                asOf={asOf}
                onSettled={() => onSettled(receipt.id)}
              />
            ))}
          </ul>
          <DealUndo
            group={group}
            bundle={pending?.bundles.find((b) => b.deal_id === group.dealId)}
            onSettled={onSettled}
          />
        </li>
      ))}
    </ul>
  );
}

/**
 * "Undo N changes" for a deal with more than one waiting change. It sends the
 * changes listed above it, at the version the server read.
 */
function DealUndo({
  group,
  bundle,
  onSettled,
}: Readonly<{
  group: DealChanges;
  bundle: MarginceChangeBundle | undefined;
  onSettled: (id: string) => void;
}>) {
  const plural = usePlural();
  const { locale } = useLocale();
  const listed = new Set(
    group.receipts
      .filter(isReviewableReceipt)
      .map((receipt) => receipt.undo?.audit_log_id ?? receipt.id),
  );
  const audits = bundle?.audit_log_ids.filter((id) => listed.has(id)) ?? [];
  if (!bundle || audits.length < 2) return null;
  return (
    <div className="brief-done-controls">
      <BundleUndo
        bundles={[{ ...bundle, audit_log_ids: audits }]}
        label={plural("brief.done.undoChanges", audits.length, {
          count: formatNumber(audits.length, locale),
        })}
        onDone={() => {
          for (const receipt of group.receipts) onSettled(receipt.id);
        }}
      />
    </div>
  );
}

/** One change: what moved and when, why, the evidence, and its verbs. */
function ChangeRow({
  receipt,
  asOf,
  onSettled,
}: Readonly<{
  receipt: Receipt;
  asOf: string | undefined;
  onSettled: () => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useViewerZone();
  const reason = receiptReason(receipt, t, locale);
  return (
    <li className="brief-done-change">
      <p className="brief-done-what">
        {receiptHeadline(receipt, t, locale, zone)}
      </p>
      <time
        className="t-caption brief-done-when"
        dateTime={receipt.occurred_at}
        title={formatDateTime(receipt.occurred_at, locale, zone)}
      >
        {changeWhen(receipt.occurred_at, asOf, locale, zone)}
      </time>
      {/* An older server sends no headline and keeps the reason in its
          summary, so only a newer one can say that none was recorded. */}
      {(reason || receipt.headline) && (
        <p className="brief-done-reason">
          {reason ?? t("brief.done.noReason")}
        </p>
      )}
      {receipt.customer_quote && (
        <figure className="brief-done-quote">
          <figcaption className="t-caption">
            {t("brief.done.customerWrote")}
          </figcaption>
          <blockquote>{receipt.customer_quote}</blockquote>
        </figure>
      )}
      <ReceiptDetail receipt={receipt} />
      <div className="brief-done-controls">
        <ReceiptReview receipt={receipt} onDone={onSettled} />
      </div>
    </li>
  );
}

// The hour for a change from today, the day for an older one: the list spans
// as many days as changes have waited.
function changeWhen(
  iso: string,
  asOf: string | undefined,
  locale: Locale,
  zone: string,
): string {
  return asOf !== undefined &&
    sameCalendarDay(new Date(iso), new Date(asOf), zone)
    ? formatTimeOfDay(iso, locale, zone)
    : formatDayMonth(iso, locale, zone);
}
