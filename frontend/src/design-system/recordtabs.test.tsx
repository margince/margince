// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
} from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { Button } from "./atoms";
import { RecordTabs } from "./recordtabs";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

type Body = "overview" | "contacts";
const LABELS: Record<Body, string> = {
  overview: "Overview",
  contacts: "Contacts",
};

function strip(trailing?: React.ReactNode) {
  return render(
    <LocaleProvider initial="en">
      <RecordTabs
        options={["overview", "contacts"]}
        value="overview"
        onChange={() => undefined}
        labels={LABELS}
        trailing={trailing}
      />
    </LocaleProvider>,
  );
}

// The row's far end is the page's control, not one more tab: it sits outside
// the strip a reader navigates, and a strip with nothing to put there draws
// no empty end.
describe("RecordTabs carries the details control at the row's end", () => {
  it("renders the trailing control outside the strip", () => {
    const { container } = strip(<Button>Details</Button>);
    const trailing = container.querySelector(".recordtabs-trailing");
    expect(trailing).not.toBeNull();
    expect(
      trailing?.contains(screen.getByRole("button", { name: "Details" })),
    ).toBe(true);
    expect(
      container.querySelector(".recordtabs-strip")?.contains(trailing),
    ).toBe(false);
  });

  it("draws no end when there is nothing to put there", () => {
    const { container } = strip();
    expect(container.querySelector(".recordtabs-trailing")).toBeNull();
  });
});

// A record read in ONE body has nothing to choose, so its lone tab is a label
// for the page the reader is already on. Drawn as a button it was a control
// that answered nothing: pressable, focusable, and dead. It keeps the tab's
// box and the current tab's mark, because the row still has to read as the row.
describe("RecordTabs draws a lone body as a label", () => {
  function lone() {
    return render(
      <LocaleProvider initial="en">
        <RecordTabs
          options={["overview"]}
          value="overview"
          labels={LABELS}
          trailing={<Button>Details</Button>}
        />
      </LocaleProvider>,
    );
  }

  it("puts no pressable tab in the strip", () => {
    const { container } = lone();
    const tabs = container.querySelectorAll(".recordtabs-tab");
    expect(tabs).toHaveLength(1);
    expect(tabs[0]?.tagName).toBe("SPAN");
    expect(container.querySelector(".recordtabs-strip button")).toBeNull();
  });

  it("marks the label as the page the reader is on", () => {
    const { container } = lone();
    const tab = container.querySelector(".recordtabs-tab");
    expect(tab?.textContent).toBe("Overview");
    expect(tab?.getAttribute("aria-current")).toBe("page");
    expect(tab?.hasAttribute("aria-pressed")).toBe(false);
    expect(tab?.getAttribute("role")).toBeNull();
  });

  it("still carries the control at the row's end", () => {
    const { container } = lone();
    expect(
      container
        .querySelector(".recordtabs-trailing")
        ?.contains(screen.getByRole("button", { name: "Details" })),
    ).toBe(true);
  });
});

// More than one body and the tabs are what they were: buttons that route, with
// the current one pressed.
describe("RecordTabs keeps pressable tabs when there is a choice", () => {
  it("draws a button per option and presses the current one", () => {
    const { container } = strip();
    const tabs = container.querySelectorAll(".recordtabs-tab");
    expect(tabs).toHaveLength(2);
    for (const tab of tabs) {
      expect(tab.tagName).toBe("BUTTON");
      expect(tab.hasAttribute("aria-current")).toBe(false);
    }
    expect(
      screen
        .getByRole("button", { name: "Overview" })
        .getAttribute("aria-pressed"),
    ).toBe("true");
    expect(
      screen
        .getByRole("button", { name: "Contacts" })
        .getAttribute("aria-pressed"),
    ).toBe("false");
  });
});

// The strip hides its scrollbar, so the fade is the only sign that tabs run
// past an edge. It marks the end that hides tabs, and a strip that fits
// carries none.
describe("RecordTabs fades only an end that hides tabs", () => {
  function sized(scrollWidth: number, clientWidth: number) {
    vi.spyOn(HTMLElement.prototype, "scrollWidth", "get").mockReturnValue(
      scrollWidth,
    );
    vi.spyOn(HTMLElement.prototype, "clientWidth", "get").mockReturnValue(
      clientWidth,
    );
    return strip().container.querySelector(".recordtabs-strip");
  }

  it("fades the far end of a strip wider than its row", () => {
    const tabs = sized(600, 300);
    expect(tabs).toHaveClass("is-clipped-end");
    expect(tabs).not.toHaveClass("is-clipped-start");
  });

  it("fades the start once the strip is scrolled to its end", () => {
    const tabs = sized(600, 300);
    if (!(tabs instanceof HTMLElement)) throw new Error("no tab strip drawn");
    act(() => {
      tabs.scrollLeft = 300;
      fireEvent.scroll(tabs);
    });
    expect(tabs).toHaveClass("is-clipped-start");
    expect(tabs).not.toHaveClass("is-clipped-end");
  });

  it("fades nothing on a strip that fits", () => {
    const tabs = sized(300, 300);
    expect(tabs).not.toHaveClass("is-clipped-start");
    expect(tabs).not.toHaveClass("is-clipped-end");
  });
});

