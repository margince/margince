/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import {
  type Locale,
  LocaleProvider,
  type Translator,
  useLocale,
  useT,
} from "../i18n";
import { seedWriter } from "../testing/mefixture";
import { companyCreateFields } from "./companyform";
import { contactCreateFields } from "./contactformfields";
import { CreateAction, type CreateField } from "./create";

// An address or domain is stored once. So a second row holding it is caught
// before the form sends, not answered as a clash with some other record.

type FieldSource = (t: Translator, locale: Locale) => CreateField[];

afterEach(() => {
  cleanup();
});

function Form({
  fields,
  sent,
}: Readonly<{
  fields: FieldSource;
  sent: unknown[];
}>) {
  const t = useT();
  const { locale } = useLocale();
  return (
    <CreateAction
      requires={{ object: "contact", action: "create" }}
      label="Open"
      fields={fields(t, locale)}
      create={(values, rows) => {
        sent.push({ values, rows });
        return Promise.resolve({ id: "1" });
      }}
      invalidate="contacts"
      screen="contacts"
    />
  );
}

async function fillRows(fields: FieldSource, add: string, entries: string[]) {
  const sent: unknown[] = [];
  const client = new QueryClient();
  seedWriter(client);
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <Form fields={fields} sent={sent} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  await userEvent.click(screen.getByText("Open"));
  const name = document.querySelector<HTMLInputElement>("input[maxlength]");
  if (!name) throw new Error("the form has no name input");
  await userEvent.type(name, "Probe");
  for (const entry of entries) {
    await userEvent.click(screen.getByRole("button", { name: add }));
    const inputs = document.querySelectorAll<HTMLInputElement>(
      ".repeatablerowsfield-value input",
    );
    const last = inputs.item(inputs.length - 1);
    await userEvent.type(last, entry);
  }
  return sent;
}

describe("repeated rows", () => {
  it("marks an address typed in two email rows and keeps Create shut", async () => {
    const sent = await fillRows(contactCreateFields, "Add email", [
      "dana@example.com",
      " Dana@Example.com",
    ]);
    await waitFor(() =>
      expect(
        screen.getByText(
          "Dana@Example.com is in two rows. Remove one of them.",
        ),
      ).toBeTruthy(),
    );
    const create = screen.getByRole("button", { name: "Create" });
    expect(create.hasAttribute("disabled")).toBe(true);
    await userEvent.click(create);
    expect(sent).toHaveLength(0);
  });

  it("points every address box at the line that names the repeat", async () => {
    await fillRows(contactCreateFields, "Add email", ["a@x.test", "a@x.test"]);
    const line = await screen.findByText(
      "a@x.test is in two rows. Remove one of them.",
    );
    const boxes = line
      .closest(".field-repeatable")
      ?.querySelectorAll(".repeatablerowsfield-value input");
    expect(line.id).not.toBe("");
    expect(boxes?.length).toBe(2);
    for (const box of boxes ?? []) {
      expect(box.getAttribute("aria-invalid")).toBe("true");
      expect(box.getAttribute("aria-describedby")?.split(" ")).toContain(
        line.id,
      );
    }
  });

  it("marks a domain typed in two rows", async () => {
    await fillRows(companyCreateFields, "Add domain", [
      "example.com",
      "EXAMPLE.com",
    ]);
    expect(
      screen.getByText("EXAMPLE.com is in two rows. Remove one of them."),
    ).toBeTruthy();
  });

  it("leaves one number in two phone rows to the server", async () => {
    const sent = await fillRows(contactCreateFields, "Add phone", [
      "+49 30 123456",
      "+49 30 123456",
    ]);
    expect(screen.queryByText(/is in two rows/)).toBeNull();
    await userEvent.click(screen.getByRole("button", { name: "Create" }));
    await waitFor(() => expect(sent).toHaveLength(1));
  });
});
