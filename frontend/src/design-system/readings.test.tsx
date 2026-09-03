/** @vitest-environment jsdom */
import { cleanup, render, screen } from "@testing-library/react";
import { Globe, MapPin } from "lucide-react";
import { afterEach, describe, expect, it } from "vitest";
import { BarList, Chip, Meter, Sparkline } from "./readings";

afterEach(cleanup);

// An optional chain collapses "the element is missing" and "the element does
// not carry that class" into the same `undefined`, so an assertion written
// through one can pass against a Meter that rendered nothing at all. Resolve
// the node first and fail on its absence, so what the assertion says is what it
// checks.
function meterBar(container: HTMLElement): HTMLElement {
  const bar = container.querySelector<HTMLElement>(".meterbar");
  if (!bar) {
    throw new Error("the Meter rendered no bar");
  }
  return bar;
}

function fillWidth(container: HTMLElement): string {
  const fill = meterBar(container).querySelector<HTMLElement>("span");
  if (!fill) {
    throw new Error("the Meter's bar rendered no fill");
  }
  return fill.style.width;
}

function polylinePoints(label: string): string | null {
  const line = screen
    .getByRole("img", { name: label })
    .querySelector("polyline");
  if (!line) {
    throw new Error(`the Sparkline "${label}" rendered no polyline`);
  }
  return line.getAttribute("points");
}

describe("Meter draws a proportion a reader can also hear", () => {
  it("carries the raw pair, not a percentage, on the ARIA node", () => {
    render(<Meter value={7} max={9} label="Dossier completeness" />);
    const meter = screen.getByRole("meter", { name: "Dossier completeness" });
    expect(meter.getAttribute("aria-valuenow")).toBe("7");
    expect(meter.getAttribute("aria-valuemax")).toBe("9");
  });

  it("fills the bar to the value's share of the max", () => {
    const { container } = render(<Meter value={1} max={4} label="Coverage" />);
    expect(fillWidth(container)).toBe("25%");
  });

  // A zero max is "nothing has been measured", not a division. It must read
  // as an empty bar rather than a NaN width the browser silently drops.
  it("draws an empty bar when nothing has been measured", () => {
    const { container } = render(<Meter value={0} max={0} label="Coverage" />);
    expect(fillWidth(container)).toBe("0%");
  });

  // A value beyond its max is a data fault, not a reason to draw a bar wider
  // than its track.
  it("clamps a value that overruns its max", () => {
    const { container } = render(<Meter value={12} max={9} label="Facts" />);
    expect(fillWidth(container)).toBe("100%");
  });

  it("colours a low-is-bad reading with its own tone", () => {
    const { container } = render(
      <Meter value={2} max={10} label="Payment" tone="danger" />,
    );
    expect(meterBar(container).classList.contains("meterbar-danger")).toBe(
      true,
    );
  });

  // No low-is-bad end: the bar stays a solid accent instead of fading toward
  // a colour that would read as a warning creeping in.
  it("draws a flat accent when the reading has no low-is-bad end", () => {
    const { container } = render(
      <Meter value={6} max={8} label="Growth fit" flat />,
    );
    expect(meterBar(container).classList.contains("meterbar-flat")).toBe(true);
  });

  // The size is the primitive's to name. Two screen sheets had reached into
  // `.meterbar` for the same 6px and the same zero margin, so the geometry had
  // three authors and would have drifted the first time any of them moved.
  it("takes its tighter geometry from the primitive, not the caller", () => {
    const { container } = render(
      <Meter value={6} max={8} label="Growth fit" flat dense />,
    );
    const bar = meterBar(container);
    expect(bar.classList.contains("meterbar-dense")).toBe(true);
    // dense is a size, so it does not spend one of the fill's choices: the
    // flat accent it was asked for is still there.
    expect(bar.classList.contains("meterbar-flat")).toBe(true);
  });

  it("draws the default geometry when no size is asked for", () => {
    const { container } = render(
      <Meter value={6} max={8} label="Growth fit" flat />,
    );
    expect(meterBar(container).classList.contains("meterbar-dense")).toBe(
      false,
    );
  });
});

describe("Sparkline is a glyph, not a chart", () => {
  it("draws one point per reading, scaled into the box", () => {
    render(<Sparkline points={[0, 10]} label="Days paid after due" />);
    // Low sits at the bottom inset, high at the top one; x spans the width.
    expect(polylinePoints("Days paid after due")).toBe("0.0,29.0 120.0,3.0");
  });

  // A flat series has no range to scale into. It reads as unchanged rather
  // than dividing by zero.
  it("draws a flat series down the middle", () => {
    render(<Sparkline points={[8, 8, 8]} label="Unchanged" />);
    expect(polylinePoints("Unchanged")).toBe("0.0,29.0 60.0,29.0 120.0,29.0");
  });

  // One point is a dot, and a dot reads as a flat trend — a claim a single
  // reading does not support.
  it("draws nothing from fewer than two points", () => {
    const { container } = render(<Sparkline points={[4]} label="One" />);
    expect(container.querySelector("svg")).toBeNull();
  });
});

