/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Mail, Phone } from "lucide-react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { Button } from "./atoms";
import { Heading } from "./heading";
import { armHoverIntent, takeHoverClock } from "./hoverintent-testing";
import { IconAction } from "./iconaction";
import { Modal } from "./modal";
import { OverflowMenu } from "./overflowmenu";
import { Popover } from "./popover";
import { useTruncationTooltip } from "./tooltip";

// Escape dismisses a tip wherever focus sits (WCAG 2.2 SC 1.4.13). A tip the
// pointer raised takes the press; a tip focus raised lets it carry on.

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.useRealTimers();
});

// The clock is taken for the settle alone and handed back, so the keys after
// it run on the real one: `userEvent` never returns on the hover clock.
function settleOn(element: HTMLElement) {
  takeHoverClock();
  armHoverIntent();
  fireEvent.pointerEnter(element);
  act(() => {
    vi.advanceTimersByTime(500);
  });
  vi.useRealTimers();
}

function tipAnchor(name: string): HTMLElement {
  const anchor = screen.getByRole("button", { name }).parentElement;
  if (anchor === null) {
    throw new Error("IconAction draws its button inside the tip's anchor");
  }
  return anchor;
}

const LONG = "In contact, but one contact carries the whole account.";

function Row({ text }: Readonly<{ text: string }>) {
  const tip = useTruncationTooltip<HTMLSpanElement>(text);
  return (
    <span ref={tip.ref} {...tip.trigger}>
      {text}
      {tip.tip}
    </span>
  );
}

function TwoVerbs() {
  return (
    <>
      <IconAction label="Mail" icon={<Mail aria-hidden="true" />} />
      <IconAction label="Call" icon={<Phone aria-hidden="true" />} />
    </>
  );
}

describe("a tip the pointer raised while focus sits elsewhere", () => {
  it("goes on Escape and leaves focus where it was", async () => {
    const user = userEvent.setup();
    render(
      <>
        <IconAction label="Call" icon={<Phone aria-hidden="true" />} />
        <Button>Elsewhere</Button>
      </>,
    );
    const elsewhere = screen.getByRole("button", { name: "Elsewhere" });
    act(() => elsewhere.focus());
    settleOn(tipAnchor("Call"));
    expect(screen.getByRole("tooltip")).toHaveTextContent("Call");

    await user.keyboard("{Escape}");

    expect(screen.queryByRole("tooltip")).toBeNull();
    expect(elsewhere).toHaveFocus();
  });

  it("goes on Escape for a truncated string too", async () => {
    const user = userEvent.setup();
    vi.spyOn(HTMLElement.prototype, "scrollWidth", "get").mockReturnValue(480);
    vi.spyOn(HTMLElement.prototype, "clientWidth", "get").mockReturnValue(220);
    render(
      <>
        <Row text={LONG} />
        <Button>Elsewhere</Button>
      </>,
    );
    act(() => screen.getByRole("button", { name: "Elsewhere" }).focus());
    settleOn(screen.getByText(LONG));
    expect(screen.getByRole("tooltip")).toHaveTextContent(LONG);

    await user.keyboard("{Escape}");

    expect(screen.queryByRole("tooltip")).toBeNull();
  });
});

describe("the press a tip takes", () => {
  it("never reaches the control that holds focus", async () => {
    const user = userEvent.setup();
    const onKeyDown = vi.fn();
    render(
      <>
        <IconAction label="Call" icon={<Phone aria-hidden="true" />} />
        <input aria-label="Search" onKeyDown={onKeyDown} />
      </>,
    );
    act(() => screen.getByRole("textbox", { name: "Search" }).focus());
    settleOn(tipAnchor("Call"));

    await user.keyboard("{Escape}");
    expect(onKeyDown).not.toHaveBeenCalled();

    await user.keyboard("{Escape}");
    expect(onKeyDown).toHaveBeenCalledOnce();
  });

  it("after a pointer tip, leaves a focus tip up for the next press", async () => {
    const user = userEvent.setup();
    render(<TwoVerbs />);
    settleOn(tipAnchor("Call"));
    await user.tab();
    expect(screen.getAllByRole("tooltip")).toHaveLength(2);

    await user.keyboard("{Escape}");
    expect(screen.getByRole("tooltip")).toHaveTextContent("Mail");

    await user.keyboard("{Escape}");
    expect(screen.queryByRole("tooltip")).toBeNull();
  });

  it("after a focus tip, takes both tips down in one press", async () => {
    const user = userEvent.setup();
    render(<TwoVerbs />);
    await user.tab();
    settleOn(tipAnchor("Call"));
    expect(screen.getAllByRole("tooltip")).toHaveLength(2);

    await user.keyboard("{Escape}");
    expect(screen.queryByRole("tooltip")).toBeNull();
  });

  it("leaves an Escape that ends an input method's composition alone", () => {
    render(<IconAction label="Call" icon={<Phone aria-hidden="true" />} />);
    settleOn(tipAnchor("Call"));

    fireEvent.keyDown(document.body, { key: "Escape", isComposing: true });

    expect(screen.getByRole("tooltip")).toHaveTextContent("Call");
  });
});

