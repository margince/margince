/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { meFixture } from "../testing/mefixture";
import { useInstallationDescribed } from "./installationcompany";
import { sessionOptions } from "./session";

const clients: QueryClient[] = [];
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => {
    client.clear();
  });
  vi.restoreAllMocks();
});
const profile: components["schemas"]["CompanyProfile"] = {
  company_id: "00000000-0000-4000-8000-000000000001",
  display_name: "Demo company",
};
function open({
  branded,
  readProfile,
}: {
  branded: boolean;
  readProfile: boolean;
}) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  });
  clients.push(client);
  const me = meFixture();
  client.setQueryData(
    sessionOptions.queryKey,
    branded
      ? { ...me, installation_brand: { display_name: profile.display_name } }
      : me,
  );
  const fetch = vi
    .spyOn(globalThis, "fetch")
    .mockResolvedValue(Response.json(profile));
  const hook = renderHook(
    () => useInstallationDescribed(true, { readProfile }),
    {
      wrapper: ({ children }: { children: ReactNode }) => (
        <QueryClientProvider client={client}>{children}</QueryClientProvider>
      ),
    },
  );
  return { ...hook, fetch };
}
it("uses the session brand when the app needs only company presence", () => {
  const { result, fetch } = open({ branded: true, readProfile: false });
  expect(result.current.described).toBe(true);
  expect(result.current.pending).toBe(false);
  expect(fetch).not.toHaveBeenCalled();
});
it("still reads the full profile for onboarding when a brand exists", async () => {
  const { result, fetch } = open({ branded: true, readProfile: true });
  await waitFor(() => expect(result.current.company.data).toEqual(profile));
  expect(result.current.described).toBe(true);
  expect(fetch).toHaveBeenCalledTimes(1);
});
it("falls back to the profile when an older session response has no brand", async () => {
  const { result, fetch } = open({ branded: false, readProfile: false });
  await waitFor(() => expect(result.current.company.data).toEqual(profile));
  expect(result.current.described).toBe(true);
  expect(fetch).toHaveBeenCalledTimes(1);
});
