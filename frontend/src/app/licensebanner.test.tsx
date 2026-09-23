/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { useMe } from "../screens/common";
import { useCan } from "./capability";
import { useLicensePosture } from "./license-posture";
import { LicenseBanner } from "./licensebanner";
import { type GrantSpec, meFixture } from "./mefixture";

type LicenseState = components["schemas"]["LicenseEntitlement"]["state"];

const LICENSE_READER: GrantSpec = { license: ["read"] };

// Rendered beside the banner, off the same hooks, so an absent banner is asserted
// against a render that has provably seen the answer rather than one that ran
// before it: a query at `success` is not yet a component that re-rendered.
function ReadProbe() {
  const me = useMe().data;
  const mayRead = useCan("license", "read");
  const posture = useLicensePosture();
  return (
    <output
      data-testid="probe"
      data-may-read={me === undefined ? "unread" : String(mayRead)}
      data-posture={posture ?? "unread"}
    />
  );
}

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
        <ReadProbe />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return { fetchMock, client };
}

const settled = (client: QueryClient, key: string) =>
  client
    .getQueryCache()
    .findAll({ queryKey: [key] })
    .some((query) => query.state.status === "success");

const probe = () => screen.getByTestId("probe");

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
  const link = screen.getByRole("link", {
    name: `Open ${en["settings.tab.seats"]}`,
  });
  expect(link.getAttribute("href")).toBe("#/settings/seats");
});

it("offers no way to dismiss a refused licence", async () => {
  mount(LICENSE_READER, "rejected");
  await screen.findByText("License refused");
  expect(screen.queryByRole("button")).toBeNull();
});

it.each<[LicenseState, string]>([
  ["absent", "none"],
  ["valid", "ok"],
])("renders nothing when the licence is %s", async (state, posture) => {
  const { client } = mount(LICENSE_READER, state);
  await waitFor(() => {
    expect(settled(client, "me")).toBe(true);
    expect(settled(client, "installation-license")).toBe(true);
    expect(probe().getAttribute("data-posture")).toBe(posture);
  });
  expect(screen.queryByText("License refused")).toBeNull();
  expect(document.querySelector(".appbanner")).toBeNull();
});

it("renders nothing and never asks without license:read", async () => {
  const { fetchMock, client } = mount({}, "rejected");
  await waitFor(() => {
    expect(settled(client, "me")).toBe(true);
    expect(probe().getAttribute("data-may-read")).toBe("false");
  });
  expect(askedForLicense(fetchMock)).toBe(false);
  expect(document.querySelector(".appbanner")).toBeNull();
});
