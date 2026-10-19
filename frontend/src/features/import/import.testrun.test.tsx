/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { readFileSync } from "node:fs";
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { UnsavedGuard } from "../../components/unsaved";
import { ImportCard } from "./import";
import {
  dryRun,
  jsonResponse,
  render,
  renderAtImport,
  renderAtTestRun,
  run,
  stubRoutes,
} from "./import.testkit";
import { IMPORT_POLL_MS } from "./importrun";

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
  it("shows counts per type and every problem, and Import anyway patches and revalidates the same run", async () => {
    const user = userEvent.setup();
    let tests = 0;
    const sent = stubRoutes({
      [REPORT]: () => jsonResponse(bundleReportFixture),
      [`GET /imports/${run.id}`]: () =>
        jsonResponse({
          ...run,
          on_broken_link: tests > 1 ? "drop_link" : "skip_row",
        }),
      [`POST /imports/${run.id}/validate`]: () => {
        tests += 1;
        return jsonResponse({ ...run, status: "validating" }, 202);
      },
    });
    await renderAtTestRun(user);
    expect(await screen.findByText("Companies")).toBeVisible();
    expect(screen.getByText("Broken link")).toBeVisible();
    expect(screen.getByRole("link", { name: "problems.csv" })).toHaveAttribute(
      "href",
      `/v1/imports/${run.id}/problems.csv?lang=en`,
    );
    await user.click(screen.getByRole("checkbox", { name: /anyway/ }));
    await waitFor(() =>
      expect(sent.map((s) => s.path)).toContain(
        `POST /imports/${run.id}/validate`,
      ),
    );
    const writes = sent.filter((s) => s.method !== "GET");
    expect(writes.slice(-2)).toEqual([
      {
        method: "PATCH",
        path: `PATCH /imports/${run.id}`,
        body: { version: run.version, on_broken_link: "drop_link" },
      },
      {
        method: "POST",
        path: `POST /imports/${run.id}/validate`,
        body: { version: run.version + 1 },
      },
    ]);
    await user.click(
      await screen.findByRole("button", { name: "Import anyway" }),
    );
    await waitFor(() =>
      expect(sent.map((s) => s.path)).toContain(
        `POST /imports/${run.id}/approve`,
      ),
    );
    expect(sent.filter((s) => s.path === "POST /imports")).toHaveLength(0);
  });

  it("says there is nothing to import when every row is unchanged, and closes the run on Done", async () => {
    const user = userEvent.setup();
    const sent = stubRoutes({
      [REPORT]: () =>
        jsonResponse({
          ...dryRun,
          rows_read: 3,
          disposition: { created: 0, updated: 0, unchanged: 3, skipped: 0 },
          issues: [],
        }),
      [`GET /imports/${run.id}`]: () => jsonResponse(run),
      [`POST /imports/${run.id}/cancel`]: () =>
        jsonResponse({ ...run, status: "cancelled" }),
    });
    await renderAtTestRun(user);
    expect(await screen.findByText("Nothing to import")).toBeVisible();
    expect(
      screen.queryByRole("button", { name: /^Import (\d|anyway)/ }),
    ).toBeNull();
    await user.click(screen.getByRole("button", { name: "Done" }));
    await waitFor(() =>
      expect(sent.map((s) => s.path)).toContain(
        `POST /imports/${run.id}/cancel`,
      ),
    );
    expect(sent.map((s) => s.path)).not.toContain(
      `POST /imports/${run.id}/approve`,
    );
  });

  it("does not say every row is in the CRM when the file held no rows", async () => {
    const user = userEvent.setup();
    stubRoutes({
      [REPORT]: () =>
        jsonResponse({
          ...dryRun,
          rows_read: 0,
          disposition: { created: 0, updated: 0, unchanged: 0, skipped: 0 },
          issues: [],
        }),
      [`GET /imports/${run.id}`]: () => jsonResponse(run),
    });
    await renderAtTestRun(user);
    expect(
      await screen.findByRole("button", { name: "Import 0 rows" }),
    ).toBeDisabled();
    expect(screen.queryByText("Nothing to import")).toBeNull();
  });

  it("reads a report that sends no issue list as a clean one", async () => {
    const user = userEvent.setup();
    stubRoutes({
      [REPORT]: () =>
        jsonResponse({
          ...dryRun,
          rows_read: 3,
          disposition: { created: 0, updated: 0, unchanged: 3, skipped: 0 },
          issues: null,
        }),
      [`GET /imports/${run.id}`]: () => jsonResponse(run),
    });
    await renderAtTestRun(user);
    expect(await screen.findByText("Nothing to import")).toBeVisible();
  });

  it("offers no Import 0 rows while only broken-link rows remain, and imports them once the box is ticked", async () => {
    const user = userEvent.setup();
    let tests = 0;
    stubRoutes({
      [REPORT]: () =>
        jsonResponse({
          ...dryRun,
          rows_read: 1,
          disposition: { created: 0, updated: 0, unchanged: 0, skipped: 1 },
          issues: [bundleReportFixture.issues[0]],
        }),
      [`GET /imports/${run.id}`]: () =>
        jsonResponse({
          ...run,
          on_broken_link: tests > 1 ? "drop_link" : "skip_row",
        }),
      [`POST /imports/${run.id}/validate`]: () => {
        tests += 1;
        return jsonResponse({ ...run, status: "validating" }, 202);
      },
    });
    await renderAtTestRun(user);
    expect(await screen.findByText("Nothing to import")).toBeVisible();
    expect(screen.queryByRole("button", { name: "Import 0 rows" })).toBeNull();
    await user.click(screen.getByRole("checkbox", { name: /anyway/ }));
    expect(
      await screen.findByRole("button", { name: /^Import anyway/ }),
    ).toBeEnabled();
  });

  it("points at problems.csv before Done when rows were skipped", async () => {
    const user = userEvent.setup();
    stubRoutes({
      [REPORT]: () =>
        jsonResponse({
          ...dryRun,
          rows_read: 3,
          disposition: { created: 0, updated: 0, unchanged: 2, skipped: 1 },
          issues: [],
        }),
      [`GET /imports/${run.id}`]: () => jsonResponse(run),
    });
    await renderAtTestRun(user);
    expect(
      await screen.findByText(/Download problems\.csv before/),
    ).toBeVisible();
  });

  it("polls a validating run until its report arrives", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    try {
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
      let status = "validating";
      const sent = stubRoutes({
        [`GET /imports/${run.id}`]: () => jsonResponse({ ...run, status }),
        [REPORT]: () => jsonResponse(bundleReportFixture),
      });
      await renderAtTestRun(user);
      expect(await screen.findByText("Testing your file…")).toBeVisible();
      expect(screen.queryByRole("button", { name: /Import \d/ })).toBeNull();
      expect(sent.map((s) => s.path)).not.toContain(
        `GET /imports/${run.id}/report`,
      );
      status = "awaiting_approval";
      await vi.advanceTimersByTimeAsync(IMPORT_POLL_MS);
      expect(
        await screen.findByRole("button", { name: "Import 4 rows" }),
      ).toBeVisible();
      expect(screen.queryByText("Testing your file…")).toBeNull();
    } finally {
      vi.useRealTimers();
    }
  });

  it("returns to the map with the reason when the worker refuses the file", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    try {
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
      const reason = "The file has no header row.";
      let refused = false;
      stubRoutes({
        [`GET /imports/${run.id}`]: () =>
          jsonResponse(
            refused
              ? { ...run, status: "pending", error: reason }
              : { ...run, status: "validating" },
          ),
      });
      await renderAtTestRun(user);
      expect(await screen.findByText("Testing your file…")).toBeVisible();
      refused = true;
      await vi.advanceTimersByTimeAsync(IMPORT_POLL_MS);
      expect(await screen.findByText(reason)).toBeVisible();
      expect(screen.getByText("Test run stopped")).toBeVisible();
      expect(screen.queryByText("Test run did not start")).toBeNull();
      expect(screen.queryByText("Testing your file…")).toBeNull();
      expect(screen.getByRole("button", { name: "Run test" })).toBeEnabled();
    } finally {
      vi.useRealTimers();
    }
  });

  it("stops a validating run's test, and the run returns to the map", async () => {
    const user = userEvent.setup();
    let stopped = false;
    const sent = stubRoutes({
      [`GET /imports/${run.id}`]: () =>
        jsonResponse({ ...run, status: stopped ? "pending" : "validating" }),
      [`POST /imports/${run.id}/cancel`]: () => {
        stopped = true;
        return jsonResponse({ ...run, status: "pending" });
      },
    });
    await renderAtTestRun(user);
    await user.click(await screen.findByRole("button", { name: "Stop test" }));
    expect(
      await screen.findByRole("button", { name: "Run test" }),
    ).toBeEnabled();
    expect(sent.map((s) => s.path)).toContain(`POST /imports/${run.id}/cancel`);
    expect(screen.queryByText("Testing your file…")).toBeNull();
  });

  it("a refused stop says to press Stop test again, and the button is there", async () => {
    const user = userEvent.setup();
    stubRoutes({
      [`GET /imports/${run.id}`]: () =>
        jsonResponse({ ...run, status: "validating" }),
      [`POST /imports/${run.id}/cancel`]: () =>
        jsonResponse({ status: 503, detail: "Try again shortly." }, 503),
    });
    await renderAtTestRun(user);
    await user.click(await screen.findByRole("button", { name: "Stop test" }));
    expect(
      await screen.findByText("The test did not stop. Press Stop test again."),
    ).toBeVisible();
    expect(screen.getByRole("button", { name: "Stop test" })).toBeEnabled();
  });

  it("a reloaded validating run shows the testing state", async () => {
    stubRoutes({
      [`GET /imports/${run.id}`]: () =>
        jsonResponse({ ...run, status: "validating" }),
    });
    renderAtImport(run.id);
    expect(await screen.findByText("Testing your file…")).toBeVisible();
    expect(screen.getByRole("status")).toHaveAttribute("aria-busy", "true");
  });

  it("approves the version of the run the reader read", async () => {
    const user = userEvent.setup();
    const sent = stubRoutes();
    await renderAtTestRun(user);
    await user.click(
      await screen.findByRole("button", { name: "Import 3 rows" }),
    );
    await waitFor(() =>
      expect(
        sent.find((s) => s.path === `POST /imports/${run.id}/approve`)?.body,
      ).toEqual({ version: run.version }),
    );
  });

  it("returns to the map with the reason when a test run of Import anyway stops", async () => {
    const user = userEvent.setup();
    let tests = 0;
    const refusal = "The file changed since it was uploaded.";
    const stored = "The test run stopped at row 3.";
    stubRoutes({
      [REPORT]: () => jsonResponse(bundleReportFixture),
      [`GET /imports/${run.id}`]: () =>
        jsonResponse(
          tests > 1 ? { ...run, status: "pending", error: stored } : run,
        ),
      [`POST /imports/${run.id}/validate`]: () => {
        tests += 1;
        return tests === 1
          ? jsonResponse({ ...run, status: "validating" }, 202)
          : jsonResponse({ status: 422, detail: refusal }, 422);
      },
    });
    await renderAtTestRun(user);
    await user.click(await screen.findByRole("checkbox", { name: /anyway/ }));
    expect(await screen.findByText("Test run did not start")).toBeVisible();
    expect(screen.getByRole("alert")).toHaveTextContent(refusal);
    expect(screen.queryByText(stored)).toBeNull();
    expect(screen.getByRole("button", { name: "Run test" })).toBeEnabled();
  });

  it("shows the reason stored on a draft whose test stopped in another tab", async () => {
    const user = userEvent.setup();
    const stored = "The test run stopped at row 3.";
    stubRoutes({
      [`POST /imports/${run.id}/validate`]: () =>
        jsonResponse({ ...run, status: "pending", error: stored }, 202),
      [`GET /imports/${run.id}`]: () =>
        jsonResponse({ ...run, status: "pending", error: stored }),
    });
    await renderAtTestRun(user);
    expect(await screen.findByText(stored)).toBeVisible();
    expect(screen.getByText("Test run stopped")).toBeVisible();
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("reads the run again after an approve refused for its version, and the next approve sends the new one", async () => {
    const user = userEvent.setup();
    let approvals = 0;
    let moved = false;
    const sent = stubRoutes({
      [`GET /imports/${run.id}`]: () =>
        jsonResponse(moved ? { ...run, version: 5 } : run),
      [`POST /imports/${run.id}/approve`]: () => {
        approvals += 1;
        if (approvals > 1) {
          return jsonResponse({ ...run, status: "complete" }, 202);
        }
        moved = true;
        return jsonResponse(
          { status: 409, code: "conflict", detail: "The run changed." },
          409,
        );
      },
    });
    await renderAtTestRun(user);
    await user.click(
      await screen.findByRole("button", { name: "Import 3 rows" }),
    );
    expect(await screen.findByText("Import did not run")).toBeVisible();
    const approve = sent.findIndex(
      (s) => s.path === `POST /imports/${run.id}/approve`,
    );
    await waitFor(() =>
      expect(sent.slice(approve).map((s) => s.path)).toContain(
        `GET /imports/${run.id}`,
      ),
    );
    await user.click(screen.getByRole("button", { name: "Import 3 rows" }));
    await waitFor(() =>
      expect(
        sent.filter((s) => s.path === `POST /imports/${run.id}/approve`).at(-1)
          ?.body,
      ).toEqual({ version: 5 }),
    );
  });

  it("reads the run again after a retest refused its mapping", async () => {
    const user = userEvent.setup();
    let tests = 0;
    const sent = stubRoutes({
      [REPORT]: () => jsonResponse(bundleReportFixture),
      [`GET /imports/${run.id}`]: () =>
        jsonResponse(tests > 1 ? { ...run, status: "pending" } : run),
      [`POST /imports/${run.id}/validate`]: () => {
        tests += 1;
        if (tests === 1) {
          return jsonResponse({ ...run, status: "validating" }, 202);
        }
        if (tests === 2) {
          return jsonResponse(
            {
              status: 422,
              code: "validation_error",
              detail: "Two columns are mapped to Email.",
              details: {
                errors: [
                  {
                    field: "mapping",
                    code: "duplicate_target",
                    message: "Two columns are mapped to Email.",
                  },
                ],
              },
            },
            422,
          );
        }
        return jsonResponse(
          { status: 409, code: "conflict", detail: "The file moved on." },
          409,
        );
      },
    });
    await renderAtTestRun(user);
    await user.click(await screen.findByRole("checkbox", { name: /anyway/ }));
    await user.click(await screen.findByRole("button", { name: "Run test" }));
    expect(await screen.findByText("The file moved on.")).toBeVisible();
    const refused = sent
      .map((s) => s.path)
      .lastIndexOf(`POST /imports/${run.id}/validate`);
    await waitFor(() =>
      expect(sent.slice(refused).map((s) => s.path)).toContain(
        `GET /imports/${run.id}`,
      ),
    );
  });

  it("names the import that holds a busy source name, with a link to it", async () => {
    const user = userEvent.setup();
    stubRoutes({
      [`POST /imports/${run.id}/validate`]: () =>
        jsonResponse(
          {
            status: 409,
            code: "import_source_busy",
            detail: "another import of this source name is still writing",
            details: { run_id: "019ff-other" },
          },
          409,
        ),
    });
    await renderAtTestRun(user);
    expect(
      await screen.findByText(
        "Another import of this source name is still writing. Wait for it to finish, then test again.",
      ),
    ).toBeVisible();
    expect(
      screen.getByRole("link", { name: "Open that import" }),
    ).toHaveAttribute("href", expect.stringContaining("run=019ff-other"));
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

  it("lays the counts across the step's width, not at their content's width", async () => {
    // The strip measures its own width as a container, so in the step's
    // align-items:flex-start column it has no content width and every card
    // collapses. jsdom cannot lay this out; the class and its rule are what
    // keep the wrapper stretched.
    const user = userEvent.setup();
    stubRoutes({ [REPORT]: () => jsonResponse(bundleReportFixture) });
    await renderAtTestRun(user);
    const strip = await screen.findByRole("region", {
      name: "Rows per record type",
    });
    expect(strip.parentElement).toHaveClass("import__counts");
    const importCss = readFileSync("src/features/import/import.css", "utf8");
    expect(importCss).toMatch(/\.import__counts\s*{[^}]*align-self:\s*stretch/);
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

  it("notes closed deals imported as history", async () => {
    const user = userEvent.setup();
    stubRoutes({
      [REPORT]: () =>
        jsonResponse({
          ...bundleReportFixture,
          parts: [
            ...bundleReportFixture.parts,
            {
              file: "deals.csv",
              object: "deal",
              rows_read: 5,
              disposition: { created: 5, updated: 0, unchanged: 0, skipped: 0 },
              closed_won: 2,
              closed_lost: 1,
            },
          ],
        }),
    });
    await renderAtTestRun(user);
    expect(
      await screen.findByText("3 deals are imported as closed history"),
    ).toBeVisible();
    expect(screen.getByText(/2 won and 1 lost/)).toBeVisible();
  });

  it("says nothing of closed history when no deal lands closed", async () => {
    const user = userEvent.setup();
    stubRoutes({ [REPORT]: () => jsonResponse(bundleReportFixture) });
    await renderAtTestRun(user);
    await screen.findByText("1 row has invalid values and will be skipped");
    expect(screen.queryByText(/closed history/)).not.toBeInTheDocument();
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

  it("asks nothing on leaving a draft, which the server keeps", async () => {
    const user = userEvent.setup();
    stubRoutes();
    globalThis.location.hash = "#/settings/import/run";
    render(
      <UnsavedGuard address="run" onKeep={() => undefined}>
        {() => <ImportCard subpage="run" />}
      </UnsavedGuard>,
    );
    await user.upload(
      await screen.findByLabelText("Upload your data"),
      new File(["Email\nada@x.test\n"], "leads.csv"),
    );
    await screen.findByRole("button", { name: /Next: map/ });
    const leaving = new Event("beforeunload", { cancelable: true });
    globalThis.dispatchEvent(leaving);
    expect(leaving.defaultPrevented).toBe(false);
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
