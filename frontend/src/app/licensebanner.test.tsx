/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import { LicenseBanner } from "./licensebanner";
import { type GrantSpec, meFixture } from "./mefixture";

type LicenseState = components["schemas"]["LicenseEntitlement"]["state"];

const LICENSE_READER: GrantSpec = { license: ["read"] };

function mount(allow: GrantSpec, state: LicenseState) {
  const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
    const path = new URL(
      input instanceof Request ? input.url : String(input),
      "https://test",
    ).pathname;
    const body = path.endsWith("/me")
      ? meFixture({ allow })
      : {
          state,
          seats_used: 3,
          over_limit: false,
          checked_at: "2026-08-01T09:00:00Z",
        };
    return new Response(JSON.stringify(body), {
      headers: { "Content-Type": "application/json" },
    });
  });
  vi.stubGlobal("fetch", fetchMock);
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <LicenseBanner />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return { fetchMock };
}

function askedForLicense(fetchMock: ReturnType<typeof vi.fn>) {
  return fetchMock.mock.calls.some(([input]) =>
    String(input instanceof Request ? input.url : input).includes(
      "/installation/license",
    ),
  );
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

it("names a refused licence and links to the seats settings", async () => {
  mount(LICENSE_READER, "rejected");
  expect(await screen.findByText("License refused")).toBeTruthy();
  expect(
    screen.getByText(
      "This installation's license token was checked and refused.",
    ),
  ).toBeTruthy();
  const link = screen.getByRole("link", { name: "Open Seats & license" });
  expect(link.getAttribute("href")).toBe("#/settings/seats");
});

it("offers no way to dismiss a refused licence", async () => {
  mount(LICENSE_READER, "rejected");
  await screen.findByText("License refused");
  expect(screen.queryByRole("button")).toBeNull();
});

it.each<LicenseState>(["absent", "valid"])(
  "renders nothing when the licence is %s",
  async (state) => {
    const { fetchMock } = mount(LICENSE_READER, state);
    await waitFor(() => expect(askedForLicense(fetchMock)).toBe(true));
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2));
    expect(screen.queryByText("License refused")).toBeNull();
    expect(document.querySelector(".appbanner")).toBeNull();
  },
);

it("renders nothing and never asks without license:read", async () => {
  const { fetchMock } = mount({}, "rejected");
  await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
  expect(askedForLicense(fetchMock)).toBe(false);
  expect(document.querySelector(".appbanner")).toBeNull();
});
