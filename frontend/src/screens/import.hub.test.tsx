/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { QueryClient } from "@tanstack/react-query";
import { act, cleanup, screen, waitFor, within } from "@testing-library/react";
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
import { handOverUpload } from "./importflow";
import { IMPORT_POLL_MS } from "./importrun";

// The ids each `GET /users/names` request carried, in the order sent.
function namesAskedFor(): string[][] {
  return vi
    .mocked(fetch)
    .mock.calls.map(
      ([input]) =>
        new URL(
          String(input instanceof Request ? input.url : input),
          "https://test.local",
        ),
    )
    .filter((url) => url.pathname.endsWith("/users/names"))
    .map((url) => url.searchParams.getAll("id"));
}

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

  it("lists a draft to continue or cancel, and leaves a replaced test out of the history", async () => {
    const user = userEvent.setup();
    const sent = stubRoutes({
      "GET /imports": () =>
        jsonResponse({
          data: [
            { ...run, id: "r3", file_name: "draft.csv", status: "pending" },
            {
              ...run,
              id: "r2",
              file_name: "gone.csv",
              status: "cancelled",
              superseded_by: "r9",
            },
            { ...run, id: "r1", file_name: "leads.csv", status: "complete" },
          ],
        }),
    });
    render(<ImportCard />);
    const rows = await screen.findAllByRole("row");
    expect(rows).toHaveLength(3);
    expect(within(rows[1]).getByText("Draft")).toBeVisible();
    expect(
      within(rows[1]).getByRole("link", { name: "Continue" }),
    ).toHaveAttribute("href", expect.stringContaining("run=r3"));
    expect(screen.queryByText("gone.csv")).toBeNull();
    // Each list is asked for on its own. A draft or a replaced test then
    // cannot push a finished import out of the one page of history.
    const asked = sent.filter((s) => s.path === "GET /imports").length;
    expect(asked).toBeGreaterThanOrEqual(2);
    await user.click(within(rows[1]).getByRole("button", { name: "Cancel" }));
    await waitFor(() =>
      expect(sent.map((s) => s.path)).toContain("POST /imports/r3/cancel"),
    );
  });

  it("says when a draft could not be canceled", async () => {
    const user = userEvent.setup();
    stubRoutes({
      "GET /imports": () =>
        jsonResponse({
          data: [
            { ...run, id: "r3", file_name: "draft.csv", status: "pending" },
          ],
        }),
      "POST /imports/r3/cancel": () =>
        jsonResponse({ status: 403, code: "permission_denied" }, 403),
    });
    render(<ImportCard />);
    const rows = await screen.findAllByRole("row");
    await user.click(within(rows[1]).getByRole("button", { name: "Cancel" }));
    expect(
      await screen.findByText("The import was not canceled"),
    ).toBeVisible();
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
        jsonResponse({ data: [{ id: "u-ada", display_name: "Ada" }] }),
    });
    render(<ImportCard />);
    const row = (await screen.findAllByRole("row"))[1];
    expect(await within(row).findByText("2 files · by Ada")).toBeVisible();
    expect(within(row).getByText("1,203")).toBeVisible();
    expect(within(row).getByText("Complete")).toBeVisible();
    // The run carries the principal, `human:<uuid>`; the names endpoint takes
    // the seat id alone and refuses the prefixed form.
    expect(namesAskedFor()).toEqual([["u-ada"]]);
  });

  it("asks no seat name for a run an agent started", async () => {
    stubRoutes({
      "GET /imports": () =>
        jsonResponse({
          data: [{ ...run, file_name: "crm.csv", captured_by: "agent:pp-1" }],
        }),
    });
    render(<ImportCard />);
    const row = (await screen.findAllByRole("row"))[1];
    expect(within(row).getByText("crm.csv")).toBeVisible();
    expect(namesAskedFor()).toEqual([]);
  });

  it("reads the import order from the format", async () => {
    stubRoutes({
      "GET /imports/format": () =>
        jsonResponse({ ...formatFixture, order: ["contact", "company"] }),
    });
    render(<ImportCard />);
    const line = await screen.findByText("Import order: Contacts Companies");
    expect(line).toHaveTextContent("Import order: Contacts → Companies");
    // The arrows are drawn for the eye only; a screen reader hears the names.
    const arrows = [...line.children].filter((one) =>
      one.textContent?.includes("→"),
    );
    expect(arrows).toHaveLength(1);
    for (const arrow of arrows) {
      expect(arrow).toHaveAttribute("aria-hidden", "true");
    }
  });

  it("keeps the history panel while a read is pending, and says a failed one", async () => {
    stubRoutes({
      "GET /imports": () =>
        jsonResponse({ data: [{ ...run, file_name: "crm.csv" }] }),
    });
    const routed = vi.mocked(fetch).getMockImplementation();
    let answerDrafts: (response: Response) => void = () => undefined;
    vi.stubGlobal(
      "fetch",
      vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
        const url = new URL(
          input instanceof Request ? input.url : String(input),
          "https://test.local",
        );
        if (url.searchParams.get("view") === "drafts") {
          return new Promise<Response>((resolve) => {
            answerDrafts = resolve;
          });
        }
        return routed?.(input, init) ?? Promise.reject(new Error("unrouted"));
      }),
    );
    render(<ImportCard />);
    const panel = await screen.findByRole("region", { name: "Recent imports" });
    expect(within(panel).queryByRole("table")).toBeNull();
    act(() =>
      answerDrafts(
        jsonResponse(
          { title: "Server error", status: 500, type: "about:blank" },
          500,
        ),
      ),
    );
    expect(await within(panel).findByRole("alert")).toBeVisible();
    expect(within(panel).queryByRole("table")).toBeNull();
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

  it("asks every record list again once an undo started here has settled", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    try {
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
      const invalidated = vi.spyOn(QueryClient.prototype, "invalidateQueries");
      let status = "complete";
      stubRoutes({
        "GET /imports": () =>
          jsonResponse({ data: [{ ...run, id: "r2", status }] }),
        "POST /imports/r2/undo": () => {
          status = "undoing";
          return jsonResponse({ ...run, id: "r2", status }, 202);
        },
      });
      render(<ImportCard />);
      await user.click(await screen.findByRole("button", { name: "Undo" }));
      const dialog = await screen.findByRole("dialog", {
        name: "Undo this import?",
      });
      await user.click(
        within(dialog).getByRole("button", { name: "Undo import" }),
      );
      expect(await screen.findByText("Undoing")).toBeVisible();
      const everything = () =>
        invalidated.mock.calls.filter((call) => call[0] === undefined).length;
      expect(everything()).toBe(0);
      status = "undone";
      await vi.advanceTimersByTimeAsync(IMPORT_POLL_MS);
      await waitFor(() => expect(everything()).toBe(1));
    } finally {
      vi.useRealTimers();
    }
  });

  it("shows no history panel before the first import", async () => {
    const sent = stubRoutes();
    render(<ImportCard />);
    expect(await screen.findByText("Upload your data")).toBeVisible();
    await waitFor(() =>
      expect(
        sent.filter((one) => one.path === "GET /imports").length,
      ).toBeGreaterThanOrEqual(2),
    );
    await act(async () => {});
    expect(
      screen.queryByRole("heading", { name: "Recent imports" }),
    ).toBeNull();
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

  it("drops a handed-over file when the run page closes before it could upload", async () => {
    stubRoutes({
      "GET /imports/format": () => jsonResponse({ title: "Unavailable" }, 503),
    });
    handOverUpload(new File(["Email\nada@x.test\n"], "leads.csv"));
    globalThis.location.hash = "#/settings/import/run";
    render(<ImportCard subpage="run" />);
    expect(await screen.findByRole("alert")).toBeVisible();
    cleanup();
    const sent = stubRoutes();
    render(<ImportCard subpage="run" />);
    await waitFor(() =>
      expect(sent.map((s) => s.path)).toContain("GET /imports/format"),
    );
    await screen.findByLabelText("Upload your data");
    expect(sent.map((s) => s.path)).not.toContain("POST /imports/sources");
  });
});
