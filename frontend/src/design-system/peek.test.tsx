// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { armHoverIntent, takeHoverClock } from "./hoverintent-testing";
import { type PeekContent, usePeek } from "./peek";

afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

const deals: PeekContent = {
  head: "Open deals on this account",
  items: [
    {
      key: "a",
      title: "Expansion",
      sub: "Negotiation · closes 30 Nov",
      right: "€95k",
      href: "#/deals/a",
    },
    { key: "b", title: "Renewal", sub: "Proposal", onOpen: () => undefined },
  ],
  foot: "Showing 2 of 4.",
};

// A tile with a door of its own: the peek hangs off the tile and the door keeps
// its press.
function Tile({
  content = deals,
  onOpen = () => undefined,
}: Readonly<{ content?: PeekContent; onOpen?: () => void }>) {
  const peek = usePeek<HTMLElement>({ content });
  return (
    <section ref={peek.ref} {...peek.props}>
      <button type="button" onClick={onOpen}>
        Open pipeline
      </button>
      {peek.panel}
    </section>
  );
}

// An element with no door of its own asks for the press.
function Reading() {
  const peek = usePeek<HTMLButtonElement>({ content: deals, pressOpens: true });
  return (
    <>
      <button ref={peek.ref} type="button" {...peek.props}>
        4 open
      </button>
      {peek.panel}
    </>
  );
}

it("opens on a settled pointer and names what it counts", () => {
  takeHoverClock();
  armHoverIntent();
  render(<Tile />);

  fireEvent.pointerEnter(
    screen.getByRole("button", { name: "Open pipeline" })
      .parentElement as HTMLElement,
  );
  act(() => {
    vi.advanceTimersByTime(10_000);
  });

  const card = screen.getByRole("region", {
    name: "Open deals on this account",
  });
  expect(card.textContent).toContain("Expansion");
  expect(card.textContent).toContain("€95k");
  expect(card.textContent).toContain("Showing 2 of 4.");
});

it("opens when keyboard focus lands on the door inside the tile", async () => {
  const user = userEvent.setup();
  render(<Tile />);

  await user.tab();

  expect(
    screen.getByRole("region", { name: "Open deals on this account" }),
  ).toBeTruthy();
});

it("leaves a door's press to the door", async () => {
  const onOpen = vi.fn();
  const user = userEvent.setup();
  render(<Tile onOpen={onOpen} />);

  await user.click(screen.getByRole("button", { name: "Open pipeline" }));

  expect(onOpen).toHaveBeenCalledOnce();
});

it("draws nothing for a count with nothing behind it", async () => {
  const user = userEvent.setup();
  render(<Tile content={{ head: "Open deals", items: [] }} />);

  await user.tab();

  expect(screen.queryByRole("region")).toBeNull();
});

it("opens on a press where there is no door, and takes focus to the first row", async () => {
  const user = userEvent.setup();
  render(<Reading />);

  await user.click(screen.getByRole("button", { name: "4 open" }));

  const first = screen.getByRole("link", { name: "Expansion" });
  expect(first.getAttribute("href")).toBe("#/deals/a");
  expect(document.activeElement).toBe(first);
});

it("closes on Escape and gives focus back to the count", async () => {
  const user = userEvent.setup();
  render(<Reading />);
  const trigger = screen.getByRole("button", { name: "4 open" });
  await user.click(trigger);

  await user.keyboard("{Escape}");

  expect(screen.queryByRole("region")).toBeNull();
  expect(document.activeElement).toBe(trigger);
});

it("stays open while the reader works inside it and closes on a click away", async () => {
  const user = userEvent.setup();
  render(
    <>
      <p>Elsewhere</p>
      <Reading />
    </>,
  );
  await user.click(screen.getByRole("button", { name: "4 open" }));

  await user.click(screen.getByRole("button", { name: "Renewal" }));
  expect(screen.getByRole("region")).toBeTruthy();

  await user.click(screen.getByText("Elsewhere"));
  expect(screen.queryByRole("region")).toBeNull();
});
