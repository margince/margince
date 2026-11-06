/** @vitest-environment happy-dom */
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import "@testing-library/jest-dom/vitest";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { ProblemError } from "../lib/problem";
import { InlineText } from "./inlinetext";

afterEach(cleanup);

function renderText(
  props: Partial<React.ComponentProps<typeof InlineText>> = {},
) {
  const onSave = props.onSave ?? vi.fn(async (_next: string) => {});
  render(
    <LocaleProvider initial="en">
      <InlineText
        label="Industry"
        value="Automotive"
        placeholder="Add industry"
        canEdit
        onSave={onSave}
        {...props}
      />
    </LocaleProvider>,
  );
  return { onSave };
}

const save = () => screen.getByRole("button", { name: en["record.save"] });

describe("a text value edited in a dialog", () => {
  it("opens a dialog titled by the field, with the stored value in focus", async () => {
    const user = userEvent.setup();
    renderText();
    await user.click(
      screen.getByRole("button", { name: "Change Industry Automotive" }),
    );
    expect(
      await screen.findByRole("dialog", { name: "Industry" }),
    ).toBeInTheDocument();
    const box = screen.getByRole("textbox", { name: "Industry" });
    expect(box).toHaveValue("Automotive");
    await waitFor(() => expect(box).toHaveFocus());
  });

  it("saves a line trimmed, and closes without a write when it trims to the stored value", async () => {
    const user = userEvent.setup();
    const { onSave } = renderText();
    await user.click(screen.getByRole("button", { name: /Change Industry/ }));
    await user.type(screen.getByRole("textbox"), "  {Enter}");
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(onSave).not.toHaveBeenCalled();

    await user.click(screen.getByRole("button", { name: /Change Industry/ }));
    const box = screen.getByRole("textbox");
    await user.clear(box);
    await user.type(box, " Aerospace ");
    await user.click(save());
    expect(onSave).toHaveBeenCalledWith("Aerospace");
  });

  it("keeps a paragraph as typed and saves it on Ctrl+Enter", async () => {
    const user = userEvent.setup();
    const { onSave } = renderText({ multiline: true, value: "One" });
    await user.click(screen.getByRole("button", { name: /Change Industry/ }));
    await user.type(screen.getByRole("textbox"), "{Enter}Two ");
    expect(onSave).not.toHaveBeenCalled();
    await user.keyboard("{Control>}{Enter}{/Control}");
    expect(onSave).toHaveBeenCalledWith("One\nTwo ");
  });

  it("edits a number at the step the caller allows", async () => {
    const user = userEvent.setup();
    renderText({ type: "number", step: "any", value: "14.6" });
    await user.click(screen.getByRole("button", { name: /Change Industry/ }));
    const box = screen.getByRole("spinbutton", { name: "Industry" });
    expect(box).toHaveAttribute("step", "any");
    expect(box).toHaveValue(14.6);
  });

  it("saves a day picked in the calendar from Save", async () => {
    const user = userEvent.setup();
    const { onSave } = renderText({
      label: "Expected close",
      type: "date",
      value: "2026-03-01",
      display: "1 Mar 2026",
    });
    await user.click(
      screen.getByRole("button", { name: "Change Expected close 1 Mar 2026" }),
    );
    await user.click(screen.getByRole("button", { name: /^Choose date/ }));
    await user.click(
      await screen.findByRole("button", { name: "Tuesday, 10 March 2026" }),
    );
    expect(onSave).not.toHaveBeenCalled();
    await user.click(save());
    expect(onSave).toHaveBeenCalledWith("2026-03-10");
  });

  it("keeps the typed text and shows the refusal on the field", async () => {
    const user = userEvent.setup();
    renderText({
      onSave: vi.fn(async () => {
        throw new ProblemError({
          code: "permission_denied",
          detail: "contact.update: permission denied",
        });
      }),
    });
    await user.click(screen.getByRole("button", { name: /Change Industry/ }));
    const box = screen.getByRole("textbox");
    await user.clear(box);
    await user.type(box, "Aerospace{Enter}");
    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent("do not have permission");
    expect(alert).not.toHaveTextContent("contact.update");
    expect(box).toHaveValue("Aerospace");
    expect(box).toHaveAttribute("aria-invalid", "true");
  });

  it("tells the caller when the editor opens and closes", async () => {
    const user = userEvent.setup();
    const onEditingChange = vi.fn();
    const onDirtyChange = vi.fn();
    renderText({ onEditingChange, onDirtyChange });
    await user.click(screen.getByRole("button", { name: /Change Industry/ }));
    expect(onEditingChange).toHaveBeenLastCalledWith(true);
    await user.type(screen.getByRole("textbox"), "s");
    expect(onDirtyChange).toHaveBeenLastCalledWith(true);
    await user.click(screen.getByRole("button", { name: en["create.cancel"] }));
    expect(onEditingChange).toHaveBeenLastCalledWith(false);
    expect(onDirtyChange).toHaveBeenLastCalledWith(false);
  });
});

