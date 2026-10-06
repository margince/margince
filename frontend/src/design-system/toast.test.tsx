/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { act, cleanup, render, screen } from "@testing-library/react";
import { type ReactNode, useEffect } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { steppedClock } from "../testing/steppedclock";
import { Button } from "./atoms";
import {
  type Toast,
  type ToastOptions,
  ToastProvider,
  ToastRegion,
  useOwnToast,
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
      <Button onClick={() => toast.dismiss()}>dismiss</Button>
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

// The controls held outside the tree, for a message that must arrive without a
// trigger press moving focus or the pointer.
function controlled(): () => Toast {
  let held: Toast | null = null;
  function Capture() {
    held = useToast();
    return null;
  }
  render(
    <LocaleProvider initial="en">
      <ToastProvider>
        <Capture />
        <ToastRegion />
      </ToastProvider>
    </LocaleProvider>,
  );
  return () => {
    if (held === null) {
      throw new Error("the toast controls were never rendered");
    }
    return held;
  };
}

const undo = (onAct = () => {}): ToastOptions => ({
  action: { kind: "undo", label: "Undo", onAct },
});
const open = (onAct = () => {}): ToastOptions => ({
  action: { kind: "open", label: "Show all", onAct },
});

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

// `Button` takes one press per commit and releases its latch in a microtask,
// which a synchronous `act` never drains; pressing the same trigger again needs this.
const release = () => act(async () => {});

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
  it("withdraws after eight seconds", async () => {
    // Longer than a report, so a reader reaching for Undo has time to get there.
    const acting = steppedClock();
    show({ options: undo() });
    await acting.click(press("show"));
    wait(7900);
    expect(screen.getByRole("status")).toHaveTextContent("Saved.");
    wait(200);
    expect(screen.queryByRole("status")).toBeNull();
  });

  it("holds its clock while a pointer is over it", async () => {
    const acting = steppedClock();
    show({ options: undo() });
    await acting.click(press("show"));
    await acting.hover(screen.getByRole("status"));
    wait(30_000);
    expect(press("Undo")).toBeInTheDocument();
  });

  it("stays when the caller asks for sticky as well", async () => {
    const acting = steppedClock();
    show({ options: { ...undo(), sticky: true } });
    await acting.click(press("show"));
    wait(30_000);
    expect(press("Undo")).toBeInTheDocument();
  });

  it("closes with one press after several identical ones", async () => {
    // Three Done presses in a row: one × must clear the region, not the first
    // of three identical messages queued behind one another.
    const acting = steppedClock();
    show({ message: "Task completed", options: undo() });
    for (let done = 0; done < 3; done += 1) {
      await acting.click(press("show"));
      await release();
    }
    expect(screen.getAllByRole("status")).toHaveLength(1);
    await acting.click(press("Close"));
    expect(screen.queryByRole("status")).toBeNull();
  });

  it("is replaced by a newer one, whose Undo is the one that runs", async () => {
    const first = vi.fn();
    const second = vi.fn();
    const acting = steppedClock();
    show({
      message: "Task completed",
      options: undo(first),
      second: "Task completed",
      secondOptions: undo(second),
    });
    await acting.click(press("show"));
    const replaced = screen.getByRole("status");
    await acting.click(press("show second"));
    // A new node, so the arrival animation plays again for the same words.
    expect(screen.getByRole("status")).not.toBe(replaced);
    await acting.click(press("Undo"));
    expect(second).toHaveBeenCalledOnce();
    expect(first).not.toHaveBeenCalled();
    expect(screen.queryByRole("status")).toBeNull();
  });

  it("gives the replacement its own full life", async () => {
    const acting = steppedClock();
    show({ options: undo() });
    await acting.click(press("show"));
    await release();
    wait(6000);
    await acting.click(press("show"));
    wait(7900);
    expect(screen.getByRole("status")).toBeInTheDocument();
    wait(200);
    expect(screen.queryByRole("status")).toBeNull();
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
    // The click left the pointer on the region, which holds whatever it shows,
    // so the reader moves away before the clock is read.
    await acting.hover(screen.getByRole("status"));
    await acting.unhover(screen.getByRole("status"));
    // And what was waiting behind it is an ordinary confirmation again, with its
    // own full life rather than the remainder of somebody else's.
    wait(3600);
    expect(screen.queryByRole("status")).toBeNull();
  });

  it("is not evicted by a verb that only leads somewhere", async () => {
    const acting = steppedClock();
    show({
      message: "Task completed",
      options: undo(),
      second: "Moved to Jana.",
      secondOptions: open(),
    });
    await acting.click(press("show"));
    await acting.click(press("show second"));
    expect(screen.getByRole("status")).toHaveTextContent("Task completed");
    await acting.click(press("Close"));
    expect(screen.getByRole("status")).toHaveTextContent("Moved to Jana.");
  });

  it("queues an undo behind a verb that leads somewhere", async () => {
    const acting = steppedClock();
    show({
      message: "Moved to Jana.",
      options: open(),
      second: "Task completed",
      secondOptions: undo(),
    });
    await acting.click(press("show"));
    await acting.click(press("show second"));
    expect(screen.getByRole("status")).toHaveTextContent("Moved to Jana.");
  });

  it("keeps a verb that leads somewhere until it is dismissed", async () => {
    const acting = steppedClock();
    show({ options: open() });
    await acting.click(press("show"));
    wait(30_000);
    expect(press("Show all")).toBeInTheDocument();
  });

  it("replaces an undo waiting in the queue rather than stacking a second", async () => {
    const acting = steppedClock();
    const toast = controlled();
    act(() => {
      toast().show("Moved to Jana.", open());
      toast().show("First done", undo());
      toast().show("Second done", undo());
    });
    await acting.click(press("Close"));
    expect(screen.getByRole("status")).toHaveTextContent("Second done");
    await acting.click(press("Close"));
    expect(screen.queryByRole("status")).toBeNull();
  });

  it("carries its own way out", async () => {
    const acting = steppedClock();
    show({ options: undo() });
    await acting.click(press("show"));
    expect(press("Close")).toBeInTheDocument();
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
    expect(view.baseElement.querySelector(".toast-dot-success")).not.toBeNull();
  });

  it("leaves a refusal unmarked", async () => {
    // A failure with a green tick beside it says the opposite of what the
    // sentence says.
    const acting = steppedClock();
    const view = show({
      message: "That did not work.",
      options: { tone: "danger" },
    });
    await acting.click(press("show"));
    expect(screen.getByRole("status")).toHaveTextContent("That did not work.");
    expect(view.baseElement.querySelector(".toast-dot-success")).toBeNull();
  });
});

