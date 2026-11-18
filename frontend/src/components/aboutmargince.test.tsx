// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { stubClipboard } from "../design-system/clipboard-testing";
import { LocaleProvider } from "../i18n";
import type { BuildRelease } from "../lib/buildrelease";
import { AboutMargince } from "./aboutmargince";

const SHA = "0123456789abcdef0123456789abcdef01234567";
const CORE = "2026.2.1";
const HINT = " (Copies to clipboard)";
const COPIED = "Copied version to clipboard";

afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

function renderAbout(build: BuildRelease) {
  render(
    <LocaleProvider initial="en">
      <AboutMargince build={build} />
    </LocaleProvider>,
  );
}

const versionButton = () => screen.getByRole("button", { name: /Copies/ });

/** Each term and its value, in the order the reader sees them. */
function facts(): string[][] {
  return [...document.querySelectorAll(".factlist-row")].map((row) => [
    row.querySelector("dt")?.textContent ?? "",
    row.querySelector("dd")?.textContent ?? "",
  ]);
}

describe("the version", () => {
  it.each<[string, BuildRelease, string, string | undefined]>([
    [
      "a core build",
      { kind: "core", release: CORE, revision: SHA },
      `${CORE} · 0123456`,
      undefined,
    ],
    [
      "an instance build, with its core under it",
      {
        kind: "instance",
        instance: "2026.2.1-acme.3",
        core: CORE,
        revision: SHA,
      },
      "2026.2.1-acme.3 · 0123456",
      `core ${CORE}`,
    ],
    [
      "an instance build without its own release, whose commit is still its own",
      { kind: "instance", core: CORE, revision: SHA },
      "Instance build · 0123456",
      `core ${CORE}`,
    ],
    [
      "an unstamped build",
      { kind: "unstamped", revision: SHA },
      "Development build · 0123456",
      undefined,
    ],
    [
      "an unstamped build without a revision",
      { kind: "unstamped" },
      "Development build",
      undefined,
    ],
  ])("is one button for %s", (_, build, line, note) => {
    renderAbout(build);
    expect(versionButton()).toHaveAccessibleName(`${line}${HINT}`);
    const versionRow = document.querySelector(".factlist-row");
    expect(versionRow?.querySelector(".factlist-note")?.textContent).toBe(note);
  });

  it("links a core build's commit to the core repository, at the size of the text around it", () => {
    renderAbout({ kind: "core", release: CORE, revision: SHA });
    const commit = screen.getByRole("link", { name: /^View commit on GitHub/ });
    expect(commit).toHaveAttribute(
      "href",
      `https://github.com/margince/margince/commit/${SHA}`,
    );
    expect(commit.closest(".t-caption")).toBeNull();
    expect(commit).toHaveAttribute("target", "_blank");
    expect(commit).toHaveAttribute("rel", "noopener noreferrer");
  });

  it.each<[string, BuildRelease]>([
    ["an instance build", { kind: "instance", core: CORE, revision: SHA }],
    ["an unstamped build", { kind: "unstamped", revision: SHA }],
  ])("links no commit for %s", (_, build) => {
    renderAbout(build);
    expect(screen.queryByRole("link", { name: /commit/i })).toBeNull();
  });
});

/** Hold the clock: the copied state lasts three seconds. `userEvent` stalls on
 *  a held clock in happy-dom, so a press is a plain click event. */
function holdClock(build: BuildRelease) {
  renderAbout(build);
  vi.useFakeTimers();
}

/** Press the version, and let the clipboard write settle with no clock moving. */
async function press(button: HTMLElement) {
  fireEvent.click(button);
  await act(async () => {
    for (let tick = 0; tick < 10; tick++) {
      await Promise.resolve();
    }
  });
}

const CORE_BUILD: BuildRelease = { kind: "core", release: CORE, revision: SHA };
const REMEDY = "Select the version below and copy it by hand.";

describe("copying the version", () => {
  it("copies the build's one report line", async () => {
    holdClock(CORE_BUILD);
    const clipboard = stubClipboard("accepts");
    const button = versionButton();
    await press(button);

    expect(button).toHaveAccessibleName(COPIED);
    expect(clipboard.written).toEqual([`Margince ${CORE} (commit ${SHA})`]);
  });

  it("says it copied, in the button and to a screen reader, for three seconds", async () => {
    holdClock(CORE_BUILD);
    stubClipboard("accepts");
    const button = versionButton();
    await press(button);

    expect(button).toHaveAccessibleName(COPIED);
    expect(screen.getByRole("status")).toHaveTextContent(COPIED);

    act(() => {
      vi.advanceTimersByTime(2999);
    });
    expect(button).toHaveAccessibleName(COPIED);
    act(() => {
      vi.advanceTimersByTime(1);
    });
    expect(button).toHaveAccessibleName(`${CORE} · 0123456${HINT}`);
    expect(screen.getByRole("status")).toHaveTextContent("");
  });

  it("hands over the whole line to copy by hand when the clipboard refuses", async () => {
    holdClock(CORE_BUILD);
    stubClipboard("refuses");
    await press(versionButton());

    expect(screen.getByText(REMEDY)).toBeVisible();
    expect(screen.getByRole("textbox", { name: "Version" })).toHaveTextContent(
      `Margince ${CORE} (commit ${SHA})`,
    );
  });

  it("drops the copied claim when a second press fails", async () => {
    holdClock(CORE_BUILD);
    const accepting = stubClipboard("accepts");
    const button = versionButton();
    await press(button);
    expect(button).toHaveAccessibleName(COPIED);

    accepting.restore();
    stubClipboard("refuses");
    await press(button);

    expect(screen.getByText(REMEDY)).toBeVisible();
    expect(button).toHaveAccessibleName(`${CORE} · 0123456${HINT}`);
    expect(screen.getByRole("status")).toHaveTextContent("");
  });
});

describe("the license, the copyright and the links", () => {
  it("are rows under the version, in one column of values", () => {
    renderAbout({ kind: "unstamped" });
    expect(facts().map(([term]) => term)).toEqual([
      "Version",
      "License",
      "Copyright",
      "Links",
    ]);
    expect(facts()[2]).toEqual(["Copyright", "© 2026 Gradion"]);
  });

  it("names the license, when it converts, and where its terms are", () => {
    renderAbout({ kind: "unstamped" });
    const license = screen.getByRole("link", {
      name: /^Business Source License 1\.1/,
    });
    expect(license).toHaveAttribute(
      "href",
      "https://github.com/margince/margince/blob/main/LICENSING.md",
    );
    expect(
      screen.getByText(
        "Each release converts to the Apache License 2.0 no later than two years after it ships.",
      ),
    ).toBeVisible();
  });

  it("opens each place in a new tab, and says so", () => {
    renderAbout({ kind: "unstamped" });
    const list = screen.getByRole("list");
    const expected = [
      ["Website", "https://margince.com/"],
      ["Source code", "https://github.com/margince/margince"],
      ["Release notes", "https://github.com/margince/margince/releases"],
      ["Third-party licenses", "/licenses/THIRD-PARTY-NOTICES.txt"],
    ];
    expect(within(list).getAllByRole("link")).toHaveLength(expected.length);
    for (const [label, href] of expected) {
      const link = within(list).getByRole("link", {
        name: `${label} (Opens in a new tab)`,
      });
      expect(link).toHaveAttribute("href", href);
      expect(link).toHaveAttribute("target", "_blank");
      expect(link).toHaveAttribute("rel", "noopener noreferrer");
    }
  });
});
