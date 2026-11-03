/** @vitest-environment happy-dom */
import { cleanup, render } from "@testing-library/react";
import { useRef } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { usePublishedScrollerHeight } from "./scrollerheight";

// A banner that appears or goes changes the scroller's height without a
// render of the shell. The height follows the box, not the tree.

type Watcher = { fire: () => void; boxes: Element[] };

function stubObservers(): Watcher[] {
  const watchers: Watcher[] = [];
  vi.stubGlobal(
    "ResizeObserver",
    class {
      private readonly watching: Watcher;
      constructor(fire: () => void) {
        this.watching = { fire, boxes: [] };
        watchers.push(this.watching);
      }
      observe(box: Element) {
        this.watching.boxes.push(box);
      }
      disconnect() {
        this.watching.boxes = [];
      }
    },
  );
  return watchers;
}

function Scroller({ layout }: Readonly<{ layout: string }>) {
  const scroller = useRef<HTMLDivElement>(null);
  usePublishedScrollerHeight(scroller, layout);
  return <div className="scroll" data-testid="scroller" ref={scroller} />;
}

function heightOf(box: HTMLElement, height: number) {
  Object.defineProperty(box, "clientHeight", {
    configurable: true,
    value: height,
  });
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("the published scroller height", () => {
  it("follows the scroller as a banner shortens it", () => {
    const watchers = stubObservers();
    const { getByTestId } = render(<Scroller layout="workspace" />);
    const box = getByTestId("scroller");
    expect(box.style.getPropertyValue("--scrollerH")).toBe("0px");

    heightOf(box, 498);
    for (const watcher of watchers) {
      watcher.fire();
    }

    expect(box.style.getPropertyValue("--scrollerH")).toBe("498px");
  });

  it("stops watching a scroller the shell has swapped out", () => {
    const watchers = stubObservers();
    const { rerender } = render(<Scroller layout="workspace" />);

    rerender(<Scroller layout="railless" />);

    expect(watchers).toHaveLength(2);
    expect(watchers[0]?.boxes, "the first scroller is still watched").toEqual(
      [],
    );
    expect(watchers[1]?.boxes).toHaveLength(1);
  });
});
