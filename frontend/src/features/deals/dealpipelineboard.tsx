// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useIsFetching } from "@tanstack/react-query";
import { type ComponentProps, useEffect, useId, useRef, useState } from "react";
import { LogActivityAction } from "../../components/logactivity.action";
import { AppLink } from "../../design-system/applink";
import { Modal } from "../../design-system/atoms";
import type { DealCardActions } from "../../design-system/dealcard";
import { Heading } from "../../design-system/heading";
import {
  type BoardDeal,
  PipelineBoard,
} from "../../design-system/pipelineboard";
import { useOwnToast } from "../../design-system/toast";
import { useT } from "../../i18n";
import { useCanWrite } from "../../lib/capability";
import { routeHash } from "../../lib/router";
import { useWriteTo } from "../../lib/writeto";
import { DealCardMenu } from "./dealcardmenu";
import { dealMailAside } from "./dealmailaside";
import type { Deal, Stage } from "./deals";
import { DealStatusCardPanel } from "./dealstatus";

// The pipeline board as the deals screen draws it: the design-system board
// with the screen's half of every card — the mail flyout read from the
// timeline, and the verbs a rep reaches for without leaving the board.
//
// Each verb opens what the deal page opens, never a copy of it: the summary is
// the deal page's own status card, the mail is the shell's one composer, and
// the task is the deal page's own task drawer.

type DealPipelineBoardProps = Omit<
  ComponentProps<typeof PipelineBoard>,
  "mailAside" | "cardActions"
> & {
  /** What each card's "…" menu acts on; absent, the cards draw no menu. */
  menu?: Readonly<{
    deals: readonly Deal[];
    /** In position order. */
    stages: readonly Stage[];
    /** Answers whether the move was saved. */
    onAdvance: (dealId: string, stageId: string) => Promise<boolean>;
  }>;
};

export function DealPipelineBoard({
  menu,
  ...props
}: Readonly<DealPipelineBoardProps>) {
  // Null when the reader has no mailbox the product can send from: the card
  // then offers no mail verb rather than one that opens onto a refusal.
  const writeTo = useWriteTo();
  // useCanWrite, not useCan: filing a task is a POST, and a read seat is
  // refused before RBAC is consulted — the rule the deal page's own Add task
  // holds for the same write.
  const canFileTask = useCanWrite("activity", "create");
  const [summary, setSummary] = useState<SummaryState | null>(null);
  const [taskFor, setTaskFor] = useState<string | null>(null);
  const cardVerbs = (deal: BoardDeal): DealCardActions => ({
    onSummary: () => setSummary({ deal, open: true }),
    // An archived deal takes no new mail and no new work, which the deal page
    // says by refusing both verbs; a card has no room for the sentence, so it
    // offers neither.
    onEmail:
      writeTo && !deal.archived
        ? () => writeTo({ entityType: "deal", entityId: deal.id })
        : undefined,
    onAddTask:
      canFileTask && !deal.archived ? () => setTaskFor(deal.id) : undefined,
  });
  const followMove = useFocusFollowsMove();
  const copyRefusal = useOwnToast();
  const records = new Map(menu?.deals.map((deal) => [deal.id, deal]));
  const cardActions = (deal: BoardDeal): DealCardActions => {
    const verbs = cardVerbs(deal);
    const record = records.get(deal.id);
    return {
      ...verbs,
      menu: menu && record && (
        <DealCardMenu
          deal={record}
          stages={menu.stages}
          verbs={verbs}
          copyRefusal={copyRefusal}
          onAdvance={(dealId, stageId) => {
            void menu.onAdvance(dealId, stageId).then((landed) => {
              if (landed) {
                followMove({ dealId, to: stageId });
              }
            });
          }}
        />
      ),
    };
  };
  return (
    <>
      <PipelineBoard
        {...props}
        mailAside={dealMailAside}
        cardActions={cardActions}
      />
      <DealSummaryDrawer
        summary={summary}
        onClose={() => setSummary((was) => was && { ...was, open: false })}
      />
      {taskFor && (
        <LogActivityAction
          key={taskFor}
          entityType="deal"
          entityId={taskFor}
          askedKind="task"
          openOnMount
          onClose={() => setTaskFor(null)}
        />
      )}
    </>
  );
}

type SavedMove = Readonly<{ dealId: string; to: string }>;

// A saved move redraws the card in its new column, which drops the focus its
// old menu trigger held. Focus follows it there, unless the reader moved on.
function useFocusFollowsMove() {
  const moving = useRef<SavedMove | null>(null);
  // Waits while the board's deal reads run. A card they leave undrawn, by a
  // filter or past a loaded page, is not followed later.
  const reading = useIsFetching({ queryKey: ["deals"] }) > 0;
  useEffect(() => {
    const move = moving.current;
    if (move === null) {
      return;
    }
    const cards = [
      ...document.querySelectorAll<HTMLElement>("[data-deal]"),
    ].filter((each) => each.dataset.deal === move.dealId);
    const landed = cards.find(
      (each) =>
        each.closest<HTMLElement>("[data-stage]")?.dataset.stage === move.to,
    );
    const held = cards.some(
      (each) => each !== landed && each.contains(document.activeElement),
    );
    if (held || (landed === undefined && reading)) {
      return;
    }
    moving.current = null;
    if (landed && document.activeElement === document.body) {
      landed.querySelector<HTMLElement>(".deal-open")?.focus();
    }
  });
  return (move: SavedMove) => {
    moving.current = move;
  };
}

// The deal stays in state after the drawer closes, so the closing drawer still
// has its panel to animate out with rather than an empty frame.
type SummaryState = Readonly<{ deal: BoardDeal; open: boolean }>;

/**
 * A deal's summary beside the board: the status card the deal page leads with,
 * in a drawer so the column a rep was triaging stays on screen behind it.
 * Headed by the deal's name, as the deal page is, because the card's own panel
 * is already titled as the summary.
 */
function DealSummaryDrawer({
  summary,
  onClose,
}: Readonly<{ summary: SummaryState | null; onClose: () => void }>) {
  const t = useT();
  const titleId = useId();
  return (
    <Modal
      open={summary?.open ?? false}
      onClose={onClose}
      labelledBy={titleId}
      intent="drawer"
    >
      <Heading size="large" id={titleId} className="modal-title">
        {summary?.deal.name}
      </Heading>
      {summary && (
        <div className="record-stack">
          <DealStatusCardPanel key={summary.deal.id} deal={summary.deal} />
          <p>
            <AppLink
              className="entity-link"
              href={routeHash({ screen: "deals", id: summary.deal.id })}
              onClick={onClose}
            >
              {t("deal.openDeal")}
            </AppLink>
          </p>
        </div>
      )}
    </Modal>
  );
}
