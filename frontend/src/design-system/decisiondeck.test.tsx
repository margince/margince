// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */
import "@testing-library/jest-dom/vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { DecisionApproval, DecisionCardLabels } from "./decisioncard";
import {
  DecisionDeck,
  type DecisionDeckChips,
  type DecisionDeckItem,
  type DecisionDeckLabels,
  type DecisionSharedFacts,
  type DeckVerdict,
  dragVerdict,
  keyVerdict,
  type StagedDecision,
} from "./decisiondeck";

afterEach(cleanup);

const NOW = Date.parse("2026-08-24T09:00:00.000Z");
const HOUR = 60 * 60 * 1000;

const CARD_LABELS: DecisionCardLabels = {
  accept: "Accept",
  edit: "Edit",
  reject: "Reject",
  skip: "Later",
  expired: "This ran out of time.",
  draftSubject: "Subject",
  draftBody: "Message",
  noContent: "Nothing to read.",
};

const LABELS: DecisionDeckLabels = {
  card: CARD_LABELS,
  deckLabel: "Decisions waiting on you",
  viewLabel: "How to work the queue",
  viewDeck: "Deck",
  viewList: "List",
  keys: "Arrow keys decide, Enter commits",
  behind: (count) => `${count} more behind`,
  staged: (count) => `${count} staged`,
  commit: "Commit",
  unstage: "Undo the last",
  clearedTitle: "The queue is clear.",
  cleared: (count) => `You decided ${count}.`,
  clearedTime: (atMs) => `Finished at ${atMs}.`,
  empty: "Nothing is waiting on you.",
  bundleSummary: (members) => `1 decision · ${members} recipients`,
  bundleMembers: (members) => `The ${members} recipients`,
};

function approval(
  seed: number,
  over: Partial<DecisionApproval> = {},
): DecisionApproval {
  return {
    id: `id-${seed}`,
    kind: "held_draft",
    status: "pending",
    proposed_by: "agent:mailroom",
    created_at: "2026-08-24T07:41:00.000Z",
    expires_at: new Date(NOW + 9 * HOUR).toISOString(),
    summary: `Reply number ${seed}.`,
    proposed_change: { subject: `Subject ${seed}` },
    ...over,
  };
}

function single(
  seed: number,
  over?: Partial<DecisionApproval>,
): DecisionDeckItem {
  return { kind: "single", id: `id-${seed}`, approval: approval(seed, over) };
}

const THREE = [single(1), single(2), single(3)] as const;

function deck(over: Partial<Parameters<typeof DecisionDeck>[0]> = {}) {
  return (
    <DecisionDeck
      items={THREE}
      now={NOW}
      labels={LABELS}
      onCommit={() => undefined}
      {...over}
    />
  );
}

/** The live card's own surface — where the arrow keys and the drag land. */
function liveSurface(): HTMLElement {
  return screen.getByRole("group", { name: "Decisions waiting on you" });
}

/**
 * The box inside that surface that actually MOVES.
 *
 * The two are deliberately separate elements: the surface holds the tab stop and
 * the handlers and never moves, so replacing the card cannot take a reader's
 * focus with it. By class because the box is chrome — it carries no role, no
 * name and nothing to read, which is the whole reason it can be replaced.
 */
function liveBox(): HTMLElement {
  const box = liveSurface().querySelector<HTMLElement>(".ddeck-card");
  if (!box) {
    throw new Error("the live card's moving box is not rendered");
  }
  return box;
}

