// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// What Margince finished for the reader, as one section on Home. The deals it
// changed come first, each change with what moved, when and why, and its
// Undo. Other updates follow in `completed`.
//
// The first five deals show here and every one shows in the drawer of all.
// Both draw the same rows, so a sixth deal loses nothing a third one had.

import { type ComponentProps, type ReactNode, useId, useState } from "react";
import { rowsOf } from "../../api/rows";
import { BundleUndo } from "../../components/marginchanges";
import { ReceiptsMore } from "../../components/receiptsmore";
import { AppLink } from "../../design-system/applink";
import { Button, Checkbox, Modal } from "../../design-system/atoms";
import {
  DrawerBody,
  DrawerFoot,
  DrawerHead,
} from "../../design-system/drawerbands";
import { Eyebrow } from "../../design-system/eyebrow";
import { Heading } from "../../design-system/heading";
import { SurfaceState } from "../../design-system/surfacestate";
import { stable } from "../../format/collate";
import { formatNumber } from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";
import { useHandledForYou } from "../../lib/handled";
import { useMagic } from "../../lib/magic.queries";
import {
  type MarginceChangeBundle,
  type MarginceChangeSummary,
  usePendingMarginceChanges,
} from "../../lib/marginchanges";
import { useRememberedUndoRun } from "../../lib/undorun";
import { replaceDial, useUrlParams } from "../../lib/urlstate";
import { listReadState } from "../../lib/worklist.listread";
import { worklistHistoryHref } from "../../lib/worklistrouting";
import { DealCard } from "./brief.doneforyou.deal";
import "./brief.feed.css";
import {
  type DealChanges,
  dealChanges,
  undoableShown,
} from "./brief.doneforyou.groups";

/** How many changed deals Home shows before "See all". */
export const DONE_PREVIEW_DEALS = 5;

export function DoneForYou({ completed }: Readonly<{ completed: ReactNode }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const titleId = useId();
  const handled = useHandledForYou();
  const summary = usePendingMarginceChanges();
  const pending = summary.data;
  const magic = useMagic();
  const [settled, setSettled] = useState<ReadonlySet<string>>(new Set());
  const settle = (id: string) => setSettled((last) => new Set([...last, id]));
  const receipts = handled.data?.receipts;
  const groups = dealChanges(receipts, settled, pending?.bundles);
  // The server's summary decides what shows, so its own read is the panel's
  // state: a failed summary is a failed panel, never a quieter list.
  const state = listReadState(summary, pending ? groups : undefined);
  const doneLines = rowsOf(magic.data?.done).length > 0;
  // The last Undo of many keeps the section up after it emptied the list.
  const lastRun = useRememberedUndoRun() !== null;
  if (
    state === "loading" ||
    (state === "empty" && !doneLines && !lastRun && !magic.isError)
  ) {
    return null;
  }
  const changed = pending?.deals;
  return (
    <section className="brief-section brief-done" aria-labelledby={titleId}>
      {/* Headed like Needs you, so Home's two lists read as one system. */}
      <div className="brief-first-head brief-done-band">
        <Eyebrow as="h2" id={titleId} className="brief-section-label">
          {t("brief.done.title")}
        </Eyebrow>
        {changed !== undefined && changed > 0 && (
          <span className="t-caption t-num brief-first-count">
            <span aria-hidden="true">{formatNumber(changed, locale)}</span>
            <span className="sr-only">
              {plural("brief.done.drawerTitle", changed, {
                count: formatNumber(changed, locale),
              })}
            </span>
          </span>
        )}
        <AppLink
          className="btn btn-ghost brief-first-queue"
          href={worklistHistoryHref()}
        >
          {t("brief.done.history")}
        </AppLink>
      </div>
      {state !== "empty" && (
        <SurfaceState
          state={state}
          emptyLabel=""
          loadingLabel={t("worklist.handled.loading")}
          detail={{
            onRetry: () => {
              void summary.refetch();
              void handled.refetch();
            },
          }}
        >
          <DealChangeList
            groups={groups.slice(0, DONE_PREVIEW_DEALS)}
            pending={pending}
            asOf={handled.data?.as_of}
            onSettled={settle}
          />
          <div className="brief-done-foot">
            <AllChanges
              count={changed ?? groups.length}
              groups={groups}
              pending={pending}
              asOf={handled.data?.as_of}
              handled={handled}
              onSettled={settle}
            />
          </div>
        </SurfaceState>
      )}
      {state === "empty" && lastRun && (
        <div className="brief-done-foot">
          <BundleUndo bundles={[]} remember label="" />
        </div>
      )}
      {completed}
    </section>
  );
}

/** The address dial that holds the drawer of all open, so Back reopens it. */
export const DONE_REVIEW_PARAM = "done_review";

