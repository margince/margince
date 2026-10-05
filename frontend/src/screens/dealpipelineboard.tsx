// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type ComponentProps, useId, useState } from "react";
import { useCanWrite } from "../app/capability";
import { routeHash } from "../app/router";
import { Modal } from "../design-system/atoms";
import { type BoardDeal, PipelineBoard } from "../design-system/composed";
import type { DealCardActions } from "../design-system/dealcard";
import { Heading } from "../design-system/heading";
import { useT } from "../i18n";
import { dealMailAside } from "./dealmailaside";
import { DealStatusCardPanel } from "./dealstatus";
import { LogActivityAction } from "./logactivity";
import { useWriteTo } from "./writeto";

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
>;

export function DealPipelineBoard(props: Readonly<DealPipelineBoardProps>) {
  // Null when the reader has no mailbox the product can send from: the card
  // then offers no mail verb rather than one that opens onto a refusal.
  const writeTo = useWriteTo();
  // useCanWrite, not useCan: filing a task is a POST, and a read seat is
  // refused before RBAC is consulted — the rule the deal page's own Add task
  // holds for the same write.
  const canFileTask = useCanWrite("activity", "create");
  const [summary, setSummary] = useState<SummaryState | null>(null);
  const [taskFor, setTaskFor] = useState<string | null>(null);
  const cardActions = (deal: BoardDeal): DealCardActions => ({
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
          triggerLabel="log.addTask"
          openOnMount
          onClose={() => setTaskFor(null)}
        />
      )}
    </>
  );
}

// The deal stays in state after the drawer closes, so the closing drawer still
// has its panel to animate out with rather than an empty frame.
type SummaryState = Readonly<{ deal: BoardDeal; open: boolean }>;

/**
 * A deal's summary beside the board: the status card the deal page leads with,
 * in a drawer so the column a rep was triaging stays on screen behind it.
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
      placement="right"
    >
      <Heading size="large" id={titleId} className="modal-title">
        {t("deal360.brief")}
      </Heading>
      {summary && (
        <>
          <div className="record-stack">
            <DealStatusCardPanel
              key={summary.deal.id}
              dealId={summary.deal.id}
              dealName={summary.deal.name}
            />
          </div>
          <p>
            <a
              className="entity-link"
              href={routeHash({ screen: "deals", id: summary.deal.id })}
              onClick={onClose}
            >
              {t("deal.openDeal")}
            </a>
          </p>
        </>
      )}
    </Modal>
  );
}
