import type { BrowserContext, Page } from "@playwright/test";
import type { components } from "../src/api/schema";
import { routes } from "./mockapi/routes";
import { answer } from "./mockapi/server";
import { createMockState, type MockApiOptions } from "./mockapi/state";

// Session preparation adds a query; the principal fixture is scoped to its path.
export async function servePrincipal(
  page: Page,
  principal: components["schemas"]["MeResponse"],
): Promise<void> {
  await page.route(
    (url) => url.pathname === "/v1/me",
    (route) => route.fulfill({ json: principal }),
  );
}

export { anna } from "./mockapi/fixtures";
export type { MockApiOptions } from "./mockapi/state";

// The coherent seed (Anna Weber, Brandt Automotive, the fleet-retrofit deal)
// mocked at the network edge, so the harness is hermetic. BASE_URL mode skips
// this and hits a live backend instead.
export async function mockApi(
  target: Page | BrowserContext,
  options?: MockApiOptions,
): Promise<void> {
  if (process.env.BASE_URL) {
    return;
  }
  // The auth gate (App.tsx) renders signup when no workspace slug resolves,
  // before it probes /me. The value is a dev-side setting, not tenant
  // authority: the mocked /me is that.
  await target.addInitScript(() => {
    globalThis.localStorage.setItem("margince.workspaceSlug", "seed");
  });
  const state = createMockState(options);
  await target.route(/\/v1\//, (route) => answer(routes, route, state));
}
