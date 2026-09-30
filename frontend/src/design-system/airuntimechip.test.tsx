// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
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
