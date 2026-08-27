/** @vitest-environment jsdom */
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it } from "vitest";
import { Popover } from "./popover";

afterEach(cleanup);

// The whole reason this is not a Disclosure: the aside is not on the page
// until it is asked for, and asking for it does not move what is around it.
it("keeps the aside off the page until the trigger is pressed", async () => {
  render(
    <Popover label="How it stands">Two of three invoices are late.</Popover>,
  );

  expect(screen.queryByText("Two of three invoices are late.")).toBeNull();
  expect(screen.getByRole("button").getAttribute("aria-expanded")).toBe(
    "false",
  );

  await userEvent.click(screen.getByRole("button", { name: "How it stands" }));

  expect(screen.getByText("Two of three invoices are late.")).toBeTruthy();
  expect(screen.getByRole("button").getAttribute("aria-expanded")).toBe("true");
});

// The panel is portalled to the body, so it is nowhere near its trigger in the
// tree. It has to say what it is an aside TO, or a screen reader meets a
// paragraph belonging to nothing.
it("names the panel by the trigger that opened it", async () => {
  render(
    <Popover label="What makes up this score">
      Payment, replies, spread.
    </Popover>,
  );
  await userEvent.click(screen.getByRole("button"));

  const panel = screen.getByRole("region", {
    name: "What makes up this score",
  });
  expect(panel.textContent).toBe("Payment, replies, spread.");
});

// Escape puts the reader back on the button rather than at the top of the
// document, which is where focus lands when the node holding it disappears.
it("closes on Escape and hands focus back to the trigger", async () => {
  render(
    <Popover label="How it stands">Two of three invoices are late.</Popover>,
  );
  const trigger = screen.getByRole("button");
  await userEvent.click(trigger);

  await userEvent.keyboard("{Escape}");

  expect(screen.queryByText("Two of three invoices are late.")).toBeNull();
  expect(document.activeElement).toBe(trigger);
});

it("closes when the reader clicks away from it", async () => {
  render(
    <>
      <p>Elsewhere on the card</p>
      <Popover label="How it stands">Two of three invoices are late.</Popover>
    </>,
  );
  await userEvent.click(screen.getByRole("button"));

  await userEvent.click(screen.getByText("Elsewhere on the card"));

  expect(screen.queryByText("Two of three invoices are late.")).toBeNull();
});

// A click INSIDE the panel is a click outside the trigger's own box, because
// the panel is portalled — the panel has to be part of "inside" or its own
// content dismisses it.
it("stays open when the reader clicks inside the panel", async () => {
  render(
    <Popover label="How it stands">
      <a href="#invoices">The three invoices</a>
    </Popover>,
  );
  await userEvent.click(screen.getByRole("button", { name: "How it stands" }));

  await userEvent.click(
    screen.getByRole("link", { name: "The three invoices" }),
  );

  expect(screen.getByRole("link", { name: "The three invoices" })).toBeTruthy();
});

// A panel with controls in it is a panel a keyboard reader has to be able to
// reach. Prose takes no focus, so the reader stays on the trigger.
it("puts focus on the panel's first control, and leaves prose alone", async () => {
  const { unmount } = render(
    <Popover label="Send options">
      <button type="button">Schedule send</button>
    </Popover>,
  );
  await userEvent.click(screen.getByRole("button", { name: "Send options" }));
  expect(document.activeElement).toBe(
    screen.getByRole("button", { name: "Schedule send" }),
  );
  unmount();

  render(
    <Popover label="How it stands">Two of three invoices are late.</Popover>,
  );
  const trigger = screen.getByRole("button", { name: "How it stands" });
  await userEvent.click(trigger);
  expect(document.activeElement).toBe(trigger);
});

// A receipt under a reading is read on the way past. It opens when the pointer
// settles and closes when it leaves — and it still answers a click, because a
// touch screen and a keyboard have no hover to give it.
it("opens on a settled pointer only when the caller asks for it", async () => {
  const { unmount } = render(
    <Popover label="How it stands">Two of three invoices are late.</Popover>,
  );
  fireEvent.pointerEnter(screen.getByRole("button"));
  expect(screen.queryByText("Two of three invoices are late.")).toBeNull();
  unmount();

  render(
    <Popover label="How it stands" onHover>
      Two of three invoices are late.
    </Popover>,
  );
  fireEvent.pointerEnter(screen.getByRole("button"));
  await waitFor(() =>
    expect(screen.getByText("Two of three invoices are late.")).toBeTruthy(),
  );
});

it("still opens on a click when it opens on hover", async () => {
  render(
    <Popover label="How it stands" onHover>
      Two of three invoices are late.
    </Popover>,
  );

  await userEvent.click(screen.getByRole("button"));

  expect(screen.getByText("Two of three invoices are late.")).toBeTruthy();
});