describe("a caller withdrawing its own message", () => {
  it("withdraws only the message its id names", () => {
    steppedClock();
    const toast = controlled();
    let saved = 0;
    act(() => {
      toast().show("Moved to Jana.", open());
      saved = toast().show("Draft saved");
    });
    act(() => toast().dismiss(saved));
    expect(screen.getByRole("status")).toHaveTextContent("Moved to Jana.");
    act(() => toast().dismiss());
    expect(screen.queryByRole("status")).toBeNull();
  });

  it("goes with a caller that asked for it to leave with it", () => {
    steppedClock();
    function Caller({ leaves }: Readonly<{ leaves: boolean }>) {
      const own = useOwnToast({ leavesWithCaller: leaves });
      useEffect(() => own.show("Moved to Jana.", open()), [own]);
      return null;
    }
    const stage = (caller: ReactNode) => (
      <LocaleProvider initial="en">
        <ToastProvider>
          {caller}
          <ToastRegion />
        </ToastProvider>
      </LocaleProvider>
    );
    const view = render(stage(<Caller leaves />));
    expect(screen.getByRole("status")).toBeInTheDocument();
    view.rerender(stage(null));
    expect(screen.queryByRole("status")).toBeNull();

    view.rerender(stage(<Caller leaves={false} />));
    view.rerender(stage(null));
    expect(screen.getByRole("status")).toBeInTheDocument();
  });

  it("leaves the screen alone once its own message has gone", () => {
    steppedClock();
    const toast = controlled();
    let saved = 0;
    act(() => {
      saved = toast().show("Draft saved");
    });
    wait(3600);
    act(() => {
      toast().show("Settings saved.", { sticky: true });
    });
    act(() => toast().dismiss(saved));
    expect(screen.getByRole("status")).toHaveTextContent("Settings saved.");
  });
});

describe("a replacement under the reader's hand", () => {
  it("keeps focus on the same control when the message is replaced", () => {
    steppedClock();
    const toast = controlled();
    act(() => {
      toast().show("First done", undo());
    });
    const first = press("Undo");
    act(() => first.focus());
    act(() => {
      toast().show("Second done", undo());
    });
    expect(screen.getByRole("status")).toHaveTextContent("Second done");
    expect(press("Undo")).not.toBe(first);
    expect(press("Undo")).toHaveFocus();
  });

  it("keeps focus on a link in the message body when the message is replaced", () => {
    steppedClock();
    const toast = controlled();
    act(() => {
      toast().show(<a href="#/contacts/p-1">Jana Brandt</a>, { sticky: true });
    });
    act(() => screen.getByRole("link", { name: "Jana Brandt" }).focus());
    act(() => {
      toast().show(<a href="#/contacts/p-2">Jonas Petersen</a>, {
        sticky: true,
      });
    });
    expect(screen.getByRole("link", { name: "Jonas Petersen" })).toHaveFocus();
  });

  it("hands focus to nobody when the reader puts the message down", () => {
    // The next message advancing is not a replacement: the reader closed one
    // toast and did not ask to be moved into the next.
    steppedClock();
    const toast = controlled();
    act(() => {
      toast().show("Task completed", undo());
      toast().show("Moved to Jana.", open());
    });
    act(() => press("Close").focus());
    act(() => toast().dismiss());
    expect(screen.getByRole("status")).toHaveTextContent("Moved to Jana.");
    expect(press("Show all")).not.toHaveFocus();
    expect(press("Close")).not.toHaveFocus();
  });

  it("keeps focus on Close when the message is replaced", () => {
    steppedClock();
    const toast = controlled();
    act(() => {
      toast().show("First done", undo());
    });
    act(() => press("Close").focus());
    act(() => {
      toast().show("Second done", undo());
    });
    expect(press("Close")).toHaveFocus();
  });

  it("holds a replacement that arrives under a resting pointer", async () => {
    const acting = steppedClock();
    const toast = controlled();
    act(() => {
      toast().show("First done", undo());
    });
    await acting.hover(screen.getByRole("status"));
    act(() => {
      toast().show("Second done", undo());
    });
    wait(30_000);
    expect(screen.getByRole("status")).toHaveTextContent("Second done");
  });

  it("forgets the hold once the region has emptied", async () => {
    // An unmounted region hears no pointerleave; the next message must not
    // inherit a hold from a pointer that left with the last one.
    const acting = steppedClock();
    const toast = controlled();
    act(() => {
      toast().show("First done", undo());
    });
    await acting.hover(screen.getByRole("status"));
    act(() => toast().dismiss());
    act(() => {
      toast().show("Saved.");
    });
    wait(3600);
    expect(screen.queryByRole("status")).toBeNull();
  });
});
