/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { type GrantSpec, meFixture } from "../app/mefixture";
import { LocaleProvider } from "../i18n";
import { FollowUpSettingsCard } from "./followupsettings";

const ADMIN: GrantSpec = { installation_settings: ["read", "update"] };
const READER: GrantSpec = { installation_settings: ["read"] };

type Call = { url: string; method: string; body: unknown };

function backend(allow: GrantSpec, calls: Call[] = []) {
  return vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const request = input instanceof Request ? input : null;
    const url = String(request ? request.url : input);
    const method = request ? request.method : (init?.method ?? "GET");
    const raw = request ? await request.text() : String(init?.body ?? "");
    calls.push({ url, method, body: raw ? JSON.parse(raw) : undefined });
    const body = url.endsWith("/v1/me")
      ? meFixture({ allow })
      : { follow_up_after_days: 2 };
    return new Response(JSON.stringify(body), {
      status: 200,
      headers: { "Content-Type": "application/json" },
    });
  });
}

function Providers({ children }: { children: ReactNode }) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return (
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{children}</LocaleProvider>
    </QueryClientProvider>
  );
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("FollowUpSettingsCard", () => {
  it("shows the window and writes a new one through one PATCH", async () => {
    const calls: Call[] = [];
    vi.stubGlobal("fetch", backend(ADMIN, calls));
    render(
      <Providers>
        <FollowUpSettingsCard />
      </Providers>,
    );
    const days = (await screen.findByTestId(
      "follow-up-after-days",
    )) as HTMLInputElement;
    expect(days.value).toBe("2");
    await waitFor(() => expect(days.disabled).toBe(false));
    await userEvent.clear(days);
    await userEvent.type(days, "5{Tab}");
    await waitFor(() =>
      expect(
        calls.some(
          (c) =>
            c.method === "PATCH" &&
            c.url.endsWith("/activities/follow-up-settings") &&
            JSON.stringify(c.body) ===
              JSON.stringify({ follow_up_after_days: 5 }),
        ),
      ).toBe(true),
    );
  });

  it("refuses a window outside 1 to 30 days without writing", async () => {
    const calls: Call[] = [];
    vi.stubGlobal("fetch", backend(ADMIN, calls));
    render(
      <Providers>
        <FollowUpSettingsCard />
      </Providers>,
    );
    const days = (await screen.findByTestId(
      "follow-up-after-days",
    )) as HTMLInputElement;
    await waitFor(() => expect(days.disabled).toBe(false));
    await userEvent.clear(days);
    await userEvent.type(days, "45{Tab}");
    expect((await screen.findByRole("alert")).textContent).toContain(
      "from 1 to 30",
    );
    expect(calls.some((c) => c.method === "PATCH")).toBe(false);
  });

  it("is read-only for a seat that may not change installation settings", async () => {
    vi.stubGlobal("fetch", backend(READER));
    render(
      <Providers>
        <FollowUpSettingsCard />
      </Providers>,
    );
    const days = (await screen.findByTestId(
      "follow-up-after-days",
    )) as HTMLInputElement;
    expect(days.disabled).toBe(true);
  });
});
