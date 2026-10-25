// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { DateInput } from "./dateinput";
import { firstRefusedControl, refuseUnreadableField } from "./formvalidity";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

function InlineEditor({
  name,
  onSave,
}: Readonly<{ name: string; onSave: (refused: boolean) => void }>) {
  return (
    <section data-date-scope>
      <DateInput aria-label={name} defaultValue="2026-05-04" />
      <div className="form-actions">
        <button
          type="button"
          onClick={(event) =>
            onSave(refuseUnreadableField(event.currentTarget))
          }
        >
          {`Save ${name}`}
        </button>
      </div>
    </section>
  );
}

it("reads only the marked box around a save outside any form", async () => {
  const onSave = vi.fn();
  render(
    <LocaleProvider initial="en">
      <InlineEditor name="Due" onSave={onSave} />
      <InlineEditor name="Start" onSave={onSave} />
    </LocaleProvider>,
  );
  const user = userEvent.setup();
  const due = screen.getByRole("textbox", { name: "Due" });
  await user.clear(due);
  await user.type(due, "31/02/2027");

  await user.click(screen.getByRole("button", { name: "Save Start" }));
  await user.click(screen.getByRole("button", { name: "Save Due" }));

  expect(onSave.mock.calls).toEqual([[false], [true]]);
});

it("reads every field of the form it is handed", async () => {
  const onSubmit = vi.fn();
  render(
    <LocaleProvider initial="en">
      <form
        onSubmit={(event) => {
          event.preventDefault();
          onSubmit(refuseUnreadableField(event.currentTarget));
        }}
        noValidate
      >
        <fieldset>
          <DateInput aria-label="Signed" />
        </fieldset>
        <button type="submit">Save</button>
      </form>
    </LocaleProvider>,
  );
  const user = userEvent.setup();
  await user.type(screen.getByRole("textbox", { name: "Signed" }), "99/99");

  await user.click(screen.getByRole("button", { name: "Save" }));

  expect(onSubmit).toHaveBeenCalledWith(true);
});

it("shows the field's own refusal, and no browser bubble, for a save before the day is settled", async () => {
  const bubble = vi.spyOn(HTMLInputElement.prototype, "reportValidity");
  const onSubmit = vi.fn();
  render(
    <LocaleProvider initial="en">
      <form
        noValidate
        onKeyDown={(event) => {
          if (event.ctrlKey && event.key === "s") {
            event.currentTarget.requestSubmit();
          }
        }}
        onSubmit={(event) => {
          event.preventDefault();
          onSubmit(refuseUnreadableField(event.currentTarget));
        }}
      >
        <DateInput aria-label="Signed" />
      </form>
    </LocaleProvider>,
  );
  const user = userEvent.setup();
  const signed = screen.getByRole("textbox", { name: "Signed" });
  await user.type(signed, "99/99");
  expect(screen.queryByRole("alert")).toBeNull();

  await user.keyboard("{Control>}s{/Control}");

  expect(onSubmit).toHaveBeenCalledWith(true);
  expect(screen.getAllByRole("alert").map((line) => line.textContent)).toEqual([
    "Enter a date as DD/MM/YYYY.",
  ]);
  expect(document.activeElement).toBe(signed);
  expect(bubble).not.toHaveBeenCalled();
});

it("throws when no form, dialog or marked box holds the save", () => {
  render(
    <LocaleProvider initial="en">
      <div>
        <DateInput aria-label="Due" />
        <button type="button">Save</button>
      </div>
    </LocaleProvider>,
  );

  expect(() =>
    refuseUnreadableField(screen.getByRole("button", { name: "Save" })),
  ).toThrow(/data-date-scope/);
});

it("names the form's first refused control in the order the form holds them", () => {
  render(
    <form aria-label="Order">
      <input aria-label="Plain" />
      <input type="hidden" />
      <input aria-label="Count" type="number" min={5} defaultValue="1" />
      <textarea aria-label="Note" required />
    </form>,
  );
  const form = screen.getByRole<HTMLFormElement>("form", { name: "Order" });
  const count = screen.getByRole<HTMLInputElement>("spinbutton", {
    name: "Count",
  });
  const note = screen.getByRole<HTMLTextAreaElement>("textbox", {
    name: "Note",
  });

  expect(firstRefusedControl(form)).toBe(count);
  count.value = "6";
  expect(firstRefusedControl(form)).toBe(note);
  note.value = "said";
  expect(firstRefusedControl(form)).toBeUndefined();
});