describe("a tip focus raised", () => {
  it("goes with the same Escape that closes the dialog around it", async () => {
    const user = userEvent.setup();
    const onClose = vi.fn();
    render(
      <Modal open onClose={onClose} labelledBy="d" intent="form">
        <Heading size="large" id="d">
          Edit deal
        </Heading>
        <IconAction label="Call" icon={<Phone aria-hidden="true" />} />
      </Modal>,
    );
    expect(screen.getByRole("tooltip")).toHaveTextContent("Call");

    await user.keyboard("{Escape}");

    expect(screen.queryByRole("tooltip")).toBeNull();
    expect(onClose).toHaveBeenCalledOnce();
  });

  it("goes on Escape and the control keeps focus", async () => {
    const user = userEvent.setup();
    render(<IconAction label="Call" icon={<Phone aria-hidden="true" />} />);
    await user.tab();
    expect(screen.getByRole("tooltip")).toHaveTextContent("Call");

    await user.keyboard("{Escape}");

    expect(screen.queryByRole("tooltip")).toBeNull();
    expect(screen.getByRole("button", { name: "Call" })).toHaveFocus();
  });
});

describe("a tip over another layer", () => {
  it("raised by the pointer inside a dialog takes the first Escape, and the dialog the second", async () => {
    const user = userEvent.setup();
    const onClose = vi.fn();
    render(
      <Modal open onClose={onClose} labelledBy="d" intent="form">
        <Heading size="large" id="d">
          Edit deal
        </Heading>
        <Button>Elsewhere</Button>
        <IconAction label="Call" icon={<Phone aria-hidden="true" />} />
      </Modal>,
    );
    settleOn(tipAnchor("Call"));

    await user.keyboard("{Escape}");
    expect(screen.queryByRole("tooltip")).toBeNull();
    expect(onClose).not.toHaveBeenCalled();

    await user.keyboard("{Escape}");
    expect(onClose).toHaveBeenCalledOnce();
  });

  it("over an open menu takes the first Escape, and the menu the second", async () => {
    const user = userEvent.setup();
    render(
      <>
        <OverflowMenu label="More actions">
          <button type="button">Merge</button>
        </OverflowMenu>
        <IconAction label="Call" icon={<Phone aria-hidden="true" />} />
      </>,
    );
    const menu = screen.getByRole("button", { name: "More actions" });
    await user.click(menu);
    settleOn(tipAnchor("Call"));

    await user.keyboard("{Escape}");
    expect(screen.queryByRole("tooltip")).toBeNull();
    expect(menu).toHaveAttribute("aria-expanded", "true");

    await user.keyboard("{Escape}");
    expect(menu).toHaveAttribute("aria-expanded", "false");
  });

  it("over an open popover takes the first Escape, and the popover the second", async () => {
    const user = userEvent.setup();
    render(
      <>
        <Popover label="How it stands">Two of three invoices are late.</Popover>
        <IconAction label="Call" icon={<Phone aria-hidden="true" />} />
      </>,
    );
    const aside = screen.getByRole("button", { name: "How it stands" });
    await user.click(aside);
    settleOn(tipAnchor("Call"));

    await user.keyboard("{Escape}");
    expect(screen.queryByRole("tooltip")).toBeNull();
    expect(aside).toHaveAttribute("aria-expanded", "true");

    await user.keyboard("{Escape}");
    expect(aside).toHaveAttribute("aria-expanded", "false");
  });

  it("under a dialog it is not part of leaves Escape to the dialog", async () => {
    const user = userEvent.setup();
    const onClose = vi.fn();
    const page = (dialogOpen: boolean) => (
      <>
        <IconAction label="Call" icon={<Phone aria-hidden="true" />} />
        <Modal open={dialogOpen} onClose={onClose} labelledBy="d" intent="form">
          <Heading size="large" id="d">
            Edit deal
          </Heading>
        </Modal>
      </>
    );
    const { rerender } = render(page(false));
    settleOn(tipAnchor("Call"));
    rerender(page(true));

    await user.keyboard("{Escape}");
    expect(onClose).toHaveBeenCalledOnce();
    rerender(page(false));
    expect(screen.getByRole("tooltip")).toHaveTextContent("Call");

    await user.keyboard("{Escape}");
    expect(screen.queryByRole("tooltip")).toBeNull();
  });

  it("inside a popover opened from a dialog takes the first Escape", async () => {
    const user = userEvent.setup();
    const onClose = vi.fn();
    render(
      <Modal open onClose={onClose} labelledBy="d" intent="form">
        <Heading size="large" id="d">
          Edit deal
        </Heading>
        <Popover label="How it stands">
          <IconAction label="Call" icon={<Phone aria-hidden="true" />} />
        </Popover>
      </Modal>,
    );
    const aside = screen.getByRole("button", { name: "How it stands" });
    await user.click(aside);
    settleOn(tipAnchor("Call"));

    await user.keyboard("{Escape}");
    expect(screen.queryByRole("tooltip")).toBeNull();
    expect(aside).toHaveAttribute("aria-expanded", "true");

    await user.keyboard("{Escape}");
    expect(aside).toHaveAttribute("aria-expanded", "false");
    expect(onClose).not.toHaveBeenCalled();
  });
});
