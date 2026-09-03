/** @vitest-environment jsdom */
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { type ResolveAnswer, ResolveSheet } from "./resolvesheet";

afterEach(cleanup);

const labels = {
  title: "Answer this check",
  outcomeLegend: "What kind of answer is this?",
  outcomes: [
    { value: "fixed_record", label: "I corrected the record" },
    { value: "added_evidence", label: "I added the evidence" },
    { value: "value_correct", label: "The value is correct" },
    { value: "not_relevant", label: "Not relevant to this deal" },
    { value: "remind_later", label: "Not now" },
    { value: "reassign", label: "Somebody else's" },
  ],
  reason: "Why",
  reasonHelp:
    "The next person to see the number is owed the reason it is not flagged.",
  remindAt: "Bring it back on",
  expiresAt: "Stops holding on",
  expiresHelp: "At most 90 days.",
  cancel: "Cancel",
  submit: "Save",
} as const;

function open(onSubmit: (answer: ResolveAnswer) => void = vi.fn()) {
  render(
    <ResolveSheet
      open
      labels={labels}
      onSubmit={onSubmit}
      onClose={() => {}}
    />,
  );
  return onSubmit;
}

describe("ResolveSheet", () => {
  // The required fields MIRROR the server's refusals. A sheet that let
  // somebody submit what the server rejects spends their attention on a round
  // trip that was always going to fail.
  it("asks for a reason only where the answer hides the finding", async () => {
    open();
    const user = userEvent.setup();

    // An answer that hides nothing needs no reason — demanding one would make
    // the common answers tedious enough that people stop giving them.
    await user.click(screen.getByLabelText("I corrected the record"));
    expect(screen.queryByLabelText("Why")).toBeNull();

    await user.click(screen.getByLabelText("The value is correct"));
    expect(screen.getByLabelText("Why")).toBeTruthy();

    await user.click(screen.getByLabelText("Not relevant to this deal"));
    expect(screen.getByLabelText("Why")).toBeTruthy();
  });

  // "Not now" is an answer about when. Without a date it is a dismissal
  // wearing a different word.
  it("asks a deferral when it comes back", async () => {
    open();
    const user = userEvent.setup();

    await user.click(screen.getByLabelText("Not now"));
    expect(screen.getByLabelText("Bring it back on")).toBeTruthy();
    // A deferral is not a suppression: no reason, no expiry.
    expect(screen.queryByLabelText("Why")).toBeNull();
    expect(screen.queryByLabelText("Stops holding on")).toBeNull();
  });

  // Nothing chosen is not an answer.
  it("cannot be submitted before an outcome is chosen", async () => {
    open();
    expect(screen.getByRole("button", { name: "Save" })).toHaveProperty(
      "disabled",
      true,
    );
  });

  it("cannot be submitted while a required field is empty", async () => {
    open();
    const user = userEvent.setup();
    const save = screen.getByRole("button", { name: "Save" });

    await user.click(screen.getByLabelText("The value is correct"));
    expect(save).toHaveProperty("disabled", true);

    await user.type(
      screen.getByLabelText("Why"),
      "Checked against the signed order",
    );
    expect(save).toHaveProperty("disabled", false);
  });

  // Only the fields the outcome takes. Sending an expiry with a deferral would
  // record a suppression nobody chose.
  it("submits exactly the fields the chosen outcome takes", async () => {
    const submitted: ResolveAnswer[] = [];
    open((answer: ResolveAnswer) => submitted.push(answer));
    const user = userEvent.setup();

    await user.click(screen.getByLabelText("Not now"));
    await user.type(screen.getByLabelText("Bring it back on"), "2026-06-30");
    await user.click(screen.getByRole("button", { name: "Save" }));

    expect(submitted).toHaveLength(1);
    expect(submitted[0].outcome).toBe("remind_later");
    expect(submitted[0].remindAt).toBe("2026-06-30");
    expect(submitted[0].reason).toBeUndefined();
    expect(submitted[0].expiresAt).toBeUndefined();
  });

  // The whole sheet is reachable without a pointer: a manager answering ten
  // findings before a call is doing it from the keyboard.
  it("completes from the keyboard alone", async () => {
    const submitted: ResolveAnswer[] = [];
    open((answer: ResolveAnswer) => submitted.push(answer));
    const user = userEvent.setup();

    await user.click(screen.getByLabelText("The value is correct"));
    await user.keyboard("{Tab}");
    await user.keyboard("Checked against the signed order");
    const save = screen.getByRole("button", { name: "Save" });
    save.focus();
    await user.keyboard("{Enter}");

    expect(submitted).toHaveLength(1);
    expect(submitted[0].reason).toBe("Checked against the signed order");
  });
});
