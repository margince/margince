// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// What Margince finished for the reader, as one section on Home. The deals it
// changed come first, each change with what moved, when and why, and the verbs
// that review or take it back. Other updates follow in `completed`.
//
// The first five deals show here and every one shows in the review drawer.
// Both draw the same rows, so a sixth deal loses nothing a third one had.

import { type ReactNode, useId, useState } from "react";
import { rowsOf } from "../../api/rows";
import { BundleUndo } from "../../components/marginchanges";
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
import { listReadState } from "../../lib/worklist.listread";
import { DealCard } from "./brief.doneforyou.deal";
import "./brief.feed.css";
import {
  type DealChanges,
  dealChanges,
  undoableShown,
} from "./brief.doneforyou.groups";

/** How many changed deals Home shows before "Review all". */
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
  // The server's summary decides what waits, so its own read is the panel's
  // state: a failed summary is a failed panel, never a quieter list.
  const state = listReadState(summary, pending ? groups : undefined);
  const doneLines = rowsOf(magic.data?.done).length > 0;
  if (
    state === "loading" ||
    (state === "empty" && !doneLines && !magic.isError)
  ) {
    return null;
  }
  const waiting = pending?.deals;
  return (
    <section className="brief-section brief-done" aria-labelledby={titleId}>
      {/* Headed like Needs you, so Home's two lists read as one system. */}
      <div className="brief-first-head brief-done-band">
        <Eyebrow as="h2" id={titleId} className="brief-section-label">
          {t("brief.done.title")}
        </Eyebrow>
        {waiting !== undefined && waiting > 0 && (
          <span className="t-caption t-num brief-first-count">
            <span aria-hidden="true">{formatNumber(waiting, locale)}</span>
            <span className="sr-only">
              {plural("brief.done.waiting", waiting, {
                count: formatNumber(waiting, locale),
              })}
            </span>
          </span>
        )}
        <AppLink className="btn btn-ghost brief-first-queue" href="#/worklist">
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
            <ReviewAll
              count={waiting ?? groups.length}
              groups={groups}
              pending={pending}
              asOf={handled.data?.as_of}
              handled={handled}
              onSettled={settle}
            />
          </div>
        </SurfaceState>
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
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const titleId = useId();
  const triggerId = useId();
  const [open, setOpen] = useState(false);
  // The deals and the Undo frozen together when the drawer opened, so the
  // list drawn is the list the Undo sends. New changes wait for the reader's
  // own refresh.
  const [chosen, setChosen] = useState<readonly MarginceChangeBundle[]>([]);
  const [shown, setShown] = useState<readonly DealChanges[]>([]);
  // Each deal's Undo names only the changes this list shows and the server
  // still allows, frozen when the drawer opens.
  const bundles = groups.flatMap((group) => {
    const bundle = pending?.bundles.find((b) => b.deal_id === group.dealId);
    const audits = undoableShown(group, bundle);
    return bundle && audits.length > 0
      ? [{ ...bundle, audit_log_ids: audits }]
      : [];
  });
  if (count === 0) return null;
  const n = formatNumber(count, locale);
  return (
    <>
      <Button
        id={triggerId}
        variant="link"
        onClick={() => {
          setChosen(bundles);
          setShown(groups);
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
            {changedSince(shown, groups) && (
              <Button
                variant="link"
                onClick={() => {
                  setChosen(bundles);
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

/**
 * The changed deals as cards, one open at a time: the first until the reader
 * opens another.
 */
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
