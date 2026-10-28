// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import { OpenSourceProvider } from "../../lib/opensource";
import { meFixture } from "../../testing/mefixture";
import { HandledForYouPanel } from "./worklist.handled";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const CALL = "01a05500-0000-7000-8000-00000000a001";

describe("a receipt opened", () => {
  it("names who acted, what it was read from and every value it moved", async () => {
    stubPages(() => ({
      as_of: "2026-09-05T09:00:00Z",
      truncated: false,
      receipts: [
        {
          id: "01a05500-0000-7000-8000-00000000e101",
          kind: "close_date_corrected",
          summary: "Moved the close date",
          occurred_at: "2026-09-05T08:00:00Z",
          actor: {
            type: "human",
            id: "human:01a05500-0000-7000-8000-00000000b001",
            label: {
              key: "magic.by.member",
              values: { name: "Kirsten Vogel" },
            },
          },
          source_record: { type: "activity", id: CALL, label: "Renewal call" },
          changes: [
            {
              field: "expected_close_date",
              before: "2026-09-30",
              after: "2026-11-14",
            },
          ],
        },
      ],
    }));
    const openSource = vi.fn();

    render(panel(openSource));
    await screen.findByText("Moved the close date");

    expect(screen.getByText("Kirsten Vogel")).toBeTruthy();
    expect(screen.getByText(en["receipt.detail.source"])).toBeTruthy();
    expect(screen.getByText(/2026-09-30/)).toBeTruthy();
    expect(screen.getByText(/2026-11-14/)).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Renewal call" }));
    expect(openSource).toHaveBeenCalledWith(CALL);
  });

  it("opens to nothing when the server sent none of it", async () => {
    stubPages(() => ({
      as_of: "2026-09-05T09:00:00Z",
      truncated: false,
      receipts: [
        {
          id: "01a05500-0000-7000-8000-00000000e102",
          kind: "email_sent",
          summary: "Sent the confirmation",
          occurred_at: "2026-09-05T08:00:00Z",
        },
      ],
    }));

    render(panel(vi.fn()));
    await screen.findByText("Sent the confirmation");

    expect(screen.queryByText(en["receipt.detail.summary"])).toBeNull();
  });
});

describe("a capped list", () => {
  it("reaches the receipts past the first page", async () => {
    stubPages((cursor) =>
      cursor === "page-2"
        ? {
            as_of: "2026-09-05T09:00:00Z",
            truncated: false,
            receipts: [receipt("e202", "Logged the second call")],
          }
        : {
            as_of: "2026-09-05T09:00:00Z",
            truncated: true,
            next_cursor: "page-2",
            receipts: [receipt("e201", "Logged the first call")],
          },
    );

    render(panel(vi.fn()));
    await screen.findByText("Logged the first call");
    expect(screen.queryByText(en["worklist.handled.truncated"])).toBeNull();
    fireEvent.click(
      screen.getByRole("button", { name: en["worklist.handled.showMore"] }),
    );

    expect(await screen.findByText("Logged the second call")).toBeTruthy();
    expect(screen.getByText("Logged the first call")).toBeTruthy();
    expect(
      screen.queryByRole("button", { name: en["worklist.handled.showMore"] }),
    ).toBeNull();
    expect(screen.getByText("Recent changes: 2")).toBeTruthy();
  });

  it("says the list is cut where the server gives no next page", async () => {
    stubPages(() => ({
      as_of: "2026-09-05T09:00:00Z",
      truncated: true,
      receipts: [receipt("e203", "Logged a call")],
    }));

    render(panel(vi.fn()));
    await screen.findByText("Logged a call");

    expect(screen.getByText(en["worklist.handled.truncated"])).toBeTruthy();
    expect(
      screen.queryByRole("button", { name: en["worklist.handled.showMore"] }),
    ).toBeNull();
  });
});

function receipt(suffix: string, summary: string) {
  return {
    id: `01a05500-0000-7000-8000-00000000${suffix}`,
    kind: "email_sent",
    summary,
    occurred_at: "2026-09-05T08:00:00Z",
  };
}

// The page the server answers for a cursor, or for none.
function stubPages(page: (cursor: string | null) => unknown) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const url = new URL(
        String(input instanceof Request ? input.url : input),
        "http://localhost",
      );
      const body = url.pathname.endsWith("/me")
        ? meFixture({ allow: { deal: ["read", "update"] } })
        : page(url.searchParams.get("cursor"));
      return new Response(JSON.stringify(body), {
        status: 200,
        headers: { "content-type": "application/json" },
      });
    }),
  );
}

function panel(openSource: (activityId: string) => void) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return (
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <OpenSourceProvider openSource={openSource}>
          <HandledForYouPanel />
        </OpenSourceProvider>
      </LocaleProvider>
    </QueryClientProvider>
  );
}
