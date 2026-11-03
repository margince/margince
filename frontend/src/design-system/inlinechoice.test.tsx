/** @vitest-environment happy-dom */
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import "@testing-library/jest-dom/vitest";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { ProblemError } from "../lib/problem";
import { InlineChoice } from "./inlinechoice";
import { pickOption } from "./select-testing";

afterEach(cleanup);

const OPTIONS = [
  { value: "prospect", label: "Prospect" },
  { value: "customer", label: "Customer" },
];

function renderChoice(
  props: Partial<React.ComponentProps<typeof InlineChoice>> = {},
) {
  const onSave = props.onSave ?? vi.fn(async () => {});
  render(
    <LocaleProvider initial="en">
      <InlineChoice
        label="Account lifecycle"
        hideLabel
        value="prospect"
        options={OPTIONS}
        canEdit
        render={(value) =>
          OPTIONS.find((option) => option.value === value)?.label
        }
        onSave={onSave}
        {...props}
      />
    </LocaleProvider>,
  );
  return { onSave };
}

const trigger = () =>
  screen.getByRole("button", { name: "Change Account lifecycle Prospect" });

async function openChooser(user: ReturnType<typeof userEvent.setup>) {
  await user.click(trigger());
  return screen.findByRole("combobox", { name: "Account lifecycle" });
}

describe("a choice edited in a dialog", () => {
  it("shows a reader who may not edit the value as plain text", () => {
    renderChoice({ canEdit: false, readOnlyReason: "Archived." });
    expect(screen.queryByRole("button")).toBeNull();
    expect(screen.getByText("Prospect")).toHaveAttribute("title", "Archived.");
  });

  it("names the trigger by the verb and the value", () => {
    renderChoice();
    expect(trigger()).toHaveAttribute("aria-haspopup", "dialog");
  });

  it("prints the field's name before the value unless the layout does", () => {
    renderChoice({ hideLabel: false });
    expect(screen.getByText(/^Account lifecycle:/)).toContainElement(trigger());
  });

  it("opens a dialog titled by the field, holding the chooser on the stored value", async () => {
    const user = userEvent.setup();
    renderChoice();
    const chooser = await openChooser(user);
    expect(
      screen.getByRole("dialog", { name: "Account lifecycle" }),
    ).toBeInTheDocument();
    expect(chooser).toHaveTextContent("Prospect");
  });

  it("saves on Save, never on the pick", async () => {
    const user = userEvent.setup();
    const { onSave } = renderChoice();
    await pickOption(user, await openChooser(user), "Customer");
    expect(onSave).not.toHaveBeenCalled();
    await user.click(screen.getByRole("button", { name: en["record.save"] }));
    expect(onSave).toHaveBeenCalledWith("customer");
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });

  it("closes without a write when the stored value is saved again", async () => {
    const user = userEvent.setup();
    const { onSave } = renderChoice();
    await openChooser(user);
    await user.click(screen.getByRole("button", { name: en["record.save"] }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(onSave).not.toHaveBeenCalled();
  });

  it("keeps the reader's choice and shows the refusal on the field", async () => {
    const user = userEvent.setup();
    renderChoice({
      onSave: vi.fn(async () => {
        throw new ProblemError({
          code: "permission_denied",
          detail: "company.update: permission denied",
        });
      }),
    });
    const chooser = await openChooser(user);
    await pickOption(user, chooser, "Customer");
    await user.click(screen.getByRole("button", { name: en["record.save"] }));
    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent("do not have permission");
    expect(alert).not.toHaveTextContent("company.update");
    expect(alert).toHaveClass("field-error");
    expect(chooser).toHaveTextContent("Customer");
    expect(chooser).toHaveAttribute("aria-invalid", "true");
  });

  it("tells the caller when the editor opens and closes", async () => {
    const user = userEvent.setup();
    const onEditingChange = vi.fn();
    renderChoice({ onEditingChange });
    await openChooser(user);
    expect(onEditingChange).toHaveBeenLastCalledWith(true);
    await user.click(screen.getByRole("button", { name: en["create.cancel"] }));
    expect(onEditingChange).toHaveBeenLastCalledWith(false);
  });

  it("closes the dialog and tells the caller when the field turns read-only", async () => {
    const user = userEvent.setup();
    const onEditingChange = vi.fn();
    const choice = (canEdit: boolean) => (
      <LocaleProvider initial="en">
        <InlineChoice
          label="Account lifecycle"
          value="prospect"
          options={OPTIONS}
          canEdit={canEdit}
          render={(value) => value}
          onEditingChange={onEditingChange}
          onSave={async () => undefined}
        />
      </LocaleProvider>
    );
    const view = render(choice(true));
    await user.click(
      screen.getByRole("button", { name: /^Change Account lifecycle/ }),
    );
    await screen.findByRole("dialog", { name: "Account lifecycle" });
    view.rerender(choice(false));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(onEditingChange).toHaveBeenLastCalledWith(false);
  });

  it("reads an unset choice as unset", () => {
    renderChoice({ value: "" });
    expect(
      screen.getByRole("button", {
        name: `Change Account lifecycle ${en["field.unset"]}`,
      }),
    ).toBeInTheDocument();
  });
});
