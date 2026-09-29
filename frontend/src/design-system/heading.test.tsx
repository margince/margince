/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";
import { readFileSync } from "node:fs";
import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import { render } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { extensionLayers, filesMatching } from "../../scripts/lib/source-tree";
import { type CssRule, rulesIn, withoutComments } from "../testing/css";
import { Heading, type HeadingSize } from "./heading";

const here = dirname(fileURLToPath(import.meta.url));
const sheet = readFileSync(join(here, "heading.css"), "utf8");
const srcDir = join(here, "..");
const sheets = filesMatching(srcDir, /\.css$/).concat(
  extensionLayers(join(srcDir, "..", "..", "extensions")).flatMap((layer) =>
    filesMatching(layer, /\.css$/),
  ),
);

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

// Split at depth 0 only, so `:where(.a, .b)` and `[x="a b"]` stay whole.
function splitTopLevel(text: string, separator: RegExp): string[] {
  const parts: string[] = [];
  let depth = 0;
  let from = 0;
  for (let i = 0; i < text.length; i++) {
    if ("([".includes(text[i])) depth++;
    else if (")]".includes(text[i])) depth--;
    else if (depth === 0 && separator.test(text[i])) {
      parts.push(text.slice(from, i));
      from = i + 1;
    }
  }
  parts.push(text.slice(from));
  return parts.map((part) => part.trim()).filter(Boolean);
}

// `&` stands for the parent; a nested selector without one is its descendant.
function resolvedSelectors(rule: CssRule): string[] {
  return [...rule.parents, rule.selector].reduce<string[]>(
    (outer, list) =>
      splitTopLevel(list, /,/).flatMap((inner) =>
        outer.length === 0
          ? [inner]
          : outer.map((parent) =>
              inner.includes("&")
                ? inner.replaceAll("&", parent)
                : `${parent} ${inner}`,
            ),
      ),
    [],
  );
}

type MarginDeclaration = { selector: string; value: string; line: number };

function marginDeclarations(css: string): MarginDeclaration[] {
  return rulesIn(css).flatMap((rule) =>
    [
      ...rule.body.matchAll(/(?:^|[;{])\s*margin[a-z-]*\s*:\s*([^;]+)/gi),
    ].flatMap(([, value]) =>
      resolvedSelectors(rule).map((selector) => ({
        selector,
        value: value.trim(),
        line: rule.line,
      })),
    ),
  );
}

// A margin whose subject's only class is `.heading` races every caller's rule.
function weightedHeadingMargins(css: string): string[] {
  return marginDeclarations(css)
    .filter(({ selector }) => {
      const subject = splitTopLevel(selector, /[\s>+~]/).at(-1) ?? "";
      const classes = new Set(
        subject
          .replaceAll(/:[\w-]+\((?:[^()]|\([^()]*\))*\)|\[[^\]]*\]/g, "")
          .match(/\.[\w-]+/g),
      );
      return classes.size === 1 && classes.has(".heading");
    })
    .map(({ selector, line }) => `${selector} (line ${line})`);
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

  // Zero, because zero is how the UA's own heading margin is refused.
  it("keeps the reset weightless, so a caller's rule sets the margin", () => {
    expect(
      withoutComments(sheet),
      "an at-rule in heading.css hides a rule from this census",
    ).not.toMatch(/@(media|supports|layer|scope|container)\b/i);
    expect(marginDeclarations(sheet)).toEqual([
      { selector: ":where(.heading)", value: "0", line: expect.any(Number) },
    ]);
  });

  it("leaves every heading's margin to its caller, in every sheet", () => {
    expect(
      sheets.length,
      "the stylesheet walk came back small",
    ).toBeGreaterThan(100);
    expect(sheets).toContain(join(here, "heading.css"));
    const found = sheets.flatMap((file) =>
      weightedHeadingMargins(readFileSync(file, "utf8")).map(
        (hit) => `${relative(srcDir, file)}: ${hit}`,
      ),
    );
    expect(
      found,
      `a margin on .heading at class weight: ${found.join(", ")}`,
    ).toEqual([]);
  });

  it("tells a margin on a bare heading from a caller's own", () => {
    expect(weightedHeadingMargins(".heading { margin: 0 }")).toEqual([
      ".heading (line 1)",
    ]);
    expect(
      weightedHeadingMargins(
        '.panel h2.heading[data-size="large"] { MARGIN-BOTTOM: 4px }',
      ),
    ).toEqual(['.panel h2.heading[data-size="large"] (line 1)']);
    expect(weightedHeadingMargins(".panel { .heading { margin: 0 } }")).toEqual(
      [".panel .heading (line 1)"],
    );
    expect(
      weightedHeadingMargins(":where(.lead), .heading { margin: 0 }"),
    ).toEqual([".heading (line 1)"]);
    expect(weightedHeadingMargins(":where(.heading) { margin: 0 }")).toEqual(
      [],
    );
    expect(
      weightedHeadingMargins(".heading.modal-title { margin-bottom: 4px }"),
    ).toEqual([]);
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
