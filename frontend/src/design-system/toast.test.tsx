/** @vitest-environment jsdom */
import "@testing-library/jest-dom/vitest";
import { act, cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { steppedClock } from "../testing/steppedclock";
import { Button } from "./atoms";
import {
  type ToastOptions,
  ToastProvider,
  ToastRegion,
  useToast,
} from "./toast";

// A harness rather than a hook-only test: the withdrawal is the behaviour worth
// pinning, and it is only observable through what is on screen.
//
// The triggers are the design-system `Button`, not native ones, because that is
// what shows a toast everywhere in the product: `Button` owns a pending state
// that stops taking clicks, and if it ever stopped delivering one the suite that
// notices should be the one whose whole subject is what a click puts on screen.
//
// The region is mounted here the way `main.tsx` mounts it — once, beside the
// tree rather than inside a screen — so what the suite drives is the real
// arrangement rather than one assembled for the test.
function Triggers({
  message,
  options,
  second,
  secondOptions,
}: Readonly<{
  message: string;
  options?: ToastOptions;
  second?: string;
  secondOptions?: ToastOptions;
}>) {
  const toast = useToast();
  return (
    <>
      <Button onClick={() => toast.show(message, options)}>show</Button>
      {second && (
        <Button onClick={() => toast.show(second, secondOptions)}>
          show second
        </Button>
      )}
      <Button onClick={toast.dismiss}>dismiss</Button>
    </>
  );
}

function Harness(props: React.ComponentProps<typeof Triggers>) {
  return (
    <LocaleProvider initial="en">
      <ToastProvider>
        <Triggers {...props} />
        <ToastRegion />
      </ToastProvider>
    </LocaleProvider>
  );
}

// A confirmation whose BODY carries a control, which is what several callers
// actually show — a name is worth linking to from the sentence that names it.
function Bodied({ sticky = false }: Readonly<{ sticky?: boolean }>) {
  const toast = useToast();
  return (
    <Button
      onClick={() =>
        toast.show(
          <span>
            Jonas Petersen is now a contact:{" "}
            <a href="#/contacts/p-1">Jana Brandt</a>
          </span>,
          { sticky },
        )
      }
    >
      show a link
    </Button>
  );
}

const show = (props: Partial<React.ComponentProps<typeof Triggers>> = {}) =>
  render(<Harness message="Saved." {...props} />);

// The clock is driven in every test, not only the ones that watch a message go:
// what this suite measures is a deadline, and `userEvent` waits on timers of its
// own between the events that make up a click. `steppedClock` puts both on the
// same clock and carries the reason it takes doing.
afterEach(() => {
  vi.useRealTimers();
  cleanup();
});

const wait = (ms: number) => {
  act(() => {
    vi.advanceTimersByTime(ms);
  });
};

const press = (name: string) => screen.getByRole("button", { name });

describe("the toast region", () => {
  it("says nothing until something is shown", () => {
    show();
    expect(screen.queryByRole("status")).toBeNull();
  });

  it("withdraws a confirmation on its own", async () => {
    const acting = steppedClock();
    show();
    await acting.click(press("show"));
    expect(screen.getByRole("status")).toHaveTextContent("Saved.");
    // Just short of the deadline it is still there — the point of the pair is
    // that the message is readable for a while, not that it eventually goes.
    wait(3400);
    expect(screen.getByRole("status")).toHaveTextContent("Saved.");
    wait(200);
    expect(screen.queryByRole("status")).toBeNull();
  });

  it("gives a second confirmation its own full life", async () => {
    // The defect this pins: a shared deadline. With the first timer left
    // running, its timeout fires while the second message is on screen and takes
    // it down early — so a reader making two quick saves sees the second blink.
    const acting = steppedClock();
    show({ message: "First.", second: "Second." });
    await acting.click(press("show"));
    wait(3000);
    await acting.click(press("show second"));
    wait(1000);
    expect(screen.getByRole("status")).toHaveTextContent("Second.");
    wait(2600);
    expect(screen.queryByRole("status")).toBeNull();
  });

  it("renders outside the tree that showed it", async () => {
    // Portalled to the body, like every other overlay in this directory. A
    // region rendered in place is a fixed box inside the content column, and any
    // ancestor carrying a transform becomes the viewport it anchors to.
    const acting = steppedClock();
    const view = show();
    await acting.click(press("show"));
    expect(view.container).not.toContainElement(screen.getByRole("status"));
    expect(document.body).toContainElement(screen.getByRole("status"));
  });

  it("cancels its timer when the tree goes away", async () => {
    // The cleanup one of the three hand-copied toasts was missing. A settings
    // tab is exactly the screen a reader leaves right after saving, so the
    // orphaned timeout fired against an unmounted tree on every save they made.
    const acting = steppedClock();
    const view = show();
    await acting.click(press("show"));
    expect(vi.getTimerCount()).toBe(1);
    view.unmount();
    expect(vi.getTimerCount()).toBe(0);
  });
});

describe("a confirmation carrying a verb", () => {
  const undo = (onAct = () => {}) => ({ action: { label: "Undo", onAct } });

  it("does not withdraw on its own", async () => {
    // A reader reaching for Undo must not lose it mid-reach, and there is no
    // timeout long enough to be safe that is also short enough to be a toast.
    const acting = steppedClock();
    show({ options: undo() });
    await acting.click(press("show"));
    wait(30_000);
    expect(screen.getByRole("status")).toHaveTextContent("Saved.");
  });

  it("runs the verb and then withdraws", async () => {
    // Withdrawing afterwards is the point: a message still offering an action it
    // has already taken is a second press waiting to happen.
    const acted = vi.fn();
    const acting = steppedClock();
    show({ options: undo(acted) });
    await acting.click(press("show"));
    await acting.click(press("Undo"));
    expect(acted).toHaveBeenCalledOnce();
    expect(screen.queryByRole("status")).toBeNull();
  });

  it("is not evicted by a confirmation that only reports", async () => {
    // The rule the queue exists for. A courtesy message must never take away the
    // reader's only route back from something they may not have meant.
    const acting = steppedClock();
    show({ message: "Deal archived.", options: undo(), second: "Saved." });
    await acting.click(press("show"));
    await acting.click(press("show second"));
    expect(screen.getByRole("status")).toHaveTextContent("Deal archived.");
  });

  it("hands the queue on when it is dismissed", async () => {
    const acting = steppedClock();
    show({ message: "Deal archived.", options: undo(), second: "Saved." });
    await acting.click(press("show"));
    await acting.click(press("show second"));
    await acting.click(press("Close"));
    expect(screen.getByRole("status")).toHaveTextContent("Saved.");
    // And what was waiting behind it is an ordinary confirmation again, with its
    // own full life rather than the remainder of somebody else's.
    wait(3600);
    expect(screen.queryByRole("status")).toBeNull();
  });

  it("gives a sticky confirmation its own way out", async () => {
    const acting = steppedClock();
    show({ options: { sticky: true } });
    await acting.click(press("show"));
    expect(press("Close")).toBeInTheDocument();
  });

  it("gives a timed confirmation none", async () => {
    // A confirmation that withdraws itself needs no control: it is gone in three
    // and a half seconds, and a button beside it invites a decision about
    // something already decided.
    const acting = steppedClock();
    show();
    await acting.click(press("show"));
    expect(screen.queryByRole("button", { name: "Close" })).toBeNull();
  });
});

describe("the clock a reader can stop", () => {
  it("holds while a pointer is over the message", async () => {
    // WCAG 2.2.1 asks for a way to extend a time limit. For a passive surface
    // the honest one is that reading it stops the clock.
    const acting = steppedClock();
    show();
    await acting.click(press("show"));
    await acting.hover(screen.getByRole("status"));
    wait(30_000);
    expect(screen.getByRole("status")).toHaveTextContent("Saved.");
  });

  it("releases the clock when the pointer leaves", async () => {
    const acting = steppedClock();
    show();
    await acting.click(press("show"));
    await acting.hover(screen.getByRole("status"));
    wait(30_000);
    await acting.unhover(screen.getByRole("status"));
    // The full life again rather than what was left of it: a reader who hovered
    // was reading, and deserves the whole time back once they move away.
    wait(3400);
    expect(screen.getByRole("status")).toHaveTextContent("Saved.");
    wait(200);
    expect(screen.queryByRole("status")).toBeNull();
  });

  it("holds while focus is inside the message, and releases on blur", async () => {
    // The keyboard half of the same courtesy: a reader who has tabbed to a
    // control the message carries is mid-reach, and a deadline that ran anyway
    // would take it out from under the focus ring.
    //
    // A TIMED toast, deliberately. Written against a sticky one this asserted
    // nothing at all: sticky means no timer, so the message would have survived
    // the wait with the pause removed entirely. The toast that actually needs
    // this — the lead-qualified confirmation, which carries a link to the new
    // contact — withdraws itself on the clock like any other.
    const acting = steppedClock();
    render(
      <LocaleProvider initial="en">
        <ToastProvider>
          <Bodied />
          <ToastRegion />
        </ToastProvider>
      </LocaleProvider>,
    );
    await acting.click(press("show a link"));
    const carried = screen.getByRole("link", { name: "Jana Brandt" });

    act(() => carried.focus());
    wait(30_000);
    expect(screen.getByRole("status")).toBeInTheDocument();

    // Blurring hands the FULL life back rather than what was left of it: a
    // reader who focused it was reading, and deserves the whole time again.
    act(() => carried.blur());
    wait(3400);
    expect(screen.getByRole("status")).toBeInTheDocument();
    wait(200);
    expect(screen.queryByRole("status")).toBeNull();
  });

  it("puts the message down on Escape from a control the MESSAGE owns", async () => {
    // The gap this closes: Escape used to be wired to the toast's own two
    // buttons, so a message carrying focusable content of its own — the
    // lead-qualified confirmation puts a link to the new contact in its body —
    // was a toast whose documented way out did nothing from inside it.
    const acting = steppedClock();
    render(
      <LocaleProvider initial="en">
        <ToastProvider>
          <Bodied sticky />
          <ToastRegion />
        </ToastProvider>
      </LocaleProvider>,
    );
    await acting.click(press("show a link"));
    act(() => screen.getByRole("link", { name: "Jana Brandt" }).focus());
    await acting.keyboard("{Escape}");
    expect(screen.queryByRole("status")).toBeNull();
  });

  it("puts the message down on Escape", async () => {
    const acting = steppedClock();
    show({ options: { sticky: true } });
    await acting.click(press("show"));
    act(() => press("Close").focus());
    await acting.keyboard("{Escape}");
    expect(screen.queryByRole("status")).toBeNull();
  });
});

describe("the completion mark", () => {
  it("marks a completion", async () => {
    const acting = steppedClock();
    const view = show();
    await acting.click(press("show"));
    expect(view.baseElement.querySelector(".dot-auto")).not.toBeNull();
  });

  it("leaves a refusal unmarked", async () => {
    // A failure with a green tick beside it says the opposite of what the
    // sentence says.
    const acting = steppedClock();
    const view = show({
      message: "That did not work.",
      options: { mark: false },
    });
    await acting.click(press("show"));
    expect(screen.getByRole("status")).toHaveTextContent("That did not work.");
    expect(view.baseElement.querySelector(".dot-auto")).toBeNull();
  });
});
