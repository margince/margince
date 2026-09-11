// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Disclosure } from "./atoms";
import { type DecisionApproval, DecisionCard } from "./decisioncard";
import type {
  DecisionDeckChips,
  DecisionDeckItem,
  DecisionDeckLabels,
  DecisionSharedFacts,
  DeckVerdict,
} from "./decisiondeck";
import { representative, sharedFacts } from "./decisiondeck.verdicts";

// One item of the deck, as the card that asks it.
//
// Its own file because it is the one place the deck's two surfaces MEET: the
// dragged stack and the list draw the same card, and a second assembly for
// either would be two answers to what a bundle's meta line says. The deck owns
// the queue and this owns one question in it.

// A bundle carries the count of what saying yes would decide on its meta line
// and its members behind an expander, because the API decides the set in one
// call and the reader answers it once.
export function DeckItemCard({
  item,
  layout,
  now,
  labels,
  chips,
  onStage,
}: Readonly<{
  item: DecisionDeckItem;
  layout: "deck" | "row";
  now: number;
  labels: DecisionDeckLabels;
  chips?: (
    approval: DecisionApproval,
    shared: DecisionSharedFacts,
  ) => DecisionDeckChips;
  onStage: (item: DecisionDeckItem, verdict: DeckVerdict) => void;
}>) {
  const approval = representative(item, now);
  const trim = chips?.(approval, sharedFacts(item)) ?? {};
  const members = item.kind === "bundle" ? item.members : [];
  return (
    <DecisionCard
      approval={approval}
      layout={layout}
      // The dense line is the LIST's density and never the stack's, and the
      // words are what turn it on: a surface with no name for the row's
      // popover and its menu keeps the full row (see DecisionCompactWords).
      compact={layout === "row" ? labels.compactRow : undefined}
      now={now}
      labels={labels.card}
      provenance={trim.provenance}
      confidence={trim.confidence}
      display={trim.display}
      aside={trim.aside}
      meta={
        <>
          {trim.meta}
          {members.length > 0 && (
            <span className="ddeck-bundle-count">
              {labels.bundleSummary(members.length)}
            </span>
          )}
        </>
      }
      detail={
        members.length > 0 ? (
          <Disclosure
            className="ddeck-bundle-open"
            summary={labels.bundleMembers(members.length)}
          >
            {/* A list, not an indent: the size and the boundaries of the set
                have to reach a reader who is hearing this page. */}
            <ul className="ddeck-bundle-members">
              {members.map((member) => (
                <li key={member.id} className="t-caption">
                  {member.summary ?? member.kind}
                </li>
              ))}
            </ul>
          </Disclosure>
        ) : undefined
      }
      onAccept={() => onStage(item, "accept")}
      onEdit={() => onStage(item, "edit")}
      onReject={() => onStage(item, "reject")}
      onSkip={() => onStage(item, "skip")}
    />
  );
}
