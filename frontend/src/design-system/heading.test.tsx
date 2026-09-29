/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";
import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { render } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { rulesIn } from "../testing/css";
import { Heading, type HeadingSize } from "./heading";

const here = dirname(fileURLToPath(import.meta.url));
const sheet = readFileSync(join(here, "heading.css"), "utf8");

// The element a size means with no `as`. This is the spec, written out: the
// component's table and this one are two statements of one rule on purpose,
// because a test that derived the answer from the component would agree with
// whatever the component did.
const DEFAULT_ELEMENT: Readonly<Record<HeadingSize, string>> = {
  xxlarge: "H1",
  xlarge: "H1",
  large: "H2",
  medium: "H3",
  small: "H4",
  xsmall: "H5",
  xxsmall: "H6",
};

// The sizes come from the TOKENS, not from a list kept here: `tokens.css` owns
// how many heading steps there are, so a token added or retired there shows up
// as a failure rather than as a step this file quietly stops covering.
function headingTokens(): string[] {
  const tokens = readFileSync(join(here, "tokens.css"), "utf8");
  return [...tokens.matchAll(/--fontHeading([A-Za-z]+)\s*:/g)]
    .map((match) => match[1])
    .filter((name, index, all) => all.indexOf(name) === index);
}

// Split at top-level commas only, so `:where(.a, .b)` stays one selector.
function selectorsIn(list: string): string[] {
  const selectors: string[] = [];
  let depth = 0;
  let from = 0;
  for (let i = 0; i < list.length; i++) {
    if ("([".includes(list[i])) depth++;
    else if (")]".includes(list[i])) depth--;
    else if (list[i] === "," && depth === 0) {
      selectors.push(list.slice(from, i));
      from = i + 1;
    }
  }
  selectors.push(list.slice(from));
  return selectors.map((selector) => selector.trim()).filter(Boolean);
}

function whollyWhere(selector: string): boolean {
  if (!selector.startsWith(":where(") || !selector.endsWith(")")) return false;
  let depth = 0;
  for (const char of selector.slice(":where(".length, -1)) {
    if (char === "(") depth++;
    if (char === ")") depth--;
    if (depth < 0) return false;
  }
  return depth === 0;
}

type MarginDeclaration = {
  selector: string;
  parents: string[];
  value: string;
  line: number;
};

function marginDeclarations(css: string): MarginDeclaration[] {
  return rulesIn(css).flatMap((rule) =>
    [
      ...rule.body.matchAll(/(?:^|[;{])\s*margin[a-z-]*\s*:\s*([^;]+)/g),
    ].flatMap(([, value]) =>
      selectorsIn(rule.selector).map((selector) => ({
        selector,
        parents: rule.parents,
        value: value.trim(),
        line: rule.line,
      })),
    ),
  );
}

function weightedMargins(css: string): string[] {
  return marginDeclarations(css)
    .filter(({ selector, parents }) =>
      [selector, ...parents.flatMap(selectorsIn)].some(
        (one) => !whollyWhere(one),
      ),
    )
    .map(
      ({ selector, parents, line }) =>
        `${[...parents, selector].join(" ")} (heading.css:${line})`,
    );
}

function elementOf(size: HeadingSize): string {
  const { container } = render(<Heading size={size}>Northwind</Heading>);
  const heading = container.querySelector(".heading");
  if (heading === null) {
    throw new Error(
      `<Heading size="${size}"> rendered nothing carrying .heading`,
    );
  }
  return heading.tagName;
}

describe("Heading", () => {
  it("covers every heading token in tokens.css and nothing else", () => {
    const sizes = headingTokens().map((name) => name.toLowerCase());
    expect(sizes.length).toBeGreaterThan(0);
    expect(sizes.sort()).toEqual(Object.keys(DEFAULT_ELEMENT).sort());
  });

  it("puts each size on the element its place in the outline implies", () => {
    for (const [size, tag] of Object.entries(DEFAULT_ELEMENT)) {
      expect(elementOf(size as HeadingSize), size).toBe(tag);
    }
  });

  it("lets `as` override the element the size implies", () => {
    const { container } = render(
      <Heading size="medium" as="h1">
        Northwind
      </Heading>,
    );
    const heading = container.querySelector(".heading");
    expect(heading?.tagName).toBe("H1");
    // The type is still the size's, which is the whole point of the two being
    // separate props: overriding the outline must not move the scale.
    expect(heading).toHaveAttribute("data-size", "medium");
  });

  it("forwards the attributes a caller labels it with", () => {
    const { container } = render(
      <Heading size="large" id="zone-title" className="panel-title" lang="de">
        Northwind
      </Heading>,
    );
    const heading = container.querySelector("#zone-title");
    expect(heading).toHaveClass("heading", "panel-title");
    expect(heading).toHaveAttribute("lang", "de");
  });

  // A screen that moves focus to the page title after a route change needs the
  // element itself; without a ref it reaches for a raw `<h1>` instead.
  it("hands the element back through ref", () => {
    const seen: (HTMLHeadingElement | null)[] = [];
    render(
      <Heading
        size="xlarge"
        tabIndex={-1}
        ref={(element) => {
          seen.push(element);
        }}
      >
        Northwind
      </Heading>,
    );
    expect(seen[0]?.tagName).toBe("H1");
    expect(seen[0]).toHaveAttribute("tabindex", "-1");
  });

  // A heading that brought its own margin would be a second opinion about the
  // gap above it, and the parent already has one. Zero is allowed because zero
  // is how the UA's own heading margin is refused.
  it("declares no margin of its own but the reset", () => {
    for (const { selector, value, line } of marginDeclarations(sheet)) {
      expect(value, `${selector} (heading.css:${line})`).toBe("0");
    }
  });

  it("keeps the reset weightless, so a caller's rule sets the margin", () => {
    const weighted = weightedMargins(sheet);
    expect(weighted, `not wrapped in :where(): ${weighted.join(", ")}`).toEqual(
      [],
    );
    expect(marginDeclarations(sheet)).toEqual([
      {
        selector: ":where(.heading)",
        parents: [],
        value: "0",
        line: expect.any(Number),
      },
    ]);
  });

  it("tells a weighted margin from a weightless one", () => {
    expect(weightedMargins(".heading { margin: 0 }")).toEqual([
      ".heading (heading.css:1)",
    ]);
    expect(
      weightedMargins(":where(.heading, .lead), .heading-lead { margin: 0 }"),
    ).toEqual([".heading-lead (heading.css:1)"]);
    expect(
      weightedMargins(".panel { :where(.heading) { margin: 0 } }"),
    ).toEqual([".panel :where(.heading) (heading.css:1)"]);
    expect(weightedMargins(":where(.heading) { margin: 0 }")).toEqual([]);
  });

  it("reads each size's type straight from its token", () => {
    const declared = new Map(
      rulesIn(sheet)
        .map((rule) => [rule.selector, rule.body] as const)
        .filter(([selector]) => selector.includes("data-size")),
    );
    for (const token of headingTokens()) {
      const size = token.toLowerCase();
      const body = declared.get(`.heading[data-size="${size}"]`);
      expect(body, `no rule sizes data-size="${size}"`).toBeDefined();
      expect(body).toMatch(
        new RegExp(`(^|[;{])\\s*font\\s*:\\s*var\\(--fontHeading${token}\\)`),
      );
    }
    // The sized rules and the tokens are the same set in both directions: a
    // rule naming a size no token backs would otherwise pass unread.
    expect(declared.size).toBe(headingTokens().length);
  });
});
