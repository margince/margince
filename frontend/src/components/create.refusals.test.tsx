/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it } from "vitest";
import { LocaleProvider } from "../i18n";
import { throwProblem } from "../lib/problem";
import { CreateAction } from "./create";
import type { CreateField } from "./create.fields";

// A 422 names the wire field. The form puts the refusal beside the input that
// field belongs to, in the reader's language, and keeps the foot of the form
// for refusals that have no input to stand beside.

afterEach(() => {
  cleanup();
});

function render(ui: ReactNode, locale: "en" | "de" = "en") {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial={locale}>{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
}

const fields: CreateField[] = [
  { key: "name", label: "create.dealName", required: true },
  {
    key: "amount",
    label: "create.amount",
    type: "number",
    step: "any",
    min: "0",
    wireField: "amount_minor",
  },
];

function refusal(field: string, code: string) {
  return {
    code: "validation_error",
    detail: `${field} refused`,
    details: { errors: [{ field, code, message: `${field} refused` }] },
  };
}

async function submitWith(problem: unknown, locale: "en" | "de" = "en") {
  render(
    <CreateAction
      label="New deal"
      fields={fields}
      create={() => throwProblem(problem)}
      invalidate="deals"
      screen="deals"
    />,
    locale,
  );
  await userEvent.click(screen.getByText("New deal"));
  await userEvent.type(screen.getByLabelText(/Deal.name/i), "Probe");
  await userEvent.click(
    screen.getByRole("button", { name: /Create|Erstellen/ }),
  );
}

describe("CreateAction field refusals", () => {
  it("declares the lowest amount the browser accepts", async () => {
    render(
      <CreateAction
        label="New deal"
        fields={fields}
        create={() => Promise.resolve()}
        invalidate="deals"
        screen="deals"
      />,
    );
    await userEvent.click(screen.getByText("New deal"));
    expect(screen.getByLabelText("Value").getAttribute("min")).toBe("0");
  });

  it("shows a negative amount beside the amount, not at the foot", async () => {
    await submitWith(refusal("amount_minor", "amount_negative"));
    await waitFor(() =>
      expect(screen.getByText("Enter an amount of zero or more.")).toBeTruthy(),
    );
    expect(screen.queryByText("amount_minor refused")).toBeNull();
  });

  it("says it in the reader's language", async () => {
    await submitWith(refusal("amount_minor", "amount_negative"), "de");
    await waitFor(() =>
      expect(
        screen.getByText("Gib einen Betrag von null oder mehr ein."),
      ).toBeTruthy(),
    );
  });

  it("keeps a refusal that names no input at the foot of the form", async () => {
    await submitWith(refusal("pipeline_id", "reference_not_found"));
    await waitFor(() =>
      expect(screen.getByText("pipeline_id refused")).toBeTruthy(),
    );
  });
});
