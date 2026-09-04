import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

// The motes layer is a positioned canvas sized every frame from its own box,
// times the device pixel ratio. A canvas is a replaced element, so `inset: 0`
// alone does not stretch it — its pixel buffer becomes its box, and on a retina
// screen the two feed each other until the page is one white canvas. The
// stylesheet has to state both dimensions, and this holds it to that: the
// failure is invisible in jsdom and on any headless run at a ratio of one, so
// nothing else in the tree would notice the two lines going missing.
describe("the crawl's motes layer", () => {
  it("pins both of its dimensions, so its buffer can never size its box", () => {
    const css = readFileSync(
      new URL("./crawl-canvas.css", import.meta.url),
      "utf8",
    );
    const rule = css.slice(css.indexOf(".crawl-motes {"));
    const block = rule.slice(0, rule.indexOf("}"));
    expect(block).toMatch(/position:\s*fixed/);
    expect(block).toMatch(/\bwidth:\s*100%/);
    expect(block).toMatch(/\bheight:\s*100%/);
  });
});
