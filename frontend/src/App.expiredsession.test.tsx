/** @vitest-environment happy-dom */
import { act, screen, waitFor } from "@testing-library/react";
import type { Root } from "react-dom/client";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
// Transformed while the file is collected, so a case waits on the sign-in screen, not on its first compile.
import "./features/sign-in/auth";

const roots = vi.hoisted((): Root[] => []);
vi.mock("react-dom/client", async (original) => {
  const actual = await original<typeof import("react-dom/client")>();
  return {
    ...actual,
    createRoot: (...args: Parameters<typeof actual.createRoot>) => {
      const root = actual.createRoot(...args);
      roots.push(root);
      return root;
    },
  };
});

beforeEach(() => {
  vi.resetModules();
  localStorage.clear();
  sessionStorage.clear();
  localStorage.setItem("margince.session.seen", "1");
  document.body.innerHTML = '<div id="root"></div>';
  location.hash = "#/deals/d1";
});
afterEach(async () => {
  for (const root of roots.splice(0)) act(() => root.unmount());
  const { queryClient } = await import("./app/queryclient.instance");
  await queryClient.cancelQueries();
  queryClient.clear();
  vi.restoreAllMocks();
  location.hash = "";
});

// Every read that needs a session answers 401. The paths in `held` wait for the
// release, so the refusals can land in either order.
function expiredSession(held: readonly string[]) {
  const asked: string[] = [];
  const answered: string[] = [];
  let release = () => {};
  const released = new Promise<void>((resolve) => {
    release = resolve;
  });
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => {
    const path = new URL(input instanceof Request ? input.url : String(input))
      .pathname;
    asked.push(path);
    if (held.includes(path)) await released;
    answered.push(path);
    if (path === "/v1/auth/capabilities") return Response.json({});
    return Response.json(
      { status: 401, code: "unauthorized" },
      { status: 401, headers: { "Content-Type": "application/problem+json" } },
    );
  });
  const sessionProbes = () => asked.filter((path) => path === "/v1/me").length;
  return { answered, release, sessionProbes };
}

const SHELL_READS = [
  "/v1/installation/settings",
  "/v1/company/context/capabilities",
  "/v1/onboarding/state",
];

async function boot() {
  await import("./bootstrap");
  await import("./main");
}

it("lands an expired session on sign-in with one session probe when the other refusals come last", async () => {
  const logged = vi.spyOn(console, "error").mockImplementation(() => {});
  const server = expiredSession(SHELL_READS);
  await boot();
  expect(await screen.findByRole("button", { name: "Sign in" })).toBeTruthy();
  server.release();
  await waitFor(() =>
    expect(SHELL_READS.every((path) => server.answered.includes(path))).toBe(
      true,
    ),
  );
  const { queryClient } = await import("./app/queryclient.instance");
  await waitFor(() => expect(queryClient.isFetching()).toBe(0));
  expect(server.sessionProbes()).toBe(1);
  expect(screen.getByRole("button", { name: "Sign in" })).toBeTruthy();
  const { AuthProbeError } = await import("./lib/session");
  for (const call of logged.mock.calls)
    expect(call.some((value) => value instanceof AuthProbeError)).toBe(true);
});

it("sends one session probe when the other refusals come first", async () => {
  vi.spyOn(console, "error").mockImplementation(() => {});
  const server = expiredSession(["/v1/me"]);
  await boot();
  await waitFor(() =>
    expect(SHELL_READS.every((path) => server.answered.includes(path))).toBe(
      true,
    ),
  );
  server.release();
  expect(await screen.findByRole("button", { name: "Sign in" })).toBeTruthy();
  const { queryClient } = await import("./app/queryclient.instance");
  await waitFor(() => expect(queryClient.isFetching()).toBe(0));
  expect(server.sessionProbes()).toBe(1);
});
