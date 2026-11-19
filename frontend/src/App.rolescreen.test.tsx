/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, screen } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { renderApp, teardownApp } from "./testing/appharness";
import { meFixture, READS_RECORDS } from "./testing/mefixture";

beforeEach(() => {
  globalThis.localStorage.setItem("margince.workspaceSlug", "acme");
});

afterEach(async () => {
  cleanup();
  await teardownApp();
  vi.unstubAllGlobals();
  window.location.hash = "";
});

it.each([
  ["#/analytics", "forecast and deal"],
  ["#/progress", "report_definition"],
  ["#/goals", "sales_target"],
])("says %s is not available for a role without %s read", async (hash) => {
  const fetched = vi.fn(async (input: Request | string | URL) => {
    const url = String(input instanceof Request ? input.url : input);
    if (url.endsWith("/v1/me"))
      return Response.json(meFixture({ allow: { contact: ["read"] } }));
    return Response.json({ code: "service_unavailable" }, { status: 503 });
  });
  vi.stubGlobal("fetch", fetched);
  window.location.hash = hash;
  renderApp();
  expect(
    await screen.findByText("Not available for your role."),
  ).toBeInTheDocument();
});

it("says Deals is not available for a role without deal read, and loads nothing", async () => {
  const { deal: _deal, ...withoutDeals } = READS_RECORDS;
  const fetched = vi.fn(async (input: Request | string | URL) => {
    const url = String(input instanceof Request ? input.url : input);
    if (url.endsWith("/v1/me"))
      return Response.json(meFixture({ allow: withoutDeals }));
    return Response.json({ code: "service_unavailable" }, { status: 503 });
  });
  vi.stubGlobal("fetch", fetched);
  window.location.hash = "#/deals";
  renderApp();
  expect(
    await screen.findByText("Not available for your role."),
  ).toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "Retry" })).toBeNull();
  const urls = fetched.mock.calls.map(([input]) =>
    String(input instanceof Request ? input.url : input),
  );
  expect(urls.some((url) => url.includes("/deals"))).toBe(false);
});
