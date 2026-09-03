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
// is held here is where the selected state LANDS: on the reading pane inside
// the slot, as a wash and an edge, never as a ring around it. A ring around a
// rounded pane is a second box drawn on a page whose language refuses one.
//
// Read from the sheet rather than from a rendered element: jsdom computes no
// background and no border, so a DOM assertion here could only ever prove the
// class name it already asked for.
describe("statstrip.css draws a selected slot on the pane inside it", () => {
  it("marks the pressed slot's pane with the accent wash and edge, never a ring", () => {
    const pressed =
      /\.stat-strip > \[aria-pressed="true"\] > \.stat-card\s*\{([^}]*)\}/.exec(
        stripCss(),
      );
    expect(pressed).not.toBeNull();
    const body = pressed?.[1] ?? "";
    expect(body).toMatch(/background:\s*var\(--accentLight\)/);
    expect(body).toMatch(/border-color:\s*var\(--accentMed\)/);
    // A ring is a second box. `inset 0 0 0 <n>px` is its spelling, and it
    // must not come back.
    expect(stripCss()).not.toMatch(/inset 0 0 0/);
  });
});

// The row draws NO plate: each slot is a reading pane of its own and the air
// between them is the only divider. A background or a border on the row would
// put five panes inside a sixth, which is the one thing the language refuses.
describe("statstrip.css is a row of panes, not a plate of slots", () => {
  it("separates the slots by the space scale and paints nothing itself", () => {
    const css = stripCss();
    const row = /\.stat-strip\s*\{([^}]*)\}/.exec(css)?.[1] ?? "";
    expect(row).toMatch(/gap:\s*var\(--space-4\)/);
    expect(row).not.toMatch(/background/);
    expect(row).not.toMatch(/border/);
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
