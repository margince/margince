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
  run,
  stubRoutes,
} from "./import.testkit";
import { IMPORT_POLL_MS } from "./importrun";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  globalThis.location.hash = "";
});

describe("the import and its result", () => {
  it("follows a running import until it is complete without a reload", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    try {
      let status = "running";
      stubRoutes({
        "GET /imports/r1": () =>
          jsonResponse({
            ...run,
            id: "r1",
            status,
            checkpoint: 2,
            parts: [{ file: "companies.csv", object: "company", row_count: 4 }],
          }),
      });
      renderAtImport("r1");
      expect(await screen.findByText(/importing/i)).toBeVisible();
      status = "complete";
      await vi.advanceTimersByTimeAsync(IMPORT_POLL_MS);
      expect(await screen.findByText("Import complete")).toBeVisible();
    } finally {
      vi.useRealTimers();
    }
  });

  it("fills each file's bar from the checkpoint in import order", async () => {
    stubRoutes({
      "GET /imports/r1": () =>
        jsonResponse({
          ...run,
          id: "r1",
          status: "running",
          checkpoint: 5,
          parts: [
            { file: "contacts.csv", object: "contact", row_count: 6 },
            { file: "companies.csv", object: "company", row_count: 3 },
          ],
        }),
    });
    renderAtImport("r1");
    expect(
      await screen.findByRole("meter", { name: "Companies: 3 of 3 rows" }),
    ).toBeVisible();
    expect(
      screen.getByRole("meter", { name: "Contacts: 2 of 6 rows" }),
    ).toBeVisible();
  });

  it("reads a file with no rows as done once the walk has passed it", async () => {
    stubRoutes({
      "GET /imports/r1": () =>
        jsonResponse({
          ...run,
          id: "r1",
          status: "running",
          checkpoint: 2,
          parts: [
            { file: "companies.csv", object: "company", row_count: 0 },
            { file: "contacts.csv", object: "contact", row_count: 6 },
          ],
        }),
    });
    renderAtImport("r1");
    const companies = await screen.findByRole("meter", {
      name: "Companies: 0 of 0 rows",
    });
    expect(companies.closest(".import__part")).toHaveTextContent(
      /^CompaniesDone/,
    );
  });

  it("says why a run stopped and resumes it", async () => {
    const user = userEvent.setup();
    const sent = stubRoutes({
      "GET /imports/r1": () =>
        jsonResponse({
          ...run,
          id: "r1",
          status: "failed",
          checkpoint: 2,
          error: "The database went away.",
          parts: [{ file: "leads.csv", object: "lead", row_count: 4 }],
        }),
    });
    renderAtImport("r1");
    expect(await screen.findByText("The database went away.")).toBeVisible();
    await user.click(screen.getByRole("button", { name: "Resume import" }));
    await waitFor(() =>
      expect(sent.map((s) => s.path)).toContain("POST /imports/r1/approve"),
    );
  });

  it("asks before an undo, follows it, and names what it kept", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    try {
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
      let status = "complete";
      const sent = stubRoutes({
        "GET /imports/r1/report": () =>
          jsonResponse({
            ...dryRun,
            run_id: "r1",
            status,
            undo:
              status === "undone"
                ? {
                    run_id: "r1",
                    status,
                    reversed_count: 2,
                    kept: [
                      {
                        object: "lead",
                        id: "l-1",
                        reason:
                          "Someone changed this record after the import, so it was left in place.",
                        reason_code: "undo_edited",
                      },
                      {
                        object: "company",
                        id: "c-1",
                        reason_code: "undo_still_referenced",
                        params: { deals: "2", contacts: "1" },
                      },
                    ],
                    errored: [],
                  }
                : undefined,
          }),
        "GET /imports/r1": () => jsonResponse({ ...run, id: "r1", status }),
        "POST /imports/r1/undo": () => {
          status = "undoing";
          return jsonResponse({ ...run, id: "r1", status }, 202);
        },
      });
      renderAtImport("r1");
      await user.click(
        await screen.findByRole("button", { name: "Undo this import" }),
      );
      const dialog = await screen.findByRole("dialog", {
        name: "Undo this import?",
      });
      await user.click(
        within(dialog).getByRole("button", { name: "Undo import" }),
      );
      await waitFor(() =>
        expect(sent.map((s) => s.path)).toContain("POST /imports/r1/undo"),
      );
      expect(await screen.findByText(/Undo is running/)).toBeVisible();
      status = "undone";
      await vi.advanceTimersByTimeAsync(IMPORT_POLL_MS);
      expect(await screen.findByText("Import undone")).toBeVisible();
      expect(
        screen.getByText(
          "Prospects: l-1 — Someone changed this record after the import, so it was left in place.",
        ),
      ).toBeVisible();
      expect(
        screen.getByText(
          "Companies: c-1 — Live deals or contacts still name this company (deals: 2, contacts: 1), so it was left in place.",
        ),
      ).toBeVisible();
    } finally {
      vi.useRealTimers();
    }
  });

  it("reads a finished import whose report sends no issue list, and says it is finished", async () => {
    stubRoutes({
      "GET /imports/r1": () =>
        jsonResponse({ ...run, id: "r1", status: "complete" }),
      "GET /imports/r1/report": () =>
        jsonResponse({
          ...dryRun,
          run_id: "r1",
          status: "complete",
          issues: null,
        }),
    });
    renderAtImport("r1");
    expect(await screen.findByText("Import complete")).toBeVisible();
    expect(screen.queryByRole("link", { name: "problems.csv" })).toBeNull();
    expect(screen.queryByText(/A test run is staged/)).toBeNull();
    expect(
      screen.getByText(/This import is finished, so its files and map/),
    ).toBeInTheDocument();
  });

  it("counts what each file wrote and the rows it left out", async () => {
    stubRoutes({
      "GET /imports/r1": () =>
        jsonResponse({ ...run, id: "r1", status: "complete" }),
      "GET /imports/r1/report": () =>
        jsonResponse({
          ...dryRun,
          run_id: "r1",
          status: "complete",
          disposition: { created: 3, updated: 2, unchanged: 0, skipped: 2 },
          parts: [
            {
              file: "leads.csv",
              object: "lead",
              rows_read: 7,
              disposition: { created: 3, updated: 2, unchanged: 0, skipped: 2 },
            },
          ],
        }),
    });
    renderAtImport("r1");
    expect(await screen.findByText("3 created · 2 updated")).toBeVisible();
    expect(
      screen.getByText("2 rows were skipped. problems.csv lists each one."),
    ).toBeVisible();
    expect(screen.getByRole("link", { name: "problems.csv" })).toHaveAttribute(
      "href",
      "/v1/imports/r1/problems.csv?lang=en",
    );
  });

  it("shows how far an undo has got and offers Continue undo only once the worker is silent", async () => {
    let silent = 0;
    stubRoutes({
      "GET /imports/r1": () =>
        jsonResponse({
          ...run,
          id: "r1",
          status: "undoing",
          phase: "undoing",
          phase_object: "contact",
          heartbeat_age_seconds: silent,
          rate_rows_per_second: 40,
          progress: {
            objects: [
              { object: "contact", rows_done: 30, rows_total: 50 },
              { object: "company", rows_done: 0, rows_total: 20 },
            ],
            counts: {
              created: 0,
              updated: 0,
              unchanged: 0,
              skipped: 0,
              problems: 0,
            },
          },
        }),
      "GET /imports/r1/report": () =>
        jsonResponse({ ...dryRun, run_id: "r1", status: "undoing" }),
    });
    const view = renderAtImport("r1");
    expect(
      await screen.findByText(/30 of 70 rows processed · 40 rows\/s/),
    ).toBeVisible();
    expect(screen.getAllByText("Undoing Contacts").length).toBeGreaterThan(0);
    expect(screen.queryByRole("button", { name: "Continue undo" })).toBeNull();
    view.unmount();
    silent = 45;
    renderAtImport("r1");
    expect(
      await screen.findByRole("button", { name: "Continue undo" }),
    ).toBeVisible();
  });

  it("says a stopped undo once, in the progress panel's notice", async () => {
    stubRoutes({
      "GET /imports/r1": () =>
        jsonResponse({
          ...run,
          id: "r1",
          status: "undoing",
          phase: "undoing",
          phase_object: "contact",
          heartbeat_age_seconds: 45,
          idle_seconds: 45,
          progress: {
            objects: [{ object: "contact", rows_done: 30, rows_total: 50 }],
            counts: {
              created: 0,
              updated: 0,
              unchanged: 0,
              skipped: 0,
              problems: 0,
            },
          },
        }),
      "GET /imports/r1/report": () =>
        jsonResponse({ ...dryRun, run_id: "r1", status: "undoing" }),
    });
    renderAtImport("r1");
    expect(
      (await screen.findAllByText("Waiting for the server")).length,
    ).toBeGreaterThan(0);
    expect(screen.queryByText(/The undo has stopped/)).toBeNull();
    expect(screen.getByRole("button", { name: "Continue undo" })).toBeVisible();
  });

  it("treats a press on a run another attempt holds as progress, not as a failure", async () => {
    const user = userEvent.setup();
    const sent = stubRoutes({
      "GET /imports/r1": () =>
        jsonResponse({
          ...run,
          id: "r1",
          status: "undoing",
          heartbeat_age_seconds: 45,
        }),
      "GET /imports/r1/report": () =>
        jsonResponse({ ...dryRun, run_id: "r1", status: "undoing" }),
      "POST /imports/r1/undo": () =>
        jsonResponse(
          {
            type: "https://errors.gradion.com/conflict",
            title: "Conflict",
            status: 409,
            code: "run_in_flight",
            detail: "This import is already running.",
          },
          409,
        ),
    });
    renderAtImport("r1");
    await user.click(
      await screen.findByRole("button", { name: "Continue undo" }),
    );
    await waitFor(() =>
      expect(sent.map((s) => s.path)).toContain("POST /imports/r1/undo"),
    );
    expect(screen.queryByText("Undo did not run")).toBeNull();
    expect(screen.queryByText(/already running/)).toBeNull();
  });
});
