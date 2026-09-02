/** @vitest-environment jsdom */
import { cleanup, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { en } from "../i18n/en";
import { DoNext } from "./brief.donext";
import { render } from "./home.testkit";
import type { Worklist, WorklistItem } from "./worklist.queries";

// The head of the ranked queue, on Home.
//
// What these are about is not how a row looks — WorklistRow owns that and the
// Worklist screen tests it. They are about the page not becoming a second
// opinion: the order is the server's, the cut is a prefix, and nothing here
// re-sorts or re-ranks.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function item(over: Partial<WorklistItem> = {}): WorklistItem {
  return {
    id: "i1",
    source: "waiting_customer",
    category: "customer_waiting",
    title: "Aster Handel",
    because: [],
    actions: ["open"],
    dispositions: [],
    overdue: false,
    ...over,
  } as unknown as WorklistItem;
}

function day(queue: WorklistItem[]): Worklist {
  return {
    as_of: "2026-06-10T06:00:00Z",
    scope: "mine",
    scope_options: ["mine"],
    queue,
    counts: [],
    reach: [],
    sources_unavailable: [],
    summary: { total: queue.length, urgent: 0 },
  } as unknown as Worklist;
}

describe("do next", () => {
  // The claim of the whole section. The server's tie-breaks need a
  // base-currency conversion and a materiality threshold the browser does not
  // hold, so a client that re-sorted would be answering a question it cannot.
  it("draws the first three rows in wire order and re-sorts nothing", () => {
    const { container } = render(
      <DoNext
        day={day([
          item({ id: "a", title: "Zeta last-alphabetically" }),
          item({ id: "b", title: "Alpha first-alphabetically" }),
          item({ id: "c", title: "Mid" }),
          item({ id: "d", title: "Fourth, below the cut" }),
        ])}
        state="ready"
      />,
    );

    const titles = [...container.querySelectorAll(".worklist-row-title")].map(
      (node) => node.textContent ?? "",
    );
    expect(titles).toHaveLength(3);
    expect(titles[0]).toContain("Zeta last-alphabetically");
    expect(titles[1]).toContain("Alpha first-alphabetically");
    expect(titles[2]).toContain("Mid");
    expect(container.textContent).not.toContain("Fourth, below the cut");
  });

  // The decisions deck is a surface of its own further up the SAME page, holding
  // the same approvals and posting to the same endpoint. A row here put one
  // decision in front of a reader twice, each copy answerable — on a page whose
  // whole claim is that it states an order once. Found by looking at the
  // rendered page, not by a test: every assertion above passed with it there.
  it("leaves the decisions the deck above already answers to the deck", () => {
    const { container } = render(
      <DoNext
        day={day([
          item({
            id: "a",
            source: "approval",
            title: "Confirm the close date",
          }),
          item({ id: "b", title: "Aster is waiting" }),
        ])}
        state="ready"
      />,
    );

    expect(container.textContent).not.toContain("Confirm the close date");
    expect(container.textContent).toContain("Aster is waiting");
  });

  // And the remainder counts what THIS section is showing, not what the queue
  // holds: a footer that counted the excluded approvals would send a reader to
  // the worklist for rows this page had deliberately not drawn.
  it("counts the remainder over what it shows, not over the whole queue", () => {
    render(
      <DoNext
        day={day([
          item({ id: "a", source: "approval" }),
          item({ id: "b" }),
          item({ id: "c" }),
          item({ id: "d" }),
          item({ id: "e" }),
        ])}
        state="ready"
      />,
    );

    // Four non-approval rows, three shown: one remains, not two.
    expect(
      screen.getByText(en["brief.donext.rest"].replace("{count}", "1")),
    ).toBeTruthy();
  });

  // A page showing three of eleven rows that did not say where the other eight
  // are has hidden them.
  it("says how many rows it left out, and where they are", () => {
    render(
      <DoNext
        day={day([
          item({ id: "a" }),
          item({ id: "b" }),
          item({ id: "c" }),
          item({ id: "d" }),
          item({ id: "e" }),
        ])}
        state="ready"
      />,
    );

    const link = screen.getByText(
      en["brief.donext.rest"].replace("{count}", "2"),
    );
    expect(link.getAttribute("href")).toBe("#/worklist");
  });

  it("says nothing about a remainder when it is showing everything", () => {
    render(<DoNext day={day([item()])} state="ready" />);

    expect(screen.queryByText(/more on the worklist/)).toBeNull();
  });

  // Home has no second column to open a row INTO. A rank button that answered
  // nothing is the dead control WorklistRow's own comment warns about.
  it("draws the rank as a number, not as a control that opens nothing", () => {
    const { container } = render(<DoNext day={day([item()])} state="ready" />);

    expect(container.querySelector(".worklist-rank")).toBeTruthy();
    expect(container.querySelector(".worklist-rank-select")).toBeNull();
    expect(screen.queryByRole("button", { name: /Aster Handel/ })).toBeNull();
  });

  // A payload with no queue at all. The optional chain has to reach the FIELD:
  // guarding only the payload threw and took the whole page with it, which is
  // the second time that exact mistake reached a gate on this screen. A page
  // that draws nothing is a bad answer; a page that throws is not an answer.
  it("draws nothing rather than throwing on an answer with no queue", () => {
    const { container } = render(
      <DoNext day={{} as unknown as Worklist} state="ready" />,
    );

    expect(container.querySelector(".brief-donext-list")).toBeNull();
    expect(screen.getByText(en["brief.donext.clear"])).toBeTruthy();
  });

  // An empty queue and a failed read are different facts. Saying "nothing is
  // waiting" over a read that never landed is the one thing this section must
  // not do — a rep would close the page believing their morning was clear.
  it("tells a clear morning apart from a read that failed", () => {
    const { rerender } = render(<DoNext day={day([])} state="ready" />);
    expect(screen.getByText(en["brief.donext.clear"])).toBeTruthy();

    rerender(<DoNext day={undefined} state="failed" />);
    expect(screen.queryByText(en["brief.donext.clear"])).toBeNull();
  });
});
