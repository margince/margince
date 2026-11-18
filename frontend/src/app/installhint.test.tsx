/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useRef } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { calendarDay } from "../format/calendarday";
import { browserZone, DAY_MS } from "../format/timezone";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import {
  INSTALL_HINT_SNOOZE_MS,
  type InstallHintRecord,
  parseInstallHint,
} from "../lib/installhint";
import type { InstallOutcome, InstallState } from "../lib/pwa";
import {
  readStoredJson,
  removeStored,
  STORAGE_KEYS,
  writeStored,
} from "../lib/storage";
import { meFixture } from "../testing/mefixture";
import { CaptureHealthBanner } from "./capturehealthbanner";
import { InstallHint } from "./installhint";

// The browser's install offer is the boundary: headless runs never make one.
const browser = vi.hoisted(() => {
  const offered: { install: InstallState } = {
    install: { kind: "unavailable" },
  };
  return offered;
});
vi.mock("../lib/pwa", () => ({
  useInstallState: () => browser.install,
}));

const NOW = Date.now();
const TODAY = calendarDay(new Date(NOW), browserZone());
const EARLIER = calendarDay(new Date(NOW - 2 * DAY_MS), browserZone());

function offer(prompt: () => Promise<InstallOutcome>): void {
  browser.install = { kind: "available", prompt };
}

function pointer(coarse: boolean): void {
  vi.stubGlobal("matchMedia", (query: string) => ({
    matches: coarse && query === "(pointer: coarse)",
    media: query,
    addEventListener: () => undefined,
    removeEventListener: () => undefined,
  }));
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

function Page({ concern }: Readonly<{ concern: boolean }>) {
  const scroller = useRef<HTMLDivElement>(null);
  return (
    <>
      {concern && <CaptureHealthBanner />}
      <InstallHint scroller={scroller} />
      <div data-testid="scroller" ref={scroller} tabIndex={-1} />
    </>
  );
}

function mount(concern = false) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const path = new URL(
        input instanceof Request ? input.url : String(input),
        "https://test",
      ).pathname;
      const body = path.endsWith("/me")
        ? meFixture({})
        : {
            data: concern
              ? [
                  {
                    id: "connection-1",
                    provider: "gmail",
                    status: "connected",
                    scopes: [],
                    can_backfill: true,
                    account_label: "mailbox@example.test",
                    concern: "reauth_required",
                    concern_since: new Date(NOW - DAY_MS).toISOString(),
                  },
                ]
              : [],
          };
      return Response.json(body);
    }),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <Page concern={concern} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

const title = () => screen.queryByText(en["installHint.title"]);

beforeEach(() => {
  vi.useFakeTimers({ now: NOW, toFake: ["Date"] });
  history.replaceState(null, "", "#/home");
  pointer(true);
  offer(() => new Promise(() => undefined));
});

afterEach(() => {
  cleanup();
  removeStored(STORAGE_KEYS.installHint);
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

describe("the install hint on Home", () => {
  it("offers to install from the second day, with Not now before Install", async () => {
    seed({ firstDay: EARLIER });
    mount();
    expect(await screen.findByText(en["installHint.title"])).toBeTruthy();
    expect(screen.getByText(en["installHint.value"])).toBeTruthy();
    expect(
      screen.getAllByRole("button").map((button) => button.textContent),
    ).toEqual([en["installHint.notNow"], en["settings.installAppAction"]]);
    expect(screen.queryByRole("alert")).toBeNull();
    expect(screen.queryByRole("status")).toBeNull();
  });

  it("stays away on the first day this device runs the app, and records that day", async () => {
    mount();
    await waitFor(() => expect(stored()?.firstDay).toBe(TODAY));
    expect(title()).toBeNull();
  });

  it("shows on the first day once the reader completed a task on Home", async () => {
    seed({ firstDay: TODAY, taskDone: true });
    mount();
    expect(await screen.findByText(en["installHint.title"])).toBeTruthy();
  });

  it("gives the Share step on an iPhone, without an Install button", async () => {
    browser.install = { kind: "manual-ios" };
    seed({ firstDay: EARLIER });
    mount();
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
    pointer(false);
    seed({ taskDone: true });
    mount();
    await waitFor(() => expect(stored()?.firstDay).toBe(TODAY));
    expect(title()).toBeNull();
  });

  it("never shows off Home", async () => {
    history.replaceState(null, "", "#/deals");
    seed({ taskDone: true });
    mount();
    await waitFor(() => expect(stored()?.firstDay).toBe(TODAY));
    expect(title()).toBeNull();
  });

  it("waits behind another shell banner", async () => {
    seed({ firstDay: EARLIER });
    mount(true);
    expect(
      await screen.findByRole("link", { name: en["captureConcern.reconnect"] }),
    ).toBeTruthy();
    expect(title()).toBeNull();
  });

  it("never shows again once the app is installed, and remembers it", async () => {
    browser.install = { kind: "installed" };
    seed({ firstDay: EARLIER });
    mount();
    await waitFor(() => expect(stored()?.installed).toBe(true));
    expect(title()).toBeNull();
  });

  it("goes away for 30 days on Not now, handing focus to the page", async () => {
    seed({ firstDay: EARLIER });
    mount();
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
    const prompt = vi.fn(
      () =>
        new Promise<InstallOutcome>((resolve) => {
          answer = resolve;
        }),
    );
    offer(prompt);
    seed({ firstDay: EARLIER, dismissals: 1, snoozedUntil: NOW - 1 });
    mount();
    const user = userEvent.setup();
    const install = await screen.findByRole("button", {
      name: en["settings.installAppAction"],
    });
    await user.click(install);
    expect(prompt).toHaveBeenCalledTimes(1);
    expect(install.getAttribute("aria-busy")).toBe("true");
    answer("dismissed");
    await waitFor(() => expect(title()).toBeNull());
    expect(stored()?.dismissals).toBe(2);
    expect(document.activeElement).toBe(screen.getByTestId("scroller"));
  });
});
