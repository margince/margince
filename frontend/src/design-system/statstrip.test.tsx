/** @vitest-environment jsdom */
import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { StatStrip } from "./statstrip";

const here = dirname(fileURLToPath(import.meta.url));

function stripCss(): string {
  return readFileSync(join(here, "statstrip.css"), "utf8");
}

afterEach(cleanup);

// A slot may be a control, and a strip of filters is what made that true. What
// is held here is the consequence nobody looking at the markup would see: the
// plate CLIPS. It is `overflow: hidden` with a corner radius, so anything a
// slot paints at its own edge is trimmed by that clip, and a rectangular
// selection ring lost its corners to the plate's curve and read as a broken box.
//
// Read from the sheet rather than from a rendered element: jsdom computes no
// box-shadow and no radius, so a DOM assertion here could only ever prove the
// class name it already asked for.
describe("statstrip.css draws a selected slot without asking the plate for room", () => {
  it("marks the pressed slot with a tint and an inset line, never a ring", () => {
    const pressed = /\.stat-strip > \[aria-pressed="true"\]\s*\{([^}]*)\}/.exec(
      stripCss(),
    );
    expect(pressed).not.toBeNull();
    const body = pressed?.[1] ?? "";
    // The tint says selected at a glance; the line is what separates it from
    // `--bgHover` for a reader who is also pointing at it.
    expect(body).toMatch(/background:\s*var\(--accentLight\)/);
    expect(body).toMatch(/inset 0 -2px 0 var\(--accent\)/);
    // A ring is exactly what the clip breaks. `inset 0 0 0 <n>px` is its
    // spelling, and it must not come back.
    expect(body).not.toMatch(/inset 0 0 0/);
  });

  it("gives the corner slots the plate's own curve", () => {
    const css = stripCss();
    // Without this a fill painted in a corner slot squares off the plate's
    // curve, which is visible the moment any slot is pressed or hovered. One
    // pixel tighter than the plate's radius, because the plate's border takes
    // that pixel.
    expect(css).toMatch(
      /\.stat-strip > \*:first-child\s*\{[^}]*border-start-start-radius:\s*calc\(var\(--r-md\) - 1px\)/,
    );
    expect(css).toMatch(
      /\.stat-strip > \*:last-child\s*\{[^}]*border-start-end-radius:\s*calc\(var\(--r-md\) - 1px\)/,
    );
  });
});

// The dividers belong to the PLATE now, not to the slots. A slot that drew its
// own left rule was only correct while the leading slot of every row sat at the
// plate's edge for the clip to eat it — true when the slot count divides the
// column count, and false the first time a record had four readings.
describe("statstrip.css draws its rules from the plate", () => {
  it("rules the gaps rather than the slots", () => {
    const css = stripCss();
    const plate = /\.stat-strip\s*\{([^}]*)\}/.exec(css)?.[1] ?? "";
    expect(plate).toMatch(/gap:\s*1px/);
    expect(plate).toMatch(/background:\s*var\(--borderSubtle\)/);
    // The old mechanism: a rule painted one pixel outside each slot. Its return
    // brings the orphaned seam back with it.
    expect(css).not.toMatch(/-1px 0 0 var\(--borderSubtle\)/);
  });
});

const labels = (count: number) =>
  Array.from({ length: count }, (_, index) => `Reading ${index + 1}`);

function Strip({ count }: Readonly<{ count: number }>) {
  return (
    <StatStrip testId="strip">
      {labels(count).map((label) => (
        <div key={label}>{label}</div>
      ))}
    </StatStrip>
  );
}

// The tail span is what removes the divisibility precondition, and it is the
// one part of the drawing the sheet cannot decide for itself: `span var()`
// resolves, `:nth-child(var() n + 1)` is dropped silently.
describe("StatStrip stretches its last slot over the rest of the row", () => {
  it("leaves an even row alone", () => {
    render(<Strip count={6} />);
    const strip = screen.getByTestId("strip");
    // Six over six, six over three and six over two all come out even, so no
    // slot is stretched at any width.
    expect(strip.style.getPropertyValue("--stat-strip-tail-6")).toBe("1");
    expect(strip.style.getPropertyValue("--stat-strip-tail-3")).toBe("1");
    expect(strip.style.getPropertyValue("--stat-strip-tail-2")).toBe("1");
  });

  it("stretches the orphan the deal page's four readings leave", () => {
    render(<Strip count={4} />);
    const strip = screen.getByTestId("strip");
    // Four over three is the reported case: the fourth slot sat alone under a
    // stub of rule. It now takes the whole row.
    expect(strip.style.getPropertyValue("--stat-strip-tail-3")).toBe("3");
    // Four over four and four over two are even.
    expect(strip.style.getPropertyValue("--stat-strip-tail-6")).toBe("1");
    expect(strip.style.getPropertyValue("--stat-strip-tail-2")).toBe("1");
  });

  it("stretches a five-slot strip, which has no divisor to fold to", () => {
    render(<Strip count={5} />);
    const strip = screen.getByTestId("strip");
    // The project readings. Five over three leaves the fifth slot beside the
    // fourth with one column spare; five over two leaves it alone with one.
    expect(strip.style.getPropertyValue("--stat-strip-tail-3")).toBe("2");
    expect(strip.style.getPropertyValue("--stat-strip-tail-2")).toBe("2");
    // At full width five slots make five columns, so nothing is left over.
    expect(strip.style.getPropertyValue("--stat-strip-tail-6")).toBe("1");
  });

  it("survives a strip whose slots all fell away", () => {
    render(<Strip count={0} />);
    const strip = screen.getByTestId("strip");
    // No slots means no row to finish. The span must still be a number: a
    // `span 0` or a `span NaN` is an invalid grid line and takes the whole
    // template down with it.
    expect(strip.style.getPropertyValue("--stat-strip-tail-3")).toBe("1");
  });
});
