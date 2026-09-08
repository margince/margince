/** @vitest-environment jsdom */
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { Popover } from "./popover";

afterEach(() => {
  cleanup();
  // A no-op unless a case below took the clock. One case does, and a fake
  // clock left standing would hand the next test a `setTimeout` nobody runs.
  vi.useRealTimers();
});

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

// The text-trigger shape, so the refusal is held on the bare `<button>` branch
// as well as on the `Button` one below — a prop honoured for one of a
// component's two shapes is a prop that works until someone drops the variant.
it("does not open when the trigger is refused", async () => {
  const user = userEvent.setup();
  render(
    <Popover label="How it stands" disabled>
      Two of three invoices are late.
    </Popover>,
  );
  const trigger = screen.getByRole("button");
  expect(trigger.hasAttribute("disabled")).toBe(true);

  await user.click(trigger);

  expect(screen.queryByText("Two of three invoices are late.")).toBeNull();
  expect(trigger.getAttribute("aria-expanded")).toBe("false");
});

// `disabled` blocks the OPENING and only that.
//
// A panel already open can hold the control that started the write — busy, and
// holding the reader's focus — so closing it under them is the one thing this
// state must not do. And the trigger stays focusable for as long as the panel
// is up, because both close paths hand focus back to it and `.focus()` on a
// natively disabled button is a silent no-op.
it("keeps an open panel, and a focusable trigger, when the caller refuses it", async () => {
  // One instance for the whole test: it carries the input-device state, so a
  // second one would forget which buttons and keys the first left held.
  const user = userEvent.setup();
  const { rerender } = render(
    <Popover label="How it stands" variant="ghost">
      Two of three invoices are late.
    </Popover>,
  );
  const trigger = screen.getByRole("button");
  await user.click(trigger);

  rerender(
    <Popover label="How it stands" variant="ghost" disabled>
      Two of three invoices are late.
    </Popover>,
  );

  expect(screen.getByText("Two of three invoices are late.")).toBeTruthy();
  expect(trigger.hasAttribute("disabled")).toBe(false);
  trigger.focus();
  expect(document.activeElement).toBe(trigger);

  await user.keyboard("{Escape}");

  // Closed by the reader, and only now refused — so the panel a write has
  // emptied cannot be opened a second time while that write is out.
  expect(screen.queryByText("Two of three invoices are late.")).toBeNull();
  expect(trigger.hasAttribute("disabled")).toBe(true);
});

// The settled pointer is the SECOND way in, and `disabled` has to refuse it
// too. The hover pair is spread on the same element as the press, so the native
// attribute looks like the guard — it is not: some browsers deliver pointer
// events to a disabled control, and the panel spreads the same pair on itself.
it("does not open on a settled pointer when the trigger is refused", async () => {
  // The hook reasons in `performance.now()`, so that clock is faked alongside
  // the timers. Left running, the poll measures a real elapsed time against a
  // simulated one, the settle never fires, and this case would pass without
  // the guard it exists to hold (hoverintent.ts says so in its own header).
  vi.useFakeTimers({
    toFake: [
      "setTimeout",
      "clearTimeout",
      "setInterval",
      "clearInterval",
      "performance",
    ],
  });
  render(
    <Popover label="How it stands" onHover disabled>
      Two of three invoices are late.
    </Popover>,
  );
  const trigger = screen.getByRole("button");

  // The pointer arrives by a raw dispatch rather than through `userEvent`,
  // which is how hoverintent.test.tsx drives this hook as well: `userEvent`
  // waits on the clock between events, and that clock is the one this case has
  // taken, so its first interaction never returns.
  fireEvent.pointerEnter(trigger);
  // Past the hook's CEILING, which settles whatever the pointer is doing — so
  // the settle callback really did run, and the guard is the only thing left
  // between it and an open panel.
  act(() => {
    vi.advanceTimersByTime(500);
  });

  expect(screen.queryByText("Two of three invoices are late.")).toBeNull();
  expect(trigger.getAttribute("aria-expanded")).toBe("false");
});
