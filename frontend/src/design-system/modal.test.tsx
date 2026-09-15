/** @vitest-environment happy-dom */
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useEffect, useRef, useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { Button, Modal } from "./atoms";
import { Popover } from "./popover";

// A dialog covers the page. `aria-modal` says so to a screen reader and does
// nothing for the Tab key, so these are the two keyboard obligations the
// attribute cannot discharge on its own.

afterEach(cleanup);

/**
 * The dialog's own way out: one per dialog, named by the shared close key, and
 * the last stop in the box. Found by role rather than by class because what
 * these specs are about is what a reader can reach.
 */
function closeControl(): HTMLElement {
  return screen.getByRole("button", { name: "Close" });
}

function Harness() {
  const [open, setOpen] = useState(false);
  return (
    <>
      <Button onClick={() => setOpen(true)}>Open</Button>
      <Button>Behind the dialog</Button>
      <Modal open={open} onClose={() => setOpen(false)} labelledBy="t">
        <h2 id="t">Log activity</h2>
        <Button>First</Button>
        <Button>Last</Button>
      </Modal>
    </>
  );
}

// A dialog whose only popover carries prose — the StatCard receipt shape.
function ProseReceipt() {
  const [open, setOpen] = useState(false);
  return (
    <>
      <Button onClick={() => setOpen(true)}>Open</Button>
      <Modal open={open} onClose={() => setOpen(false)} labelledBy="p">
        <h2 id="p">Won this quarter</h2>
        <Popover label="Basis">
          <p>Six of nine, since April.</p>
        </Popover>
        <Button onClick={() => setOpen(false)}>Done</Button>
      </Modal>
    </>
  );
}

// Two receipts open at once. A popover shuts on a press outside itself, so a
// second CLICK never leaves the first standing — but a hover-opened one is
// raised by a settling pointer and presses nothing. The prose one is FIRST in
// the dialog and opens on hover, so DOM order and focus disagree.
function TwoReceipts() {
  const [open, setOpen] = useState(false);
  return (
    <>
      <Button onClick={() => setOpen(true)}>Open</Button>
      <Modal open={open} onClose={() => setOpen(false)} labelledBy="w">
        <h2 id="w">Won this quarter</h2>
        <Popover label="Basis" onHover>
          <p>Six of nine, since April.</p>
        </Popover>
        <Popover label="Rows">
          <Button>Only row</Button>
        </Popover>
        <Button onClick={() => setOpen(false)}>Done</Button>
      </Modal>
    </>
  );
}

// A pointer that arrives and stops. The hook reads `timeStamp` off the event,
// which jsdom does not fill in from fake timers, and it treats silence as the
// answer — so the settle is two moves at one place and then a wait.
function settleOn(trigger: HTMLElement) {
  fireEvent.pointerEnter(trigger);
  for (const _ of [0, 1]) {
    const move = new Event("pointermove") as PointerEvent & {
      clientX: number;
      clientY: number;
    };
    Object.defineProperties(move, {
      clientX: { value: 10 },
      clientY: { value: 10 },
      timeStamp: { value: performance.now() },
    });
    document.dispatchEvent(move);
  }
  act(() => {
    vi.advanceTimersByTime(400);
  });
}

