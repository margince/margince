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
    const glyph = badge.firstElementChild;
    expect(glyph?.tagName.toLowerCase()).toBe("svg");
    expect(glyph).toHaveAttribute("aria-hidden", "true");
    expect(badge).toHaveTextContent(/^Replied$/);
    expect(badge.childNodes[badge.childNodes.length - 1]?.textContent).toBe(
      "Replied",
    );
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

  // The variants are fills and nothing else: an edge on a pill reads as a
  // control, and a tracked or uppercased label reads as a kicker.
  it("never gives a badge a border, a shadow edge or a transformed label", () => {
    const sheet = readFileSync(join(here, "atoms.css"), "utf8").replace(
      /\/\*[\s\S]*?\*\//g,
      "",
    );
    const rules = [...sheet.matchAll(/([^{}]*)\{([^{}]*)\}/g)].filter(
      ([, selector]) => /\.badge\b/.test(selector),
    );
    expect(rules.length).toBeGreaterThan(5);
    const offenders = rules
      .filter(([, , body]) =>
        /(?:^|[;\s])(border(?!-radius)[\w-]*|outline[\w-]*|box-shadow|text-transform|letter-spacing)\s*:/.test(
          body,
        ),
      )
      .map(([, selector]) => selector.trim());
    expect(offenders).toEqual([]);
  });
});
