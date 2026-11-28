/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { cleanup, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { en as englishCopy } from "../../i18n/en";
import { meFixture } from "../../testing/mefixture";
import { jsonResponse, render } from "../../testing/settings.testkit";
import { tableRowCells } from "../../testing/tablerow.testkit";
import { VacuumHealthCard } from "./vacuumhealth";

const ENDPOINT = "GET /admin/vacuum-recommendations";

function stubRoutes(overrides: Record<string, () => Response> = {}) {
  const sent: string[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request = input instanceof Request ? input : null;
      const url = new URL(
        request ? request.url : String(input),
        "https://test.local",
      );
      const key = `${request?.method ?? init?.method ?? "GET"} ${url.pathname.replace(/^\/v1/, "")}`;
      sent.push(key);
      const override = overrides[key];
      if (override) return override();
      if (key === ENDPOINT) return jsonResponse(REPORT);
      if (key === "GET /me")
        return jsonResponse(
          meFixture({ roles: ["admin"], allow: { job_health: ["read"] } }),
        );
      return jsonResponse({});
    }),
  );
  return sent;
}

const REPORT = {
  generated_at: "2026-11-21T09:30:00Z",
  min_waste_percent: 20,
  min_waste_bytes: 100_000_000,
  unmeasured_tables: 0,
  tables: [
    {
      schema: "public",
      table: "event_outbox",
      owner: "margince_owner",
      table_bytes: 800_000_000,
      waste_bytes: 480_000_000,
      waste_percent: 60,
      statement: 'VACUUM (FULL, ANALYZE) "public"."event_outbox";',
    },
  ],
};

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

it("lists a table with its size, its empty space, the statement and the role to run it as", async () => {
  stubRoutes();

  render(<VacuumHealthCard />);

  const row = await tableRowCells("Table space", "public.event_outbox");
  expect(row.Size).toHaveTextContent("800 MB");
  expect(row[englishCopy["vacuumHealth.col.waste"]]).toHaveTextContent(
    "480 MB",
  );
  expect(row[englishCopy["vacuumHealth.col.waste"]]).toHaveTextContent("60%");
  expect(row["What to run"]).toHaveTextContent(
    'VACUUM (FULL, ANALYZE) "public"."event_outbox";',
  );
  expect(row["What to run"]).toHaveTextContent("Run as margince_owner.");
  expect(screen.getByText(/at least 20% of its file/)).toBeInTheDocument();
  expect(screen.getByText(/VACUUM FULL locks the table/)).toBeInTheDocument();
  expect(
    screen.getByText(/indexes and the separate store/),
  ).toBeInTheDocument();
});

it("says no table needs it rather than showing an empty table", async () => {
  stubRoutes({ [ENDPOINT]: () => jsonResponse({ ...REPORT, tables: [] }) });

  render(<VacuumHealthCard />);

  expect(
    await screen.findByText(/There is nothing to run/),
  ).toBeInTheDocument();
  expect(
    screen.queryByText(englishCopy["vacuumHealth.howTitle"]),
  ).not.toBeInTheDocument();
});

it("counts the large tables it could not estimate", async () => {
  stubRoutes({
    [ENDPOINT]: () =>
      jsonResponse({ ...REPORT, tables: [], unmeasured_tables: 2 }),
  });

  render(<VacuumHealthCard />);

  expect(
    await screen.findByText(/2 large tables have no statistics yet/),
  ).toBeInTheDocument();
});

it("reports a malformed payload rather than claiming no table needs it", async () => {
  stubRoutes({
    [ENDPOINT]: () =>
      jsonResponse({ generated_at: REPORT.generated_at, tables: [] }),
  });

  render(<VacuumHealthCard />);

  expect(await screen.findByRole("alert")).toBeInTheDocument();
  expect(screen.queryByText(/There is nothing to run/)).not.toBeInTheDocument();
});

it("withholds the card from a seat without the grant, and asks the server nothing", async () => {
  const sent = stubRoutes({
    "GET /me": () => jsonResponse(meFixture({ roles: ["rep"] })),
  });

  render(<VacuumHealthCard />);

  expect(
    await screen.findByText(/needs a permission your role does not have/),
  ).toBeInTheDocument();
  expect(sent).not.toContain(ENDPOINT);
});
