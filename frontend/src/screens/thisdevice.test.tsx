/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { InstallOutcome } from "../app/pwa";
import { en } from "../i18n/en";
import { InstallPanel } from "./thisdevice";

const INSTALL = { name: en["settings.installAppAction"] };

function panel() {
  return screen.queryByRole("region", { name: en["settings.deviceCard"] });
}

describe("the install row, by install state", () => {
  it("offers Install where the browser kept its offer", () => {
    render(<InstallPanel install={{ kind: "available", prompt: vi.fn() }} />);
    expect(panel()).not.toBeNull();
    expect(screen.getByText(en["settings.installAppHelp"])).toBeTruthy();
    expect(screen.getByRole("button", INSTALL)).toBeTruthy();
  });

  it("points a reader who turned the offer down at the browser, with nothing to press", () => {
    render(<InstallPanel install={{ kind: "dismissed" }} />);
    expect(screen.getByText(en["settings.installAppDismissed"])).toBeTruthy();
    expect(screen.queryByRole("button", INSTALL)).toBeNull();
  });

  it("tells an iPhone or iPad reader how to add it, with nothing to press", () => {
    render(<InstallPanel install={{ kind: "manual-ios" }} />);
    expect(screen.getByText(en["settings.installAppManual"])).toBeTruthy();
    expect(screen.queryByRole("button", INSTALL)).toBeNull();
  });

  it("states that it is installed, with nothing to press", () => {
    render(<InstallPanel install={{ kind: "installed" }} />);
    expect(screen.getByText(en["settings.installAppInstalled"])).toBeTruthy();
    expect(screen.queryByRole("button", INSTALL)).toBeNull();
  });

  it("is absent, panel and all, where the browser cannot install", () => {
    const { container } = render(
      <InstallPanel install={{ kind: "unavailable" }} />,
    );
    expect(container.childElementCount).toBe(0);
  });

  it("takes no focus when it opens already dismissed", () => {
    render(<InstallPanel install={{ kind: "dismissed" }} />);
    expect(document.activeElement).toBe(document.body);
  });
});

/** The browser's install offer, whose dialog the test answers. */
function offer() {
  let answer: (outcome: InstallOutcome) => void = () => undefined;
  const userChoice = new Promise<{ outcome: InstallOutcome; platform: string }>(
    (resolve) => {
      answer = (outcome) => resolve({ outcome, platform: "web" });
    },
  );
  const prompt = vi.fn(async () => undefined);
  const event = Object.assign(
    new Event("beforeinstallprompt", { cancelable: true }),
    { platforms: ["web"], userChoice, prompt },
  );
  act(() => {
    window.dispatchEvent(event);
  });
  return {
    prompt,
    answer: (outcome: InstallOutcome) =>
      act(async () => {
        answer(outcome);
      }),
  };
}

const listening: (() => void)[] = [];

afterEach(() => {
  for (const stop of listening.splice(0)) {
    stop();
  }
});

/** A fresh page, because the install store lives as long as the page does. */
async function page() {
  vi.resetModules();
  const pwa = await import("../app/pwa");
  const { ThisDevicePanel } = await import("./thisdevice");
  listening.push(pwa.listenForInstall());
  return ThisDevicePanel;
}

describe("installing from the row", () => {
  it("waits on the browser's dialog, then reads installed once the reader accepts", async () => {
    const ThisDevicePanel = await page();
    const browser = offer();
    render(<ThisDevicePanel />);
    const user = userEvent.setup();

    await user.click(screen.getByRole("button", INSTALL));
    expect(browser.prompt).toHaveBeenCalledTimes(1);
    expect(screen.getByRole("button", INSTALL).getAttribute("aria-busy")).toBe(
      "true",
    );

    await browser.answer("accepted");
    expect(screen.queryByRole("button", INSTALL)).toBeNull();
    expect(document.activeElement?.textContent).toBe(
      en["settings.installAppInstalled"],
    );
  });

  it("hands focus to the browser's way in when the reader dismisses, with no error", async () => {
    const ThisDevicePanel = await page();
    const browser = offer();
    render(<ThisDevicePanel />);
    const user = userEvent.setup();

    await user.click(screen.getByRole("button", INSTALL));
    await browser.answer("dismissed");

    expect(screen.queryByRole("button", INSTALL)).toBeNull();
    expect(document.activeElement?.textContent).toBe(
      en["settings.installAppDismissed"],
    );
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("offers Install again when the browser offers after a dismissal", async () => {
    const ThisDevicePanel = await page();
    const first = offer();
    render(<ThisDevicePanel />);
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", INSTALL));
    await first.answer("dismissed");

    const again = offer();
    const install = screen.getByRole("button", INSTALL);
    expect(install.getAttribute("aria-busy")).toBeNull();
    await user.click(install);
    expect(again.prompt).toHaveBeenCalledTimes(1);
  });

  it("leaves focus where the reader put it when the browser installs on its own", async () => {
    const ThisDevicePanel = await page();
    offer();
    render(
      <>
        <input aria-label="elsewhere" />
        <ThisDevicePanel />
      </>,
    );
    const elsewhere = screen.getByRole("textbox", { name: "elsewhere" });
    elsewhere.focus();

    act(() => {
      window.dispatchEvent(new Event("appinstalled"));
    });

    expect(screen.getByText(en["settings.installAppInstalled"])).toBeTruthy();
    expect(document.activeElement).toBe(elsewhere);
  });
});
