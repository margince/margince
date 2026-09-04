/** @vitest-environment jsdom */
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import type { ReviewRow } from "./company-review-state";
import { ProfileDigest, type ProfileDigestRead } from "./profile-digest";

afterEach(cleanup);

function row(
  field: ReviewRow["field"],
  label: string,
  value: string,
  multiline = false,
): ReviewRow {
  return {
    field,
    label,
    value,
    multiline,
    state: value === "" ? "empty" : "typed",
    evidence: null,
    confidence: null,
    emptyHintKey: "ob.conv.triage.emptyHint",
    omissionReasonKey: null,
  };
}

const ROWS: ReviewRow[] = [
  row("display_name", "Company name", "Acme"),
  row("offer_summary", "What you sell", "Inventory software", true),
  row("usp", "Why they choose you", ""),
];

const READ: ProfileDigestRead = {
  root_url: "https://acme.example",
  pages: [],
  facts: [],
  people: [],
};

function mount(ui: React.ReactNode) {
  return render(<LocaleProvider initial="en">{ui}</LocaleProvider>);
}

describe("the profile digest", () => {
  // The door to the whole record sits on the record: a reader looking at the
  // companion finds it there, not in the deck's tray two panels away.
  it("carries the door to the whole record in the companion's head", async () => {
    const onReadWhole = vi.fn();
    mount(<ProfileDigest rows={ROWS} onReadWhole={onReadWhole} />);
    await userEvent.click(
      screen.getByRole("button", { name: "Read the whole profile" }),
    );
    expect(onReadWhole).toHaveBeenCalledTimes(1);
  });

  // A written line is edited where it stands: pressing the value opens the
  // field, and leaving it hands the correction to the draft the deck shares.
  it("corrects a written line in place and hands the change to the draft", async () => {
    const onField = vi.fn();
    mount(<ProfileDigest rows={ROWS} read={READ} onField={onField} />);
    await userEvent.click(
      screen.getByRole("button", { name: "Edit Company name" }),
    );
    const control = screen.getByRole("textbox", { name: "Company name" });
    await userEvent.clear(control);
    await userEvent.type(control, "Acme GmbH{Enter}");
    expect(onField).toHaveBeenCalledWith("display_name", "Acme GmbH");
    // Committed: the line is prose again, reading what was typed is the
    // caller's job through `rows`, and the control is gone.
    expect(screen.queryByRole("textbox", { name: "Company name" })).toBeNull();
  });

  // Escape is the way out without a change: the value the reader started from
  // comes back and nothing reaches the draft.
  it("puts the old value back on Escape", async () => {
    const onField = vi.fn();
    mount(<ProfileDigest rows={ROWS} read={READ} onField={onField} />);
    await userEvent.click(
      screen.getByRole("button", { name: "Edit What you sell" }),
    );
    const control = screen.getByRole("textbox", { name: "What you sell" });
    expect(control.tagName).toBe("TEXTAREA");
    await userEvent.type(control, " for retailers{Escape}");
    expect(onField).not.toHaveBeenCalled();
    expect(
      screen.getByRole("button", { name: "Edit What you sell" }),
    ).toBeTruthy();
  });

  // Without `onField` the article is an article: nothing on it is a control,
  // which is what the companion beside the deck relies on.
  it("draws plain prose when nothing may be edited", () => {
    mount(<ProfileDigest rows={ROWS} read={READ} />);
    expect(screen.queryByRole("button", { name: /^Edit / })).toBeNull();
  });
});
