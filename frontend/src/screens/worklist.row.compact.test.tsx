/** @vitest-environment jsdom */
import "@testing-library/jest-dom/vitest";
import { cleanup, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import { en } from "../i18n/en";
import { render } from "./brief.testkit";
import type { WorklistItem } from "./worklist.queries";
import { WorklistRow } from "./worklist.row";

// The row at LIST DENSITY.
//
// Driven through `WorklistRow density="compact"` and never through the line
// component under it: the density is a property of the ROW, and a suite that
// mounted the line on its own would prove nothing about what a surface draws.
//
// What is held here is the one claim the density makes — everything the reader
// needs is on ONE line, and nothing the row holds is lost to get it there.

afterEach(cleanup);

function item(over: Partial<WorklistItem> = {}): WorklistItem {
  return {
    id: "row-1",
    source: "brief_item",
    category: "deals_at_risk",
    level: 2,
    title: "Turbinenbau retrofit",
    // REQUIRED on the wire, and `none` is a real value: this row costs nothing
    // to leave. A fixture that omitted it would be a payload the server cannot
    // send, and the row would print the key it composed from `undefined`.
    consequence: "none",
    because: [],
    actions: ["open"],
    dispositions: [],
    overdue: false,
    subject: {
      type: "deal",
      id: "01a05500-0000-7000-8000-0000000000da",
      label: "Turbinenbau retrofit",
    },
    ...over,
  } as unknown as WorklistItem;
}

function row(over: Partial<WorklistItem> = {}) {
  return <WorklistRow item={item(over)} density="compact" owner="" />;
}

/** Four reasons, so three are said and the fourth goes behind the count. */
const MANY_REASONS: WorklistItem["because"] = [
  { kind: "quiet_days", value: { kind: "days", days: 21 } },
  { kind: "closing_soon" },
  { kind: "no_champion" },
  { kind: "pinned" },
];

describe("a worklist row at list density", () => {
  // THE ORDINAL IS GONE. It is a claim about order, and the ordered list a
  // compact row sits in already carries that claim.
  it("draws no rank", () => {
    const { container } = render(row());

    expect(container.querySelector(".worklist-rank")).toBeNull();
    expect(container.querySelector(".worklist-row-line")).toBeTruthy();
  });

  // THE TITLE IS THE LINK, and it is the ONLY way to the record on the line:
  // two controls opening the same page ask the reader to choose between the
  // same thing twice.
  it("lets the title carry the link and withholds the verb that repeats it", () => {
    const { container } = render(row());

    const title = container.querySelector(".worklist-row-title a");
    expect(title?.getAttribute("href")).toContain(
      "01a05500-0000-7000-8000-0000000000da",
    );
    expect(
      screen.queryByRole("link", { name: en["worklist.verb.open"] }),
    ).toBeNull();
  });

  // The default density keeps that verb. Holding both halves in one test is
  // what makes this a statement about the DENSITY rather than about the row:
  // asserting only the compact half would pass just as well on a row that had
  // lost the verb everywhere.
  it("keeps that verb at the default density", () => {
    render(<WorklistRow item={item()} position={1} owner="" />);

    expect(
      screen.getByRole("link", { name: en["worklist.verb.open"] }),
    ).toBeTruthy();
  });

  // What the row says about itself, as ONE fragment with ONE truncation over
  // it — drawn as separate spans the line would clip whichever happened to be
  // last and leave the reader no way to see what went.
  it("says why the row is here on the line, in one fragment", () => {
    const { container } = render(
      row({ because: MANY_REASONS, consequence: "deal_drifts" }),
    );

    // ONE element, holding the reasons AND what it costs, dot-separated. Two
    // elements is the shape this replaced: the line then clipped whichever was
    // last and the reader had no way to see what went.
    const inline = container.querySelectorAll(".worklist-row-inline");
    expect(inline).toHaveLength(1);
    expect(inline[0].textContent).toContain(
      en["worklist.consequence.deal_drifts"],
    );
    expect(inline[0].textContent).toContain(" · ");
  });

  // NOTHING IS DISCARDED. The count names everything the line could not hold,
  // not just the reasons — a count that promised less than the press delivers
  // is a count a reader learns not to spend a press on.
  it("puts everything the line could not hold behind one press", async () => {
    const user = userEvent.setup();
    render(
      row({
        because: MANY_REASONS,
        detail: "The last four messages were all outbound.",
      }),
    );

    const trigger = screen.getByRole("button", { name: /more/ });
    await user.click(trigger);

    const panel = await screen.findByRole("region");
    expect(
      within(panel).getByText("The last four messages were all outbound."),
    ).toBeTruthy();
  });

  // A popover and not a fold, so Escape closes it and the trigger takes its
  // focus back. A press a keyboard can make and not leave is a trap.
  it("closes on Escape and hands the focus back", async () => {
    const user = userEvent.setup();
    render(row({ because: MANY_REASONS }));

    const trigger = screen.getByRole("button", { name: /more/ });
    await user.click(trigger);
    expect(await screen.findByRole("region")).toBeTruthy();

    await user.keyboard("{Escape}");

    expect(screen.queryByRole("region")).toBeNull();
    expect(trigger).toHaveFocus();
  });

  // A row with nothing beside its name draws neither the fragment nor the
  // count. A line of furniture on every quiet row is what a reader learns to
  // look past, and then they look past the rows that have something to say.
  it("draws neither the fragment nor the count when there is nothing to say", () => {
    const { container } = render(row());

    expect(container.querySelector(".worklist-row-inline")).toBeNull();
    expect(screen.queryByRole("button", { name: /more/ })).toBeNull();
  });

  // The reader's own mark stays a glyph on the trailing edge at this density:
  // the pin already IS the verb, and it is the one control on the row with no
  // word to lose.
  it("keeps the pin as a named glyph", () => {
    render(row());

    expect(
      screen.getByRole("button", { name: en["worklist.verb.pin"] }),
    ).toBeTruthy();
  });
});
