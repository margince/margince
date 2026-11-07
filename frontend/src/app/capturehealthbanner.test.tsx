/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { CaptureHealthBanner } from "./capturehealthbanner";

function mount(
  concern?: string,
  failure = false,
  extra?: string,
  dismissConflict = false,
) {
  let dismissed = false;
  const fetch = vi.fn(async (input: RequestInfo | URL) => {
    const req = input instanceof Request ? input : new Request(String(input));
    if (req.method === "POST") {
      expect(await req.json()).toEqual({ since: "2026-10-01T09:00:00Z" });
      if (dismissConflict)
        return Response.json(
          {
            code: "conflict",
            detail: "The connection changed. Refresh and try again.",
          },
          { status: 409 },
        );
      dismissed = true;
    }
    return new Response(
      JSON.stringify({
        data:
          concern && !dismissed
            ? [
                {
                  id: "connection-1",
                  provider: "gmail",
                  status: "connected",
                  scopes: [],
                  can_backfill: true,
                  account_label: "mailbox@example.test",
                  concern,
                  concern_since: "2026-10-01T09:00:00Z",
                },
                ...(extra
                  ? [
                      {
                        id: "connection-2",
                        provider: "graph",
                        status: "connected",
                        scopes: [],
                        can_backfill: true,
                        account_label: "other@example.test",
                        concern: extra,
                        concern_since: "2026-10-01T09:00:00Z",
                      },
                    ]
                  : []),
              ]
            : [],
      }),
      {
        status: failure ? 503 : 200,
        headers: { "Content-Type": "application/json" },
      },
    );
  });
  vi.stubGlobal("fetch", fetch);
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <CaptureHealthBanner />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return { client, fetch };
}
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});
it("names the stalled mailbox and opens its connection", async () => {
  mount("sync_stalled");
  expect(
    await screen.findByText("mailbox@example.test: Sync has stopped"),
  ).toBeTruthy();
  expect(
    screen.getByRole("link", { name: "Open connection" }).getAttribute("href"),
  ).toBe("/#/settings/connections");
  expect(
    screen.queryByRole("button", { name: "Dismiss silence warning" }),
  ).toBeNull();
});
it("dismisses only the observed silence and refreshes the shared read", async () => {
  const user = userEvent.setup();
  const { client } = mount("capture_silent");
  client.setQueryData(["system-health", "reader"], { assessment: "attention" });
  await user.click(
    await screen.findByRole("button", { name: "Dismiss silence warning" }),
  );
  await waitFor(() =>
    expect(
      screen.queryByText("mailbox@example.test: No recent mail delivered"),
    ).toBeNull(),
  );
  expect(client.getQueryState(["system-health", "reader"])?.isInvalidated).toBe(
    true,
  );
});
it("offers reconnect for a revoked grant", async () => {
  mount("reauth_required");
  expect(await screen.findByRole("link", { name: "Reconnect" })).toBeTruthy();
});
it.each([undefined, "future_unknown_concern"])(
  "hides absent or unknown concerns: %s",
  async (concern) => {
    const { client } = mount(concern);
    await waitFor(() =>
      expect(client.getQueryState(["connectors"])?.status).toBe("success"),
    );
    expect(screen.queryByRole("link")).toBeNull();
  },
);
it("does not turn a failed health read into a warning", async () => {
  const { client } = mount("sync_stalled", true);
  await waitFor(() =>
    expect(client.getQueryState(["connectors"])?.status).toBe("error"),
  );
  expect(screen.queryByRole("link")).toBeNull();
});

it("shows the worst mailbox first and names one extra connection", async () => {
  mount("capture_silent", false, "sync_stalled");
  expect(
    await screen.findByText("other@example.test: Sync has stopped"),
  ).toBeTruthy();
  expect(screen.getByText("1 more connection needs attention")).toBeTruthy();
  expect(
    screen.queryByRole("button", { name: "Dismiss silence warning" }),
  ).toBeNull();
});

it("shows a refused dismissal and refreshes its health evidence", async () => {
  const user = userEvent.setup();
  const { fetch } = mount("capture_silent", false, undefined, true);
  await user.click(
    await screen.findByRole("button", { name: "Dismiss silence warning" }),
  );
  expect(await screen.findByRole("alert")).toBeTruthy();
  await waitFor(() =>
    expect(
      fetch.mock.calls.filter(
        ([input]) => input instanceof Request && input.method === "GET",
      ).length,
    ).toBeGreaterThan(1),
  );
  expect(
    screen.getByText("mailbox@example.test: No recent mail delivered"),
  ).toBeTruthy();
});