describe("a dialog holds the keyboard", () => {
  it("moves focus in when it opens", async () => {
    render(<Harness />);
    await userEvent.click(screen.getByRole("button", { name: "Open" }));
    expect(document.activeElement).toBe(
      screen.getByRole("button", { name: "First" }),
    );
  });

  it("wraps Tab at the last stop instead of leaving for the page behind", async () => {
    render(<Harness />);
    await userEvent.click(screen.getByRole("button", { name: "Open" }));
    await userEvent.tab();
    expect(document.activeElement).toBe(
      screen.getByRole("button", { name: "Last" }),
    );
    // The way out is the LAST stop of every dialog: drawn over the trailing
    // corner, rendered after the content, so the reader walks what they came
    // for before they are offered the door.
    await userEvent.tab();
    expect(document.activeElement).toBe(closeControl());
    await userEvent.tab();
    expect(document.activeElement).toBe(
      screen.getByRole("button", { name: "First" }),
    );
    expect(document.activeElement).not.toBe(
      screen.getByRole("button", { name: "Behind the dialog" }),
    );
  });

  it("wraps backwards from the first stop", async () => {
    render(<Harness />);
    await userEvent.click(screen.getByRole("button", { name: "Open" }));
    await userEvent.tab({ shift: true });
    expect(document.activeElement).toBe(closeControl());
  });

  it("closes from the control drawn in its own corner", async () => {
    render(<Harness />);
    await userEvent.click(screen.getByRole("button", { name: "Open" }));
    await userEvent.click(closeControl());
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("pulls Tab back in when focus is already outside, in either direction", async () => {
    render(<Harness />);
    await userEvent.click(screen.getByRole("button", { name: "Open" }));
    const behind = screen.getByRole("button", { name: "Behind the dialog" });

    // Something on the covered page took focus while the dialog was open. A
    // plain Tab from there would keep walking that page, so both directions
    // have to catch it — not only Shift+Tab.
    behind.focus();
    await userEvent.tab();
    expect(document.activeElement).toBe(
      screen.getByRole("button", { name: "First" }),
    );

    behind.focus();
    await userEvent.tab({ shift: true });
    expect(document.activeElement).toBe(closeControl());
  });

  it("keeps Tab working when a popover in the dialog is prose", async () => {
    // A receipt under a reading is frequently a sentence and nothing else. The
    // trap hands Tab to the panel a dialog has opened, and a panel with no
    // stops in it can only answer by swallowing the key — the dialog is then
    // as unwalkable as if it had no controls at all.
    render(<ProseReceipt />);
    await userEvent.click(screen.getByRole("button", { name: "Open" }));
    await userEvent.click(screen.getByRole("button", { name: "Basis" }));
    expect(screen.getByText("Six of nine, since April.")).toBeTruthy();

    await userEvent.tab();
    expect(document.activeElement).toBe(
      screen.getByRole("button", { name: "Done" }),
    );
  });

  it("holds Tab in the open panel the reader is in, not the first one", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    try {
      render(<TwoReceipts />);
      await user.click(screen.getByRole("button", { name: "Open" }));
      await user.click(screen.getByRole("button", { name: "Rows" }));
      const row = screen.getByRole("button", { name: "Only row" });
      expect(document.activeElement).toBe(row);

      // The prose receipt rises beside it under a settling pointer, pressing
      // nothing, so both are open at once.
      settleOn(screen.getByRole("button", { name: "Basis" }));
      expect(screen.getByText("Six of nine, since April.")).toBeTruthy();

      // Picked by DOM order, the trap would find the prose panel first, have
      // nothing to focus in it, and hand Tab back to the dialog — taking the
      // reader out of the panel they are standing in.
      await user.tab();
      expect(document.activeElement).toBe(row);
    } finally {
      vi.useRealTimers();
    }
  });

  // A dialog whose body is prose or an empty list has ONE control: the way out.
  // Landing a reader on it made the control name itself in a tip, and the tip
  // answered their first Escape — one press doing nothing in front of a surface
  // covering the page. These two pin both halves of the fix.
  function NothingToAnswer() {
    const [open, setOpen] = useState(true);
    return (
      <Modal open={open} onClose={() => setOpen(false)} labelledBy="n">
        <h2 id="n">Nothing to answer</h2>
      </Modal>
    );
  }

  it("takes Escape on the first press when the way out is all it has", async () => {
    render(<NothingToAnswer />);
    // The box, not the door: a dialog with nothing to answer still receives
    // focus — that is what its tabIndex of -1 is for — and raises no tip.
    expect(document.activeElement).toBe(screen.getByRole("dialog"));
    expect(screen.queryByRole("tooltip")).toBeNull();

    await userEvent.keyboard("{Escape}");
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("takes Escape on the first press from the way out itself", async () => {
    render(<NothingToAnswer />);
    await userEvent.tab();
    expect(document.activeElement).toBe(closeControl());
    // The control names itself when focus arrives, and that tip is content the
    // reader did not ask for: it goes with the same press that closes the
    // dialog, rather than spending the press on itself.
    expect(screen.getByRole("tooltip").textContent).toBe("Close");

    await userEvent.keyboard("{Escape}");
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(screen.queryByRole("tooltip")).toBeNull();
  });

  it("gives focus back to whatever opened it, so the reader keeps their place", async () => {
    render(<Harness />);
    const opener = screen.getByRole("button", { name: "Open" });
    await userEvent.click(opener);
    await userEvent.keyboard("{Escape}");
    expect(document.activeElement).toBe(opener);
  });
});

// The case the plain restore above cannot serve: the dialog's own mutation
// removes the control that opened it. focus() on a detached node is a silent
// no-op, so focus lands on <body> and the next Tab restarts at the top of the
// document — the reader's place is lost by the action succeeding.
describe("a dialog whose mutation removes its own opener", () => {
  // The member row shape this exists for: Deactivate is replaced by Reactivate,
  // and the row survives both.
  function RowHarness({ named }: Readonly<{ named: boolean }>) {
    const [open, setOpen] = useState(false);
    const [off, setOff] = useState(false);
    const row = useRef<HTMLLIElement | null>(null);
    return (
      <ul>
        <li ref={row} tabIndex={-1}>
          Ada Active
          {/* Two slots rather than one ternary, as the member row spells it: a
              ternary would let React reuse the same <button> node for both, and
              a reused node is never the detached opener this is about. */}
          {!off && <Button onClick={() => setOpen(true)}>Deactivate</Button>}
          {off && <Button onClick={() => setOff(false)}>Reactivate</Button>}
        </li>
        <Modal
          open={open}
          onClose={() => setOpen(false)}
          labelledBy="row-h"
          returnFocusTo={named ? () => row.current : undefined}
        >
          <h2 id="row-h">Deactivate Ada Active?</h2>
          <Button
            onClick={() => {
              setOff(true);
              setOpen(false);
            }}
          >
            Confirm
          </Button>
        </Modal>
      </ul>
    );
  }

  it("hands focus to the named target once the opener is gone", async () => {
    render(<RowHarness named />);
    await userEvent.click(screen.getByRole("button", { name: "Deactivate" }));
    await userEvent.click(screen.getByRole("button", { name: "Confirm" }));

    const row = screen.getByRole("listitem");
    expect(document.activeElement).toBe(row);
    // Named, not merely "somewhere in the row": the row reads back the member
    // and the status the confirm just changed, which is why it is the target.
    expect(row.textContent).toContain("Ada Active");
  });

  it("drops focus to the document when nothing is named — the failure this fixes", async () => {
    render(<RowHarness named={false} />);
    await userEvent.click(screen.getByRole("button", { name: "Deactivate" }));
    await userEvent.click(screen.getByRole("button", { name: "Confirm" }));

    // Nothing here can honestly take focus back: the opener no longer exists,
    // which is exactly the state a caller passes returnFocusTo to answer.
    expect(screen.queryByRole("button", { name: "Deactivate" })).toBeNull();
    expect(document.activeElement).toBe(document.body);
  });

  function PrecedenceHarness({
    resolve,
  }: Readonly<{ resolve: () => HTMLElement | null }>) {
    const [open, setOpen] = useState(false);
    return (
      <>
        <Button onClick={() => setOpen(true)}>Open</Button>
        <Button>Elsewhere</Button>
        <Modal
          open={open}
          onClose={() => setOpen(false)}
          labelledBy="prec-h"
          returnFocusTo={resolve}
        >
          <h2 id="prec-h">Confirm</h2>
          <Button>Confirm</Button>
        </Modal>
      </>
    );
  }

  // A caller names a target because the mutation unmakes the opener, and the
  // unmaking usually lands with the refetch a moment AFTER the dialog closes.
  // Preferring a still-attached opener would restore focus to a button that is
  // about to be removed, which is the same lost place one tick later.
  it("prefers the named target over an opener that is still attached", async () => {
    render(
      <PrecedenceHarness
        resolve={() => screen.getByRole("button", { name: "Elsewhere" })}
      />,
    );
    await userEvent.click(screen.getByRole("button", { name: "Open" }));
    await userEvent.keyboard("{Escape}");
    expect(document.activeElement).toBe(
      screen.getByRole("button", { name: "Elsewhere" }),
    );
  });

  it("falls back to the opener when the named target is not in the document", async () => {
    // A resolver can answer with a node the DOM no longer holds — a row the
    // refetch dropped. Focusing it would be the same silent no-op, so the
    // opener, which is still there, gets the focus instead.
    const detached = document.createElement("button");
    render(<PrecedenceHarness resolve={() => detached} />);
    const opener = screen.getByRole("button", { name: "Open" });
    await userEvent.click(opener);
    await userEvent.keyboard("{Escape}");
    expect(document.activeElement).toBe(opener);
  });
});

// The exit an animation needs time to play, and the frames after the reader
// dismissed it are frames the dialog is still painted over the page. jsdom runs
// no animations, so the Modal specs above see an unmount and nothing else —
// which is the behaviour a browser without motion also gets. This is the other
// half: what the dialog owes the page while it is leaving it.
describe("a dialog that is leaving", () => {
  function twoStops(open: boolean, onClose: () => void) {
    return (
      <Modal open={open} onClose={onClose} labelledBy="x">
        <h2 id="x">Log activity</h2>
        <Button>First</Button>
      </Modal>
    );
  }

  // A mount-count probe, because "the same element" is only half the claim: a
  // dialog that left the tree for one render would put its CHILDREN back new,
  // re-firing their reads and replaying their entry animations on a surface
  // that is leaving.
  //
  // The counter belongs to the ONE test that reads it and is handed in, rather
  // than living beside this component where every other test in the file shares
  // its identity. A count that survives the test that owns it is a fact about
  // the whole file, and a fact about the whole file is what a full run changes
  // and an isolated run does not.
  function Probe({ mounted }: Readonly<{ mounted: { count: number } }>) {
    useEffect(() => {
      mounted.count += 1;
    }, [mounted]);
    return <p>Body</p>;
  }

  // Synchronous, and deliberately so: every fact below is settled by the commit
  // `render` and `rerender` each flush inside `act`. An `async` test with no
  // `await` in it still hands the runner a microtask boundary to schedule
  // around, and there is nothing here that needs one.
  it("never leaves the tree between open and closing", () => {
    const mounted = { count: 0 };
    const exit = new Promise<void>(() => undefined);
    const animations = vi
      .spyOn(HTMLElement.prototype, "getAnimations")
      .mockReturnValue([
        {
          finished: exit,
          effect: { getComputedTiming: () => ({ iterations: 1 }) },
        } as unknown as Animation,
      ]);
    try {
      const withProbe = (open: boolean) => (
        <Modal open={open} onClose={() => undefined} labelledBy="p">
          <h2 id="p">Log activity</h2>
          <Probe mounted={mounted} />
        </Modal>
      );
      const { baseElement, rerender } = render(withProbe(true));
      // Scoped to THIS render's own root rather than the document: a dialog is
      // portalled to the body, which is also where anything another test in
      // this file left would be, and a document-wide query answers with
      // whichever of them the DOM holds first.
      const box = baseElement.querySelector('[role="dialog"]');
      expect(box).not.toBeNull();
      expect(mounted.count).toBe(1);

      rerender(withProbe(false));

      // Read off the DOM rather than by role: a leaving dialog is deliberately
      // out of the accessibility tree, and the subject here is the NODE.
      // Answered during the render that carried the flip, so React never got a
      // commit saying this dialog was gone.
      expect(baseElement.querySelector('[role="dialog"]')).toBe(box);
      expect(mounted.count).toBe(1);
    } finally {
      animations.mockRestore();
    }
  });

  it("stays on the page, inert and deaf to the backdrop, until its exit ends", async () => {
    let finish: () => void = () => undefined;
    const exit = new Promise<void>((resolve) => {
      finish = resolve;
    });
    const animations = vi
      .spyOn(HTMLElement.prototype, "getAnimations")
      // The two properties `usePresence` reads: whether this animation can end
      // at all, and when it did.
      .mockReturnValue([
        {
          finished: exit,
          effect: { getComputedTiming: () => ({ iterations: 1 }) },
        } as unknown as Animation,
      ]);
    try {
      const onClose = vi.fn();
      const { baseElement, rerender } = render(twoStops(true, onClose));
      rerender(twoStops(false, onClose));

      const overlay = baseElement.querySelector(".overlay");
      expect(overlay?.getAttribute("data-state")).toBe("closing");
      // Painted, and nothing else: no tab stop, no hit target, and no ROLE. A
      // dialog mid-exit that still took a click would swallow the first press
      // meant for the page it is uncovering; one that still answered to
      // "dialog" would be a second dialog for as long as the exit lasts, which
      // is what a verb opening the next dialog straight from this one finds.
      expect(overlay?.hasAttribute("inert")).toBe(true);
      expect(overlay?.getAttribute("aria-hidden")).toBe("true");
      expect(screen.queryByRole("dialog")).toBeNull();
      if (overlay !== null) {
        fireEvent.click(overlay);
      }
      expect(onClose).not.toHaveBeenCalled();

      await act(async () => {
        finish();
        await exit;
      });
      // The OVERLAY, not the role: `aria-hidden` above already takes the dialog
      // out of every role query, so asking for the role again would have been
      // true the whole way through and would pass just as happily over an inert
      // overlay left on the page for the rest of the session — which is the one
      // failure this spec exists to catch.
      expect(baseElement.querySelector(".overlay")).toBeNull();
    } finally {
      animations.mockRestore();
    }
  });
});

// A right-anchored dialog is the same dialog: same portal, same Esc, same
// trap. Only where it sits changes.
describe("a drawer is a dialog anchored to the right edge", () => {
  it("keeps the dialog role and the Escape close", async () => {
    function DrawerHarness() {
      const [open, setOpen] = useState(true);
      return (
        <Modal
          open={open}
          onClose={() => setOpen(false)}
          labelledBy="d"
          placement="right"
        >
          <h2 id="d">Write email</h2>
          {/* A drawer a rep works IN always has a control before the way out,
              and that is what puts the initial focus somewhere other than the
              close — see the tip spec below for why that distinction matters. */}
          <Button>Send</Button>
        </Modal>
      );
    }
    render(<DrawerHarness />);
    const drawer = screen.getByRole("dialog", { name: "Write email" });
    expect(drawer.classList.contains("modal-drawer")).toBe(true);
    await userEvent.keyboard("{Escape}");
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  // The width of a drawer comes from the viewport, so the centred-box size
  // variants must not also apply — two width rules would fight.
  it("ignores the centred-box size variant", () => {
    render(
      <Modal
        open
        onClose={() => {}}
        labelledBy="d"
        placement="right"
        size="wide"
      >
        <h2 id="d">Evidence</h2>
      </Modal>,
    );
    const dialog = screen.getByRole("dialog", { name: "Evidence" });
    expect(dialog.classList.contains("modal-drawer")).toBe(true);
    expect(dialog.classList.contains("modal-wide")).toBe(false);
  });
});