/** The way into the drawer with every changed deal. */
function AllChanges({
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
  const triggerId = useId();
  const [params] = useUrlParams();
  if (count === 0) return null;
  return (
    <>
      <Button
        id={triggerId}
        variant="link"
        onClick={() => replaceDial(DONE_REVIEW_PARAM, "1")}
      >
        {plural("brief.done.seeAll", count, {
          count: formatNumber(count, locale),
        })}
      </Button>
      {params.get(DONE_REVIEW_PARAM) === "1" && (
        <AllChangesDrawer
          count={count}
          groups={groups}
          pending={pending}
          asOf={asOf}
          handled={handled}
          onSettled={onSettled}
          triggerId={triggerId}
        />
      )}
    </>
  );
}

/**
 * The drawer with every changed deal, each ticked for the Undo at its foot.
 * The deals and the Undo freeze when it opens, so the list drawn is the list
 * the Undo sends. New changes wait for the reader's own refresh.
 */
function AllChangesDrawer({
  count,
  groups,
  pending,
  asOf,
  handled,
  onSettled,
  triggerId,
}: Readonly<{
  count: number;
  groups: readonly DealChanges[];
  pending: MarginceChangeSummary | undefined;
  asOf: string | undefined;
  handled: ReturnType<typeof useHandledForYou>;
  onSettled: (id: string) => void;
  triggerId: string;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const titleId = useId();
  const [shown, setShown] = useState(groups);
  const [chosen, setChosen] = useState(() => undoableBundles(groups, pending));
  // The deals the reader unticked. A deal shown later starts ticked.
  const [left, setLeft] = useState<ReadonlySet<string>>(new Set());
  const remembered = useRememberedUndoRun() !== null;
  const ticked = chosen.filter((bundle) => !left.has(bundle.deal_id));
  const offered = new Set(chosen.map((bundle) => bundle.deal_id));
  const tick = (dealId: string, on: boolean) =>
    setLeft((last) => {
      const next = new Set(last);
      if (on) next.delete(dealId);
      else next.add(dealId);
      return next;
    });
  const n = formatNumber(count, locale);
  return (
    <Modal
      open
      intent="drawer"
      labelledBy={titleId}
      returnFocusTo={() => document.getElementById(triggerId)}
      onClose={() => replaceDial(DONE_REVIEW_PARAM, undefined)}
    >
      <DrawerHead>
        <Heading id={titleId} size="medium" as="h2" className="modal-title">
          {plural("brief.done.drawerTitle", count, { count: n })}
        </Heading>
      </DrawerHead>
      <DrawerBody>
        {changedSince(shown, groups) && (
          <Button
            variant="link"
            onClick={() => {
              setChosen(undoableBundles(groups, pending));
              setShown(groups);
            }}
          >
            {t("brief.done.showNew")}
          </Button>
        )}
        <DealChangeList
          groups={shown}
          pending={pending}
          asOf={asOf}
          onSettled={onSettled}
          pick={(dealId) =>
            offered.has(dealId)
              ? { on: !left.has(dealId), onChange: (on) => tick(dealId, on) }
              : undefined
          }
        />
        <ReceiptsMore handled={handled} />
      </DrawerBody>
      {(chosen.length > 0 || remembered) && (
        <DrawerFoot className="brief-done-foot">
          {chosen.length > 1 && (
            <Checkbox
              label={t("bulk.selectAll")}
              checked={ticked.length === chosen.length}
              indeterminate={ticked.length > 0 && ticked.length < chosen.length}
              onChange={(event) =>
                setLeft(event.target.checked ? new Set() : offered)
              }
            />
          )}
          <BundleUndo
            bundles={ticked}
            remember
            label={plural("changes.undoDeals", ticked.length, {
              count: formatNumber(ticked.length, locale),
            })}
          />
        </DrawerFoot>
      )}
    </Modal>
  );
}

// Each deal's Undo names only the changes the list shows and the server still
// allows.
function undoableBundles(
  groups: readonly DealChanges[],
  pending: MarginceChangeSummary | undefined,
): MarginceChangeBundle[] {
  return groups.flatMap((group) => {
    const bundle = pending?.bundles.find((b) => b.deal_id === group.dealId);
    const audits = undoableShown(group, bundle);
    return bundle && audits.length > 0
      ? [{ ...bundle, audit_log_ids: audits }]
      : [];
  });
}

/**
 * The changed deals as cards, one open at a time: the first until the reader
 * opens another.
 */
function DealChangeList({
  groups,
  pending,
  asOf,
  onSettled,
  pick,
}: Readonly<{
  groups: readonly DealChanges[];
  pending: MarginceChangeSummary | undefined;
  asOf: string | undefined;
  onSettled: (id: string) => void;
  pick?: (dealId: string) => ComponentProps<typeof DealCard>["pick"];
}>) {
  const t = useT();
  const [chosen, choose] = useState<string | null | undefined>(undefined);
  const openId = chosen === undefined ? groups[0]?.dealId : chosen;
  return (
    <ul className="brief-done-deals" aria-label={t("brief.done.dealsLabel")}>
      {groups.map((group) => (
        <DealCard
          key={group.dealId}
          group={group}
          bundle={pending?.bundles.find((b) => b.deal_id === group.dealId)}
          currency={pending?.base_currency}
          open={openId === group.dealId}
          onOpen={(open) => choose(open ? group.dealId : null)}
          asOf={asOf}
          onSettled={onSettled}
          pick={pick?.(group.dealId)}
        />
      ))}
    </ul>
  );
}

// Whether the live list holds changes the frozen one does not, or lost some.
function changedSince(
  shown: readonly DealChanges[],
  live: readonly DealChanges[],
): boolean {
  const ids = (groups: readonly DealChanges[]) =>
    groups
      .flatMap((group) => group.receipts.map((receipt) => receipt.id))
      .sort(stable)
      .join(",");
  return ids(shown) !== ids(live);
}