describe("DecisionDeck — stage, then commit", () => {
  // The whole reason this component exists. A recorded decision has no reverse
  // (approving mints a token and executes the effect), so a surface that sent a
  // verdict on the swipe would be a surface where a flick of the wrist sends an
  // email. Nothing may leave the browser until the commit.
  it("stages a verdict without committing anything", async () => {
    const user = userEvent.setup();
    const onCommit = vi.fn();
    render(deck({ onCommit }));
    await user.click(screen.getByRole("button", { name: "Accept" }));
    expect(onCommit).not.toHaveBeenCalled();
    expect(screen.getByText("1 staged")).toBeInTheDocument();
  });

  it("commits exactly what is in the tray, once", async () => {
    const user = userEvent.setup();
    const onCommit = vi.fn();
    render(deck({ onCommit }));
    await user.click(screen.getByRole("button", { name: "Accept" }));
    await user.click(screen.getByRole("button", { name: "Reject" }));
    await user.click(screen.getByRole("button", { name: "Commit" }));
    expect(onCommit).toHaveBeenCalledTimes(1);
    expect(onCommit).toHaveBeenCalledWith([
      { id: "id-1", verdict: "accept" },
      { id: "id-2", verdict: "reject" },
    ] satisfies StagedDecision[]);
  });

  // Un-staging IS the undo, and it is free right up to the commit — which is why
  // it takes the LAST one rather than asking which.
  it("un-stages the last verdict and leaves the rest", async () => {
    const user = userEvent.setup();
    const onCommit = vi.fn();
    render(deck({ onCommit }));
    await user.click(screen.getByRole("button", { name: "Accept" }));
    await user.click(screen.getByRole("button", { name: "Reject" }));
    await user.click(screen.getByRole("button", { name: "Undo the last" }));
    expect(screen.getByText("1 staged")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Commit" }));
    expect(onCommit).toHaveBeenCalledWith([{ id: "id-1", verdict: "accept" }]);
  });

  // An empty tray has nothing to send, and a commit that fired anyway would ask
  // the server to decide nothing at all.
  // Focus FIRST: the shortcut lives on the deck's own surface, so an Enter
  // pressed at the document proves nothing about the empty-tray guard — the
  // handler was never reached, and dropping the guard would leave this green.
  it("sends nothing while the tray is empty", async () => {
    const user = userEvent.setup();
    const onCommit = vi.fn();
    render(deck({ onCommit }));
    expect(
      screen.queryByRole("button", { name: "Commit" }),
    ).not.toBeInTheDocument();
    liveSurface().focus();
    await user.keyboard("{Enter}");
    expect(onCommit).not.toHaveBeenCalled();
  });

  // A verdict already on the wire cannot be re-sent by a second press.
  it("refuses a second commit while the first is still out", async () => {
    const user = userEvent.setup();
    const onCommit = vi.fn();
    render(deck({ onCommit, commitState: "sending" }));
    await user.click(screen.getByRole("button", { name: "Accept" }));
    await user.click(screen.getByRole("button", { name: "Commit" }));
    expect(onCommit).not.toHaveBeenCalled();
  });

  // Re-answering one item replaces its verdict: a tray holding "accept" and
  // "reject" for the same proposal is a tray with no answer in it.
  it("replaces a verdict rather than queueing a second one for the same item", async () => {
    const user = userEvent.setup();
    const onCommit = vi.fn();
    render(deck({ onCommit, items: [single(1)] }));
    await user.click(screen.getByRole("button", { name: "Accept" }));
    await user.click(screen.getByRole("button", { name: "Undo the last" }));
    await user.click(screen.getByRole("button", { name: "Reject" }));
    await user.click(screen.getByRole("button", { name: "Commit" }));
    expect(onCommit).toHaveBeenCalledWith([{ id: "id-1", verdict: "reject" }]);
  });
});

describe("DecisionDeck — the keyboard is the equal of the pointer", () => {
  // Two lists that happen to agree today are two lists. Both readings come from
  // the same vocabulary, so this asserts the mapping rather than the drawing.
  it("maps a key and a drag to the same four verdicts", () => {
    expect(keyVerdict("ArrowRight")).toBe("accept");
    expect(dragVerdict(200, 0)).toBe("accept");
    expect(keyVerdict("ArrowLeft")).toBe("reject");
    expect(dragVerdict(-200, 0)).toBe("reject");
    expect(keyVerdict("ArrowUp")).toBe("edit");
    expect(dragVerdict(0, -200)).toBe("edit");
    expect(keyVerdict("ArrowDown")).toBe("skip");
    expect(dragVerdict(0, 200)).toBe("skip");
    expect(keyVerdict("q")).toBeNull();
  });

  // A short drag springs back. Without a threshold, resting a finger on a card
  // decides it.
  it("reads a drag that did not travel as no verdict at all", () => {
    expect(dragVerdict(20, 0)).toBeNull();
    expect(dragVerdict(0, 30)).toBeNull();
    // The dominant axis decides, so a diagonal is one verdict and not two.
    expect(dragVerdict(200, 90)).toBe("accept");
    expect(dragVerdict(90, -200)).toBe("edit");
  });

  // The same three verdicts, staged twice: once by pressing the buttons and once
  // by pressing the arrows. The committed trays must be identical, or the deck is
  // two products.
  it("commits the same tray whichever input staged it", async () => {
    const byPointer = vi.fn();
    const user = userEvent.setup();
    render(deck({ onCommit: byPointer }));
    await user.click(screen.getByRole("button", { name: "Accept" }));
    await user.click(screen.getByRole("button", { name: "Reject" }));
    await user.click(screen.getByRole("button", { name: "Later" }));
    await user.click(screen.getByRole("button", { name: "Commit" }));
    cleanup();

    const byKeyboard = vi.fn();
    render(deck({ onCommit: byKeyboard }));
    liveSurface().focus();
    await user.keyboard("{ArrowRight}{ArrowLeft}{ArrowDown}{Enter}");
    expect(byKeyboard.mock.calls).toEqual(byPointer.mock.calls);
  });

  // Staging the last card takes the keyboard surface off the screen with it, so
  // the commit control has to take the focus the deck just lost. Without this a
  // reader who worked the whole queue from the keyboard is left with focus on the
  // document body and the one press that matters reachable only by tabbing back
  // in from the top of the page.
  it("hands focus to the commit control when the last card is staged", async () => {
    const user = userEvent.setup();
    render(deck({ items: [single(1)] }));
    liveSurface().focus();
    await user.keyboard("{ArrowRight}");
    expect(screen.getByRole("button", { name: "Commit" })).toHaveFocus();
  });

  it("un-stages from the keyboard as well", async () => {
    const user = userEvent.setup();
    const onCommit = vi.fn();
    render(deck({ onCommit }));
    liveSurface().focus();
    await user.keyboard("{ArrowRight}{ArrowLeft}u{Enter}");
    expect(onCommit).toHaveBeenCalledWith([{ id: "id-1", verdict: "accept" }]);
  });

  // Enter on the Accept button must accept that card, not commit the tray behind
  // it. The shortcuts belong to the deck's own surface and to nothing inside it.
  it("leaves a key pressed inside a control to that control", async () => {
    const user = userEvent.setup();
    const onCommit = vi.fn();
    render(deck({ onCommit }));
    await user.click(screen.getByRole("button", { name: "Accept" }));
    screen.getByRole("button", { name: "Reject" }).focus();
    await user.keyboard("{Enter}");
    // The press rejected the live card; it did not commit the tray.
    expect(onCommit).not.toHaveBeenCalled();
    expect(screen.getByText("2 staged")).toBeInTheDocument();
  });

  // The drag as the browser delivers it: down, move past the threshold, up.
  it("stages the drag's verdict when the finger travels far enough", () => {
    const onCommit = vi.fn();
    render(deck({ onCommit }));
    const surface = liveSurface();
    fireEvent.pointerDown(surface, { pointerId: 1, clientX: 0, clientY: 0 });
    fireEvent.pointerMove(surface, { pointerId: 1, clientX: 220, clientY: 4 });
    fireEvent.pointerUp(surface, { pointerId: 1, clientX: 220, clientY: 4 });
    expect(screen.getByText("1 staged")).toBeInTheDocument();
  });

  it("springs back from a drag that stopped short", () => {
    render(deck({ onCommit: () => undefined }));
    const surface = liveSurface();
    fireEvent.pointerDown(surface, { pointerId: 1, clientX: 0, clientY: 0 });
    fireEvent.pointerMove(surface, { pointerId: 1, clientX: 18, clientY: 0 });
    fireEvent.pointerUp(surface, { pointerId: 1, clientX: 18, clientY: 0 });
    expect(screen.queryByText("1 staged")).not.toBeInTheDocument();
  });
});

describe("DecisionDeck — what it refuses", () => {
  // The card withholds the button; the arrow key has to be refused too, or the
  // keyboard path can stage a verdict the pointer path cannot.
  it("offers no Accept on a lapsed proposal, from either input", async () => {
    const user = userEvent.setup();
    const onCommit = vi.fn();
    render(
      deck({
        onCommit,
        items: [single(9, { expires_at: new Date(NOW - HOUR).toISOString() })],
      }),
    );
    expect(
      screen.queryByRole("button", { name: "Accept" }),
    ).not.toBeInTheDocument();
    liveSurface().focus();
    await user.keyboard("{ArrowRight}");
    expect(screen.queryByText("1 staged")).not.toBeInTheDocument();
    // Rejecting one is still a decision somebody may want recorded — only the
    // Accept is impossible.
    await user.keyboard("{ArrowLeft}");
    expect(screen.getByText("1 staged")).toBeInTheDocument();
  });
});

describe("DecisionDeck — a bundle is one decision", () => {
  const BUNDLE: DecisionDeckItem = {
    kind: "bundle",
    id: "bundle-1",
    bundleId: "bundle-1",
    members: [approval(11), approval(12), approval(13)],
  };

  it("collapses an act into one question with its members behind it", () => {
    render(deck({ items: [BUNDLE] }));
    expect(screen.getByText("1 decision · 3 recipients")).toBeInTheDocument();
    expect(screen.getByText("The 3 recipients")).toBeInTheDocument();
    // ONE set of verbs, not three: the API decides the whole bundle in one call.
    expect(screen.getAllByRole("button", { name: "Accept" })).toHaveLength(1);
  });

  it("stages the whole bundle under its own id", async () => {
    const user = userEvent.setup();
    const onCommit = vi.fn();
    render(deck({ items: [BUNDLE], onCommit }));
    await user.click(screen.getByRole("button", { name: "Accept" }));
    await user.click(screen.getByRole("button", { name: "Commit" }));
    expect(onCommit).toHaveBeenCalledWith([
      { id: "bundle-1", verdict: "accept" as DeckVerdict },
    ]);
  });
});

describe("DecisionDeck — a card states only what its members agree on", () => {
  // The chips as a caller draws them, one line per fact, and a fact the caller
  // was not given simply has no line. Reading them back out of the DOM is what
  // proves the deck hands over the AGREEMENT rather than the member it drew.
  const CHIPS = (
    _approval: DecisionApproval,
    shared: DecisionSharedFacts,
  ): DecisionDeckChips => ({
    meta: (
      <>
        {shared.kind !== undefined && <span>{`kind: ${shared.kind}`}</span>}
        {shared.proposedBy !== undefined && (
          <span>{`by: ${shared.proposedBy}`}</span>
        )}
        {shared.confidence !== undefined && (
          <span>{`confidence: ${shared.confidence}`}</span>
        )}
      </>
    ),
  });

  function bundle(...members: readonly DecisionApproval[]): DecisionDeckItem {
    return { kind: "bundle", id: "bundle-1", bundleId: "bundle-1", members };
  }

  it("states a fact every member carries", () => {
    render(
      deck({
        items: [
          bundle(
            approval(11, { confidence: 0.9 }),
            approval(12, { confidence: 0.9 }),
          ),
        ],
        chips: CHIPS,
      }),
    );
    expect(screen.getByText("kind: held_draft")).toBeInTheDocument();
    expect(screen.getByText("by: agent:mailroom")).toBeInTheDocument();
    expect(screen.getByText("confidence: 0.9")).toBeInTheDocument();
  });

  // The one the drawn member would have answered wrongly: it names an agent,
  // and the card would have said that agent staged all of them.
  it("drops the fact the members disagree on, and only that one", () => {
    render(
      deck({
        items: [
          bundle(
            approval(11, { proposed_by: "agent:deepread", confidence: 0.9 }),
            approval(12, { proposed_by: "agent:site-read", confidence: 0.9 }),
          ),
        ],
        chips: CHIPS,
      }),
    );
    expect(screen.queryByText(/^by: /)).not.toBeInTheDocument();
    expect(screen.getByText("kind: held_draft")).toBeInTheDocument();
    expect(screen.getByText("confidence: 0.9")).toBeInTheDocument();
  });

  // Absence is a disagreement too. One member scored and one unscored is not a
  // scored act, and taking the reading that exists is exactly how a bundle came
  // to report a confidence nobody claimed for the rest of it.
  it("counts a fact one member never carried as a disagreement", () => {
    render(
      deck({
        items: [bundle(approval(11, { confidence: 0.9 }), approval(12))],
        chips: CHIPS,
      }),
    );
    expect(screen.queryByText(/^confidence: /)).not.toBeInTheDocument();
    expect(screen.getByText("by: agent:mailroom")).toBeInTheDocument();
  });

  // The other end: a single agrees with itself, so nothing is withheld from the
  // card that was never a bundle. A rule that quietly blanked every chip would
  // satisfy every case above.
  it("leaves a single's own facts whole", () => {
    render(deck({ items: [single(1, { confidence: 0.42 })], chips: CHIPS }));
    expect(screen.getByText("kind: held_draft")).toBeInTheDocument();
    expect(screen.getByText("by: agent:mailroom")).toBeInTheDocument();
    expect(screen.getByText("confidence: 0.42")).toBeInTheDocument();
  });
});

describe("DecisionDeck — the states it can honestly be in", () => {
  it("says the queue is empty only when it is", () => {
    render(deck({ items: [] }));
    expect(screen.getByText("Nothing is waiting on you.")).toBeInTheDocument();
  });

  // A read that failed must not read as a queue with nothing in it.
  it("hands a failed read to the state vocabulary rather than drawing it empty", () => {
    render(deck({ items: [], state: "failed" }));
    expect(
      screen.queryByText("Nothing is waiting on you."),
    ).not.toBeInTheDocument();
  });

  it("counts what is still behind the live card", () => {
    render(deck());
    expect(screen.getByText("2 more behind")).toBeInTheDocument();
  });

  // The cleared plate is the deck REMEMBERING what it watched leave: a staged
  // verdict whose item is gone from `items` was decided, which is a better signal
  // than a success callback because it cannot claim a decision the list still
  // shows as waiting.
  it("shows the cleared plate once the committed items have left the queue", async () => {
    const user = userEvent.setup();
    const { rerender } = render(deck({ items: [single(1)] }));
    await user.click(screen.getByRole("button", { name: "Accept" }));
    await user.click(screen.getByRole("button", { name: "Commit" }));
    rerender(deck({ items: [] }));
    expect(screen.getByText("The queue is clear.")).toBeInTheDocument();
    expect(screen.getByText("You decided 1.")).toBeInTheDocument();
    expect(screen.getByText(`Finished at ${NOW}.`)).toBeInTheDocument();
    // NOT the empty sentence: "there was never anything" and "you finished it"
    // are different facts about the same blank screen.
    expect(
      screen.queryByText("Nothing is waiting on you."),
    ).not.toBeInTheDocument();
  });

  // A refused commit keeps the tray. The verdicts are the only copy of a
  // person's answers, and clearing them would ask for all of them again.
  it("keeps the staged verdicts when the commit came back refused", async () => {
    const user = userEvent.setup();
    const { rerender } = render(deck());
    await user.click(screen.getByRole("button", { name: "Accept" }));
    rerender(deck({ commitState: "failed" }));
    expect(screen.getByText("1 staged")).toBeInTheDocument();
  });

  it("draws the same cards as rows in the list view", async () => {
    const user = userEvent.setup();
    render(deck());
    await user.click(screen.getByRole("button", { name: "List" }));
    // Every waiting item is answerable at once, rather than one at a time.
    expect(screen.getAllByRole("button", { name: "Accept" })).toHaveLength(3);
    expect(screen.queryByText("2 more behind")).not.toBeInTheDocument();
  });
});

describe("DecisionDeck — a press on a control belongs to the control", () => {
  // The deck's verbs stopped working while the swipe surface captured the
  // pointer: capture retargets every later event for that pointer at the
  // capturing element, the compatibility `click` included, so the click went to
  // the fieldset and the button under the finger never heard about it. jsdom
  // implements neither capture nor that retargeting, so the observable half is
  // what is asserted here — a press that starts on a control starts no drag.
  it("starts no drag from a press on one of the card's verbs", () => {
    render(deck());
    const accept = screen.getByRole("button", { name: "Accept" });
    fireEvent.pointerDown(accept, { button: 0, pointerId: 1 });
    expect(liveBox()).not.toHaveAttribute("data-dragging");
  });

  it("still starts a drag from the card itself", () => {
    render(deck());
    fireEvent.pointerDown(liveSurface(), {
      button: 0,
      pointerId: 1,
      clientX: 0,
      clientY: 0,
    });
    expect(liveBox()).toHaveAttribute("data-dragging");
  });
});

describe("DecisionDeck — what the swipe tells the reader", () => {
  // The whole point of the hint: which direction means what, learned while the
  // finger is still down rather than after the card has gone.
  it("names the verdict a drag would stage, once it has travelled far enough", () => {
    render(deck());
    const surface = liveSurface();
    fireEvent.pointerDown(surface, { pointerId: 1, clientX: 0, clientY: 0 });
    fireEvent.pointerMove(surface, { pointerId: 1, clientX: 20, clientY: 0 });
    // Short of the threshold there is no verdict yet, and claiming one would
    // promise a card that is about to spring back.
    expect(liveBox()).not.toHaveAttribute("data-verdict");
    fireEvent.pointerMove(surface, { pointerId: 1, clientX: 120, clientY: 0 });
    expect(liveBox()).toHaveAttribute("data-verdict", "accept");
  });

  // The exit continues the gesture. Starting the flight from the middle of the
  // plate is what made a successful swipe read as the same card snapping back.
  it("starts the card's exit where the hand let go", () => {
    const { container } = render(deck());
    const surface = liveSurface();
    fireEvent.pointerDown(surface, { pointerId: 1, clientX: 0, clientY: 0 });
    fireEvent.pointerMove(surface, { pointerId: 1, clientX: 130, clientY: 12 });
    fireEvent.pointerUp(surface, { pointerId: 1, clientX: 130, clientY: 12 });
    const ghost = container.querySelector<HTMLElement>(".ddeck-ghost");
    expect(ghost).not.toBeNull();
    expect(ghost?.getAttribute("data-verdict")).toBe("accept");
    expect(ghost?.style.getPropertyValue("--ddeck-from-x")).toBe("130px");
    expect(ghost?.style.getPropertyValue("--ddeck-from-y")).toBe("12px");
  });

  // The card that moves is replaced per verdict; the surface that holds focus is
  // not. Without the split, a reader working the queue from the keyboard lost
  // their tab stop on the first arrow key and the rest of the queue was
  // unreachable without tabbing back in from the top.
  it("keeps the keyboard's tab stop through a verdict", async () => {
    const user = userEvent.setup();
    render(deck());
    liveSurface().focus();
    await user.keyboard("{ArrowRight}");
    expect(document.activeElement).toBe(liveSurface());
  });
});

describe("DecisionDeck — the head", () => {
  it("puts the title and the view toggle on one row", () => {
    render(deck({ title: "Waiting on you" }));
    const heading = screen.getByRole("heading", { name: "Waiting on you" });
    const row = heading.closest(".section-header");
    expect(row).not.toBeNull();
    // The toggle is INSIDE the header the title is in, which is the whole
    // point: a title above the deck and a control inside it are two rows
    // saying one thing.
    expect(row?.contains(screen.getByRole("button", { name: "Deck" }))).toBe(
      true,
    );
  });

  it("draws no heading at all without a title", () => {
    render(deck());
    expect(screen.queryByRole("heading")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Deck" })).toBeInTheDocument();
  });

  // The toggle asks HOW to draw what is waiting, so a cleared plate keeps the
  // title and loses the control — there is nothing left to switch between.
  it("keeps the title but drops the toggle once nothing is waiting", () => {
    render(deck({ items: [], title: "Waiting on you" }));
    expect(
      screen.getByRole("heading", { name: "Waiting on you" }),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Deck" }),
    ).not.toBeInTheDocument();
  });
});

describe("DecisionDeck — a verdict that sends nothing", () => {
  // "Later" is answered by the deck itself: the caller sends nothing for it, the
  // item stays pending on the server, and it therefore never leaves `items`. Left
  // in the tray it sat under a commit control that could be pressed forever with
  // nothing happening, while the plate behind it read "clear".
  it("leaves the tray on commit, and the deck with it", async () => {
    const user = userEvent.setup();
    const onCommit = vi.fn();
    render(deck({ items: [single(1), single(2)], onCommit }));
    await user.click(screen.getByRole("button", { name: "Later" }));
    expect(screen.getByText("1 staged")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Commit" }));
    // The caller was told what the reader answered...
    expect(onCommit).toHaveBeenCalledWith([{ id: "id-1", verdict: "skip" }]);
    // ...and the tray is empty rather than holding a verdict nobody will send.
    expect(screen.queryByText("1 staged")).not.toBeInTheDocument();
    // The deferred card does not come back in this session either: later means
    // later, and re-offering it immediately is the one thing "later" rules out.
    expect(screen.getByText("Subject 2")).toBeInTheDocument();
    expect(screen.queryByText("Subject 1")).not.toBeInTheDocument();
  });
});
