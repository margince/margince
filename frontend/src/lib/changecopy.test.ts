import { describe, expect, it } from "vitest";
import { en, type MessageKey } from "../i18n/en";
import { undoResultText } from "./changecopy";

// The English catalogue with its {name} slots filled, as useT fills them.
const t = (key: MessageKey, params?: Record<string, string>): string =>
  Object.entries(params ?? {}).reduce(
    (text, [name, value]) => text.replace(`{${name}}`, value),
    `${en[key]}`,
  );

const result = (undone: string[], kept: { kind: string }[] = []) => ({
  deal_id: "d",
  outcome: kept.length === 0 ? ("undone" as const) : ("partly_undone" as const),
  undone,
  kept: kept.map((k) => ({
    ...k,
    audit_log_id: "a",
    reason: "changed_since" as const,
  })),
});

describe("the words for an Undo of Margince's changes", () => {
  it("keeps the close's own sentence, which asks for a note", () => {
    expect(undoResultText(result(["close_lost", "deal_amount"]), t)).toBe(
      en["worklist.handled.closeLostUndone"],
    );
  });

  it("names every field Margince will now leave alone", () => {
    expect(undoResultText(result(["deal_amount", "deal_probability"]), t)).toBe(
      "Undone. Margince will not change the amount and probability again until something new happens on this deal.",
    );
  });

  it("names the field a later edit kept", () => {
    expect(
      undoResultText(result(["deal_close_date"], [{ kind: "deal_amount" }]), t),
    ).toBe(
      "Undone, except the amount, which was changed since Margince set it.",
    );
  });
});

describe("an Undo of a change its owner kept", () => {
  it("says the change was accepted rather than that the deal moved", () => {
    expect(
      undoResultText(
        {
          deal_id: "d",
          outcome: "refused",
          undone: [],
          kept: [
            { audit_log_id: "a", kind: "deal_amount", reason: "accepted" },
          ],
        },
        t,
      ),
    ).toBe("You already accepted this change, so it stays.");
  });
});
