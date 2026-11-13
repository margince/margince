/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { meFixture } from "../testing/mefixture";
import { TrialPill } from "./trialpill";

const NOW = Date.parse("2026-11-12T10:00:00Z");
const DAY = 86_400_000;

beforeEach(() => {
  // Only the clock the count reads; the timers user-event and queries run on stay real.
  vi.useFakeTimers({ toFake: ["Date"] });
  vi.setSystemTime(NOW);
});

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

it("opens the dialog to the Margince team from the pill", async () => {
  const user = userEvent.setup();
  const ends = NOW + 10 * DAY;
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const url = input instanceof Request ? input.url : String(input);
      const body = url.includes("/installation/license")
        ? {
            state: "absent",
            seats_used: 3,
            over_limit: false,
            checked_at: new Date(NOW).toISOString(),
            trial: {
              started_at: new Date(ends - 14 * DAY).toISOString(),
              ends_at: new Date(ends).toISOString(),
              ended: false,
            },
          }
        : meFixture({ allow: { license: ["read"] } });
      return new Response(JSON.stringify(body), {
        headers: { "Content-Type": "application/json" },
      });
    }),
  );
  render(
    <QueryClientProvider client={new QueryClient()}>
      <LocaleProvider initial="en">
        <TrialPill />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  const pill = await screen.findByRole("button", { name: /10 days left/ });
  expect(pill.getAttribute("aria-haspopup")).toBe("dialog");
  await user.click(pill);
  expect(
    await screen.findByRole("dialog", { name: en["trial.contact.title"] }),
  ).toBeTruthy();
});
