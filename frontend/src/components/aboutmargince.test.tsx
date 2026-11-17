// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import { stubClipboard } from "../design-system/clipboard-testing";
import { LocaleProvider } from "../i18n";
import type { BuildRelease } from "../lib/buildrelease";
import { AboutMargince } from "./aboutmargince";

const SHA = "0123456789abcdef0123456789abcdef01234567";

afterEach(cleanup);

function renderAbout(build: BuildRelease) {
  render(
    <LocaleProvider initial="en">
      <AboutMargince build={build} />
    </LocaleProvider>,
  );
}

/** Each term and its value, in the order the reader sees them. */
function facts(): string[][] {
  return [...document.querySelectorAll(".factlist-row")].map((row) => [
    row.querySelector("dt")?.textContent ?? "",
    row.querySelector("dd")?.textContent ?? "",
  ]);
}

async function copied(build: BuildRelease): Promise<readonly string[]> {
  const user = userEvent.setup();
  const clipboard = stubClipboard("accepts");
  renderAbout(build);
  await user.click(
    screen.getByRole("button", { name: "Copy version details" }),
  );
  await screen.findByRole("button", { name: "Copied" });
  return clipboard.written;
}

describe("the version block", () => {
  it.each<[string, BuildRelease, string[][]]>([
    [
      "a core build",
      { kind: "core", release: "2026.2.1", revision: SHA },
      [
        ["Version", "2026.2.1"],
        ["Commit", "0123456 (Opens in a new tab)"],
      ],
    ],
    [
      "an instance build, which names both repositories",
      { kind: "instance", instance: "v0.4.1", core: "v0.4.1", revision: SHA },
      [
        ["Instance", "v0.4.1"],
        ["Core", "v0.4.1"],
        ["Commit", "0123456"],
      ],
    ],
    [
      "an instance build without its own release",
      { kind: "instance", core: "v0.0.5", revision: SHA },
      [
        ["Core", "v0.0.5"],
        ["Commit", "0123456"],
      ],
    ],
    [
      "an unstamped build",
      { kind: "unstamped", revision: SHA },
      [
        ["Version", "Development build"],
        ["Commit", "0123456"],
      ],
    ],
    [
      "an unstamped build without a revision",
      { kind: "unstamped" },
      [["Version", "Development build"]],
    ],
    [
      "a revision of dev, which no build knows",
      { kind: "core", release: "2026.2.1", revision: "dev" },
      [["Version", "2026.2.1"]],
    ],
  ])("names %s", (_, build, expected) => {
    renderAbout(build);
    expect(facts().slice(0, expected.length)).toEqual(expected);
    expect(facts()[expected.length]?.[0]).toBe("License");
  });

  it("links a core build's commit to the core repository", () => {
    renderAbout({ kind: "core", release: "2026.2.1", revision: SHA });
    const commit = screen.getByRole("link", { name: /^0123456/ });
    expect(commit).toHaveAttribute(
      "href",
      `https://github.com/margince/margince/commit/${SHA}`,
    );
    expect(commit).toHaveAttribute("target", "_blank");
    expect(commit).toHaveAttribute("rel", "noopener noreferrer");
  });

  it.each<[string, BuildRelease]>([
    ["an instance build", { kind: "instance", core: "v0.0.5", revision: SHA }],
    ["an unstamped build", { kind: "unstamped", revision: SHA }],
  ])("leaves the commit of %s as text", (_, build) => {
    renderAbout(build);
    expect(screen.getByText("0123456").closest("a")).toBeNull();
  });
});

describe("the copied version", () => {
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
    "is one plain line for %s, with the full commit",
    async (_, build, line) => {
      expect(await copied(build)).toEqual([line]);
    },
  );
});

describe("the license and the links", () => {
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
    expect(screen.getByText("© 2026 Gradion")).toBeVisible();
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
