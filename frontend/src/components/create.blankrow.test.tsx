/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { fieldCheckSentence } from "../lib/problem";
import type { CreateField } from "./create";
import { EditAction } from "./edit";

// The record forms set noValidate, so the browser no longer holds a row's
// required value. The form does: a blanked address is refused, not deleted.

afterEach(cleanup);

const fields: CreateField[] = [
  { key: "full_name", label: "create.fullName", required: true },
  {
    key: "emails",
    label: "create.email",
    type: "repeatable",
    addLabel: "field.addEmail",
    rowFields: [
      { key: "email", label: "create.email", type: "email", required: true },
    ],
  },
];

it("refuses an existing address blanked on edit and sends nothing", async () => {
  const user = userEvent.setup();
  const update = vi.fn(async () => ({ id: "p1" }));
  render(
    <QueryClientProvider client={new QueryClient()}>
      <LocaleProvider initial="en">
        <EditAction
          label="Edit"
          fields={fields}
          record={{
            id: "p1",
            version: 1,
            full_name: "Dana",
            emails: [{ email: "dana@example.com" }],
          }}
          update={update}
          invalidate="contacts"
          recordKey="contact"
          savedMessage="Saved."
        />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  await user.click(screen.getByTestId("edit-record"));
  const box = screen.getByDisplayValue("dana@example.com");
  await user.clear(box);
  await user.click(screen.getByRole("button", { name: "Save" }));

  expect(
    await screen.findByText(fieldCheckSentence("required", "en", "Email")),
  ).toBeInTheDocument();
  expect(update).not.toHaveBeenCalled();
});
