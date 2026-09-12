/** @vitest-environment jsdom */
import { cleanup, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import { en } from "../i18n/en";
import { deckItems } from "./brief";
import { DecisionsSection } from "./brief.decisions";
import { proposal } from "./brief.fixtures";
import { render } from "./brief.testkit";

// What is WAITING ON YOU, as a zone of the page.
//
// These are not about how a decision is answered — `DecisionDeck` owns the
// staging tray and the commit, and its own suite tests them. They are about the
// shape this surface gives the deck: one panel with one header band, the
// Deck/List switch inside that band, a LINE per decision, and three of them
// before the page hands the reader on to the approvals lane.

const NOW = Date.parse("2026-08-20T06:00:00Z");

afterEach(cleanup);

function items(count: number) {
  return deckItems(
    Array.from({ length: count }, (_, at) =>
      proposal(`ap-${at}`, `Send the follow-up ${at}`, {
        expires_at: "2026-08-20T12:00:00Z",
      }),
    ),
  );
}

function section(count: number) {
  return (
    <DecisionsSection
      items={items(count)}
      nowMs={NOW}
      state="ready"
      onAlreadyDecided={() => undefined}
    />
  );
}

describe("waiting on you", () => {
  // ONE BAND, the same one Today wears. A bare heading with a toolbar beside it
  // read as a different kind of block from every panel under it, on a page
  // whose whole claim is a column of zones at one interval.
  it("is a panel whose header band carries the view switch", () => {
    const { container } = render(section(1));

    const head = container.querySelector(".panel-head");
    expect(head).toBeTruthy();
    expect(
      within(head as HTMLElement).getByRole("heading", {
        name: en["brief.panel.decisions"],
      }),
    ).toBeTruthy();
    // The switch is IN the band, not in the body: it says how this zone is
    // drawn, which is what a zone's header keeps.
    expect(
      within(head as HTMLElement).getByRole("group", {
        name: en["brief.deck.view"],
      }),
    ).toBeTruthy();
  });

  // The panel names the region; the wrapper must not name it again, or one zone
  // stands in a screen reader's landmark list twice.
  it("names the region once", () => {
    const { container } = render(section(1));

    const outer = container.querySelector("#brief-decisions");
    expect(outer?.getAttribute("aria-label")).toBeNull();
  });

  // THE LIST IS THE DEFAULT. A reader arriving at their morning wants to see
  // what is waiting before they start answering it; a deck shows them one and
  // tells them a count.
  it("opens as a list rather than as a deck", () => {
    const { container } = render(section(2));

    expect(container.querySelectorAll(".ddeck-list > li")).toHaveLength(2);
    expect(container.querySelector(".ddeck-stack")).toBeNull();
  });

  // ONE LINE per decision: the question, its chips and its verbs. The proposal
  // itself is behind the line's own control, so opening one row moves no other
  // row's verbs out from under the reader's pointer.
  it("draws each decision as one line with its proposal behind a control", async () => {
    const user = userEvent.setup();
    const { container } = render(section(1));

    const row = container.querySelector(".dcard[data-density='compact']");
    expect(row).toBeTruthy();
    const line = row?.querySelector(".dcard-line");
    expect(line).toBeTruthy();
    // The drafted message is not on the line — it is what opening it shows.
    expect(line?.textContent).not.toContain(en["decision.draftBody"]);

    await user.click(
      screen.getByRole("button", { name: en["brief.deck.rowDetail"] }),
    );

    expect(await screen.findByText(en["decision.draftBody"])).toBeTruthy();
  });

  // Accept and Later keep their words; the two heavier verdicts go in the menu,
  // where each has a whole line to say what it does.
  it("spells Accept and Later and folds reject and edit into a menu", async () => {
    const user = userEvent.setup();
    render(section(1));

    expect(
      screen.getByRole("button", { name: en["trust.accept"] }),
    ).toBeTruthy();
    expect(
      screen.getByRole("button", { name: en["brief.deck.later"] }),
    ).toBeTruthy();
    expect(
      screen.queryByRole("button", { name: en["decision.reject"] }),
    ).toBeNull();

    await user.click(
      screen.getByRole("button", { name: en["brief.deck.rowMore"] }),
    );

    expect(
      await screen.findByRole("button", { name: en["decision.reject"] }),
    ).toBeTruthy();
    expect(screen.getByRole("button", { name: en["trust.edit"] })).toBeTruthy();
  });

  // Three questions a reader can answer on the way past, and the rest where
  // every one of them is. A block showing three of nine that did not say where
  // the other six are has hidden them.
  it("draws three rows and says where the rest are", () => {
    const { container } = render(section(9));

    expect(container.querySelectorAll(".ddeck-list > li")).toHaveLength(3);
    const link = screen.getByRole("link", {
      name: en["brief.deck.rest_other"].replace("{count}", "6"),
    });
    expect(link.getAttribute("href")).toBe("#/worklist?filter=decisions");
  });

  it("says nothing about a remainder when it is showing everything", () => {
    render(section(3));

    expect(screen.queryByRole("link", { name: /more/ })).toBeNull();
  });

  // EVERYTHING THE QUEUE DRAWS PAYS THE PANEL'S INSET. These rows are cards
  // rather than `PanelRow`s, so without a body around them the deck's plate ran
  // to the pane's own edges and the count behind it and the keyboard legend hung
  // at the panel's x=0.
  it("draws the queue inside the panel's body", () => {
    const { container } = render(section(2));

    const body = container.querySelector(".panel .panel-body");
    expect(body).toBeTruthy();
    expect(body?.querySelector(".ddeck-list")).toBeTruthy();
  });

  // THE TRAY IS THE PANEL'S FOOT: one band, edge to edge, a hairline over it.
  // Held in the body it drew its own floating box and read as a second card
  // overlapping the pane's bottom corners.
  it("puts the staging tray in the panel's foot, inside the same panel", async () => {
    const user = userEvent.setup();
    const { container } = render(section(2));

    const [accept] = screen.getAllByRole("button", {
      name: en["trust.accept"],
    });
    await user.click(accept);

    const panel = container.querySelector(".panel");
    const foot = panel?.querySelector(".panel-foot");
    expect(foot?.querySelector(".ddeck-tray")).toBeTruthy();
    // ONE panel, not a box beside it: the tray is inside the pane that names it.
    expect(container.querySelectorAll(".panel")).toHaveLength(1);
  });

  // With every decision staged the body says NOTHING rather than "nothing is
  // waiting on you" — which would contradict the reader's own unsent verdicts
  // counted in the foot one line below it.
  it("never says nothing is waiting while the tray holds the answers", async () => {
    const user = userEvent.setup();
    const { container } = render(section(1));

    await user.click(screen.getByRole("button", { name: en["trust.accept"] }));

    expect(screen.queryByText(en["brief.deck.empty"])).toBeNull();
    expect(container.querySelector(".panel-body")).toBeNull();
    expect(
      container.querySelector(".panel-foot .ddeck-tray")?.textContent,
    ).toContain(en["brief.deck.staged_one"]);
  });

  // A read that has not landed is not an empty queue. Saying "nothing is
  // waiting on you" over a failed read would send a reader away believing
  // nobody was blocked on them.
  it("tells an empty queue apart from a read that failed", () => {
    render(
      <DecisionsSection
        items={[]}
        nowMs={NOW}
        state="ready"
        onAlreadyDecided={() => undefined}
      />,
    );
    expect(screen.getByText(en["brief.deck.empty"])).toBeTruthy();
    cleanup();

    render(
      <DecisionsSection
        items={[]}
        nowMs={NOW}
        state="failed"
        onAlreadyDecided={() => undefined}
      />,
    );
    expect(screen.queryByText(en["brief.deck.empty"])).toBeNull();
  });
});
