/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import { ContractForm } from "./contractform";

// The custom half of this form is seeded from a SECOND read. The catalog can
// land after the form opens, and it can change while the form stays open, and
// the agreement under it can change too — three arrivals, and what the reader
// has already typed has to survive all of them.

type Contract = components["schemas"]["Contract"];
type CustomField = components["schemas"]["CustomField"];

const CATALOG_KEY = ["custom-fields", "contract"];

function field(slug: string, label: string): CustomField {
  return {
    id: `cf-${slug}`,
    object: "contract",
    label,
    slug,
    type: "number",
    status: "active",
    column_name: `cf_${slug}`,
    created_by: "human:u-1",
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
  };
}

const RETAINER = field("retainer", "Retainer");
const HEADCOUNT = field("headcount", "Headcount");

function contract(id: string, over: Record<string, unknown>): Contract {
  return {
    id,
    company_id: "o-1",
    title: `MSA ${id}`,
    status: "active",
    under_contract: true,
    auto_renew: false,
    value_basis: "total",
    version: 1,
    source: "manual",
    captured_by: "human:u-1",
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
    ...over,
  } as Contract;
}

function jsonResponse(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}

// The catalog is answered from a variable the case can move, because what these
// cases are about is the catalog ARRIVING — seeding the query cache instead is
// undone by the read react-query fires on mount.
let catalog: CustomField[] = [];

function stubApi() {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request =
        input instanceof Request ? input : new Request(String(input), init);
      const path = new URL(request.url).pathname;
      if (path.endsWith("/installation/settings")) {
        return jsonResponse({
          company_name: "Brandt Automotive GmbH",
          timezone: "Europe/Berlin",
          base_currency: "EUR",
          base_currency_locked: false,
          max_upload_bytes: 26_214_400,
        });
      }
      if (path.endsWith("/custom-fields")) {
        return jsonResponse({ data: catalog });
      }
      return jsonResponse({ data: [] });
    }),
  );
}

function show(client: QueryClient, ui: React.ReactNode) {
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
}

function published(fields: CustomField[]): QueryClient {
  catalog = fields;
  return new QueryClient({ defaultOptions: { queries: { retry: false } } });
}

/** The catalog changes, and the open form reads it again. */
async function publish(client: QueryClient, fields: CustomField[]) {
  catalog = fields;
  await act(async () => {
    await client.refetchQueries({ queryKey: CATALOG_KEY });
  });
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  catalog = [];
});

it("keeps what the reader typed when the catalog grows under them", async () => {
  stubApi();
  const client = published([RETAINER]);
  show(
    client,
    <ContractForm
      companyId="o-1"
      contract={contract("c-1", { cf_retainer: null })}
      open
      onClose={() => {}}
    />,
  );

  // fireEvent rather than userEvent: these controls are number inputs, and
  // userEvent's keystroke path does not settle a value on one under happy-dom.
  // What the case is about is a value already in the control, not how it got
  // there.
  const retainer = (await screen.findByLabelText(
    "Retainer",
  )) as HTMLInputElement;
  fireEvent.change(retainer, { target: { value: "1" } });

  // A second field is published while the reader is mid-edit.
  await publish(client, [RETAINER, HEADCOUNT]);

  // The new control appears, and the typed answer is still there. Reseeding the
  // whole custom slice on a catalog change replaced it with the stored value —
  // which for a field nobody had filled in is blank.
  expect(await screen.findByLabelText("Headcount")).toBeTruthy();
  expect((screen.getByLabelText("Retainer") as HTMLInputElement).value).toBe(
    "1",
  );
});

it("shows the new agreement's answers when the form is handed a different one", async () => {
  stubApi();
  const client = published([RETAINER]);
  const { rerender } = show(
    client,
    <ContractForm
      companyId="o-1"
      contract={contract("c-1", { cf_retainer: 45_000 })}
      open
      onClose={() => {}}
    />,
  );
  await waitFor(() =>
    expect((screen.getByLabelText("Retainer") as HTMLInputElement).value).toBe(
      "45000",
    ),
  );

  rerender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ContractForm
          companyId="o-1"
          contract={contract("c-2", { cf_retainer: 999 })}
          open
          onClose={() => {}}
        />
      </LocaleProvider>
    </QueryClientProvider>,
  );

  // Keyed on the catalog alone, this skipped: the catalog had not changed, so
  // the second agreement's stored answer never reached the control and the
  // reader was shown the first one's — or nothing at all.
  await waitFor(() =>
    expect((screen.getByLabelText("Retainer") as HTMLInputElement).value).toBe(
      "999",
    ),
  );
});
