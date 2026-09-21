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
    for (const rule of rulesIn(sheet)) {
      for (const [, value] of rule.body.matchAll(
        /(?:^|[;{])\s*margin[a-z-]*\s*:\s*([^;]+)/g,
      )) {
        expect(
          value.trim(),
          `${rule.selector} (heading.css:${rule.line})`,
        ).toBe("0");
      }
    }
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
