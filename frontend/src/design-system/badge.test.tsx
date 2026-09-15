/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { cleanup, render, screen } from "@testing-library/react";
import { Mail } from "lucide-react";
import { afterEach, describe, expect, it } from "vitest";
import { Badge } from "./atoms";

const here = dirname(fileURLToPath(import.meta.url));

// happy-dom resolves no custom properties, so the fills themselves are held by
// tokens.test.ts, which measures every soft and primary pair in both themes.
// What is testable here is the class contract those rules key on, and the
// order of what the pill draws.

afterEach(cleanup);

function badgeFor(label: string): HTMLElement {
  const badge = screen.getByText(label).closest(".badge");
  if (!(badge instanceof HTMLElement)) {
    throw new Error(`no .badge carries "${label}"`);
  }
  return badge;
}

describe("Badge", () => {
  it("draws a soft neutral pill when given no variant and no tone", () => {
    render(<Badge>Open</Badge>);
    expect(badgeFor("Open").className).toBe("badge");
  });

  it("treats an undefined tone as the neutral one", () => {
    render(<Badge tone={undefined}>Open</Badge>);
    expect(badgeFor("Open").className).toBe("badge");
  });

  it("names each tone, and adds badge-primary only for the solid fill", () => {
    const tones = ["accent", "success", "warn", "danger", "ai"] as const;
    render(
      <>
        {tones.map((tone) => (
          <Badge key={`soft-${tone}`} tone={tone}>
            {`soft ${tone}`}
          </Badge>
        ))}
        {tones.map((tone) => (
          <Badge key={`primary-${tone}`} variant="primary" tone={tone}>
            {`primary ${tone}`}
          </Badge>
        ))}
        <Badge variant="primary">primary default</Badge>
      </>,
    );
    for (const tone of tones) {
      expect(badgeFor(`soft ${tone}`).className).toBe(`badge badge-${tone}`);
      expect(badgeFor(`primary ${tone}`).className).toBe(
        `badge badge-primary badge-${tone}`,
      );
    }
    expect(badgeFor("primary default").className).toBe("badge badge-primary");
  });

  it("draws the icon before the label and hides it from assistive tech", () => {
    render(
      <Badge tone="success" icon={Mail}>
        Replied
      </Badge>,
    );
    const badge = badgeFor("Replied");
    const [glyph, label] = [...badge.children];
    expect(glyph?.tagName.toLowerCase()).toBe("svg");
    expect(glyph).toHaveAttribute("aria-hidden", "true");
    expect(label).toHaveClass("badge-label");
    expect(label).toHaveTextContent(/^Replied$/);
    expect(badge.children).toHaveLength(2);
  });

  it("holds the whole label in the one span that truncates", () => {
    render(<Badge>extensions/acme/routes/partner-portal/settings</Badge>);
    const label = screen.getByText(
      "extensions/acme/routes/partner-portal/settings",
    );
    expect(label).toHaveClass("badge-label");
    expect(label.parentElement).toHaveClass("badge");
  });

  it("marks an ai badge with Sparkles in both variants, and nothing else", () => {
    // A tone that arrives as an expression may still be ai.
    const arriving: readonly ("ai" | "success")[] = ["ai"];
    render(
      <>
        <Badge tone="ai">Drafted</Badge>
        <Badge variant="primary" tone="ai">
          Proposed
        </Badge>
        {/* @ts-expect-error an ai badge's glyph is not the caller's to choose */}
        <Badge tone="ai" icon={Mail}>
          Chosen
        </Badge>
        {/* @ts-expect-error nor may it breathe: provenance is not a live status */}
        <Badge tone="ai" live>
          Breathing
        </Badge>
        {/* @ts-expect-error a tone that may resolve to ai takes no dot either */}
        <Badge tone={arriving[0]} live>
          Arriving
        </Badge>
      </>,
    );
    for (const label of [
      "Drafted",
      "Proposed",
      "Chosen",
      "Breathing",
      "Arriving",
    ]) {
      const [glyph, text, ...rest] = [...badgeFor(label).children];
      expect(glyph, label).toHaveClass("lucide-sparkles");
      expect(glyph, label).toHaveAttribute("aria-hidden", "true");
      expect(text, label).toHaveClass("badge-label");
      expect(rest, label).toEqual([]);
    }
  });

  it("marks a live status with a leading dot, and an ordinary one without", () => {
    render(
      <>
        <Badge tone="success" live>
          Live
        </Badge>
        <Badge tone="success">Closed</Badge>
      </>,
    );
    const dot = badgeFor("Live").firstElementChild;
    expect(dot).toHaveClass("badge-live-dot");
    expect(dot).toHaveAttribute("aria-hidden");
    expect(badgeFor("Closed").querySelector(".badge-live-dot")).toBeNull();
  });

  it("refuses an icon and a live dot together, and the retired quiet prop", () => {
    render(
      <>
        {/* @ts-expect-error the leading slot holds one mark, not two */}
        <Badge icon={Mail} live>
          Both
        </Badge>
        {/* @ts-expect-error quiet is gone: soft is the column spelling */}
        <Badge quiet>Quiet</Badge>
      </>,
    );
    expect(badgeFor("Quiet")).not.toHaveClass("badge-quiet");
  });

  // happy-dom applies no stylesheet, so the look is held where it is written.
  // A soft badge draws a hairline in its tone and a primary one reserves the
  // same edge transparent, so mixed variants share a height; nothing else draws
  // an edge. The type is stated rather than inherited, so an uppercase, tracked
  // or mono parent cannot turn a badge into a kicker.
  describe("its stylesheet", () => {
    const sheet = readFileSync(join(here, "atoms.css"), "utf8").replace(
      /\/\*[\s\S]*?\*\//g,
      "",
    );
    const rules = [...sheet.matchAll(/([^{}]*)\{([^{}]*)\}/g)]
      .filter(([, selector]) => /\.badge\b/.test(selector))
      .map(([, selector, body]) => ({
        selector: selector.trim(),
        declarations: new Map(
          body
            .split(";")
            .map((line) => line.split(":").map((part) => part.trim()))
            .filter((pair): pair is [string, string] => pair.length === 2),
        ),
      }));
    const declared = (selector: string, name: string) =>
      rules.find((rule) => rule.selector === selector)?.declarations.get(name);

    it("reads the badge rules it is pointed at", () => {
      expect(rules.length).toBeGreaterThan(5);
    });

    it("edges every soft tone in its own token, and primary in transparent", () => {
      expect(declared(".badge", "border")).toBe(
        "1px solid var(--borderSubtle)",
      );
      const edges = {
        ".badge-success": "var(--successBorder)",
        ".badge-warn": "var(--warnBorder)",
        ".badge-danger": "var(--dangerBorder)",
        ".badge-ai": "var(--aiMed)",
        ".badge-accent": "var(--accentMed)",
        ".badge-primary": "transparent",
      };
      for (const [selector, colour] of Object.entries(edges)) {
        expect(declared(selector, "border-color"), selector).toBe(colour);
      }
      const stray = rules.flatMap(({ selector, declarations }) =>
        [...declarations.keys()]
          .filter((name) =>
            /^(border(?!-radius)|outline|box-shadow)/.test(name),
          )
          .filter((name) =>
            name === "border"
              ? selector !== ".badge"
              : !(name === "border-color" && selector in edges),
          )
          .map((name) => `${selector} { ${name} }`),
      );
      expect(stray).toEqual([]);
    });

    it("paints every ground as one plain colour, never a gradient", () => {
      const layered = rules.flatMap(({ selector, declarations }) =>
        [...declarations]
          .filter(
            ([name, value]) =>
              /^background/.test(name) && /gradient|,/.test(value),
          )
          .map(([name, value]) => `${selector} { ${name}: ${value} }`),
      );
      expect(layered).toEqual([]);
    });

    it("sets its label in sentence case at normal tracking, and nowhere else", () => {
      const resets: Record<string, string[]> = {
        "text-transform": ["none"],
        "letter-spacing": ["normal", "0", "var(--tracking-normal)"],
      };
      const shouted = rules.flatMap(({ selector, declarations }) =>
        Object.entries(resets)
          .filter(([name, allowed]) => {
            const value = declarations.get(name);
            return value !== undefined && !allowed.includes(value);
          })
          .map(
            ([name]) => `${selector} { ${name}: ${declarations.get(name)} }`,
          ),
      );
      expect(shouted).toEqual([]);
    });

    // A badge in an eyebrow heading or inside a code sample reads as the badge
    // beside it everywhere else: the face and the slope are the badge's own,
    // and the floor is what gives a row of mixed variants one height.
    it("states the face, the slope and the floor rather than inheriting them", () => {
      expect(
        Object.fromEntries(
          ["font-family", "font-style", "min-block-size"].map((name) => [
            name,
            declared(".badge", name),
          ]),
        ),
      ).toEqual({
        "font-family": "var(--f-body)",
        "font-style": "normal",
        "min-block-size": "20px",
      });
    });

    // A container that must not squeeze its badge (a panel's head band) says
    // so at zero specificity; a `flex` here would override it on sheet order.
    it("caps itself at its container and leaves shrinking to the container", () => {
      const badge = rules.find(({ selector }) => selector === ".badge");
      const label = rules.find(({ selector }) => selector === ".badge-label");
      expect(badge?.declarations.get("box-sizing")).toBe("border-box");
      expect(badge?.declarations.get("max-inline-size")).toBe("100%");
      expect(badge?.declarations.get("min-inline-size")).toBe("0");
      expect([...(badge?.declarations.keys() ?? [])]).not.toContain("flex");
      expect(label?.declarations.get("text-overflow")).toBe("ellipsis");
      expect(label?.declarations.get("min-inline-size")).toBe("0");
    });
  });
});
