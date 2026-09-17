/** @vitest-environment happy-dom */
import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

// jsdom applies no stylesheet, so every interaction test above passes whether
// or not these popups can actually be seen. This reads the stylesheet itself:
// the applied-filter row hosts the popups for its condition, its value and its
// delete step INSIDE its own box, so a rule that clips the row clips all three
// — the menus open, and the reader sees nothing to click.
describe("the applied filter row does not clip what it hosts", () => {
  // Every declaration the stylesheet makes under one exact selector, joined:
  // a rule moved elsewhere in the file still counts, a rule deleted does not.
  const declarationsFor = (selector: string) => {
    const css = readFileSync(
      join(dirname(fileURLToPath(import.meta.url)), "listtable.css"),
      "utf8",
    ).replace(/\/\*[\s\S]*?\*\//g, "");
    const blocks = css
      .split("}")
      .filter((block) => block.slice(0, block.indexOf("{")).trim() === selector)
      .map((block) => block.slice(block.indexOf("{") + 1));
    expect(blocks.length).toBeGreaterThan(0);
    return blocks.join("\n");
  };

  it("leaves its own overflow visible", () => {
    const block = declarationsFor(".lt-frow");
    expect(block).not.toContain("overflow: hidden");
    expect(block).toContain("overflow: visible");
  });

  // The row gave up its own clipping, so the rounding has to live on the
  // segments at each end. Without these the pill reads as a bare rectangle.
  // The row stands at --controlHeight and its segments take that height from
  // it; the more segment sits in a wrapper that centres instead of stretching,
  // so it has to claim the height itself or it is as tall as its glyph — and
  // axe then refuses the leads page for a 15px pointer target (WCAG 2.2 AA,
  // 2.5.8).
  it("fills the row with the more segment, like the segment beside it", () => {
    expect(declarationsFor(".lt-frow-seg")).toContain("height: 100%");
    expect(declarationsFor(".lt-frow-more")).toContain("height: 100%");
  });

  // The height above is a percentage of a wrapper the row stretches, and twice
  // now that chain has given way and left the target at the glyph's size. The
  // floor is what holds either way — 24px, WCAG 2.2 AA 2.5.8, which axe runs
  // over the leads page in both themes.
  it("floors the more segment at the pointer target size", () => {
    const block = declarationsFor(".lt-frow-more");
    expect(block).toContain("min-inline-size: var(--space-6)");
    expect(block).toContain("min-block-size: var(--space-6)");
  });

  it("rounds the segments at each of its ends", () => {
    const left = declarationsFor(".lt-frow > :first-child");
    expect(left).toContain("border-top-left-radius: var(--r-full)");
    expect(left).toContain("border-bottom-left-radius: var(--r-full)");
    const right = declarationsFor(".lt-frow-more");
    expect(right).toContain("border-top-right-radius: var(--r-full)");
    expect(right).toContain("border-bottom-right-radius: var(--r-full)");
  });
});
