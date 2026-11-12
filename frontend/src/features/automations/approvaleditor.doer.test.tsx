/** @vitest-environment happy-dom */

import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import { editableStrings, StagedEditor } from "./approvaleditor";

vi.mock("../../components/assigneepicker", () => ({
  useAssignableUserOptions: () => ({
    options: [{ value: "seat-priya", label: "Priya Raman" }],
    roster: { data: [], isError: false },
  }),
}));

afterEach(cleanup);

it("offers who does an accepted promise, kept with the reader by default", async () => {
  const fields = editableStrings("commitment_task", {
    summary: "Send the pricing",
    due_date: "2026-11-20",
    doer_id: "",
  });
  const onChange = vi.fn();
  render(
    <LocaleProvider initial="en">
      <StagedEditor
        fields={fields}
        draft={{
          summary: "Send the pricing",
          due_date: "2026-11-20",
          doer_id: "",
        }}
        onChange={onChange}
        pending={false}
        onApprove={() => undefined}
        onCancel={() => undefined}
      />
    </LocaleProvider>,
  );
  const picker = screen.getByRole("combobox", { name: "Who does this" });
  expect(picker).toHaveTextContent("Me");
  await userEvent.click(picker);
  await userEvent.click(screen.getByRole("option", { name: "Priya Raman" }));
  expect(onChange).toHaveBeenCalledWith("doer_id", "seat-priya");
});

it("offers a follow-up's words, day and doer, never its deal or evidence", () => {
  const fields = editableStrings("deal_follow_up", {
    deal_id: "0192f0a0-0000-7000-8000-000000000001",
    subject: "Follow up",
    body: "Ask about the pilot",
    due_date: "2026-11-20",
    evidence_kind: "call",
    doer_id: "",
  });
  expect(fields.map((field) => field.field)).toEqual([
    "subject",
    "body",
    "due_date",
    "doer_id",
  ]);
});

it("offers no doer on a card staged before the doer existed", () => {
  const fields = editableStrings("commitment_task", {
    summary: "Send the pricing",
    due_date: "2026-11-20",
  });
  expect(fields.map((field) => field.field)).not.toContain("doer_id");
});
