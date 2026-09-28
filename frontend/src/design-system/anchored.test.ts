/** @vitest-environment happy-dom */
import { cleanup, renderHook } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { useAnchoredToTrigger } from "./anchored";

afterEach(cleanup);

// A panel is CAPPED to the room on the side it opened toward and scrolls inside
// itself past that. So the height it reports is the height it was GIVEN, and
// asking that is how a popover near the fold came to hide its own button: it
// opened downward into a hundred pixels, reported a hundred pixels, and the
// test "does it fit below?" was then trivially true however much it held.
//
// Stated over the two heights an element carries, because jsdom and happy-dom
// both give every element a zero-sized rectangle — there is no layout to
// observe, so the rule has to be asserted over the measurements themselves.
function panelOf(wants: number, given: number) {
  const el = document.createElement("div");
  Object.defineProperty(el, "scrollHeight", {
    value: wants,
    configurable: true,
    writable: true,
  });
  Object.defineProperty(el, "offsetHeight", {
    value: given,
    configurable: true,
  });
  Object.defineProperty(el, "offsetWidth", { value: 200, configurable: true });
  document.body.appendChild(el);
  return { current: el };
}

function triggerNear(top: number) {
  const el = document.createElement("button");
  el.getBoundingClientRect = () => new DOMRect(0, top, 100, 30);
  document.body.appendChild(el);
  return { current: el };
}

it("places a clamped panel by what it holds, not by the cap it was given", () => {
  const viewport = 800;
  vi.stubGlobal("innerHeight", viewport);
  vi.stubGlobal("innerWidth", 1200);
  // A trigger sitting 100px of usable room off the bottom, with 654px above it.
  // The panel holds 300px of content and has ALREADY been clamped to those 100
  // — which is the state that traps it: asked its offsetHeight it answers 100,
  // "does 100 fit in 100?" is yes, and it re-decides to open downward every
  // time. The trap needs the given height to equal the room exactly, which is
  // precisely what a previous pass's clamp produces.
  const trigger = triggerNear(662);
  const panel = panelOf(300, 100);

  const { result } = renderHook(() =>
    useAnchoredToTrigger(true, trigger, panel, "start"),
  );

  // Upward: the panel ends at or above the trigger rather than running off the
  // bottom edge with its actions past it.
  expect(result.current.top).toBeLessThan(662);
  // And it is capped to the room it actually opened into, which is the roomy
  // side — not to the 30px it had been squeezed into.
  expect(result.current.maxHeight).toBeGreaterThan(300);
});

it("still hangs a panel that fits below from its trigger", () => {
  vi.stubGlobal("innerHeight", 800);
  vi.stubGlobal("innerWidth", 1200);
  const trigger = triggerNear(100);
  const panel = panelOf(200, 200);

  const { result } = renderHook(() =>
    useAnchoredToTrigger(true, trigger, panel, "start"),
  );

  // Below the trigger's own bottom edge, which is 130.
  expect(result.current.top).toBeGreaterThan(130);
});

// A popover whose content ARRIVES while it is already open — this one fetches
// a register verdict — grows under a placement decided before it existed. The
// first pass runs against a panel this hook has capped to 0, so without a
// re-measure every such panel is placed for a height it does not have.
it("places the panel again when its own content changes size", () => {
  vi.stubGlobal("innerHeight", 800);
  vi.stubGlobal("innerWidth", 1200);
  let grow: (() => void) | undefined;
  let observed: Element | undefined;
  vi.stubGlobal(
    "ResizeObserver",
    class {
      constructor(fire: () => void) {
        grow = fire;
      }
      observe(el: Element) {
        observed = el;
      }
      disconnect() {}
    },
  );

  const trigger = triggerNear(662);
  // Empty when it opens, which is what a panel awaiting a fetch measures as.
  const panel = panelOf(0, 0);
  const { result, rerender } = renderHook(() =>
    useAnchoredToTrigger(true, trigger, panel, "start"),
  );

  // Nothing to place yet, so it hangs below — correctly, for an empty panel.
  expect(result.current.top).toBeGreaterThan(692);
  expect(observed).toBe(panel.current);

  // The verdict arrives and the panel now wants more than the room below it.
  Object.defineProperty(panel.current, "scrollHeight", {
    value: 300,
    configurable: true,
  });
  grow?.();
  rerender();

  expect(result.current.top).toBeLessThan(662);
});

// The panel ref before React has attached it. The hook runs on open and the
// portal may not have committed, so this is a real first frame rather than a
// defensive branch — and a placement computed from a null panel must not throw
// the popover away before it ever renders.
it("places nothing on a panel that is not in the DOM yet", () => {
  vi.stubGlobal("innerHeight", 800);
  vi.stubGlobal("innerWidth", 1200);
  // Both refs hoisted: a fresh object per render is a new dependency every
  // time, and the effect would re-run forever placing a panel that never is.
  const trigger = triggerNear(100);
  const panel: { current: HTMLElement | null } = { current: null };
  const { result } = renderHook(() =>
    useAnchoredToTrigger(true, trigger, panel, "start"),
  );

  // Below the trigger, which is where an unmeasured panel belongs: it is the
  // answer a panel of no height deserves, and the ResizeObserver corrects it
  // the moment there is something to measure.
  expect(result.current.top).toBeGreaterThan(130);
});

// Content added to a panel that is ALREADY at its cap.
//
// The box does not move — that is what a cap means — so a ResizeObserver
// watching it never fires, and the placement that capped the panel stands
// however much arrives afterwards. This is the same trap offsetHeight sets,
// reached from the other side: the panel's own measurement stops reporting
// growth at exactly the point where growth is the thing that should flip it.
it("places the panel again when capped content grows without moving its box", () => {
  vi.stubGlobal("innerHeight", 800);
  vi.stubGlobal("innerWidth", 1200);
  let contentChanged: (() => void) | undefined;
  vi.stubGlobal(
    "ResizeObserver",
    class {
      observe() {}
      disconnect() {}
    },
  );
  // The callback is exposed by observe(), not by the constructor: a stub that
  // hands it over on construction passes whether or not the hook ever asks to
  // be told, which is the wiring this test exists to hold.
  vi.stubGlobal(
    "MutationObserver",
    class {
      private fire: () => void;
      constructor(fire: () => void) {
        this.fire = fire;
      }
      observe() {
        contentChanged = this.fire;
      }
      disconnect() {}
    },
  );

  const trigger = triggerNear(662);
  // Already capped to the 100px below it, and holding exactly that much.
  const panel = panelOf(100, 100);
  const { result, rerender } = renderHook(() =>
    useAnchoredToTrigger(true, trigger, panel, "start"),
  );
  expect(result.current.top).toBeGreaterThan(692);

  // More arrives. offsetHeight does NOT move, because the cap holds the box.
  Object.defineProperty(panel.current, "scrollHeight", {
    value: 300,
    configurable: true,
  });
  contentChanged?.();
  rerender();

  expect(result.current.top).toBeLessThan(662);
});