describe("Chip is a fact, and a link when the fact has somewhere to go", () => {
  it("renders a plain chip with no destination", () => {
    render(<Chip icon={MapPin}>London, UK</Chip>);
    expect(screen.queryByRole("link")).toBeNull();
    expect(screen.getByText("London, UK")).toBeTruthy();
  });

  // These hrefs come from records, and a record's fields are typed by whoever
  // captured them. A javascript: href is script execution on click.
  it.each([
    "javascript:alert(1)",
    "data:text/html,<script>x</script>",
    "/local",
  ])("refuses %s as a destination but still shows the fact", (href) => {
    render(
      <Chip icon={Globe} href={href}>
        glazedfrog.example
      </Chip>,
    );
    expect(screen.queryByRole("link")).toBeNull();
    expect(screen.getByText("glazedfrog.example")).toBeTruthy();
  });

  it("opens an off-origin destination in a new tab without a referrer", () => {
    render(
      <Chip icon={Globe} href="https://glazedfrog.example">
        glazedfrog.example
      </Chip>,
    );
    const link = screen.getByRole("link", { name: "glazedfrog.example" });
    expect(link.getAttribute("target")).toBe("_blank");
    expect(link.getAttribute("rel")).toBe("noreferrer");
  });
});

describe("BarList", () => {
  const rows = [
    { key: "a", label: "Qualified", value: 40, amount: "40" },
    { key: "b", label: "Proposal", value: 10, amount: "10" },
  ] as const;

  function bars(container: HTMLElement): readonly HTMLElement[] {
    return [...container.querySelectorAll<HTMLElement>(".meterbar span")];
  }

  // The point of a list over N separate Meters: one denominator. Drawn against
  // itself each row fills its own track and the list says every stage is equal,
  // which is the opposite of what a ranking is for.
  it("draws every bar against one denominator, not against itself", () => {
    const { container } = render(<BarList rows={rows} label="Deals by stage" />);
    const [first, second] = bars(container);
    expect(first.style.width).toBe("100%");
    expect(second.style.width).toBe("25%");
  });

  // A caller's whole is the denominator where the rows do not reach it: four
  // stages of a hundred-deal pipeline must not each read as the whole pipeline.
  it("takes the caller's whole as the denominator when it exceeds every row", () => {
    const { container } = render(
      <BarList rows={rows} label="Deals by stage" max={80} />,
    );
    const [first, second] = bars(container);
    expect(first.style.width).toBe("50%");
    expect(second.style.width).toBe("12.5%");
  });

  // A max BELOW the largest row would pin several rows at the clamp, and a list
  // where three rows all read as full is one that has stopped comparing
  // anything. The larger of the two wins, so the shares stay honest.
  it("never lets a max below the largest row flatten the list", () => {
    const { container } = render(
      <BarList rows={rows} label="Deals by stage" max={5} />,
    );
    const [first, second] = bars(container);
    expect(first.style.width).toBe("100%");
    expect(second.style.width).toBe("25%");
    expect(first.style.width).not.toBe(second.style.width);
  });

  // The bars are aria-hidden, so the table is the ONLY thing a screen reader
  // gets. Every row's label and figure has to be in it or that reader is handed
  // a caption and nothing else.
  it("puts every row's label and amount in the table equivalent", () => {
    render(<BarList rows={rows} label="Deals by stage" />);
    const table = screen.getByRole("table", { name: "Deals by stage" });
    for (const row of rows) {
      expect(table.textContent).toContain(row.label);
      expect(table.textContent).toContain(row.amount);
    }
  });

  // Two rows may legitimately read the same. Keyed by display text one of them
  // would disappear, so the list keys on identity and both rows draw.
  it("draws two rows that share a label", () => {
    const sameName = [
      { key: "one", label: "Qualified", value: 3, amount: "3" },
      { key: "two", label: "Qualified", value: 1, amount: "1" },
    ] as const;
    const { container } = render(
      <BarList rows={sameName} label="Deals by stage" />,
    );
    expect(bars(container)).toHaveLength(2);
  });

  // An empty report is a real answer and must not be a crash: no rows means a
  // zero denominator, which is the division this would otherwise do.
  it("renders no bars and no NaN when there is nothing to rank", () => {
    const { container } = render(<BarList rows={[]} label="Deals by stage" />);
    expect(bars(container)).toHaveLength(0);
    expect(container.textContent).not.toContain("NaN");
  });
});
