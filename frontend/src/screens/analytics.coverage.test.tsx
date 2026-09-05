/** @vitest-environment jsdom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { meFixture } from "../app/mefixture";
import { viewerZone } from "../format/timezone";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { AnalyticsScreen } from "./analytics";

afterEach(() => {
  cleanup();
  window.location.hash = "";
  vi.unstubAllGlobals();
});

it("explains a bookmarked coverage page without fetching a forbidden panel", async () => {
  const fetch = vi.fn(async (input: RequestInfo | URL) => {
    const url = input instanceof Request ? input.url : String(input);
    const body = url.endsWith("/me")
      ? meFixture({ roles: ["rep"], allow: {} })
      : url.includes("/analytics/context")
        ? {
            default_scope: { kind: "workspace", label: "Workspace" },
            allowed_scopes: [{ kind: "workspace", label: "Workspace" }],
            capabilities: {},
            timezone: viewerZone(),
            base_currency: "EUR",
          }
        : { data: [] };
    return new Response(JSON.stringify(body), {
      headers: { "Content-Type": "application/json" },
    });
  });
  vi.stubGlobal("fetch", fetch);
  window.location.hash = "#/analytics/coverage";
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <AnalyticsScreen />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  expect(await screen.findByText(en["common.permissionDenied"])).toBeTruthy();
  expect(
    fetch.mock.calls.some(([input]) =>
      String(input instanceof Request ? input.url : input).includes(
        "/analytics/coverage",
      ),
    ),
  ).toBe(false);
});
