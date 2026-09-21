/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { EconomyBanner } from "./economybanner";
import { type GrantSpec, meFixture } from "./mefixture";

function mount(
  allow: GrantSpec,
  readBand: string | (() => string),
  seat: "full" | "read" = "full",
) {
  const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
    const path = new URL(
      input instanceof Request ? input.url : String(input),
      "https://test",
    ).pathname;
    const body = path.endsWith("/me")
      ? meFixture({ allow, seat })
      : {
          monthly_tokens: 100,
          spent_tokens: 80,
          band: typeof readBand === "function" ? readBand() : readBand,
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
        <EconomyBanner />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return { client, fetchMock };
}

// The one grant this surface needs, named once.
const AI_RUNTIME_READER: GrantSpec = { ai_budget: ["read", "update"] };

// Every sentence the banner can say. Silence is proved against all three: the
// notice is standing rather than announced, so it carries no ARIA role, and an
// assertion on one would pass whether or not a banner was on screen.
const BANNER_LINES = [
  "80% AI allowance threshold reached — review feature impacts",
  "AI allowance reached — review deferred work",
  "AI budget status is not recognized",
];

function bannerLinesOnScreen(): string[] {
  return BANNER_LINES.filter((line) => screen.queryByText(line) !== null);
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

it("does not probe usage for a non-admin", async () => {
  const { fetchMock } = mount({}, "degraded");
  await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
  expect(
    fetchMock.mock.calls.some(([input]) =>
      String(input).includes("/ai/budget"),
    ),
  ).toBe(false);
  expect(
    screen.queryByText(
      "80% AI allowance threshold reached — review feature impacts",
    ),
  ).toBeNull();
});

it("shows and dismisses economy mode for an admin", async () => {
  mount(AI_RUNTIME_READER, "degraded");
  expect(
    await screen.findByText(
      "80% AI allowance threshold reached — review feature impacts",
    ),
  ).toBeTruthy();
  const user = userEvent.setup({ delay: null });
  await user.click(screen.getByLabelText("Dismiss"));
  expect(
    screen.queryByText(
      "80% AI allowance threshold reached — review feature impacts",
    ),
  ).toBeNull();
});

it("shows queued while normal stays silent", async () => {
  mount(AI_RUNTIME_READER, "queued");
  expect(
    await screen.findByText("AI allowance reached — review deferred work"),
  ).toBeTruthy();
  cleanup();
  const { fetchMock } = mount(AI_RUNTIME_READER, "normal");
  await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2));
  expect(bannerLinesOnScreen()).toEqual([]);
});

it("shows a recurring band as a new occurrence", async () => {
  let band = "degraded";
  const { client } = mount(AI_RUNTIME_READER, () => band);
  expect(
    await screen.findByText(
      "80% AI allowance threshold reached — review feature impacts",
    ),
  ).toBeTruthy();
  const user = userEvent.setup({ delay: null });
  await user.click(screen.getByLabelText("Dismiss"));
  expect(bannerLinesOnScreen()).toEqual([]);

  band = "normal";
  await client.refetchQueries({ queryKey: ["ai-budget"] });
  await waitFor(() =>
    expect(client.getQueryData<{ band: string }>(["ai-budget"])?.band).toBe(
      "normal",
    ),
  );
  band = "degraded";
  await client.refetchQueries({ queryKey: ["ai-budget"] });
  expect(
    await screen.findByText(
      "80% AI allowance threshold reached — review feature impacts",
    ),
  ).toBeTruthy();
});

it("surfaces an unknown budget band", async () => {
  mount(AI_RUNTIME_READER, "future-band");
  expect(
    await screen.findByText("AI budget status is not recognized"),
  ).toBeTruthy();
});

it("keeps a diagnostics-only management reader free of allowance banners", async () => {
  const { fetchMock } = mount(
    { ai_diagnostics: ["read"], ai_budget: ["read"] },
    "degraded",
  );
  await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
  expect(bannerLinesOnScreen()).toEqual([]);
});
it("links an allowance editor to usage instead of model bindings", async () => {
  mount(AI_RUNTIME_READER, "degraded");
  expect(
    (
      await screen.findByRole("link", { name: "Manage allowance" })
    ).getAttribute("href"),
  ).toBe("#/settings/usage");
});

it("does not show management notices to a read seat even with role grants", async () => {
  const { fetchMock } = mount(AI_RUNTIME_READER, "degraded", "read");
  await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
  expect(bannerLinesOnScreen()).toEqual([]);
});
