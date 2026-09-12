/** @vitest-environment happy-dom */
import { cleanup, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { en } from "../i18n/en";
import { BriefFeed } from "./brief.feed";
import { render } from "./brief.testkit";
import type { Worklist, WorklistItem } from "./worklist.queries";

// The morning as ONE feed.
//
// These are not about how a row looks — WorklistRow owns that and the Worklist
// screen tests it. They are about the page not becoming a second opinion: the
// order is the server's, the cut is a prefix, the section badge is a label, and
// nothing here re-sorts, re-ranks or regroups.

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

function titles(container: HTMLElement): string[] {
  return [...container.querySelectorAll(".worklist-row-title")].map(
    (node) => node.textContent ?? "",
  );
}

describe("the morning feed", () => {
  // THE claim of the whole surface. The server's tie-breaks need a
  // base-currency conversion and a materiality threshold the browser does not
  // hold, so a client that re-sorted would be answering a question it cannot.
  it("draws the rows in wire order and re-sorts nothing", () => {
    const { container } = render(
      <BriefFeed
        day={day([
          item({ id: "a", title: "Zeta last-alphabetically" }),
          item({ id: "b", title: "Alpha first-alphabetically" }),
          item({ id: "c", title: "Mid" }),
        ])}
        state="ready"
      />,
    );

    const drawn = titles(container);
    expect(drawn[0]).toContain("Zeta last-alphabetically");
    expect(drawn[1]).toContain("Alpha first-alphabetically");
    expect(drawn[2]).toContain("Mid");
  });

  // THE case the section label exists to survive. Sections alternate down the
  // page — respond, move, respond again — because the ranking put them there,
  // and a client that grouped by section would reorder the page into three
  // blocks and disagree with the order the server chose.
  it("keeps the server order even when the section labels alternate", () => {
    const { container } = render(
      <BriefFeed
        day={day([
          item({ id: "a", title: "First", brief_section: "respond_now" }),
          item({ id: "b", title: "Second", brief_section: "move_revenue" }),
          item({ id: "c", title: "Third", brief_section: "respond_now" }),
          item({ id: "d", title: "Fourth", brief_section: "move_revenue" }),
        ])}
        state="ready"
      />,
    );

    const drawn = titles(container);
    expect(drawn[0]).toContain("First");
    expect(drawn[1]).toContain("Second");
    expect(drawn[2]).toContain("Third");
    expect(drawn[3]).toContain("Fourth");
  });

  // A run-length label, not a grouping: the second row of a section says
  // nothing again.
  it("draws a section label once per run rather than once per row", () => {
    render(
      <BriefFeed
        day={day([
          item({ id: "a", brief_section: "move_revenue" }),
          item({ id: "b", brief_section: "move_revenue" }),
          item({ id: "c", brief_section: "respond_now" }),
        ])}
        state="ready"
      />,
    );

    expect(
      screen.getAllByText(en["brief.feed.section.move_revenue"]),
    ).toHaveLength(1);
    expect(
      screen.getAllByText(en["brief.feed.section.respond_now"]),
    ).toHaveLength(1);
  });

  // And a section that comes BACK after another one draws its label again —
  // the reader has arrived at it a second time, and a label suppressed by
  // "have I drawn this before" would leave the second run unlabelled.
  it("labels a section again when the order returns to it", () => {
    render(
      <BriefFeed
        day={day([
          item({ id: "a", brief_section: "respond_now" }),
          item({ id: "b", brief_section: "move_revenue" }),
          item({ id: "c", brief_section: "respond_now" }),
        ])}
        state="ready"
      />,
    );

    expect(
      screen.getAllByText(en["brief.feed.section.respond_now"]),
    ).toHaveLength(2);
  });

  // A label that is true of EVERY row under it says nothing about where the
  // reader is — it names the panel a second time. So the headings arrive only
  // once the morning has more than one part on screen.
  it("draws no group heading while the rows are all one group", () => {
    const { container } = render(
      <BriefFeed
        day={day([
          item({ id: "a", brief_section: "review_and_repair" }),
          item({ id: "b", brief_section: "review_and_repair" }),
        ])}
        state="ready"
      />,
    );

    expect(container.querySelector(".brief-feed-section")).toBeNull();
    expect(titles(container)).toHaveLength(2);
  });

  // The same two rows with a third from another part: now the headings say
  // which is which, and both groups get one.
  it("draws the group headings as soon as a second group is on screen", () => {
    render(
      <BriefFeed
        day={day([
          item({ id: "a", brief_section: "review_and_repair" }),
          item({ id: "b", brief_section: "review_and_repair" }),
          item({ id: "c", brief_section: "respond_now" }),
        ])}
        state="ready"
      />,
    );

    expect(
      screen.getAllByText(en["brief.feed.section.review_and_repair"]),
    ).toHaveLength(1);
    expect(
      screen.getAllByText(en["brief.feed.section.respond_now"]),
    ).toHaveLength(1);
  });

  // WHAT IS ON SCREEN, out of what the day holds. The motto that stood here
  // told a reader nothing they could act on; the urgent figure is the summary's
  // own, so this line and the readings strip above cannot disagree.
  it("says how much is waiting and how much of it is urgent", () => {
    render(
      <BriefFeed
        day={
          {
            ...day([item({ id: "a" }), item({ id: "b" })]),
            summary: { total: 2, urgent: 1 },
          } as unknown as Worklist
        }
        state="ready"
      />,
    );

    expect(
      screen.getByText(
        en["brief.feed.counts"]
          .replace("{items}", "2")
          .replace("{urgent}", "1"),
      ),
    ).toBeTruthy();
  });

  // A count line over a read that landed on no summary would report a figure
  // nobody measured. The panel says nothing instead.
  it("says nothing about a count when the answer carried no summary", () => {
    const { container } = render(
      <BriefFeed day={{ queue: [] } as unknown as Worklist} state="ready" />,
    );

    expect(container.querySelector(".panel-head-sub")).toBeNull();
  });

  // The head's counts link INTO this panel, so the id is part of the page's
  // contract rather than decoration.
  it("carries the address the page's counts link to", () => {
    const { container } = render(
      <BriefFeed day={day([item()])} state="ready" />,
    );

    expect(container.querySelector("#brief-today")).toBeTruthy();
  });

  // What has MOVED since the brief was cut, in the header band, as a way
  // through to the rows that moved. Absent when the page has not been told —
  // a zero badge would claim a measurement nobody made.
  it("badges what changed since the brief, and links to it", () => {
    render(
      <BriefFeed
        day={day([item()])}
        state="ready"
        changed={{ count: 4, href: "#/worklist?filter=changed_since_brief" }}
      />,
    );

    const link = screen.getByRole("link", {
      name: en["brief.feed.changedBadge_other"].replace("{count}", "4"),
    });
    expect(link.getAttribute("href")).toBe(
      "#/worklist?filter=changed_since_brief",
    );
  });

  it("draws no changed badge when the page has not been told", () => {
    const { container } = render(
      <BriefFeed day={day([item()])} state="ready" />,
    );

    expect(container.querySelector(".panel-head .badge")).toBeNull();
  });

  // A row the server did not place carries no section, and the feed invents no
  // heading for it: a label chosen here would put the row under a part of the
  // morning nobody decided.
  it("draws no heading for a row the server did not place", () => {
    const { container } = render(
      <BriefFeed day={day([item({ id: "a" })])} state="ready" />,
    );

    expect(container.querySelector(".brief-feed-section")).toBeNull();
    expect(titles(container)).toHaveLength(1);
  });

  // Five is a morning somebody can finish. The sixth row is on the worklist.
  it("draws at most five rows", () => {
    const { container } = render(
      <BriefFeed
        day={day(
          Array.from({ length: 11 }, (_, at) =>
            item({ id: `row-${at}`, title: `Row ${at}` }),
          ),
        )}
        state="ready"
      />,
    );

    // The BOUNDARY, both sides: the fifth row is drawn and the sixth is not.
    // Asserting only the length restates the cap rather than locating it.
    expect(container.querySelectorAll(".brief-feed-list > li")).toHaveLength(5);
    expect(container.textContent).toContain("Row 4");
    expect(container.textContent).not.toContain("Row 5");
  });

  // A page showing five of eleven rows that did not say where the other six
  // are has hidden them.
  it("says how many rows it left out, and where they are", () => {
    render(
      <BriefFeed
        day={day(
          Array.from({ length: 11 }, (_, at) => item({ id: `row-${at}` })),
        )}
        state="ready"
      />,
    );

    const link = screen.getByText(
      en["brief.feed.rest"].replace("{count}", "6"),
    );
    // The door narrows to the same population the count was taken over. The
    // test below proves the count drops approvals; a bare `#/worklist` would
    // then open a queue holding them, so a rep told "11 more" landed on 14.
    expect(link.getAttribute("href")).toBe(
      "#/worklist?filter=except_decisions",
    );
  });

  // The remainder counts what THIS surface is showing, not what the queue
  // holds. A footer that counted the excluded approvals would send a reader to
  // the worklist for rows this page deliberately did not draw.
  //
  // Carried across from the deleted "Do next" suite, which had this case and
  // this reason. Without it a regression computing the remainder from
  // `day.queue.length` rather than from the approval-filtered rows passes every
  // other test in this file, because none of them mixes an approval into a
  // queue long enough to have a remainder.
  it("counts the remainder over what it shows, not over the whole queue", () => {
    render(
      <BriefFeed
        day={day([
          item({ id: "decision", source: "approval" }),
          ...Array.from({ length: 6 }, (_, at) => item({ id: `row-${at}` })),
        ])}
        state="ready"
      />,
    );

    // Six drawable rows, five shown: ONE remains, not the two a count over the
    // whole seven-row queue would report.
    expect(
      screen.getByText(en["brief.feed.rest"].replace("{count}", "1")),
    ).toBeTruthy();
  });

  it("says nothing about a remainder when it is showing everything", () => {
    render(<BriefFeed day={day([item()])} state="ready" />);

    expect(screen.queryByText(/more on the worklist/)).toBeNull();
  });

  // The decisions deck is a surface of its own further up the SAME page,
  // holding the same approvals and posting to the same endpoint. A row here put
  // one decision in front of a reader twice, each copy answerable — on a page
  // whose whole claim is that it states an order once.
  it("leaves the decisions the deck above already answers to the deck", () => {
    const { container } = render(
      <BriefFeed
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

  // THE ORDINAL IS GONE. The ordered list carries the order for a reader
  // hearing the page, so a digit per row spent a column saying again what the
  // page says once — and Brief has no second column to open a row INTO, so the
  // rank could never even be the control it is on the Worklist.
  it("draws no rank column, and no control on one", () => {
    const { container } = render(
      <BriefFeed day={day([item()])} state="ready" />,
    );

    expect(container.querySelector("ol.brief-feed-list")).toBeTruthy();
    expect(container.querySelector(".worklist-rank")).toBeNull();
    expect(container.querySelector(".worklist-rank-select")).toBeNull();
  });

  // ONE LINE per row, which is what the density is. The row's own suite holds
  // what that line says; what is held here is that the Brief asks for it.
  it("draws the rows at list density", () => {
    const { container } = render(
      <BriefFeed
        day={day([item({ id: "a" }), item({ id: "b" })])}
        state="ready"
      />,
    );

    expect(container.querySelectorAll(".worklist-row-compact")).toHaveLength(2);
    expect(container.querySelectorAll(".worklist-row-line")).toHaveLength(2);
  });

  // THE TITLE IS THE LINK, so the verb that only reached the record is gone:
  // two controls on one line opening the same page ask the reader to choose
  // between the same thing twice.
  it("lets the title carry the link and draws no second way to the record", () => {
    const { container } = render(
      <BriefFeed
        day={day([
          item({
            id: "a",
            source: "task",
            actions: ["open"],
            subject: {
              type: "deal",
              id: "018f3a1b-0000-7000-8000-00000000d001",
            },
          }),
        ])}
        state="ready"
      />,
    );

    const title = container.querySelector(".worklist-row-title a");
    expect(title).toBeTruthy();
    expect(
      screen.queryByRole("link", { name: en["worklist.verb.open"] }),
    ).toBeNull();
  });

  // A payload with no queue at all. The optional chain has to reach the FIELD:
  // guarding only the payload threw and took the whole page with it. A page
  // that draws nothing is a bad answer; a page that throws is not an answer.
  it("draws nothing rather than throwing on an answer with no queue", () => {
    const { container } = render(
      <BriefFeed day={{} as unknown as Worklist} state="ready" />,
    );

    expect(container.querySelector(".brief-feed-list")).toBeNull();
    expect(screen.getByText(en["brief.feed.clear"])).toBeTruthy();
  });

  // An empty queue and a failed read are different facts. Saying "nothing is
  // waiting" over a read that never landed is the one thing this surface must
  // not do — a rep would close the page believing their morning was clear.
  it("tells a clear morning apart from a read that failed", () => {
    const { rerender } = render(<BriefFeed day={day([])} state="ready" />);
    expect(screen.getByText(en["brief.feed.clear"])).toBeTruthy();

    rerender(<BriefFeed day={undefined} state="failed" />);
    expect(screen.queryByText(en["brief.feed.clear"])).toBeNull();
  });

  // CARRIED ACROSS FROM "Do next", which this feed replaces. The surface draws
  // a waiting message in full — sender, subject, preview, access badge — and
  // without the drawer a reader is shown the message and then refused it.
  // worklist.row.tsx offers the opener only when the caller passes onOpenEmail,
  // so losing the mount loses the ability silently.
  it("opens a message it drew in full, as Do next did before it", () => {
    render(
      <BriefFeed
        day={day([
          item({
            id: "e1",
            title: "Aster Handel",
            email_summary: {
              activity_id: "01a05500-0000-7000-8000-00000000ee01",
              occurred_at: "2026-06-09T09:15:00Z",
              version: 2,
              subject: "Re: the renewal quote",
              preview: "Can you hold the price until Friday?",
              counterparty: "Dana Buyer",
              direction: "inbound",
              display_status: "team",
              move: "needs_reply",
              attachment_count: 0,
            },
          }),
        ])}
        state="ready"
      />,
    );

    // A control, not a paragraph: the row says it opens a dialog.
    const row = screen.getByRole("button", { name: /Re: the renewal quote/ });
    expect(row.getAttribute("aria-haspopup")).toBe("dialog");
  });
});
