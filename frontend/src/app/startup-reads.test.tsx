/** @vitest-environment happy-dom */
import { type QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, expect, it, vi } from "vitest";
import { api } from "../api/client";
import { useOnboardingProgress } from "../features/onboarding/onboarding-progress";
import { stable } from "../format/collate";
import { useCompanyContextCapabilities } from "../lib/companycontextreach";
import { AuthProbeError, sessionRefusalWatch, useMe } from "../lib/session";
import { useInstallationSettings } from "../lib/uploadlimit";
import { meFixture } from "../testing/mefixture";
import { createQueryClient } from "./queryclient";
import { prefetchStartupReads } from "./startup-reads";

const clients: QueryClient[] = [];
afterEach(async () => {
  cleanup();
  for (const client of clients.splice(0)) {
    await client.cancelQueries();
    client.clear();
  }
  vi.restoreAllMocks();
});

function freshClient() {
  const client = createQueryClient();
  clients.push(client);
  return client;
}

function serve(answer: (path: string) => Response) {
  const paths: string[] = [];
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => {
    const request = input instanceof Request ? input : new Request(input);
    const path = new URL(request.url).pathname;
    paths.push(path);
    return answer(path);
  });
  return paths;
}

const STARTUP_READS = [
  "/v1/company/context/capabilities",
  "/v1/installation/settings",
  "/v1/me",
  "/v1/onboarding/state",
];

function useShellGates() {
  return [
    useMe(),
    useInstallationSettings(),
    useCompanyContextCapabilities(),
    useOnboardingProgress(true),
  ];
}

it("asks each read the splash waits for once, and the shell's hooks take those answers", async () => {
  const paths = serve((path) =>
    path === "/v1/me"
      ? Response.json(meFixture())
      : path === "/v1/onboarding/state"
        ? Response.json({ step: "complete" })
        : Response.json({ onboarding_enabled: true, read_enabled: true }),
  );
  const client = freshClient();
  prefetchStartupReads(client);
  expect([...paths].sort(stable)).toEqual(STARTUP_READS);
  const { result } = renderHook(useShellGates, {
    wrapper: ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={client}>{children}</QueryClientProvider>
    ),
  });
  await waitFor(() =>
    expect(result.current.every((read) => read.isSuccess)).toBe(true),
  );
  expect([...paths].sort(stable)).toEqual(STARTUP_READS);
});

function refusing(status: 401 | 403, code: string) {
  return () =>
    Response.json(
      { status, code },
      { status, headers: { "Content-Type": "application/problem+json" } },
    );
}

it.each([
  ["an expired session", refusing(401, "unauthorized")],
  [
    "a session locked until it enrols a factor",
    refusing(403, "mfa_enrolment_required"),
  ],
])(
  "leaves %s to its own screen: one probe, and no log but the probe's",
  async (_, refusal) => {
    const logged = vi.spyOn(console, "error").mockImplementation(() => {});
    const paths = serve(refusal);
    const client = freshClient();
    const watch = sessionRefusalWatch(client);
    api.use(watch);
    try {
      prefetchStartupReads(client);
      await waitFor(() =>
        expect(
          STARTUP_READS.map((path) => paths.filter((p) => p === path).length),
        ).toEqual([1, 1, 1, 1]),
      );
      await waitFor(() => expect(client.isFetching()).toBe(0));
    } finally {
      api.eject(watch);
    }
    expect(paths.filter((path) => path === "/v1/me")).toHaveLength(1);
    for (const call of logged.mock.calls)
      expect(call.some((value) => value instanceof AuthProbeError)).toBe(true);
  },
);
