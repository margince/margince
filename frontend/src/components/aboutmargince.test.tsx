// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { act, cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
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
      <AboutMargince build={build} surface="page" />
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
      { kind: "core", release: "2026.2.1", revision: SHA },
      "2026.2.1 · 0123456",
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
      "an instance build without its own release",
      { kind: "instance", core: CORE, revision: SHA },
      `core ${CORE} · 0123456`,
      undefined,
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
    [
      "a revision of dev, which no build knows",
      { kind: "core", release: "2026.2.1", revision: "dev" },
      "2026.2.1",
      undefined,
    ],
  ])("is one button for %s", (_, build, line, note) => {
    renderAbout(build);
    expect(versionButton()).toHaveAccessibleName(`${line}${HINT}`);
    const versionRow = document.querySelector(".factlist-row");
    expect(versionRow?.querySelector(".factlist-note")?.textContent).toBe(note);
  });

  it("links a core build's commit to the core repository, at the size of the text around it", () => {
    renderAbout({ kind: "core", release: "2026.2.1", revision: SHA });
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
    [
      "an instance build",
      { kind: "instance", core: "2026.2.1", revision: SHA },
    ],
    ["an unstamped build", { kind: "unstamped", revision: SHA }],
  ])("links no commit for %s", (_, build) => {
    renderAbout(build);
    expect(screen.queryByRole("link", { name: /commit/i })).toBeNull();
  });
});

describe("copying the version", () => {
  it.each<[string, BuildRelease, string]>([
    [
      "a core build",
      { kind: "core", release: "2026.2.1", revision: SHA },
      `Margince 2026.2.1 (commit ${SHA})`,
    ],
    [
      "an instance build",
      { kind: "instance", instance: "v0.4.1", core: "v0.0.5", revision: SHA },
      `Margince core v0.0.5, instance v0.4.1 (commit ${SHA})`,
    ],
    [
      "an instance build without its own release",
      { kind: "instance", core: "v0.0.5", revision: SHA },
      `Margince core v0.0.5 (commit ${SHA})`,
    ],
    [
      "an unstamped build",
      { kind: "unstamped", revision: SHA },
      `Margince development build (commit ${SHA})`,
    ],
    [
      "an unstamped build without a revision",
      { kind: "unstamped" },
      "Margince development build",
    ],
  ])(
    "copies one plain line for %s, with the full commit",
    async (_, build, line) => {
      const user = userEvent.setup();
      const clipboard = stubClipboard("accepts");
      renderAbout(build);
      await user.click(versionButton());
      await screen.findByRole("button", { name: COPIED });
      expect(clipboard.written).toEqual([line]);
    },
  );

  it("says it copied, in the button and to a screen reader, then shows the version again", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    stubClipboard("accepts");
    renderAbout({ kind: "core", release: "2026.2.1", revision: SHA });
    const button = versionButton();
    await user.click(button);

    expect(await screen.findByRole("button", { name: COPIED })).toBe(button);
    expect(screen.getByRole("status")).toHaveTextContent(COPIED);

    act(() => {
      vi.advanceTimersByTime(10_000);
    });
    expect(button).toHaveAccessibleName(`2026.2.1 · 0123456${HINT}`);
    expect(screen.getByRole("status")).toHaveTextContent("");
  });

  it("hands over the whole line to copy by hand when the clipboard refuses", async () => {
    const user = userEvent.setup();
    stubClipboard("refuses");
    renderAbout({ kind: "core", release: "2026.2.1", revision: SHA });
    await user.click(versionButton());

    expect(
      await screen.findByText("Select the version below and copy it by hand."),
    ).toBeVisible();
    expect(screen.getByRole("textbox", { name: "Version" })).toHaveTextContent(
      `Margince 2026.2.1 (commit ${SHA})`,
    );
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
