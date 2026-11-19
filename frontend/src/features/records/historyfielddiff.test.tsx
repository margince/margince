/** @vitest-environment happy-dom */

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render as rtlRender, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import { HistoryFieldDiff } from "./historyfielddiff";

function show(
  node: ReactNode,
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } }),
) {
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{node}</LocaleProvider>
    </QueryClientProvider>,
  );
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const NEGOTIATION = "01a1e47b-0afd-7dc3-a750-72c1bd631d7b";
const QUALIFIED = "01a1e47b-0afc-766e-9f87-b5ae37545df9";
const OWNER_BEFORE = "01a1e489-5d05-7e59-a427-e897c19b4879";
const OWNER_AFTER = "01a1e489-5d12-7486-9f86-666ea4c6556f";
const COMPANY = "01a1e489-5d12-7486-9f86-000000000001";

// The reads the page names these records with: the pipelines, the colleague
// names and the record references.
function serveNames() {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const url = new URL(input instanceof Request ? input.url : String(input));
      if (url.pathname.endsWith("/pipelines")) {
        return Response.json({
          data: [
            {
              id: "p-1",
              name: "Sales",
              stages: [
                { id: NEGOTIATION, name: "Negotiation" },
                { id: QUALIFIED, name: "Qualified" },
              ],
            },
          ],
        });
      }
      if (url.pathname.endsWith("/users/names")) {
        return Response.json({
          data: [
            { id: OWNER_BEFORE, display_name: "QA Deals" },
            { id: OWNER_AFTER, display_name: "QA Spare One" },
          ],
        });
      }
      if (url.pathname.endsWith("/records/references")) {
        return Response.json({
          data: [{ id: COMPANY, name: "Akeneo", archived: false }],
        });
      }
      return Response.json(
        { title: "Not found", status: 404 },
        { status: 404 },
      );
    }),
  );
}

describe("HistoryFieldDiff names the records a change points at", () => {
  const values = { currency: null, locale: "en" as const, zone: "UTC" };

  it("names a stage by its pipeline", async () => {
    serveNames();
    show(
      <HistoryFieldDiff
        field="stage_id"
        oldValue={NEGOTIATION}
        newValue={QUALIFIED}
        values={values}
      />,
    );
    expect(await screen.findByText("Negotiation")).toBeTruthy();
    expect(screen.getByText("Qualified")).toBeTruthy();
    expect(screen.queryByText(NEGOTIATION)).toBeNull();
  });

  it("names a stage from the pipelines the page holds, with no second read", async () => {
    serveNames();
    const client = new QueryClient();
    client.setQueryData(
      ["pipelines", "all"],
      [
        {
          id: "p-1",
          name: "Sales",
          stages: [{ id: QUALIFIED, name: "Qualified" }],
        },
      ],
    );
    show(
      <HistoryFieldDiff
        field="stage_id"
        oldValue={null}
        newValue={QUALIFIED}
        values={values}
      />,
      client,
    );
    expect(await screen.findByText("Qualified")).toBeTruthy();
    expect(vi.mocked(fetch)).not.toHaveBeenCalled();
  });

  it("names an owner as a colleague", async () => {
    serveNames();
    show(
      <HistoryFieldDiff
        field="owner_id"
        oldValue={OWNER_BEFORE}
        newValue={OWNER_AFTER}
        values={values}
      />,
    );
    expect(await screen.findByText("QA Deals")).toBeTruthy();
    expect(screen.getByText("QA Spare One")).toBeTruthy();
  });

  it("names a company by its record", async () => {
    serveNames();
    show(
      <HistoryFieldDiff
        field="company_id"
        oldValue={null}
        newValue={COMPANY}
        values={values}
      />,
    );
    expect(await screen.findByText("Akeneo")).toBeTruthy();
  });

  it("keeps a stage's id as stored in a field that does not hold a stage", async () => {
    serveNames();
    const client = new QueryClient();
    client.setQueryData(
      ["pipelines", "all"],
      [
        {
          id: "p-1",
          name: "Sales",
          stages: [{ id: QUALIFIED, name: "Qualified" }],
        },
      ],
    );
    show(
      <HistoryFieldDiff
        field="title"
        oldValue={null}
        newValue={QUALIFIED}
        values={values}
      />,
      client,
    );
    expect(await screen.findByText(QUALIFIED)).toBeTruthy();
    expect(screen.queryByText("Qualified")).toBeNull();
  });

  it("keeps an id no reader can name exactly as stored", async () => {
    serveNames();
    const unknown = "01a1e47b-0afd-7dc3-a750-ffffffffffff";
    show(
      <HistoryFieldDiff
        field="stage_id"
        oldValue={NEGOTIATION}
        newValue={unknown}
        values={values}
      />,
    );
    expect(await screen.findByText("Negotiation")).toBeTruthy();
    expect(screen.getByText(unknown)).toBeTruthy();
  });
});

describe("HistoryFieldDiff", () => {
  // The audit spine projects every value as a string, including the integer
  // minor-unit count money is stored in. Rendered raw, a deal moving from
  // 2500000 to 4150000 reads as millions beside tiles showing the same figure
  // as €25,000.00 — and a value misread by a hundredfold is the one thing that
  // must not sit next to a button that writes.
  it("scales a minor-unit field by the record's currency", () => {
    show(
      <HistoryFieldDiff
        field="amount_minor"
        oldValue="2500000"
        newValue="4150000"
        values={{ currency: "EUR", locale: "en", zone: "UTC" }}
      />,
    );
    expect(screen.getByText(/25,000\.00/)).toBeTruthy();
    expect(screen.getByText(/41,500\.00/)).toBeTruthy();
  });

  // A move to a date-only entry keeps the clock on the side that had one. The
  // side that never had one shows its date alone.
  it("reads each side of an activity time with that side's precision", () => {
    show(
      <HistoryFieldDiff
        field="occurred_at"
        oldValue="2026-08-26T09:30:00Z"
        newValue="2026-08-27T12:00:00Z"
        oldPrecision="instant"
        newPrecision="day"
        values={{ currency: null, locale: "en", zone: "UTC" }}
      />,
    );
    expect(screen.getByText("26/08/2026, 09:30")).toBeTruthy();
    expect(screen.getByText("27/08/2026")).toBeTruthy();
    expect(screen.queryByText(/12:00/)).toBeNull();
  });

  // A field with no minor-unit suffix is not money and keeps its own text.
  it("leaves a non-money field alone", () => {
    show(
      <HistoryFieldDiff
        field="title"
        oldValue="Ops Lead"
        newValue="Head of Ops"
        values={{ currency: "EUR", locale: "en", zone: "UTC" }}
      />,
    );
    expect(screen.getByText("Ops Lead")).toBeTruthy();
    expect(screen.getByText("Head of Ops")).toBeTruthy();
  });

  // A null side is the created/cleared origin, and the diff draws its own
  // wording for it. Asserted through the rendered text rather than re-spelled
  // here: one owner of that phrasing, and it is FieldDiff.
  it("hands a null side to the diff as null rather than as empty text", () => {
    show(
      <HistoryFieldDiff
        field="title"
        oldValue={null}
        newValue="Head of Ops"
        values={{ currency: null, locale: "en", zone: "UTC" }}
      />,
    );
    expect(screen.queryByText("null")).toBeNull();
    expect(screen.getByText("Head of Ops")).toBeTruthy();
  });
});
