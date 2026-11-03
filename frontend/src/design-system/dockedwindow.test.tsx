/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { Modal } from "./atoms";
import { coveredByDialog } from "./dialogfocus";
import { DockedWindow, type DockState } from "./dockedwindow";
import { Heading } from "./heading";

afterEach(cleanup);

// The title bar a pointer slides the window by; happy-dom has no capture.
function titleBar(): Element {
  const bar = screen
    .getByRole("heading", { name: "Re: Budget" })
    .closest(".docked-head");
  if (!bar) throw new Error("the window has no title bar");
  bar.setPointerCapture ??= () => undefined;
  return bar;
}

function Harness({ onClose = vi.fn() }: Readonly<{ onClose?: () => void }>) {
  const [state, setState] = useState<DockState>("open");
  return (
    <LocaleProvider initial="en">
      <button type="button">On the page</button>
      <DockedWindow
        title="Re: Budget"
        state={state}
        onStateChange={setState}
        onClose={onClose}
      >
        <input aria-label="Words" />
      </DockedWindow>
    </LocaleProvider>
  );
}

// The verb that opened the window gets focus back when it closes.
function Opener() {
  const [from, setFrom] = useState<HTMLElement | null>(null);
  return (
    <LocaleProvider initial="en">
      <button type="button" onClick={(event) => setFrom(event.currentTarget)}>
        Write email
      </button>
      {from && (
        <DockedWindow
          title="Re: Budget"
          state="open"
          onStateChange={() => undefined}
          onClose={() => setFrom(null)}
          openedFrom={from}
        >
          <input aria-label="Words" />
        </DockedWindow>
      )}
    </LocaleProvider>
  );
}

