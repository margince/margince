/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { type CreateField, CreateRecordModal } from "./create";

afterEach(cleanup);

const RULE = "Use lowercase letters only.";
const fields: CreateField[] = [
  {
    key: "code",
    labelText: "Code",
    type: "text",
    hint: "Short name.",
    validate: (value) => (/^[a-z]+$/.test(value) ? undefined : RULE),
  },
];

function open(onSubmit = vi.fn()) {
  render(
    <LocaleProvider initial="en">
      <CreateRecordModal
        open
        onClose={() => {}}
        title="New thing"
        fields={fields}
        pending={false}
        error={null}
        onSubmit={onSubmit}
      />
    </LocaleProvider>,
  );
  return { onSubmit, input: screen.getByRole("textbox", { name: /Code/ }) };
}

describe("a field's own validate on an untouched form", () => {
  it("marks nothing red when the form opens", () => {
    const { input } = open();
    expect(input).not.toHaveAttribute("aria-invalid");
    expect(screen.queryByText(RULE)).toBeNull();
  });

  it("says the refusal once the reader types something invalid", () => {
    const { input } = open();
    fireEvent.change(input, { target: { value: "1abc" } });
    expect(input).toHaveAttribute("aria-invalid", "true");
    expect(screen.getAllByText(RULE)).toHaveLength(1);
  });

  it("says the refusal when the reader saves untouched, and sends nothing", () => {
    const { input, onSubmit } = open();
    fireEvent.submit(document.querySelector("form") as HTMLFormElement);
    expect(input).toHaveAttribute("aria-invalid", "true");
    expect(onSubmit).not.toHaveBeenCalled();
  });
});
