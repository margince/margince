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
import { en as englishCopy } from "../i18n/en";
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
  it("asks for a keyboard of digits only for a whole number", async () => {
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
    expect(screen.getByLabelText("Value").getAttribute("inputmode")).toBe(
      "numeric",
    );
  });

  it("shows a negative amount beside the amount, not at the foot", async () => {
    await submitWith(refusal("amount_minor", "amount_negative"));
    await waitFor(() =>
      expect(
        screen.getByText(englishCopy["create.amountNegative"]),
      ).toBeTruthy(),
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
        screen.getByText(englishCopy["create.referenceNotFound"]),
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

describe("CreateAction email boxes", () => {
  const emailFields: CreateField[] = [
    { key: "name", label: "create.dealName", required: true },
    { key: "email", label: "create.email", type: "email" },
    {
      key: "emails",
      label: "create.email",
      type: "repeatable",
      addLabel: "field.addEmail",
      rowFields: [{ key: "email", label: "create.email", type: "email" }],
    },
  ];

  // The browser's own check answers in the browser's language and marks
  // nothing, so the form checks the address itself, in the app's language.
  it.each([
    ["en", "Enter an email address for Email."],
    ["de", "Gib für E-Mail eine E-Mail-Adresse ein."],
  ] as const)(
    "refuses an address with no @ in its own words (%s)",
    async (locale, sentence) => {
      const user = userEvent.setup();
      const sent: unknown[] = [];
      render(
        <CreateAction
          requires={{ object: "contact", action: "create" }}
          label="New contact"
          fields={emailFields}
          create={(values) => {
            sent.push(values);
            return Promise.resolve({ id: "1" });
          }}
          invalidate="contacts"
          screen="contacts"
        />,
        locale,
      );
      await user.click(screen.getByText("New contact"));
      await user.type(screen.getByLabelText(/^(Deal.name|Dealname)/i), "Probe");
      const box = screen.getAllByLabelText(/^E-?mail/i)[0];
      await user.type(box, "keine-mail");
      await user.click(
        screen.getByRole("button", { name: /^(Create|Erstellen)$/ }),
      );

      expect(await screen.findByText(sentence)).toBeTruthy();
      expect(box.getAttribute("aria-invalid")).toBe("true");
      expect(box.closest("form")?.noValidate).toBe(true);
      expect(sent).toHaveLength(0);
    },
  );

  it("refuses an address with no @ in an email row", async () => {
    const user = userEvent.setup();
    const sent: unknown[] = [];
    render(
      <CreateAction
        requires={{ object: "contact", action: "create" }}
        label="New contact"
        fields={emailFields}
        create={(values) => {
          sent.push(values);
          return Promise.resolve({ id: "1" });
        }}
        invalidate="contacts"
        screen="contacts"
      />,
    );
    await user.click(screen.getByText("New contact"));
    await user.type(screen.getByLabelText(/Deal.name/i), "Probe");
    await user.click(screen.getByRole("button", { name: "Add email" }));
    const rowBox = screen.getAllByLabelText(/^Email/)[1];
    await user.type(rowBox, "keine-mail");
    await user.click(screen.getByRole("button", { name: "Create" }));

    expect(
      await screen.findByText("Enter an email address for Email."),
    ).toBeTruthy();
    expect(sent).toHaveLength(0);
  });
});

describe("CreateAction refusals of a row list or a set", () => {
  const listFields: CreateField[] = [
    { key: "name", label: "create.dealName", required: true },
    {
      key: "emails",
      label: "create.email",
      type: "repeatable",
      addLabel: "field.addEmail",
      rowFields: [{ key: "email", label: "create.email", type: "email" }],
    },
    {
      key: "tags",
      label: "nav.tags",
      type: "multiselect",
      options: [{ value: "a", label: "A" }],
    },
  ];

  async function submitList(problem: unknown) {
    const user = userEvent.setup();
    render(
      <CreateAction
        requires={{ object: "contact", action: "create" }}
        label="New contact"
        fields={listFields}
        create={() => throwProblem(problem)}
        invalidate="contacts"
        screen="contacts"
      />,
    );
    await user.click(screen.getByText("New contact"));
    await user.type(screen.getByLabelText(/Deal.name/i), "Probe");
    await user.click(screen.getByRole("button", { name: "Create" }));
    return user;
  }

  it("shows a refused address under the email rows", async () => {
    await submitList(refusal("emails", "email_invalid"));
    await waitFor(() =>
      expect(screen.getByText("Check Email and try again.")).toBeTruthy(),
    );
  });

  it("shows a refused set beside its picker", async () => {
    await submitList(refusal("tags", "unknown_tag"));
    await waitFor(() =>
      expect(screen.getByText("Check Tag and try again.")).toBeTruthy(),
    );
  });

  it("drops a refused address once its rows are edited", async () => {
    const user = await submitList(refusal("emails", "email_invalid"));
    expect(await screen.findByText("Check Email and try again.")).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Add email" }));
    expect(screen.queryByText("Check Email and try again.")).toBeNull();
  });

  it("drops a refused set once another pick is made", async () => {
    const user = await submitList(refusal("tags", "unknown_tag"));
    expect(await screen.findByText("Check Tag and try again.")).toBeTruthy();
    await user.click(screen.getByRole("combobox", { name: /^Tag/ }));
    await user.click(screen.getByRole("option", { name: "A" }));
    expect(screen.queryByText("Check Tag and try again.")).toBeNull();
  });

  it("shows a picker's own refusal after a pick, not when the form opens", async () => {
    const user = userEvent.setup();
    render(
      <CreateAction
        requires={{ object: "contact", action: "create" }}
        label="New contact"
        fields={listFields.map((field) =>
          field.type === "multiselect"
            ? { ...field, validate: () => "Pick one tag at most." }
            : field,
        )}
        create={() => throwProblem(refusal("tags", "unknown_tag"))}
        invalidate="contacts"
        screen="contacts"
      />,
    );
    await user.click(screen.getByText("New contact"));
    await screen.findByRole("combobox", { name: /^Tag/ });
    expect(screen.queryByText("Pick one tag at most.")).toBeNull();
    await user.click(screen.getByRole("combobox", { name: /^Tag/ }));
    await user.click(screen.getByRole("option", { name: "A" }));
    expect(await screen.findByText("Pick one tag at most.")).toBeTruthy();
  });
});