describe("a window docked beside the page", () => {
  it("hands focus back to the verb that opened it when it closes", async () => {
    const user = userEvent.setup();
    render(<Opener />);
    const verb = screen.getByRole("button", { name: "Write email" });
    await user.click(verb);
    expect(screen.getByRole("dialog").contains(document.activeElement)).toBe(
      true,
    );
    await user.click(screen.getByRole("button", { name: "Close" }));
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(document.activeElement).toBe(verb);
  });

  // Safari does not focus a button it clicks; the press still names the verb.
  it("hands focus back to the pressed verb the click never focused", async () => {
    const user = userEvent.setup();
    render(<Opener />);
    const verb = screen.getByRole("button", { name: "Write email" });
    fireEvent.click(verb);
    expect(document.activeElement).not.toBe(verb);
    await user.click(screen.getByRole("button", { name: "Close" }));
    expect(document.activeElement).toBe(verb);
  });

  it("is a dialog that leaves the page behind it in reach", async () => {
    const user = userEvent.setup();
    render(<Harness />);
    const window = screen.getByRole("dialog", { name: "Re: Budget" });
    expect(window.getAttribute("aria-modal")).toBe("false");
    await user.click(screen.getByRole("button", { name: "On the page" }));
    expect(document.activeElement?.textContent).toBe("On the page");
  });

  // Folding keeps what was typed: the content stays mounted under the bar.
  it("folds to its title bar and opens again with the words kept", async () => {
    const user = userEvent.setup();
    render(<Harness />);
    await user.type(screen.getByLabelText("Words"), "Half a sentence");
    await user.click(screen.getByRole("button", { name: "Minimize" }));
    expect(screen.queryByRole("dialog")).toBeNull();
    await user.click(screen.getByRole("button", { name: "Open Re: Budget" }));
    expect((screen.getByLabelText("Words") as HTMLInputElement).value).toBe(
      "Half a sentence",
    );
  });

  it("folds on Escape, and closes only through its own control", async () => {
    const user = userEvent.setup();
    const onClose = vi.fn();
    render(<Harness onClose={onClose} />);
    fireEvent.keyDown(screen.getByLabelText("Words"), { key: "Escape" });
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(onClose).not.toHaveBeenCalled();
    await user.click(screen.getByRole("button", { name: "Open Re: Budget" }));
    await user.click(screen.getByRole("button", { name: "Close" }));
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("grows and shrinks back from its title bar", async () => {
    const user = userEvent.setup();
    render(<Harness />);
    await user.click(screen.getByRole("button", { name: "Expand" }));
    expect(screen.getByRole("dialog").classList.contains("is-grown")).toBe(
      true,
    );
    await user.click(screen.getByRole("button", { name: "Collapse" }));
    expect(screen.getByRole("dialog").classList.contains("is-grown")).toBe(
      false,
    );
  });

  it("slides along the bottom edge by its title bar, no further than the glass", () => {
    render(<Harness />);
    const dialog = screen.getByRole("dialog");
    // 300px of glass lie between the window and the far edge.
    vi.spyOn(dialog, "getBoundingClientRect").mockReturnValue(
      DOMRect.fromRect({ x: 300, y: 0, width: 400, height: 500 }),
    );
    const bar = titleBar();

    fireEvent.pointerDown(bar, { clientX: 600, pointerId: 1 });
    fireEvent.pointerMove(bar, { clientX: 480, pointerId: 1 });
    expect(dialog.style.marginInlineEnd).toBe("120px");

    fireEvent.pointerMove(bar, { clientX: 0, pointerId: 1 });
    expect(dialog.style.marginInlineEnd).toBe("300px");
    fireEvent.pointerUp(bar, { pointerId: 1 });
  });

  it("keeps a press on a link in the title bar from sliding the window", () => {
    // The slide captures the pointer, which retargets the click at the bar.
    // A reference beside the title could not be followed while only buttons
    // were spared. Nothing in the bar was a link until one was put there.
    //
    // Asserted on the press, not on the margin a slide would set. This DOM has
    // no setPointerCapture, so a press reaching the slide leaves no mark to
    // read. Leaving the event uncancelled is what the guard does.
    render(
      <LocaleProvider initial="en">
        <DockedWindow
          title="Re: Budget"
          aside={<a href="#/contacts/c-1">Anna Weber</a>}
          state="open"
          onStateChange={vi.fn()}
          onClose={vi.fn()}
        >
          <input aria-label="Words" />
        </DockedWindow>
      </LocaleProvider>,
    );
    const followed = fireEvent.pointerDown(
      screen.getByRole("link", { name: "Anna Weber" }),
      { clientX: 600, pointerId: 1 },
    );

    expect(followed).toBe(true);
  });

  it("keeps a press on the title bar's buttons from sliding the window", () => {
    render(<Harness />);

    const pressed = fireEvent.pointerDown(
      screen.getByRole("button", { name: "Expand" }),
      { clientX: 600, pointerId: 1 },
    );

    expect(pressed).toBe(true);
  });

  it("comes back into view when the glass narrows under a slid window", () => {
    render(<Harness />);
    const dialog = screen.getByRole("dialog");
    const rect = vi
      .spyOn(dialog, "getBoundingClientRect")
      .mockReturnValue(DOMRect.fromRect({ x: 300, width: 400, height: 500 }));
    const bar = titleBar();
    fireEvent.pointerDown(bar, { clientX: 600, pointerId: 1 });
    fireEvent.pointerMove(bar, { clientX: 400, pointerId: 1 });
    fireEvent.pointerUp(bar, { pointerId: 1 });
    expect(dialog.style.marginInlineEnd).toBe("200px");

    // The glass narrows until the window's start edge is 50px past it.
    rect.mockReturnValue(DOMRect.fromRect({ x: -50, width: 400, height: 500 }));
    fireEvent(window, new Event("resize"));

    expect(dialog.style.marginInlineEnd).toBe("150px");
  });
});

// A verb inside a drawer opens the window: the Worklist's "Draft follow-up".
function OverADrawer({
  onCloseDrawer = vi.fn(),
}: Readonly<{ onCloseDrawer?: () => void }>) {
  const [writing, setWriting] = useState(false);
  const [state, setState] = useState<DockState>("open");
  return (
    <LocaleProvider initial="en">
      <Modal open onClose={onCloseDrawer} labelledBy="queue" intent="drawer">
        <Heading size="large" id="queue">
          Worklist
        </Heading>
        <button type="button" onClick={() => setWriting(true)}>
          Draft follow-up
        </button>
        {writing && (
          <DockedWindow
            title="Re: Budget"
            state={state}
            onStateChange={setState}
            onClose={() => setWriting(false)}
          >
            <input aria-label="Words" />
            <button type="button">Send</button>
          </DockedWindow>
        )}
      </Modal>
    </LocaleProvider>
  );
}

describe("a window opened from inside a dialog", () => {
  // It shares the overlays' level, so document order is what stacks it. Inside
  // the drawer, the drawer's clip and transform held it under the rows.
  it("is drawn after the dialog's overlay, outside it", async () => {
    const user = userEvent.setup();
    render(<OverADrawer />);
    await user.click(screen.getByRole("button", { name: "Draft follow-up" }));

    const docked = screen.getByRole("dialog", { name: "Re: Budget" });
    const overlay = document.querySelector(".overlay");
    expect(overlay?.contains(docked)).toBe(false);
    expect(docked.parentElement).toBe(document.body);
    expect(overlay?.compareDocumentPosition(docked) ?? 0).toBe(
      Node.DOCUMENT_POSITION_FOLLOWING,
    );
  });

  it("keeps Tab in the window while the reader is writing in it", async () => {
    const user = userEvent.setup();
    render(<OverADrawer />);
    await user.click(screen.getByRole("button", { name: "Draft follow-up" }));

    const docked = screen.getByRole("dialog", { name: "Re: Budget" });
    screen.getByRole("button", { name: "Send" }).focus();
    await user.tab();
    expect(docked.contains(document.activeElement)).toBe(true);
  });

  // A menu in the window answers its own Escape only when nothing covers it.
  it("is not covered by the dialog it was opened from", async () => {
    const user = userEvent.setup();
    render(<OverADrawer />);
    await user.click(screen.getByRole("button", { name: "Draft follow-up" }));

    expect(coveredByDialog(screen.getByRole("button", { name: "Send" }))).toBe(
      false,
    );
  });

  it("folds on Escape and leaves the dialog under it open", async () => {
    const onCloseDrawer = vi.fn();
    const user = userEvent.setup();
    render(<OverADrawer onCloseDrawer={onCloseDrawer} />);
    await user.click(screen.getByRole("button", { name: "Draft follow-up" }));

    await user.keyboard("{Escape}");
    expect(screen.queryByRole("dialog", { name: "Re: Budget" })).toBeNull();
    expect(onCloseDrawer).not.toHaveBeenCalled();
  });
});
