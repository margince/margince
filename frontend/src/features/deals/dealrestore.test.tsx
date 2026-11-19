// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { DealScreen } from "./deals";
import { deal, jsonResponse, render, stubBackend } from "./deals.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  localStorage.clear();
});

const archived = deal({
  id: "x",
  name: "ERP connector",
  archived_at: "2026-07-13T00:00:00Z",
});

function archiveEntry(undoable: Record<string, unknown>) {
  return {
    id: "audit-archive",
    action: "archive",
    occurred_at: "2026-07-13T00:00:00Z",
    undoable,
  };
}

// The deal screens' backend, plus its archive entry and the restore route.
function serve(undoable: Record<string, unknown>, restores: Request[]) {
  const rest = stubBackend([archived], { single: archived });
  return vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const request = input instanceof Request ? input : null;
    const url = new URL(String(request ? request.url : input), "http://x");
    if (url.pathname.endsWith("/records/deal/x/history")) {
      expect(url.searchParams.get("action")).toBe("archive");
      return jsonResponse({
        data: [archiveEntry(undoable)],
        page: { next_cursor: null, has_more: false },
      });
    }
    if (url.pathname.endsWith("/history/audit-archive/restore") && request) {
      restores.push(request);
      return jsonResponse(archiveEntry({ undoable: false }));
    }
    return rest(input, init);
  });
}

describe("an archived deal", () => {
  beforeEach(() => localStorage.setItem("margince.workspaceSlug", "acme"));

  it("says it is archived beside its status", async () => {
    vi.stubGlobal("fetch", serve({ undoable: true }, []));
    render(<DealScreen id="x" />);
    expect((await screen.findAllByText("Archived")).length).toBeGreaterThan(0);
  });

  it("offers Restore deal in place of Archive, and restores its archive entry", async () => {
    const restores: Request[] = [];
    vi.stubGlobal("fetch", serve({ undoable: true }, restores));
    render(<DealScreen id="x" />);
    await userEvent.click(
      await screen.findByRole("button", { name: "More actions" }),
    );
    expect(screen.queryByTestId("archive-record")).toBeNull();
    const restore = await screen.findByTestId("restore-record");
    await waitFor(() => expect(restore.hasAttribute("disabled")).toBe(false));
    await userEvent.click(restore);
    await waitFor(() => expect(restores.length).toBe(1));
    expect(restores[0]?.headers.get("If-Match")).toBeTruthy();
    expect(await screen.findByText("“ERP connector” restored")).toBeTruthy();
  });

  it("names the server's reason when the archive cannot be put back", async () => {
    vi.stubGlobal(
      "fetch",
      serve({ undoable: false, reason: "not_writable_by_caller" }, []),
    );
    render(<DealScreen id="x" />);
    await userEvent.click(
      await screen.findByRole("button", { name: "More actions" }),
    );
    const restore = await screen.findByTestId("restore-record");
    await waitFor(() => expect(restore.hasAttribute("disabled")).toBe(true));
    expect(
      document.getElementById(restore.getAttribute("aria-describedby") ?? "")
        ?.textContent,
    ).toBe("You do not have permission to write these fields.");
  });
});
