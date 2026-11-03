/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { MoveButton } from "./movebutton";

const DEAL = "01a05500-0000-7000-8000-0000000000b1";
const FIRST = "01a05500-0000-7000-8000-0000000000e1";
const SECOND = "01a05500-0000-7000-8000-0000000000e2";
const receipt: components["schemas"]["RecommendationReceipt"] = {
  deal_id: DEAL,
  rule: "open_task",
  policy_version: "deal-status-projection-10",
  basis_fingerprint: "a".repeat(64),
  checked_at: "2026-10-12T09:00:00Z",
};

beforeEach(() => {
  vi.useFakeTimers({ shouldAdvanceTime: true, advanceTimeDelta: 5 });
  vi.setSystemTime(new Date("2026-10-12T09:00:00Z"));
});

afterEach(() => {
  cleanup();
  vi.clearAllTimers();
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

function currentButton(client: QueryClient, activity: string) {
  return (
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <MoveButton
          dealId={DEAL}
          move={{
            action: "open_task",
            arguments: { activity_id: activity },
            recommendation: receipt,
          }}
        />
      </LocaleProvider>
    </QueryClientProvider>
  );
}

function clientForCheck() {
  return new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: Infinity } },
  });
}

it("refreshes obsolete advice and does not open its task", async () => {
  const urls: URL[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const url = new URL(input instanceof Request ? input.url : String(input));
      urls.push(url);
      if (url.pathname.endsWith("/status")) {
        return Response.json(
          {
            type: "about:blank",
            title: "Changed suggestion",
            status: 409,
            code: "version_skew",
            detail: "Refresh this suggestion.",
          },
          { status: 409 },
        );
      }
      return Response.json({ data: [] });
    }),
  );
  const client = clientForCheck();
  client.setQueryData(["deal-status", DEAL], {});
  client.setQueryData(["worklist"], {});
  render(currentButton(client, FIRST));
  const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
  await user.click(screen.getByRole("button", { name: "Open task" }));
  await screen.findByRole("alert");
  const checked = urls.find((url) => url.pathname.endsWith("/status"));
  expect(checked?.searchParams.get("facts_only")).toBe("true");
  expect(checked?.searchParams.get("if_recommendation")).toBe(
    receipt.basis_fingerprint,
  );
  expect(checked?.searchParams.get("if_rule")).toBe(receipt.rule);
  expect(urls.some((url) => url.pathname.includes("/activities/"))).toBe(false);
  expect(client.getQueryState(["deal-status", DEAL])?.isInvalidated).toBe(true);
  expect(client.getQueryState(["worklist"])?.isInvalidated).toBe(true);
});

it("opens the operand that was checked when props change during the request", async () => {
  const urls: URL[] = [];
  let release: ((response: Response) => void) | undefined;
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const url = new URL(input instanceof Request ? input.url : String(input));
      urls.push(url);
      if (url.pathname.endsWith("/status")) {
        return new Promise<Response>((resolve) => {
          release = resolve;
        });
      }
      if (url.pathname.includes("/activities/")) {
        return Response.json({
          id: FIRST,
          kind: "task",
          subject: "Chosen task",
          writable: false,
          is_done: false,
          version: 1,
          occurred_at: receipt.checked_at,
          source: "manual",
          links: [],
        });
      }
      return Response.json({ data: [] });
    }),
  );
  const client = clientForCheck();
  const mounted = render(currentButton(client, FIRST));
  const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
  await user.click(screen.getByRole("button", { name: "Open task" }));
  mounted.rerender(currentButton(client, SECOND));
  const finish = release;
  if (!finish)
    throw new Error("The preflight did not reach the server boundary.");
  await act(async () => {
    finish(Response.json({ next: { recommendation: receipt } }));
  });
  await waitFor(() =>
    expect(
      urls.some((url) => url.pathname.endsWith(`/activities/${FIRST}`)),
    ).toBe(true),
  );
  expect(
    urls.some((url) => url.pathname.endsWith(`/activities/${SECOND}`)),
  ).toBe(false);
});

it.each([undefined, { ...receipt, basis_fingerprint: "b".repeat(64) }])(
  "does not open when a successful response has not verified the observation",
  async (returnedReceipt) => {
    const urls: URL[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        const url = new URL(
          input instanceof Request ? input.url : String(input),
        );
        urls.push(url);
        if (url.pathname.endsWith("/status")) {
          return Response.json({ next: { recommendation: returnedReceipt } });
        }
        return Response.json({ data: [] });
      }),
    );
    const client = clientForCheck();
    client.setQueryData(["deal-status", DEAL], {});
    render(currentButton(client, FIRST));
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    await user.click(screen.getByRole("button", { name: "Open task" }));
    await screen.findByRole("alert");
    expect(urls.some((url) => url.pathname.includes("/activities/"))).toBe(
      false,
    );
    expect(client.getQueryState(["deal-status", DEAL])?.isInvalidated).toBe(
      true,
    );
  },
);
