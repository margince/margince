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
import { useState } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { en as englishCopy } from "../i18n/en";
import { dropKeptPlace, keepPlaceOnNextMove } from "../lib/scrollmemory";
import { Button } from "./atoms";
import { RecordTabs } from "./recordtabs";

// The mark the strip leaves for the page's scroll memory, watched as calls.
vi.mock("../lib/scrollmemory", async (importOriginal) => {
  const actual = await importOriginal<typeof import("../lib/scrollmemory")>();
  return {
    ...actual,
    keepPlaceOnNextMove: vi.fn(actual.keepPlaceOnNextMove),
    dropKeptPlace: vi.fn(actual.dropKeptPlace),
  };
});

beforeEach(() => {
  vi.mocked(keepPlaceOnNextMove).mockClear();
  vi.mocked(dropKeptPlace).mockClear();
});

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

describe("RecordTabs names what a count counts", () => {
  function counted(countLabels?: Partial<Record<Body, string>>) {
    render(
      <LocaleProvider initial="en">
        <RecordTabs
          options={["overview", "contacts"]}
          value="overview"
          onChange={() => undefined}
          labels={LABELS}
          counts={{ contacts: 2 }}
          countLabels={countLabels}
        />
      </LocaleProvider>,
    );
  }

  it("speaks the label in place of the bare figure", () => {
    counted({ contacts: "2 columns not imported" });
    expect(
      screen.getByRole("button", {
        name: /^Contacts\s*, 2 columns not imported$/,
      }),
    ).toBeInTheDocument();
  });

  it("speaks the figure alone when no label says what it counts", () => {
    counted();
    expect(
      screen.getByRole("button", { name: /^Contacts\s*, 2$/ }),
    ).toBeInTheDocument();
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

    expect(
      screen.getByRole("button", {
        name: englishCopy["record.tabs.moreLabel"],
      }),
    ).toHaveFocus();
  });

  it("moves focus from More to the open tab", () => {
    const grow = foldedStrip();
    const more = screen.getByRole("button", {
      name: englishCopy["record.tabs.moreLabel"],
    });
    expect(screen.queryByRole("button", { name: "Contacts" })).toBeNull();

    act(() => more.focus());
    grow();

    expect(screen.getByRole("button", { name: "Overview" })).toHaveFocus();
  });

  it("moves focus from an entry in the open menu to the open tab", () => {
    const grow = foldedStrip();
    const more = screen.getByRole("button", {
      name: englishCopy["record.tabs.moreLabel"],
    });
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

// A page pane under the 50px top bar (`--topbarH`), standing at `offset`. Its
// strip sits `stripTop` px below the pane's top edge; its offset clamps.
function inPane(
  stripTop: number,
  { keepPlace = true, offset = 1000, routed = false } = {},
) {
  let bottom = 10_000;
  let top = offset;
  let arrival: number | null = null;
  let stripAt = stripTop + offset;
  let grownAbove = 0;
  const writes: number[] = [];
  let pane: HTMLElement | null = null;
  function Live() {
    const [value, setValue] = useState<Body>("overview");
    return (
      <div className="scroll" style={{ marginTop: "-50px" }}>
        <RecordTabs
          options={["overview", "contacts"]}
          value={value}
          onChange={(next) => {
            setValue(next);
            if (routed) window.location.hash = `#/companies/o-1/${next}`;
            stripAt += grownAbove;
            if (arrival !== null) {
              bottom = arrival;
              top = Math.min(top, bottom);
            }
          }}
          keepPlace={keepPlace}
          labels={LABELS}
        />
        <button type="button" onClick={() => setValue("overview")}>
          Back
        </button>
      </div>
    );
  }
  const view = render(
    <LocaleProvider initial="en">
      <Live />
    </LocaleProvider>,
  );
  pane = view.container.querySelector<HTMLElement>(".scroll");
  const row = view.container.querySelector<HTMLElement>(".recordtabs-row");
  if (!pane || !row) {
    throw new Error("the strip was not drawn inside its pane");
  }
  Object.defineProperty(pane, "scrollTop", {
    configurable: true,
    get: () => top,
    set: (next: number) => {
      writes.push(next);
      top = Math.max(0, Math.min(next, bottom));
    },
  });
  // A smooth scroll lands later, so a glide cannot pass for a jump.
  const scrollTo = vi.fn((options?: ScrollToOptions | number) => {
    const asked = typeof options === "object" ? (options.top ?? 0) : 0;
    setTimeout(() => {
      top = asked;
    }, 50);
  });
  pane.scrollTo = scrollTo;
  vi.spyOn(pane, "clientHeight", "get").mockReturnValue(844);
  vi.spyOn(pane, "scrollHeight", "get").mockImplementation(() => bottom + 844);
  vi.spyOn(pane, "getBoundingClientRect").mockReturnValue(
    new DOMRect(0, 0, 390, 844),
  );
  vi.spyOn(row, "getBoundingClientRect").mockImplementation(
    () => new DOMRect(0, stripAt - top, 390, 44),
  );
  const scrolled = pane;
  return {
    pane: scrolled,
    writes,
    scrollTo,
    unmount: view.unmount,
    stripTop: () => row.getBoundingClientRect().top,
    // The next tab draws more above the strip: a line under the record's name.
    growAbove: (height: number) => {
      grownAbove = height;
    },
    // The next body arrives short, and the browser pulls the offset up to it.
    arriveShort: (height: number) => {
      arrival = height;
    },
    // The body grows: its range widens and a node lands in the pane.
    grow: async () => {
      bottom = 10_000;
      await act(async () => {
        scrolled.append(document.createElement("div"));
      });
    },
  };
}

const press = (name: string) =>
  fireEvent.click(screen.getByRole("button", { name }));

describe("RecordTabs keeps a record reader's place across a press", () => {
  it("holds the offset through a new body that arrives short and grows", async () => {
    const at = inPane(120);
    at.arriveShort(200);
    press("Contacts");
    expect(at.writes).toEqual([1000]);
    expect(at.pane.scrollTop).toBe(200);

    await at.grow();
    expect(at.pane.scrollTop).toBe(1000);
  });

  it("keeps the strip where it stood when the next tab draws more above it", () => {
    const at = inPane(120);
    at.growAbove(66);
    press("Contacts");
    expect(at.pane.scrollTop).toBe(1066);
    expect(at.stripTop()).toBe(120);
  });

  it("lets go once the offset has landed", async () => {
    const at = inPane(120);
    at.arriveShort(200);
    press("Contacts");
    await at.grow();
    // Dragged by its scrollbar, which no gesture listener sees on every engine.
    at.pane.scrollTop = 50;

    await at.grow();
    expect(at.pane.scrollTop).toBe(50);
    expect(at.writes).toEqual([1000, 1000, 50]);
  });

  it.each(["wheel", "pointerdown"] as const)(
    "lets go once the reader takes hold with a %s",
    async (gesture) => {
      const at = inPane(120);
      at.arriveShort(200);
      press("Contacts");
      fireEvent(at.pane, new Event(gesture));

      await at.grow();
      expect(at.pane.scrollTop).toBe(200);
    },
  );

  it("lets go when the tab changes under it, as Back does", async () => {
    const at = inPane(120);
    at.arriveShort(200);
    press("Contacts");
    press("Back");

    await at.grow();
    expect(at.writes).toEqual([1000]);
  });

  it("lets go when the strip goes", async () => {
    const at = inPane(120);
    at.arriveShort(200);
    press("Contacts");
    at.unmount();

    await at.grow();
    expect(at.writes).toEqual([1000]);
  });

  it("jumps up just far enough to show a strip left above the screen", () => {
    const at = inPane(-300);
    press("Contacts");
    // 300px above the pane's edge and 50 more under the bar.
    expect(at.pane.scrollTop).toBe(650);
    expect(at.scrollTo).not.toHaveBeenCalled();
  });

  it("leaves the pane to the page on a strip that does not keep the place", async () => {
    const at = inPane(-300, { keepPlace: false });
    at.arriveShort(200);
    press("Contacts");

    await at.grow();
    expect(at.writes).toEqual([]);
  });

  it("does not move for a press on the tab already open", () => {
    const at = inPane(-300);
    press("Overview");
    expect(at.writes).toEqual([]);
  });
});

describe("RecordTabs marks a press for the page's scroll memory", () => {
  afterEach(() => {
    window.location.hash = "";
  });

  it("marks a press on another tab that moves the address", () => {
    inPane(120, { routed: true });
    press("Contacts");
    expect(keepPlaceOnNextMove).toHaveBeenCalledOnce();
    expect(dropKeptPlace).not.toHaveBeenCalled();
  });

  it("takes the mark back when the tab lives in state and no address moved", () => {
    inPane(120);
    press("Contacts");
    expect(keepPlaceOnNextMove).toHaveBeenCalledOnce();
    expect(dropKeptPlace).toHaveBeenCalledOnce();
  });

  it("marks nothing for a press on the tab already open", () => {
    inPane(120, { routed: true });
    press("Overview");
    expect(keepPlaceOnNextMove).not.toHaveBeenCalled();
  });

  it("marks nothing on a strip that does not keep the place", () => {
    inPane(120, { keepPlace: false, routed: true });
    press("Contacts");
    expect(keepPlaceOnNextMove).not.toHaveBeenCalled();
  });
});
