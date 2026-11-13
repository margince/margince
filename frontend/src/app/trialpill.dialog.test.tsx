/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { meFixture } from "../testing/mefixture";
import { TrialPill } from "./trialpill";

const DAY = 86_400_000;

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

it("opens the dialog to the Margince team from the pill", async () => {
  const user = userEvent.setup();
  const ends = Date.now() + 10 * DAY;
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const url = input instanceof Request ? input.url : String(input);
      const body = url.includes("/installation/license")
        ? {
            state: "absent",
            seats_used: 3,
            over_limit: false,
            checked_at: new Date().toISOString(),
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
