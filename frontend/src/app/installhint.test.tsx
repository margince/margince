/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useRef } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { calendarDay } from "../format/calendarday";
import { browserZone, DAY_MS } from "../format/timezone";
import { en } from "../i18n/en";
import {
  INSTALL_HINT_SNOOZE_MS,
  type InstallHintRecord,
  parseInstallHint,
} from "../lib/installhint";
import type { InstallOutcome } from "../lib/pwa";
import {
  readStoredJson,
  removeStored,
  STORAGE_KEYS,
  writeStored,
} from "../lib/storage";
import { meFixture } from "../testing/mefixture";

const NOW = Date.now();
const TODAY = calendarDay(new Date(NOW), browserZone());
const EARLIER = calendarDay(new Date(NOW - 2 * DAY_MS), browserZone());
const IPHONE_SAFARI =
  "Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Mobile/15E148 Safari/604.1";
const CONCERN = {
  id: "connection-1",
  provider: "gmail",
  status: "connected",
  scopes: [],
  can_backfill: true,
  account_label: "mailbox@example.test",
  concern: "reauth_required",
  concern_since: new Date(NOW - DAY_MS).toISOString(),
};

type Device = Readonly<{
  coarse?: boolean;
  standalone?: boolean;
  agent?: string;
  touchPoints?: number;
  homeScreen?: boolean;
}>;

type Routes = Readonly<Record<string, () => unknown>>;

const shadowed: string[] = [];
let stopListening: () => void = () => undefined;

function device({
  coarse = true,
  standalone = false,
  agent,
  touchPoints,
  homeScreen,
}: Device): void {
  vi.stubGlobal("matchMedia", (query: string) => ({
    matches:
      (coarse && query === "(pointer: coarse)") ||
      (standalone && query === "(display-mode: standalone)"),
    media: query,
    addEventListener: () => undefined,
    removeEventListener: () => undefined,
  }));
  const properties = {
    userAgent: agent,
    maxTouchPoints: touchPoints,
    standalone: homeScreen,
  };
  for (const [name, value] of Object.entries(properties)) {
    if (value !== undefined) {
      Object.defineProperty(navigator, name, { configurable: true, value });
      shadowed.push(name);
    }
  }
}

function offerEvent(
  answer: Promise<InstallOutcome>,
  prompt = vi.fn(async (): Promise<void> => undefined),
) {
  const event = Object.assign(
    new Event("beforeinstallprompt", { cancelable: true }),
    {
      platforms: ["web"],
      userChoice: answer.then((outcome) => ({ outcome, platform: "web" })),
      prompt,
    },
  );
  return { event, prompt };
}

function seed(record: Partial<InstallHintRecord>): void {
  writeStored(
    STORAGE_KEYS.installHint,
    JSON.stringify({ ...parseInstallHint(null), ...record }),
  );
}

function stored(): InstallHintRecord | null {
  return readStoredJson(STORAGE_KEYS.installHint, parseInstallHint);
}

function stubApi(routes: Routes, me = meFixture({})): string[] {
  const asked: string[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const path = new URL(
        input instanceof Request ? input.url : String(input),
        "https://test",
      ).pathname.replace(/^\/v1/, "");
      asked.push(path);
      if (path.endsWith("/me")) return Response.json(me);
      const route = routes[path];
      return Response.json(route ? await route() : { data: [] });
    }),
  );
  return asked;
}