// Focus stays on the row as tabs fold and return. A tab that folds hands it
// to "More". "More" hands it to the open tab once every tab fits.
describe("RecordTabs keeps focus on the row as tabs fold", () => {
  // Two tabs on a row that starts at `start` wide. The returned call resizes it.
  function sizedStrip(start: number) {
    // The open menu watches its own size too, so every observer hears a resize.
    const watchers: (() => void)[] = [];
    vi.stubGlobal(
      "ResizeObserver",
      class {
        constructor(changed: () => void) {
          watchers.push(changed);
        }
        observe() {}
        disconnect() {}
      },
    );
    let room = start;
    vi.spyOn(HTMLElement.prototype, "offsetWidth", "get").mockReturnValue(100);
    vi.spyOn(HTMLElement.prototype, "clientWidth", "get").mockImplementation(
      () => room,
    );
    strip();
    return (next: number) => {
      room = next;
      act(() => {
        for (const changed of watchers) changed();
      });
    };
  }

  // Two tabs on a row too narrow for both. The returned call widens it.
  function foldedStrip() {
    const resize = sizedStrip(150);
    return () => resize(400);
  }

  it("moves focus from a tab that folds to More", () => {
    const resize = sizedStrip(400);
    act(() => screen.getByRole("button", { name: "Contacts" }).focus());
    resize(150);

    expect(screen.getByRole("button", { name: "More sections" })).toHaveFocus();
  });

  it("moves focus from More to the open tab", () => {
    const grow = foldedStrip();
    const more = screen.getByRole("button", { name: "More sections" });
    expect(screen.queryByRole("button", { name: "Contacts" })).toBeNull();

    act(() => more.focus());
    grow();

    expect(screen.getByRole("button", { name: "Overview" })).toHaveFocus();
  });

  it("moves focus from an entry in the open menu to the open tab", () => {
    const grow = foldedStrip();
    const more = screen.getByRole("button", { name: "More sections" });
    act(() => more.focus());
    fireEvent.click(more);
    act(() => screen.getByRole("button", { name: "Contacts" }).focus());
    grow();

    expect(screen.getByRole("button", { name: "Overview" })).toHaveFocus();
  });
});

// Widths here depend on where a tab stands: the first one drops its start
// padding. The fold must settle on one row, not flip between two.
describe("RecordTabs folds to one stable row", () => {
  it("keeps the same tabs when the first tab's padding moves", () => {
    let resized: () => void = () => undefined;
    vi.stubGlobal(
      "ResizeObserver",
      class {
        constructor(changed: () => void) {
          resized = changed;
        }
        observe() {}
        disconnect() {}
      },
    );
    const base: Record<string, number> = { a: 92, b: 100, c: 200 };
    const first = (el: Element) => el.parentElement?.firstElementChild === el;
    vi.spyOn(HTMLElement.prototype, "offsetWidth", "get").mockImplementation(
      function (this: HTMLElement) {
        const tab = this.dataset.tab;
        if (tab === undefined) return 80;
        return (base[tab] ?? 0) - (first(this) ? 12 : 0);
      },
    );
    vi.spyOn(HTMLElement.prototype, "clientWidth", "get").mockReturnValue(254);
    const computed = window.getComputedStyle.bind(window);
    vi.spyOn(window, "getComputedStyle").mockImplementation((el) => {
      const style = computed(el);
      if (!(el instanceof HTMLElement) || el.dataset.tab === undefined) {
        return style;
      }
      return {
        ...style,
        paddingInlineStart: first(el) ? "0px" : "12px",
        paddingInlineEnd: "12px",
      };
    });
    render(
      <LocaleProvider initial="en">
        <RecordTabs
          options={["a", "b", "c"]}
          value="b"
          onChange={() => undefined}
          labels={{ a: "Alpha", b: "Beta", c: "Gamma" }}
        />
      </LocaleProvider>,
    );
    const shown = () =>
      [...document.querySelectorAll<HTMLElement>("[data-tab]")].map(
        (tab) => tab.dataset.tab,
      );

    act(() => resized());
    expect(shown()).toEqual(["b"]);
    act(() => resized());
    expect(shown()).toEqual(["b"]);
  });
});
