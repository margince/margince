// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { en } from "../../i18n/en";
import { stubWithSession } from "../../testing/story-utils";
import { ArchivedDealBand } from "./deal360/dealrestore";
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

// The deal screens' backend, plus its archive entry and the restore route,
// whose answer names the links the restore could not bring back.
function serve(
  undoable: Record<string, unknown>,
  restores: Request[],
  leftBehind: Record<string, unknown>[] = [],
) {
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
      return jsonResponse({
        ...archiveEntry({ undoable: false }),
        left_behind: leftBehind,
      });
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

  it("says where a merged-away deal went and offers no Restore", async () => {
    const merged = { ...archived, merged_into_id: "y" };
    const rest = stubBackend([merged], { single: merged });
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const request = input instanceof Request ? input : null;
        const url = new URL(String(request ? request.url : input), "http://x");
        if (url.pathname.endsWith("/records/deal/x/history")) {
          return jsonResponse({
            data: [archiveEntry({ undoable: true })],
            page: { next_cursor: null, has_more: false },
          });
        }
        return rest(input, init);
      }),
    );
    render(<DealScreen id="x" />);
    expect(
      (
        await screen.findAllByText(en["history.field.merged_into_id"], {
          exact: false,
        })
      ).length,
    ).toBeGreaterThan(0);
    expect(screen.queryByTestId("restore-record")).toBeNull();
  });

  it("offers Restore deal on the band beside the archive sentence, and restores its archive entry", async () => {
    const user = userEvent.setup();
    const restores: Request[] = [];
    vi.stubGlobal("fetch", serve({ undoable: true }, restores));
    render(<DealScreen id="x" />);
    const sentence = await screen.findByText(en["deal.archivedReadOnly"]);
    const restore = await screen.findByTestId("restore-record");
    expect(sentence.closest(".record-band")?.contains(restore)).toBe(true);
    await waitFor(() => expect(restore.hasAttribute("disabled")).toBe(false));
    await user.click(restore);
    await waitFor(() => expect(restores.length).toBe(1));
    expect(restores[0]?.headers.get("If-Match")).toBeTruthy();
    expect(await screen.findByText("“ERP connector” restored")).toBeTruthy();
  });

  it("says how many links stayed behind when the restore could not bring them all back", async () => {
    const user = userEvent.setup();
    vi.stubGlobal(
      "fetch",
      serve({ undoable: true }, [], [{ kind: "list" }, { kind: "tag" }]),
    );
    render(<DealScreen id="x" />);
    const restore = await screen.findByTestId("restore-record");
    await waitFor(() => expect(restore.hasAttribute("disabled")).toBe(false));
    await user.click(restore);
    expect(
      await screen.findByText("Put back. 2 items could not come back with it."),
    ).toBeTruthy();
    expect(screen.queryByText("“ERP connector” restored")).toBeNull();
  });

  it("draws no Restore deal and says why when the archive cannot be put back", async () => {
    vi.stubGlobal(
      "fetch",
      serve({ undoable: false, reason: "restore_archiver_required" }, []),
    );
    render(<DealScreen id="x" />);
    expect(
      await screen.findByText(en["history.undo.restoreArchiverRequired"]),
    ).toBeTruthy();
    expect(screen.queryByTestId("restore-record")).toBeNull();
  });
});

// The server marks an archived deal not writable, so the way back must not
// wait on the write gate the archive already closed. It asks what the archive
// asks: delete, or edit rights for the deal's owner.
describe("the archived deal's band", () => {
  function mountBand(allow: Parameters<typeof stubWithSession>[1]) {
    stubWithSession(
      {
        "GET /records/deal/x/history": () =>
          jsonResponse({
            data: [archiveEntry({ undoable: true })],
            page: { next_cursor: null, has_more: false },
          }),
      },
      allow,
    );
    render(
      <ArchivedDealBand
        deal={{ ...archived, writable: false }}
        reason={en["deal.archivedReadOnly"]}
        reasonId="deal-refusal"
      />,
    );
  }

  it("offers Restore deal to a reader who may archive deals", async () => {
    mountBand({ deal: ["read", "delete"] });
    expect(await screen.findByTestId("restore-record")).toBeTruthy();
  });

  it("offers Restore deal to a reader who may edit deals, as their owner archives with edit rights", async () => {
    mountBand({ deal: ["read", "update"] });
    expect(await screen.findByTestId("restore-record")).toBeTruthy();
  });

  it("says the deal is archived and offers no way back to a reader who may only read deals", async () => {
    mountBand({ deal: ["read"] });
    const sentence = await screen.findByText(en["deal.archivedReadOnly"]);
    expect(sentence.id).toBe("deal-refusal");
    expect(screen.queryByTestId("restore-record")).toBeNull();
  });
});
