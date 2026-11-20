// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion
/** @vitest-environment happy-dom */

import { afterEach, describe, expect, it } from "vitest";
import { POINTER_OVER_ATTRIBUTE, startPointerTarget } from "./pointertarget";

const MOUSE = 1;
const TOUCH = 2;

const marked = () =>
  Array.from(document.querySelectorAll(`[${POINTER_OVER_ATTRIBUTE}]`)).map(
    (element) => element.id,
  );

function element(id: string): Element {
  const found = document.getElementById(id);
  if (found === null) throw new Error(`the fixture drew no #${id}`);
  return found;
}

function enter(target: Element, pointerId = MOUSE) {
  target.dispatchEvent(
    new PointerEvent("pointerover", { bubbles: true, pointerId }),
  );
}

function leave(from: Element, to: Element | null, pointerId = MOUSE) {
  from.dispatchEvent(
    new PointerEvent("pointerout", {
      bubbles: true,
      pointerId,
      relatedTarget: to,
    }),
  );
}

describe("startPointerTarget", () => {
  let stop = () => {};
  afterEach(() => {
    stop();
    document.body.replaceChildren();
  });

  it("marks only the innermost element the pointer is over", () => {
    document.body.innerHTML =
      '<div id="board"><section id="stage"><a id="card">x</a></section></div>';
    stop = startPointerTarget();

    enter(element("card"));
    expect(marked()).toEqual(["card"]);
    enter(element("stage"));
    expect(marked()).toEqual(["stage"]);
  });

  it("clears the mark when the pointer leaves the window, and when stopped", () => {
    document.body.innerHTML = '<div id="pane"><p id="line">x</p></div>';
    stop = startPointerTarget();

    enter(element("line"));
    leave(element("line"), element("pane"));
    expect(marked()).toEqual(["line"]);
    leave(element("line"), null);
    expect(marked()).toEqual([]);

    enter(element("pane"));
    stop();
    expect(marked()).toEqual([]);
    enter(element("line"));
    expect(marked()).toEqual([]);
  });

  it("keeps a resting mouse's mark when a touch comes and goes", () => {
    document.body.innerHTML =
      '<div id="list"><p id="row">x</p></div><p id="other">y</p>';
    stop = startPointerTarget();

    enter(element("list"), MOUSE);
    enter(element("list"), TOUCH);
    leave(element("list"), null, TOUCH);
    expect(marked()).toEqual(["list"]);

    enter(element("other"), TOUCH);
    expect(marked()).toEqual(["list", "other"]);
    leave(element("other"), null, TOUCH);
    expect(marked()).toEqual(["list"]);
  });
});
