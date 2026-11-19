/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { pickOption } from "../../design-system/select-testing";
import { ToastProvider, ToastRegion } from "../../design-system/toast";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import { ProblemError } from "../../lib/problem";
import { FieldBuilder } from "./customfields.builder";

afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

const wrap = (ui: React.ReactNode) =>
  render(<LocaleProvider initial="en">{ui}</LocaleProvider>);

function builder(
  overrides: Partial<React.ComponentProps<typeof FieldBuilder>> = {},
  user = userEvent.setup(),
) {
  const onSubmit = vi.fn();
  const onCancel = vi.fn();
  // The real region, not a spy. The builder owes a reader a sentence on screen
  // with no completion mark beside it, and a `vi.fn()` would prove only a call.
  // The kinds an installed extension declared, seeded where the directory reads
  // them so the builder offers the record type without a request.
  const kinds = new QueryClient();
  kinds.setQueryData(["record-kinds"], {
    data: [{ name: "ext:alpha:visit", display_name: "Visit" }],
  });
  wrap(
    <QueryClientProvider client={kinds}>
      <ToastProvider>
        <FieldBuilder
          object="company"
          pending={false}
          error={null}
          onSubmit={onSubmit}
          onCancel={onCancel}
          {...overrides}
        />
        <ToastRegion />
      </ToastProvider>
    </QueryClientProvider>,
  );
  const pickType = (type: string) =>
    pickOption(
      user,
      screen.getByRole("combobox", { name: en["cf.typeLabel"] }),
      type,
    );
  return { user, onSubmit, onCancel, pickType };
}