describe("a field that changes under its open dialog", () => {
  it("closes the dialog and tells the caller when the field turns read-only", async () => {
    const user = userEvent.setup();
    const onEditingChange = vi.fn();
    const field = (canEdit: boolean) => (
      <LocaleProvider initial="en">
        <InlineText
          label="Industry"
          value="Automotive"
          placeholder="Add industry"
          canEdit={canEdit}
          onEditingChange={onEditingChange}
          onSave={async () => undefined}
        />
      </LocaleProvider>
    );
    const view = render(field(true));
    await user.click(screen.getByRole("button", { name: /Change Industry/ }));
    await screen.findByRole("dialog", { name: "Industry" });
    view.rerender(field(false));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(onEditingChange).toHaveBeenLastCalledWith(false);
    view.rerender(field(true));
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("gives focus to the mark when the first value saved turns the pill into a link", async () => {
    const user = userEvent.setup();
    function Email() {
      const [value, setValue] = useState("");
      return (
        <InlineText
          label="Email"
          value={value}
          link={value ? <a href={`mailto:${value}`}>{value}</a> : undefined}
          placeholder="Add email"
          canEdit
          onSave={async (next) => setValue(next)}
        />
      );
    }
    render(
      <LocaleProvider initial="en">
        <Email />
      </LocaleProvider>,
    );
    await user.click(
      screen.getByRole("button", { name: "Change Email Add email" }),
    );
    await user.type(
      await screen.findByRole("textbox", { name: "Email" }),
      "ana@example.com{Enter}",
    );
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(screen.getByRole("link", { name: "ana@example.com" })).toBeVisible();
    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: "Change Email" }),
      ).toHaveFocus(),
    );
  });
});

describe("the value at rest", () => {
  it("draws the reading for a reader who may not edit, with no control", () => {
    renderText({
      canEdit: false,
      value: "48000",
      display: "€48,000.00",
      readOnlyReason: "Archived.",
    });
    expect(screen.queryByRole("button")).toBeNull();
    expect(screen.getByText("€48,000.00")).toHaveAttribute(
      "title",
      "Archived.",
    );
  });

  it("stands a suggested value in for an empty one, muted, for editor and reader alike", () => {
    renderText({ value: "", suggested: "Head of Sales" });
    expect(screen.getByText("Head of Sales")).toHaveClass(
      "inlinetext-suggested",
    );
    cleanup();
    renderText({ value: "", suggested: "Head of Sales", canEdit: false });
    expect(screen.getByText("Head of Sales")).toHaveClass(
      "inlinetext-suggested",
    );
  });

  it("invites an empty field by its placeholder, and reads unset to a viewer", () => {
    renderText({ value: "" });
    expect(
      screen.getByRole("button", { name: "Change Industry Add industry" }),
    ).toBeInTheDocument();
    cleanup();
    renderText({ value: "", canEdit: false });
    expect(screen.getByText(en["field.unset"])).toBeInTheDocument();
    expect(screen.queryByText("Add industry")).toBeNull();
  });

  it("keeps a linked value's own action, and edits from the bubble", async () => {
    const user = userEvent.setup();
    renderText({
      label: "Email",
      value: "ana@example.test",
      type: "email",
      link: <a href="mailto:ana@example.test">ana@example.test</a>,
    });
    expect(
      screen.getByRole("link", { name: "ana@example.test" }),
    ).toHaveAttribute("href", "mailto:ana@example.test");
    await user.click(screen.getByRole("button", { name: "Change Email" }));
    expect(
      await screen.findByRole("textbox", { name: "Email" }),
    ).toHaveAttribute("type", "email");
  });

  it("draws a linked value bare for a reader who may not edit", () => {
    renderText({
      canEdit: false,
      link: <a href="https://example.test">example.test</a>,
    });
    expect(screen.getByRole("link")).toBeInTheDocument();
    expect(screen.queryByRole("button")).toBeNull();
  });
});