/** A fresh page: the install store lives as long as the page does. */
async function mount(
  options: Readonly<{ offer?: Event; concern?: boolean }> = {},
) {
  vi.resetModules();
  const pwa = await import("../lib/pwa");
  const { InstallHint } = await import("./installhint");
  const { CaptureHealthBanner } = await import("./capturehealthbanner");
  const { LocaleProvider } = await import("../i18n");
  await (await import("../i18n/catalogs")).loadLocale("en");
  const { QueryClient, QueryClientProvider } = await import(
    "@tanstack/react-query"
  );
  stopListening = pwa.listenForInstall();
  if (options.offer) window.dispatchEvent(options.offer);
  function Page() {
    const scroller = useRef<HTMLDivElement>(null);
    return (
      <>
        {options.concern && <CaptureHealthBanner />}
        <InstallHint scroller={scroller} />
        <div data-testid="scroller" ref={scroller} tabIndex={-1} />
      </>
    );
  }
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <Page />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

const title = () => screen.queryByText(en["installHint.title"]);
const unanswered = () => new Promise<InstallOutcome>(() => undefined);

beforeEach(() => {
  vi.useFakeTimers({ now: NOW, toFake: ["Date"] });
  history.replaceState(null, "", "#/home");
  device({});
  stubApi({});
});

afterEach(() => {
  cleanup();
  stopListening();
  for (const name of shadowed.splice(0)) {
    Reflect.deleteProperty(navigator, name);
  }
  removeStored(STORAGE_KEYS.installHint);
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

describe("the install hint on Home", () => {
  it("offers to install from the second day, with Not now before Install", async () => {
    seed({ firstDay: EARLIER });
    await mount({ offer: offerEvent(unanswered()).event });
    expect(await screen.findByText(en["installHint.title"])).toBeTruthy();
    expect(screen.getByText(en["installHint.value"])).toBeTruthy();
    expect(
      screen.getAllByRole("button").map((button) => button.textContent),
    ).toEqual([en["installHint.notNow"], en["settings.installAppAction"]]);
    expect(screen.queryByRole("alert")).toBeNull();
    expect(screen.queryByRole("status")).toBeNull();
  });

  it("stays away on the first day this device runs the app, and records that day", async () => {
    await mount({ offer: offerEvent(unanswered()).event });
    await waitFor(() => expect(stored()?.firstDay).toBe(TODAY));
    expect(title()).toBeNull();
  });

  it("shows on the first day once the reader completed a task on Home", async () => {
    seed({ firstDay: TODAY, taskDone: true });
    await mount({ offer: offerEvent(unanswered()).event });
    expect(await screen.findByText(en["installHint.title"])).toBeTruthy();
  });

  it("gives the Share step in Safari on an iPhone, with Not now alone", async () => {
    device({ agent: IPHONE_SAFARI, touchPoints: 5 });
    seed({ firstDay: EARLIER });
    await mount();
    expect(await screen.findByText(en["installHint.title"])).toBeTruthy();
    const [before, after] = en["installHint.manual"].split("{share}");
    const step = screen.getByText(
      (_, node) => node?.textContent === `${before}${after}`,
      { selector: ".callout-text" },
    );
    expect(step.querySelector("svg")?.getAttribute("aria-hidden")).toBe("true");
    expect(
      screen.getAllByRole("button").map((button) => button.textContent),
    ).toEqual([en["installHint.notNow"]]);
  });

  it("never shows where the reader points with a mouse", async () => {
    device({ coarse: false });
    seed({ taskDone: true });
    await mount({ offer: offerEvent(unanswered()).event });
    await waitFor(() => expect(stored()?.firstDay).toBe(TODAY));
    expect(title()).toBeNull();
  });

  it("never shows off Home", async () => {
    history.replaceState(null, "", "#/deals");
    seed({ taskDone: true });
    await mount({ offer: offerEvent(unanswered()).event });
    await waitFor(() => expect(stored()?.firstDay).toBe(TODAY));
    expect(title()).toBeNull();
  });

  it("waits behind another shell banner", async () => {
    stubApi({ "/connectors": () => ({ data: [CONCERN] }) });
    seed({ firstDay: EARLIER });
    await mount({ offer: offerEvent(unanswered()).event, concern: true });
    expect(
      await screen.findByRole("link", { name: en["captureConcern.reconnect"] }),
    ).toBeTruthy();
    expect(title()).toBeNull();
  });

  it("waits while a banner's read is still out, and shows once it says nothing", async () => {
    let answer: (body: unknown) => void = () => undefined;
    const budget = new Promise((resolve) => {
      answer = resolve;
    });
    const asked = stubApi(
      { "/ai/budget": () => budget },
      meFixture({ allow: { ai_budget: ["read", "update"] } }),
    );
    seed({ taskDone: true });
    await mount({ offer: offerEvent(unanswered()).event });
    await waitFor(() => expect(stored()?.firstDay).toBe(TODAY));
    await waitFor(() => expect(asked).toContain("/ai/budget"));
    expect(title()).toBeNull();
    answer({ monthly_tokens: 100, spent_tokens: 10, band: "normal" });
    expect(await screen.findByText(en["installHint.title"])).toBeTruthy();
  });

  it("goes away for 30 days on Not now, handing focus to the page", async () => {
    seed({ firstDay: EARLIER });
    await mount({ offer: offerEvent(unanswered()).event });
    const user = userEvent.setup();
    await user.click(
      await screen.findByRole("button", { name: en["installHint.notNow"] }),
    );
    expect(title()).toBeNull();
    expect(stored()).toMatchObject({
      dismissals: 1,
      snoozedUntil: NOW + INSTALL_HINT_SNOOZE_MS,
    });
    expect(document.activeElement).toBe(screen.getByTestId("scroller"));
  });

  it("counts a dismissed browser dialog as Not now, and waits on the dialog meanwhile", async () => {
    let answer: (outcome: InstallOutcome) => void = () => undefined;
    const { event, prompt } = offerEvent(
      new Promise<InstallOutcome>((resolve) => {
        answer = resolve;
      }),
    );
    seed({ firstDay: EARLIER, dismissals: 1, snoozedUntil: NOW - 1 });
    await mount({ offer: event });
    const user = userEvent.setup();
    const install = await screen.findByRole("button", {
      name: en["settings.installAppAction"],
    });
    await user.click(install);
    expect(prompt).toHaveBeenCalledTimes(1);
    expect(install.getAttribute("aria-busy")).toBe("true");
    answer("dismissed");
    await waitFor(() => expect(title()).toBeNull());
    await waitFor(() => expect(stored()?.dismissals).toBe(2));
    expect(document.activeElement).toBe(screen.getByTestId("scroller"));
  });
});

describe("the install hint inside the installed app", () => {
  beforeEach(() => {
    seed({ firstDay: EARLIER });
  });

  it("never shows again once the browser installs the app, and remembers it", async () => {
    await mount({ offer: offerEvent(unanswered()).event });
    act(() => {
      window.dispatchEvent(new Event("appinstalled"));
    });
    await waitFor(() => expect(stored()?.installed).toBe(true));
    expect(title()).toBeNull();
  });

  it("never shows in a standalone window, even with an offer and a due schedule", async () => {
    device({ standalone: true });
    await mount({ offer: offerEvent(unanswered()).event });
    await waitFor(() => expect(stored()?.installed).toBe(true));
    expect(title()).toBeNull();
  });

  it("never shows when iOS runs it from the home screen", async () => {
    device({ agent: IPHONE_SAFARI, touchPoints: 5, homeScreen: true });
    await mount();
    await waitFor(() => expect(stored()?.installed).toBe(true));
    expect(title()).toBeNull();
  });
});