describe("FieldBuilder", () => {
  it("mirrors the label into the immutable disabled api key", async () => {
    const { user } = builder();
    await user.type(screen.getByLabelText(/Label/i), "Contract end date");
    const key = screen.getByLabelText(/API name/i) as HTMLInputElement;
    expect(key.value).toBe("company.cf_contract_end_date");
    expect(key).toBeDisabled();
  });

  it("shows the pending DDL preview reflecting the type", async () => {
    const { user, pickType } = builder();
    await user.type(screen.getByLabelText(/Label/i), "Contract end date");
    await pickType("Date");
    expect(
      screen.getByText(
        /ALTER company ADD COLUMN cf_contract_end_date \(date\)/,
      ),
    ).toBeInTheDocument();
  });

  it("refuses a structural label and disables Confirm", async () => {
    const { user } = builder();
    await user.type(screen.getByLabelText(/Label/i), "Link to parent account");
    expect(
      screen.getByText(/looks like a new object or relationship/i),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Add field" })).toBeDisabled();
  });

  it("asks for a label on its field and sends nothing", async () => {
    const { user, onSubmit } = builder();
    await user.click(screen.getByRole("button", { name: "Add field" }));
    expect(await screen.findByText("Label is required.")).toBeVisible();
    const label = screen.getByRole("textbox", { name: en["cf.label"] });
    expect(label).toHaveAttribute("aria-invalid", "true");
    await vi.waitFor(() => expect(label).toHaveFocus());
    expect(onSubmit).not.toHaveBeenCalled();
    await user.type(label, "Renewal date");
    expect(screen.queryByText("Label is required.")).toBeNull();
  });

  it("leaves a conflict that is not the label's at the foot", async () => {
    const { user, onSubmit } = builder({
      error: new ProblemError({
        status: 409,
        code: "conflict",
        detail: "The table is busy. Try again.",
      }),
    });
    onSubmit.mockImplementation((_draft, refused: (error: unknown) => void) =>
      refused(new ProblemError({ status: 409, code: "conflict" })),
    );
    const label = screen.getByRole("textbox", { name: en["cf.label"] });
    await user.type(label, "Budget");
    await user.click(screen.getByRole("button", { name: "Add field" }));
    expect(screen.getByText("The table is busy. Try again.")).toBeVisible();
    expect(screen.queryByText(en["cf.labelTaken"])).toBeNull();
    expect(label).not.toHaveAttribute("aria-invalid");
  });

  it("says on the label that the record type already has it", async () => {
    const { user, onSubmit } = builder();
    onSubmit.mockImplementation((_draft, refused: (error: unknown) => void) =>
      refused(
        new ProblemError({
          status: 409,
          code: "custom_field_label_taken",
          detail: 'a custom field labelled "Budget" already exists',
        }),
      ),
    );
    const label = screen.getByRole("textbox", { name: en["cf.label"] });
    await user.type(label, "Budget");
    await user.click(screen.getByRole("button", { name: "Add field" }));
    expect(await screen.findByText(en["cf.labelTaken"])).toBeVisible();
    expect(label).toHaveAttribute("aria-invalid", "true");
    expect(screen.queryByText(/labelled "Budget"/)).toBeNull();
  });

  it("reveals the ISO-4217 input for currency", async () => {
    const { pickType } = builder();
    await pickType("Currency");
    expect(screen.getByLabelText(/Currency code/i)).toBeInTheDocument();
  });

  it("reveals the options editor for picklist and blocks removing the last option", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const view = builder(
      {},
      userEvent.setup({ advanceTimers: vi.advanceTimersByTime }),
    );
    const { user, pickType } = view;
    await pickType("Picklist");
    const removes = screen.getAllByRole("button", { name: /remove option/i });
    // start with one row; removing it is blocked
    await user.click(removes[removes.length - 1]);
    expect(screen.getByRole("status")).toHaveTextContent(
      "A picklist needs at least one option",
    );
    // And marked as a refusal: the completion dot beside it said the opposite
    // of what the sentence says, so the message wears the danger one.
    expect(document.body.querySelector(".toast-dot-success")).toBeNull();
    expect(document.body.querySelector(".toast-dot-danger")).not.toBeNull();
    expect(view.onSubmit).not.toHaveBeenCalled();
  });

  it("submits a well-formed draft on Confirm", async () => {
    const { user, onSubmit, pickType } = builder();
    await user.type(screen.getByLabelText(/Label/i), "Renewal date");
    await pickType("Date");
    await user.click(screen.getByRole("button", { name: "Add field" }));
    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({
        object: "company",
        label: "Renewal date",
        type: "date",
      }),
      expect.any(Function),
    );
  });

  it("submits a record field with the one kind it points at", async () => {
    const { user, onSubmit, pickType } = builder();
    await user.type(screen.getByLabelText(/Label/i), "Last visit");
    await pickType(en["cf.type.record"]);
    const add = screen.getByRole("button", { name: "Add field" });
    await user.click(add);
    expect(
      await screen.findByText(`${en["cf.recordKind"]} is required.`),
    ).toBeVisible();
    expect(onSubmit).not.toHaveBeenCalled();
    await pickOption(
      user,
      screen.getByRole("combobox", { name: en["cf.recordKind"] }),
      "Visit",
    );
    await user.click(add);
    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({ type: "record", options: ["ext:alpha:visit"] }),
      expect.any(Function),
    );
  });

  it("submits a field kept off create forms when the reader unticks it", async () => {
    const { user, onSubmit } = builder();
    await user.type(screen.getByLabelText(/Label/i), "Previous owner");
    await user.click(
      screen.getByRole("checkbox", { name: en["cf.showInCreate"] }),
    );
    await user.click(screen.getByRole("button", { name: "Add field" }));
    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({ label: "Previous owner", showInCreate: false }),
      expect.any(Function),
    );
  });

  it("leaves through Cancel without submitting a draft", async () => {
    const { user, onCancel, onSubmit } = builder();
    await user.type(screen.getByLabelText(/Label/i), "Renewal date");
    await user.click(screen.getByRole("button", { name: /^Cancel$/i }));
    expect(onCancel).toHaveBeenCalled();
    expect(onSubmit).not.toHaveBeenCalled();
  });

  it("asks for a real option before a picklist is sent", async () => {
    const { user, onSubmit, pickType } = builder();
    await user.type(screen.getByLabelText(/Label/i), "Deal source");
    await pickType("Picklist");
    await user.click(screen.getByRole("button", { name: "Add field" }));
    expect(await screen.findByRole("alert")).toHaveTextContent(
      en["cf.lastOptionBlocked"],
    );
    const option = screen.getByLabelText(/Option label/i);
    expect(option).toHaveAttribute("aria-invalid", "true");
    await vi.waitFor(() => expect(option).toHaveFocus());
    expect(onSubmit).not.toHaveBeenCalled();
    await user.type(screen.getByLabelText(/Option label/i), "Referral");
    expect(screen.queryByRole("alert")).toBeNull();
  });
});
