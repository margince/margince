import { describe, expect, it } from "vitest";
import { conditionOf, eyebrowKeyFor } from "./worklist.eyebrow";
import type { WorklistItem } from "./worklist.queries";

function row(over: Partial<WorklistItem>): WorklistItem {
  return {
    id: "i1",
    source: "deal_at_risk",
    category: "deals_at_risk",
    actions: [],
    ...over,
  } as WorklistItem;
}

describe("the word above a queue row", () => {
  // Every overnight row sits in deals_at_risk, because that is where the
  // worklist folds it against the sibling risk row for the same deal. The night
  // knows why it picked each one, and a deal it ranked as winnable announced as
  // "Deal at risk" is the label a reader learns to distrust.
  it("names the night's own reason on a brief row", () => {
    expect(
      eyebrowKeyFor(row({ source: "brief_item", kind: "opportunity" })),
    ).toBe("worklist.signal.opportunity");
    expect(
      eyebrowKeyFor(row({ source: "brief_item", kind: "closing_soon" })),
    ).toBe("worklist.signal.closing_soon");
  });

  it("keeps the category on every other row", () => {
    expect(eyebrowKeyFor(row({ category: "customer_waiting" }))).toBe(
      "worklist.category.customer_waiting",
    );
  });

  // A signal from a newer build must not reach `t` with a key nothing
  // translates, which renders the key itself on the page.
  it("falls back to the category for a signal it does not know", () => {
    expect(
      eyebrowKeyFor(row({ source: "brief_item", kind: "invented_later" })),
    ).toBe("worklist.category.deals_at_risk");
  });

  it("falls back when the night named nothing", () => {
    expect(eyebrowKeyFor(row({ source: "brief_item" }))).toBe(
      "worklist.category.deals_at_risk",
    );
  });

  // SIX CATEGORIES NAME A KIND OF WORK AND THE SEVENTH NAMED THE SOFTWARE.
  // `system` is the queue's catch-all for the product reporting on itself — a
  // mailbox that stopped, a failed automation, a bounced message, a privacy
  // deadline — so a column of different broken things all read "System" and a
  // reader running down it learned nothing about any of them.
  it("names the broken thing on a system row", () => {
    expect(
      conditionOf(
        row({ category: "system", cause_label: "Gmail · lena@acme.example" }),
      ),
    ).toBe("Gmail · lena@acme.example");
  });

  // ABSENT IS A REAL ANSWER. A lane whose candidate is a vocabulary rather than
  // a record mints no label, and an empty eyebrow would be worse than a vague
  // one — so the caller keeps the category word.
  it("says nothing where a system row named no cause", () => {
    expect(conditionOf(row({ category: "system" }))).toBeNull();
  });

  // And never on the six that already name their work: a deal at risk whose
  // lane happened to mint a cause is still a deal at risk, and swapping its
  // eyebrow for a condition's name would lose the grouping the column exists
  // to show.
  it("leaves every other category its own word", () => {
    expect(
      conditionOf(row({ category: "deals_at_risk", cause_label: "a cause" })),
    ).toBeNull();
    expect(
      conditionOf(row({ category: "decisions", cause_label: "a cause" })),
    ).toBeNull();
  });

  // A FOLDED INCIDENT KEEPS THE WORD TOO. The fold mints a fresh row and puts
  // the group's name in `batch.label`, never in `cause_label` — so the helper
  // finds nothing and the category word stands. Asserted rather than left to
  // chance: a later fold that started copying the label would change what this
  // column says on a whole class of rows, and the group's headline already
  // reads "{cause} failed {count} times".
  it("keeps the category word on a folded incident", () => {
    expect(
      conditionOf(
        row({
          category: "system",
          source: "batch",
          batch: { key: "system_incident", count: 3, label: "Nightly sync" },
        } as Partial<WorklistItem>),
      ),
    ).toBeNull();
  });
});
