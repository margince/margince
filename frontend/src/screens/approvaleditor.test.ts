import { describe, expect, it } from "vitest";
import { editableSeed, editableStrings } from "./approvaleditor";

// What the inline editor OFFERS. A kind that declares its editable fields is
// saying what a person may change; everything else in the payload is what the
// question is about, and offering it as a text box invites an edit the server
// will refuse.

describe("a kind that declares what may be edited", () => {
  const closeDate = {
    deal_id: "01a03781-912c-774e-a74e-7cdceca98829",
    expected_close_date: "2026-09-27",
    previous_close_date: "2026-09-27",
    basis: "This deal has gone quiet.",
  };

  it("offers the proposed date and nothing else", () => {
    const fields = editableStrings("close_date_correction", closeDate);

    expect(fields.map((f) => f.field)).toEqual(["expected_close_date"]);
  });

  it("never offers an identifier as something to retype", () => {
    const fields = editableStrings("close_date_correction", closeDate);

    expect(fields.map((f) => f.field)).not.toContain("deal_id");
  });

  it("never offers the server's own reason as the reader's to rewrite", () => {
    const fields = editableStrings("close_date_correction", closeDate);

    expect(fields.map((f) => f.field)).not.toContain("basis");
  });

  it("gives a date the calendar control rather than a text box", () => {
    const [date] = editableStrings("close_date_correction", closeDate);

    expect(date.as).toBe("date");
  });

  // A declared field the payload does not carry is skipped: rendering it empty
  // would ADD the path on approve, which the server reads as a retargeted edit.
  it("skips a declared field the payload does not carry", () => {
    const fields = editableStrings("close_date_correction", {
      deal_id: "01a03781-912c-774e-a74e-7cdceca98829",
    });

    expect(fields).toEqual([]);
  });
});

describe("a kind that declares nothing", () => {
  // The raw-args kinds carry an agent tool's arguments, and inventing labels
  // for a bag of unknown keys would be guessing — so they keep the generic
  // editor rather than losing their fields to a declaration nobody wrote.
  it("still offers every string in the payload", () => {
    const fields = editableStrings("some_agent_tool_call", {
      note: "hello",
      subject: "there",
      count: 3,
    });

    expect(fields.map((f) => f.field)).toEqual(["note", "subject"]);
    expect(fields.every((f) => f.as === "text")).toBe(true);
  });

  // `kind` is a wire string. A value naming an Object prototype member would
  // otherwise find a function, pass the truthy check, and crash the queue.
  it("does not mistake a prototype member for a declaration", () => {
    const fields = editableStrings("constructor", { note: "hello" });

    expect(fields.map((f) => f.field)).toEqual(["note"]);
  });
});

describe("what the editor starts with", () => {
  const dateField = { field: "expected_close_date", as: "date" } as const;
  const textField = { field: "subject", as: "text" } as const;

  it("keeps a wire date, which is what the control accepts", () => {
    expect(editableSeed(dateField, "2026-09-27")).toBe("2026-09-27");
  });

  // The control discards a value it cannot parse. Seeding the raw string would
  // show an empty box while the original still went out on approve — the editor
  // displaying one thing and submitting another.
  it("blanks a date the control cannot show, so shown and sent agree", () => {
    expect(editableSeed(dateField, "27/09/2026")).toBe("");
  });

  // A shape check alone would pass this: it is spelled YYYY-MM-DD and there is
  // no 30th of February. The control blanks it, so the seed must too, or the
  // reader sees an empty box while the impossible date rides out on approve.
  it("blanks a well-shaped date that does not exist", () => {
    expect(editableSeed(dateField, "2026-02-30")).toBe("");
    expect(editableSeed(dateField, "2026-13-01")).toBe("");
  });

  it("keeps a leap day in a year that has one", () => {
    expect(editableSeed(dateField, "2028-02-29")).toBe("2028-02-29");
    expect(editableSeed(dateField, "2026-02-29")).toBe("");
  });

  it("leaves a text field's value alone whatever it looks like", () => {
    expect(editableSeed(textField, "27/09/2026")).toBe("27/09/2026");
  });

  // A non-string here means a payload that does not match its kind's
  // declaration; inviting somebody to edit "[object Object]" is worse than
  // an empty box.
  it("blanks a value that is not a string rather than stringifying it", () => {
    expect(editableSeed(textField, { nested: true })).toBe("");
    expect(editableSeed(dateField, undefined)).toBe("");
  });
});
