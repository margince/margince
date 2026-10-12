/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { cleanup, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ImportCard } from "./import";
import {
  formatFixture,
  jsonResponse,
  render,
  run,
  stubRoutes,
} from "./import.testkit";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  globalThis.location.hash = "";
});

describe("the data import hub", () => {
  it("lists recent imports newest first with Undo on a complete run", async () => {
    stubRoutes({
      "GET /imports": () =>
        jsonResponse({
          data: [
            {
              ...run,
              id: "r2",
              file_name: "hubspot.zip",
              status: "complete",
              parts: [
                { file: "companies.csv", object: "company", row_count: 3 },
              ],
            },
            { ...run, id: "r1", file_name: "leads.csv", status: "undone" },
          ],
        }),
      "GET /imports/format": () => jsonResponse(formatFixture),
    });
    render(<ImportCard />);
    const rows = await screen.findAllByRole("row");
    expect(within(rows[1]).getByText("hubspot.zip")).toBeVisible();
    expect(within(rows[1]).getByRole("button", { name: "Undo" })).toBeEnabled();
    expect(within(rows[2]).queryByRole("button", { name: "Undo" })).toBeNull();
  });

  it("names who ran an import and how many files and rows it took", async () => {
    stubRoutes({
      "GET /imports": () =>
        jsonResponse({
          data: [
            {
              ...run,
              file_name: "crm.zip",
              status: "complete",
              parts: [
                { file: "companies.csv", object: "company", row_count: 3 },
                { file: "contacts.csv", object: "contact", row_count: 1200 },
              ],
            },
          ],
        }),
      "GET /users/names": () =>
        jsonResponse({ data: [{ id: run.captured_by, display_name: "Ada" }] }),
    });
    render(<ImportCard />);
    const row = (await screen.findAllByRole("row"))[1];
    expect(await within(row).findByText("2 files · by Ada")).toBeVisible();
    expect(within(row).getByText("1,203")).toBeVisible();
    expect(within(row).getByText("Complete")).toBeVisible();
  });

  it("reads the import order from the format", async () => {
    stubRoutes({
      "GET /imports/format": () =>
        jsonResponse({ ...formatFixture, order: ["contact", "company"] }),
    });
    render(<ImportCard />);
    const line = (await screen.findByText("Import order:")).parentElement;
    expect(line).toHaveTextContent("Import order:Contacts→Companies");
  });

  it("asks before an undo, then starts it", async () => {
    const user = userEvent.setup();
    const sent = stubRoutes({
      "GET /imports": () =>
        jsonResponse({ data: [{ ...run, id: "r2", status: "complete" }] }),
      "POST /imports/r2/undo": () =>
        jsonResponse({ ...run, id: "r2", status: "undoing" }, 202),
    });
    render(<ImportCard />);
    await user.click(await screen.findByRole("button", { name: "Undo" }));
    const dialog = await screen.findByRole("dialog", {
      name: "Undo this import?",
    });
    expect(
      within(dialog).getByText(
        "Undo runs in reverse order: Prospects, Contacts, Companies.",
      ),
    ).toBeVisible();
    expect(sent.some((s) => s.path === "POST /imports/r2/undo")).toBe(false);
    await user.click(
      within(dialog).getByRole("button", { name: "Undo import" }),
    );
    await vi.waitFor(() =>
      expect(sent.some((s) => s.path === "POST /imports/r2/undo")).toBe(true),
    );
  });

  it("says so when there is no import yet", async () => {
    stubRoutes();
    render(<ImportCard />);
    expect(await screen.findByText("No imports yet.")).toBeVisible();
  });

  it("opens the run page with the dropped file", async () => {
    const user = userEvent.setup();
    stubRoutes();
    render(<ImportCard />);
    await user.upload(
      await screen.findByLabelText("Upload your data"),
      new File(["Email\n"], "leads.csv"),
    );
    expect(globalThis.location.hash).toBe("#/settings/import/run");
  });
});
