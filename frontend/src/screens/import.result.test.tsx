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
      await vi.advanceTimersByTimeAsync(2000);
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
                    kept: [{ object: "lead", id: "l-1" }],
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
      await vi.advanceTimersByTimeAsync(2000);
      expect(await screen.findByText("Import undone")).toBeVisible();
      expect(screen.getByText("Prospects: l-1")).toBeVisible();
    } finally {
      vi.useRealTimers();
    }
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
      "/v1/imports/r1/problems.csv",
    );
  });
});
