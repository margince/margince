/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  dryRun,
  jsonResponse,
  renderAtImport,
  renderAtTestRun,
  run,
  stubRoutes,
} from "./import.testkit";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  globalThis.location.hash = "";
});

// A ZIP's test run: companies and contacts, one contact whose company link
// finds nothing and one company row with a value its field refuses.
const bundleReportFixture = {
  ...dryRun,
  rows_read: 5,
  disposition: { created: 3, updated: 1, unchanged: 0, skipped: 1 },
  parts: [
    {
      file: "contacts.csv",
      object: "contact",
      rows_read: 3,
      disposition: { created: 2, updated: 0, unchanged: 0, skipped: 1 },
    },
    {
      file: "companies.csv",
      object: "company",
      rows_read: 2,
      disposition: { created: 1, updated: 1, unchanged: 0, skipped: 0 },
    },
  ],
  issues: [
    {
      line: 4,
      file: "contacts.csv",
      column: "company_ref",
      kind: "broken_link",
      value: "CO-9",
      reason: "No company has the ref CO-9.",
    },
    {
      line: 3,
      file: "companies.csv",
      column: "Tier",
      kind: "invalid_value",
      value: "Gold",
      reason: "“Gold” is not one of this field’s allowed values.",
    },
  ],
};

const REPORT = `GET /imports/${run.id}/report`;

describe("the test run", () => {
  it("shows counts per type and every problem, and Import anyway restages with drop_link", async () => {
    const user = userEvent.setup();
    const sent = stubRoutes({
      [REPORT]: () => jsonResponse(bundleReportFixture),
    });
    await renderAtTestRun(user);
    expect(await screen.findByText("Companies")).toBeVisible();
    expect(screen.getByText("Broken link")).toBeVisible();
    expect(screen.getByRole("link", { name: "problems.csv" })).toHaveAttribute(
      "href",
      `/v1/imports/${run.id}/problems.csv`,
    );
    await user.click(screen.getByRole("checkbox", { name: /anyway/ }));
    await user.click(
      await screen.findByRole("button", { name: "Import anyway" }),
    );
    const staged = sent.filter((s) => s.path === "POST /imports").at(-1);
    expect(staged?.body).toMatchObject({ on_broken_link: "drop_link" });
    await waitFor(() =>
      expect(sent.map((s) => s.path)).toContain(
        `POST /imports/${run.id}/approve`,
      ),
    );
  });

  it("orders the counts by the format and splits them into new, update and same", async () => {
    const user = userEvent.setup();
    stubRoutes({ [REPORT]: () => jsonResponse(bundleReportFixture) });
    await renderAtTestRun(user);
    const strip = await screen.findByRole("region", {
      name: "Rows per record type",
    });
    expect(strip).toHaveTextContent(
      /Companies2.*1 new · 1 update · 0 same.*Contacts2.*2 new · 0 update · 0 same/,
    );
  });

  it("warns that rows with invalid values are left out", async () => {
    const user = userEvent.setup();
    stubRoutes({ [REPORT]: () => jsonResponse(bundleReportFixture) });
    await renderAtTestRun(user);
    expect(
      await screen.findByText("1 row has invalid values and will be skipped"),
    ).toBeVisible();
    const row = screen.getByRole("row", { name: /Gold/ });
    expect(within(row).getByText("Row 3 · Tier")).toBeVisible();
    expect(within(row).getByText("Invalid value")).toBeVisible();
  });

  it("names the file alone for a problem of the whole file, one row per tag", async () => {
    const user = userEvent.setup();
    const consoleError = vi.spyOn(console, "error");
    const newTag = (value: string) => ({
      file: "companies.csv",
      kind: "new_tag",
      value,
      reason: `The tag “${value}” does not exist yet.`,
    });
    stubRoutes({
      [REPORT]: () =>
        jsonResponse({
          ...bundleReportFixture,
          issues: [newTag("vip"), newTag("partner")],
        }),
    });
    await renderAtTestRun(user);
    for (const value of ["vip", "partner"]) {
      const row = await screen.findByRole("row", { name: new RegExp(value) });
      expect(within(row).getByText("Whole file")).toBeVisible();
      expect(within(row).queryByText(/Row/)).toBeNull();
    }
    const duplicateKeys = consoleError.mock.calls.filter((call) =>
      String(call[0]).includes("same key"),
    );
    expect(duplicateKeys).toEqual([]);
  });

  it("names the run's context tag when the page is opened on the run", async () => {
    stubRoutes({
      [`GET /imports/${run.id}`]: () =>
        jsonResponse({ ...run, context_tag_id: "t-1" }),
      "GET /tags": () =>
        jsonResponse({
          data: [
            {
              id: "t-1",
              workspace_id: "w",
              name: "K5 Conference",
              color: "amber",
            },
          ],
          page: { has_more: false, next_cursor: null },
        }),
    });
    renderAtImport(run.id);
    expect(await screen.findByText(/K5 Conference/)).toBeVisible();
  });

  it("offers no choice about links when no link is broken", async () => {
    const user = userEvent.setup();
    stubRoutes();
    await renderAtTestRun(user);
    expect(
      await screen.findByRole("button", { name: "Import 3 rows" }),
    ).toBeEnabled();
    expect(screen.queryByRole("checkbox", { name: /anyway/ })).toBeNull();
  });

  it("starts again from the upload on Fix and re-upload", async () => {
    const user = userEvent.setup();
    stubRoutes();
    await renderAtTestRun(user);
    await user.click(
      await screen.findByRole("button", { name: "Fix and re-upload" }),
    );
    expect(await screen.findByLabelText("Upload your data")).toBeVisible();
    expect(globalThis.location.hash).not.toContain("run=");
  });

  it("says why the import did not start", async () => {
    const user = userEvent.setup();
    stubRoutes({
      [`POST /imports/${run.id}/approve`]: () =>
        jsonResponse(
          { status: 409, code: "conflict", detail: "This run already ran." },
          409,
        ),
    });
    await renderAtTestRun(user);
    await user.click(
      await screen.findByRole("button", { name: "Import 3 rows" }),
    );
    expect(await screen.findByText("Import did not run")).toBeVisible();
  });
});
