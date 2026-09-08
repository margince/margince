/** @vitest-environment jsdom */

import { act, cleanup, fireEvent, render } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { RailSaying } from "./agentrail";
import { plain, type SpokenLine } from "./ai-activity-lines";

// The rail's own line, split out of agentrail.test.tsx because that file is over
// the 1000-line ceiling this tree holds test files to.

const IDLE = "Nothing needs you";
const READING = "Reading companies";
const WRITING = "Saving the deal";

/** The sentence that names a record, which is the one that carries a link. */
const NAMED: SpokenLine = {
  before: "I'm pulling together what I know about ",
  subject: { name: "Acme", route: { screen: "companies" } },
  after: ".",
};

function live(container: HTMLElement): HTMLElement | null {
  return container.querySelector(".arline");
}

function leaving(container: HTMLElement): HTMLElement | null {
  return container.querySelector(".argone");
}

/**
 * The end of the fade, delivered by hand: jsdom runs no animations.
 *
 * BOTH spellings, and the reason is jsdom rather than the component. React picks
 * the animation event name by feature detection, and an environment that defines
 * no `AnimationEvent` — which jsdom does not — leaves it subscribed to the
 * vendor-prefixed name. A browser sends `animationend`; sending only that here
 * reaches no handler, and sending only the prefixed one would stop working the
 * day jsdom grows the constructor. The handler is idempotent, so both is safe.
 */
function endTheFade(layer: HTMLElement) {
  fireEvent.animationEnd(layer, { bubbles: true });
  fireEvent(layer, new Event("webkitAnimationEnd", { bubbles: true }));
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

// The preference the snap arm reads. jsdom's matchMedia answers `false` to
// everything, so saying otherwise means stubbing it — the listener included,
// because the hook subscribes for a preference that changes mid-session.
function stubReducedMotion() {
  vi.stubGlobal("matchMedia", (query: string) => ({
    matches: query.includes("prefers-reduced-motion"),
    media: query,
    onchange: null,
    addEventListener: () => undefined,
    removeEventListener: () => undefined,
    addListener: () => undefined,
    removeListener: () => undefined,
    dispatchEvent: () => false,
  }));
}

// The same preference as a SWITCH, for the one case where it changes while a
// fade is running. The stub above answers a fixed value; this one holds the
// listener the hook subscribes with, so flipping it delivers the `change` event
// a system setting would — which is the only way the component sees the
// preference arrive mid-sentence.
function reducedMotionSwitch(): { turnOn: () => void } {
  let on = false;
  const listeners = new Set<() => void>();
  vi.stubGlobal("matchMedia", (query: string) => {
    const watched = query.includes("prefers-reduced-motion");
    return {
      get matches() {
        return watched && on;
      },
      media: query,
      onchange: null,
      addEventListener: (_: string, listen: () => void) => {
        if (watched) listeners.add(listen);
      },
      removeEventListener: (_: string, listen: () => void) => {
        listeners.delete(listen);
      },
      addListener: () => undefined,
      removeListener: () => undefined,
      dispatchEvent: () => false,
    };
  });
  return {
    turnOn: () => {
      on = true;
      for (const listen of listeners) listen();
    },
  };
}

describe("the rail's line changing", () => {
  it("mounts one sentence and nothing on its way out", () => {
    const { container } = render(<RailSaying line={plain(IDLE)} />);
    expect(live(container)?.textContent).toBe(IDLE);
    expect(leaving(container)).toBeNull();
  });

  it("holds the old sentence over the new one while the fade runs", () => {
    const { container, rerender } = render(<RailSaying line={plain(IDLE)} />);
    rerender(<RailSaying line={plain(READING)} />);
    // The live layer is the new sentence ALONE: everything that reads this rail
    // — the panel's caption cases, the record link — asks `.arline` for the one
    // line, and a box that answered with both would be answering with neither.
    expect(live(container)?.textContent).toBe(READING);
    expect(leaving(container)?.textContent).toBe(IDLE);
  });

  it("retires the outgoing sentence when its fade ends", () => {
    const { container, rerender } = render(<RailSaying line={plain(IDLE)} />);
    rerender(<RailSaying line={plain(READING)} />);
    const going = leaving(container);
    if (!going) throw new Error("no outgoing layer to retire");
    endTheFade(going);
    expect(leaving(container)).toBeNull();
    expect(live(container)?.textContent).toBe(READING);
  });

  it("says nothing about a re-render that carries the same words", () => {
    const { container, rerender } = render(<RailSaying line={plain(IDLE)} />);
    // A new object with the same sentence in it, which is what every read this
    // tab makes hands this block. Identity is the words, so this is not news.
    rerender(<RailSaying line={plain(IDLE)} />);
    expect(leaving(container)).toBeNull();
  });

  it("replaces the sentence on its way out rather than queueing another", () => {
    const { container, rerender } = render(<RailSaying line={plain(IDLE)} />);
    rerender(<RailSaying line={plain(READING)} />);
    // The working state ticks faster than the fade is long. The second change
    // arrives with the first fade still running and no animation end delivered.
    rerender(<RailSaying line={plain(WRITING)} />);
    expect(container.querySelectorAll(".argone").length).toBe(1);
    expect(leaving(container)?.textContent).toBe(READING);
    expect(live(container)?.textContent).toBe(WRITING);
  });

  it("keeps the outgoing record link out of the tab order and out of the name", () => {
    const { container, rerender } = render(<RailSaying line={NAMED} />);
    expect(live(container)?.querySelector("a")?.textContent).toBe("Acme");
    rerender(<RailSaying line={plain(IDLE)} />);
    const going = leaving(container);
    expect(going?.querySelector("a")?.textContent).toBe("Acme");
    // Announced once and reachable once: the link a reader can still see for a
    // third of a second is not one they can Tab to or hear.
    expect(going?.getAttribute("aria-hidden")).toBe("true");
    expect(going?.hasAttribute("inert")).toBe(true);
    expect(live(container)?.querySelector("a")).toBeNull();
  });

  it("snaps to the new sentence under reduced motion, with nothing to retire", () => {
    stubReducedMotion();
    const { container, rerender } = render(<RailSaying line={plain(IDLE)} />);
    rerender(<RailSaying line={plain(READING)} />);
    expect(live(container)?.textContent).toBe(READING);
    expect(leaving(container)).toBeNull();
  });

  it("retires the outgoing sentence when the preference turns on mid-fade", () => {
    const preference = reducedMotionSwitch();
    const { container, rerender } = render(<RailSaying line={plain(IDLE)} />);
    rerender(<RailSaying line={plain(READING)} />);
    expect(leaving(container)?.textContent).toBe(IDLE);
    act(() => {
      preference.turnOn();
    });
    // NOTHING ends this fade: `@media (prefers-reduced-motion: reduce)` in
    // agentrail.css sets `animation: none` on the outgoing layer, so the
    // `animationend` that normally retires it is never delivered. No fade is
    // sent here for that reason — a test that delivered one would prove the
    // handler works and say nothing about the case.
    expect(leaving(container)).toBeNull();
    expect(live(container)?.textContent).toBe(READING);
  });
});
