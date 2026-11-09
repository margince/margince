/** @vitest-environment happy-dom */
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useRef, useState } from "react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { Button, Modal } from "./atoms";
import { Heading } from "./heading";
import { holdExits, watchHiding } from "./presence-testing";

// The exit is held, so a closed dialog's overlay is still mounted, inert and
// hidden, when each case asks where focus went.
let exits: ReturnType<typeof holdExits>;
let hiding: ReturnType<typeof watchHiding>;

beforeEach(() => {
  exits = holdExits();
  hiding = watchHiding();
});

afterEach(() => {
  cleanup();
  hiding.restore();
  exits.mockRestore();
});

function overlayOf(name: string): Element {
  const found = document.getElementById(name)?.closest(".overlay");
  if (!found) throw new Error(`no dialog headed ${name} is on the page`);
  return found;
}

// Focus is out of the overlay before it leaves the accessibility tree, and the
// overlay did leave it: a check that saw no hiding at all would pass vacuously.
function expectHiddenAfterFocusLeft(overlay: Element) {
  expect(hiding.hidings.filter((write) => write.heldFocus)).toEqual([]);
  expect(hiding.hidings.map((write) => write.element)).toContain(overlay);
}

function LogActivity({ named }: Readonly<{ named?: boolean }>) {
  const [open, setOpen] = useState(false);
  const row = useRef<HTMLButtonElement>(null);
  return (
    <>
      <Button onClick={() => setOpen(true)}>Open</Button>
      <button type="button" ref={row}>
        The new row
      </button>
      <Modal
        open={open}
        onClose={() => setOpen(false)}
        labelledBy="log"
        intent="form"
        returnFocusTo={named ? () => row.current : undefined}
      >
        <Heading size="large" id="log">
          Log activity
        </Heading>
        <Button onClick={() => setOpen(false)}>Save</Button>
      </Modal>
    </>
  );
}

describe("a dialog hands focus back before it hides its overlay", () => {
  it("returns focus to the opener", async () => {
    const user = userEvent.setup();
    render(<LogActivity />);
    const opener = screen.getByRole("button", { name: "Open" });
    await user.click(opener);
    await user.click(screen.getByRole("button", { name: "Save" }));

    expectHiddenAfterFocusLeft(overlayOf("log"));
    expect(document.activeElement).toBe(opener);
  });

  it("still lands on the target the caller names", async () => {
    const user = userEvent.setup();
    render(<LogActivity named />);
    await user.click(screen.getByRole("button", { name: "Open" }));
    await user.keyboard("{Escape}");

    expectHiddenAfterFocusLeft(overlayOf("log"));
    expect(document.activeElement).toBe(
      screen.getByRole("button", { name: "The new row" }),
    );
  });
});

function NoteInsideDeal() {
  const [deal, setDeal] = useState(false);
  const [note, setNote] = useState(false);
  return (
    <>
      <Button onClick={() => setDeal(true)}>Open deal</Button>
      <Modal
        open={deal}
        onClose={() => setDeal(false)}
        labelledBy="deal"
        intent="form"
      >
        <Heading size="large" id="deal">
          Deal
        </Heading>
        <Button onClick={() => setNote(true)}>Add a note</Button>
      </Modal>
      <Modal
        open={note}
        onClose={() => setNote(false)}
        labelledBy="note"
        intent="form"
      >
        <Heading size="large" id="note">
          Note
        </Heading>
        <Button onClick={() => setNote(false)}>Done</Button>
      </Modal>
    </>
  );
}

describe("a dialog opened from inside another", () => {
  it("returns focus into the dialog beneath, which stays live", async () => {
    const user = userEvent.setup();
    render(<NoteInsideDeal />);
    await user.click(screen.getByRole("button", { name: "Open deal" }));
    const addNote = screen.getByRole("button", { name: "Add a note" });
    await user.click(addNote);
    await user.click(screen.getByRole("button", { name: "Done" }));

    expectHiddenAfterFocusLeft(overlayOf("note"));
    expect(document.activeElement).toBe(addNote);
    const beneath = overlayOf("deal");
    expect(beneath.hasAttribute("aria-hidden")).toBe(false);
    expect(beneath.hasAttribute("inert")).toBe(false);
    expect(screen.getByRole("dialog", { name: "Deal" })).toBeDefined();
  });
});
