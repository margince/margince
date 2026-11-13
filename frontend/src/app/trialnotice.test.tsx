/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import {
  MARGINCE_CONTACT_FORM,
  MARGINCE_TEAM_ADDRESS,
} from "../lib/trialcontact";
import { meFixture } from "../testing/mefixture";
import { LicenseBanner } from "./licensebanner";

type LicenseEntitlement = components["schemas"]["LicenseEntitlement"];

const NOW = Date.parse("2026-11-12T10:00:00Z");
const DAY = 86_400_000;

function entitlement(endsIn: number, ended = false): LicenseEntitlement {
  return {
    state: "absent",
    seats_used: 12,
    over_limit: false,
    checked_at: new Date(NOW).toISOString(),
    trial: {
      started_at: new Date(NOW + endsIn - 14 * DAY).toISOString(),
      ends_at: new Date(NOW + endsIn).toISOString(),
      ended,
    },
  };
}

// Each read of the licence takes the next answer, and the last one stays.
function mount(...answers: LicenseEntitlement[]) {
  const reads: string[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      reads.push(input instanceof Request ? input.url : String(input));
      const url = input instanceof Request ? input.url : String(input);
      const body = url.includes("/installation/license")
        ? answers.length > 1
          ? answers.shift()
          : answers[0]
        : meFixture({ allow: { license: ["read"] } });
      return new Response(JSON.stringify(body), {
        headers: { "Content-Type": "application/json" },
      });
    }),
  );
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
  return reads;
}

beforeEach(() => {
  // Only the clock the count reads; the timers user-event and queries run on stay real.
  vi.useFakeTimers({ toFake: ["Date"] });
  vi.setSystemTime(NOW);
});

afterEach(() => {
  cleanup();
  localStorage.clear();
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

it("leaves the chrome to the orb while more than a week is left", async () => {
  const reads = mount(entitlement(10 * DAY));
  await vi.waitFor(() =>
    expect(reads.some((url) => url.includes("/installation/license"))).toBe(
      true,
    ),
  );
  expect(document.querySelector(".callout")).toBeNull();
});

it("may be put away until tomorrow in the last week, and comes back the next day", async () => {
  const user = userEvent.setup();
  mount(entitlement(6 * DAY));
  await user.click(
    await screen.findByRole("button", { name: en["trial.notice.hideToday"] }),
  );
  expect(screen.queryByText("6 days left of your trial")).toBeNull();

  cleanup();
  mount(entitlement(6 * DAY));
  await vi.waitFor(() =>
    expect(document.querySelector(".appbanner")).toBeNull(),
  );

  cleanup();
  vi.setSystemTime(NOW + DAY);
  mount(entitlement(6 * DAY));
  expect(await screen.findByText("5 days left of your trial")).toBeTruthy();
});

it("cannot be put away on the last day", async () => {
  mount(entitlement(DAY / 2));
  expect(await screen.findByText("1 day left of your trial")).toBeTruthy();
  expect(
    screen.queryByRole("button", { name: en["trial.notice.hideToday"] }),
  ).toBeNull();
});

it("turns to a warning in the trial's last week", async () => {
  mount(entitlement(6 * DAY));
  const notice = await screen.findByText("6 days left of your trial");
  expect(notice.closest(".callout")?.classList).toContain("callout-warning");
});

it("writes the email the reader chose, with the facts the team needs to answer", async () => {
  const user = userEvent.setup();
  mount(entitlement(3 * DAY));
  await user.click(
    await screen.findByRole("button", { name: en["trial.contact.title"] }),
  );
  const dialog = await screen.findByRole("dialog");
  await user.click(
    within(dialog).getByRole("radio", {
      name: new RegExp(en["trial.contact.ask.time"]),
    }),
  );
  await user.type(
    within(dialog).getByRole("textbox", { name: en["trial.contact.note"] }),
    "We onboard a second team next month.",
  );

  const open = within(dialog).getByRole("link", {
    name: en["trial.contact.open"],
  });
  const mail = new URL(open.getAttribute("href") ?? "").searchParams;
  expect(mail.get("subject")).toBe(
    "Test Workspace: more time for the Margince trial",
  );
  const body = mail.get("body") ?? "";
  expect(body).toContain(en["trial.mail.ask.time"]);
  expect(body).toContain("Company: Test Workspace");
  expect(body).toContain("Seats in use: 12");
  expect(body).toContain("We onboard a second team next month.");
  expect(body.endsWith("Best regards,\nTest User")).toBe(true);
});

it("leads a reader with no mail app to the website's form and shows the team's address", async () => {
  const user = userEvent.setup();
  mount(entitlement(3 * DAY));
  await user.click(
    await screen.findByRole("button", { name: en["trial.contact.title"] }),
  );
  const dialog = await screen.findByRole("dialog");
  const form = within(dialog).getByRole("link", {
    name: en["trial.contact.form"],
  });
  expect(form.getAttribute("href")).toBe(MARGINCE_CONTACT_FORM);
  expect(form.getAttribute("target")).toBe("_blank");
  expect(within(dialog).getByText(MARGINCE_TEAM_ADDRESS)).toBeTruthy();
});

it("reads the licence again once the trial's end passes under an open page", async () => {
  // The server's clock is a little behind the reader's. At the end the licence
  // still says the trial runs, and the next read says it has ended.
  mount(entitlement(-1_000), entitlement(-1_000, true));
  expect(await screen.findByText(en["trial.notice.endedTitle"])).toBeTruthy();
});
