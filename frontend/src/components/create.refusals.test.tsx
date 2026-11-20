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
import { seedWriter } from "../testing/mefixture";
import { CreateAction } from "./create";
import type { CreateField } from "./create.fields";

// A 422 names the wire field. The form puts the refusal beside the matching
// input, in the reader's language. The foot of the form keeps the refusals
// that have no input.

afterEach(() => {
  cleanup();
});

function render(ui: ReactNode, locale: "en" | "de" = "en") {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  seedWriter(client);
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
      requires={{ object: "deal", action: "create" }}
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
        requires={{ object: "deal", action: "create" }}
        label="New deal"
        fields={fields}
        create={() => Promise.resolve({ id: "1" })}
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
      expect(
        screen.getByText("This record no longer exists, or you cannot see it."),
      ).toBeTruthy(),
    );
    expect(screen.queryByText("pipeline_id refused")).toBeNull();
  });

  it("names the field it stands beside by its label", async () => {
    await submitWith(refusal("amount_minor", "not_integer"));
    await waitFor(() =>
      expect(screen.getByText("Enter a whole number for Value.")).toBeTruthy(),
    );
  });

  it("puts a 409 conflict on the form's unique field, in the form's own words", async () => {
    const user = userEvent.setup();
    render(
      <CreateAction
        requires={{ object: "deal", action: "create" }}
        label="New deal"
        fields={fields}
        conflict={{ field: "name", sentence: "A deal already has this name." }}
        create={() =>
          throwProblem({ code: "conflict", detail: "name: conflict" })
        }
        invalidate="deals"
        screen="deals"
      />,
    );
    await user.click(screen.getByText("New deal"));
    const name = screen.getByLabelText(/Deal name/i);
    await user.type(name, "Probe");
    await user.click(screen.getByRole("button", { name: "Create" }));

    expect(
      await screen.findByText("A deal already has this name."),
    ).toBeTruthy();
    expect(name.getAttribute("aria-invalid")).toBe("true");
    expect(screen.queryByText(/conflict/)).toBeNull();

    await user.type(name, " two");
    expect(screen.queryByText("A deal already has this name.")).toBeNull();
  });

  it("states a bound in the unit the field declares", async () => {
    const user = userEvent.setup();
    render(
      <CreateAction
        requires={{ object: "deal", action: "create" }}
        label="New deal"
        fields={[
          { key: "name", label: "create.dealName", required: true },
          {
            key: "win_probability",
            label: "deal.winProbability",
            limit: (wire) => `${wire} %`,
          },
        ]}
        create={() =>
          throwProblem({
            code: "validation_error",
            details: {
              errors: [
                {
                  field: "win_probability",
                  code: "out_of_range",
                  message: "refused",
                  params: { min: "0", max: "100" },
                },
              ],
            },
          })
        }
        invalidate="deals"
        screen="deals"
      />,
    );
    await user.click(screen.getByText("New deal"));
    await user.type(screen.getByLabelText(/Deal name/i), "Probe");
    await user.click(screen.getByRole("button", { name: "Create" }));

    expect(
      await screen.findByText(/must be from 0 % to 100 %\.$/),
    ).toBeTruthy();
  });

  it("says a required field is required once it is emptied", async () => {
    const user = userEvent.setup();
    render(
      <CreateAction
        requires={{ object: "deal", action: "create" }}
        label="New deal"
        fields={fields}
        create={() => Promise.resolve({ id: "1" })}
        invalidate="deals"
        screen="deals"
      />,
    );
    await user.click(screen.getByText("New deal"));
    expect(screen.queryByText("Deal name is required.")).toBeNull();
    const name = screen.getByLabelText(/Deal name/i);
    await user.type(name, "P");
    await user.clear(name);

    expect(await screen.findByText("Deal name is required.")).toBeTruthy();
    expect(name.getAttribute("aria-invalid")).toBe("true");
  });

  it("drops a refusal from an earlier opening", async () => {
    const user = userEvent.setup();
    await submitWith(refusal("amount_minor", "not_integer"));
    expect(
      await screen.findByText("Enter a whole number for Value."),
    ).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Cancel" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());

    await user.click(screen.getByText("New deal"));
    await screen.findByRole("dialog");
    expect(screen.queryByText("Enter a whole number for Value.")).toBeNull();
  });
});
