// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";

import { act, cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {
  afterEach,
  beforeEach,
  describe,
  expect,
  it,
  type MockInstance,
  vi,
} from "vitest";
import { AiRuntimeChip, type AiRuntimeLabels } from "./airuntimechip";
import { Heading } from "./heading";
import { Modal } from "./modal";

// Hover and keyboard focus BOTH open the popover, so a press that only clears
// the pin would leave it open under a focus that is still there.

const LABELS: AiRuntimeLabels = {
  configured: "Configured",
  used: "Answered by",
  route: "Route",
  calls: "Calls",
  tokens: "Tokens",
  latency: "Latency",
  estimatedCost: "Estimated cost",
  partial: "Partial",
  awaiting: "Shown after the first model call",
  unavailable: "Not available yet",
  chip: "Active model and cost",
  answering: "Answering right now",
  scope: "This run only",
};

afterEach(cleanup);

function chip() {
  return (
    <AiRuntimeChip configured="ollama/gemma3" labels={LABELS} locale="en" />
  );
}

describe("the runtime chip", () => {
  it("pins the popover open on the first press, even though focus already opened it", async () => {
    render(chip());
    const button = screen.getByRole("button", {
      name: new RegExp(LABELS.chip),
    });

    await userEvent.tab();
    expect(button).toHaveFocus();
    expect(button).toHaveAttribute("aria-expanded", "true");

    // The press pins what focus opened rather than reading "already open" as
    // a request to close it.
    await userEvent.keyboard("[Space]");
    expect(button).toHaveAttribute("aria-expanded", "true");

    await userEvent.keyboard("[Space]");
    expect(button).toHaveAttribute("aria-expanded", "false");

    await userEvent.keyboard("[Space]");
    expect(button).toHaveAttribute("aria-expanded", "true");
  });

  it("puts the spend it shows into the name a screen reader hears", () => {
    render(chip());

    expect(
      screen.getByRole("button", {
        name: `${LABELS.chip}: ${LABELS.awaiting}`,
      }),
    ).toBeInTheDocument();
  });

  it("closes on Escape when the keyboard is what opened it", async () => {
    render(chip());
    const button = screen.getByRole("button", {
      name: new RegExp(LABELS.chip),
    });

    await userEvent.tab();
    expect(button).toHaveAttribute("aria-expanded", "true");

    await userEvent.keyboard("{Escape}");
    expect(button).toHaveAttribute("aria-expanded", "false");
  });

  it("leaves Escape to a dialog raised over it", async () => {
    const onDialogClose = vi.fn();
    const page = (dialogOpen: boolean) => (
      <>
        {chip()}
        <Modal open={dialogOpen} onClose={onDialogClose} labelledBy="edit">
          <Heading size="large" id="edit">
            Edit deal
          </Heading>
        </Modal>
      </>
    );
    const user = userEvent.setup();
    const { rerender } = render(page(false));
    const button = screen.getByRole("button", {
      name: new RegExp(LABELS.chip),
    });
    await user.click(button);
    rerender(page(true));

    await user.keyboard("{Escape}");

    expect(onDialogClose).toHaveBeenCalledOnce();
    expect(button).toHaveAttribute("aria-expanded", "true");
  });

  it("takes no tab stop in the popover while its rows fit", async () => {
    render(chip());

    await userEvent.tab();
    await userEvent.tab();

    expect(document.body).toHaveFocus();
    expect(screen.queryByRole("region")).not.toBeInTheDocument();
  });

  describe("with rows taller than the room below it", () => {
    // happy-dom lays nothing out, so the popover is given a height it cannot hold.
    let scrollHeight: MockInstance<() => number>;
    beforeEach(() => {
      scrollHeight = vi
        .spyOn(Element.prototype, "scrollHeight", "get")
        .mockImplementation(function (this: Element) {
          return this.classList.contains("mw-aistat-pop") ? 1200 : 0;
        });
    });
    afterEach(() => {
      scrollHeight.mockRestore();
      vi.unstubAllGlobals();
    });

    it("lets Tab into the popover, named by its own heading, and Escape back out", async () => {
      render(chip());
      const button = screen.getByRole("button", {
        name: new RegExp(LABELS.chip),
      });

      await userEvent.tab();
      await userEvent.tab();

      const region = screen.getByRole("region", { name: LABELS.answering });
      expect(region).toHaveFocus();
      expect(button).toHaveAttribute("aria-expanded", "true");
      expect(region).toHaveTextContent(LABELS.scope);

      await userEvent.keyboard("{Escape}");
      expect(button).toHaveFocus();
      expect(button).toHaveAttribute("aria-expanded", "false");
    });

    it("re-measures its room when the chip moves under it without a render", async () => {
      vi.stubGlobal("innerHeight", 800);
      // Each observer's callback is reached through the boxes it was asked to
      // watch, so the test holds which box the popover's room listens to.
      const observers: { fire: () => void; boxes: Element[] }[] = [];
      vi.stubGlobal(
        "ResizeObserver",
        class {
          private readonly watching: { fire: () => void; boxes: Element[] };
          constructor(fire: () => void) {
            this.watching = { fire, boxes: [] };
            observers.push(this.watching);
          }
          observe(box: Element) {
            this.watching.boxes.push(box);
          }
          disconnect() {
            this.watching.boxes = [];
          }
        },
      );
      const { container } = render(chip());
      const chipBox = container.firstElementChild;
      if (!(chipBox instanceof HTMLElement))
        throw new Error("no chip rendered");
      const foot = vi
        .spyOn(chipBox, "getBoundingClientRect")
        .mockReturnValue(new DOMRect(0, 60, 120, 40));

      await userEvent.tab();
      await userEvent.tab();
      const region = screen.getByRole("region", { name: LABELS.answering });
      expect(region.style.getPropertyValue("--aistatRoom")).toBe("700px");

      // The band wraps and pushes the chip 200px down; nothing re-renders it.
      foot.mockReturnValue(new DOMRect(0, 260, 120, 40));
      const watcher = observers.find(({ boxes }) => boxes.includes(chipBox));
      act(() => watcher?.fire());
      expect(region.style.getPropertyValue("--aistatRoom")).toBe("500px");

      await userEvent.keyboard("{Escape}");
      expect(watcher?.boxes).toEqual([]);
    });

    it("stays open when the window loses focus while the reader is in it", async () => {
      render(chip());
      await userEvent.tab();
      await userEvent.tab();
      const region = screen.getByRole("region", { name: LABELS.answering });

      // What a window blur delivers: focus leaves for nowhere, yet stays put.
      act(() => {
        region.dispatchEvent(
          new FocusEvent("focusout", { bubbles: true, relatedTarget: null }),
        );
      });

      expect(region).toHaveFocus();
      expect(
        screen.getByRole("button", { name: new RegExp(LABELS.chip) }),
      ).toHaveAttribute("aria-expanded", "true");
    });
  });

  it("stays closed when the pointer leaves a chip its press closed", async () => {
    const user = userEvent.setup();
    render(chip());
    const button = screen.getByRole("button", {
      name: new RegExp(LABELS.chip),
    });

    await user.hover(button);
    await user.click(button);
    await user.click(button);
    expect(button).toHaveAttribute("aria-expanded", "false");

    await user.unhover(button);
    expect(button).toHaveFocus();
    expect(button).toHaveAttribute("aria-expanded", "false");
  });

  it("opens again after focus leaves and comes back", async () => {
    render(chip());
    const button = screen.getByRole("button", {
      name: new RegExp(LABELS.chip),
    });

    await userEvent.tab();
    await userEvent.keyboard("{Escape}");
    expect(button).toHaveAttribute("aria-expanded", "false");

    button.blur();
    await userEvent.tab();
    expect(button).toHaveFocus();
    expect(button).toHaveAttribute("aria-expanded", "true");
  });
});
