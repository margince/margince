/** @vitest-environment happy-dom */

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { type ReactNode, StrictMode, useState } from "react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { Disclosure, DisclosureGroup } from "./disclosure";

// happy-dom toggles a `<details>` while a click passes it, before React's root
// listener can cancel it. A browser toggles after the dispatch: so does this.
const ACTIVATION = "a[href], button, label, input, select, textarea, summary";

function activate(event: Event) {
  if (event.defaultPrevented || !(event.target instanceof Element)) return;
  const summary = event.target.closest(ACTIVATION);
  const details = summary?.parentElement;
  if (
    summary?.localName === "summary" &&
    details instanceof HTMLDetailsElement
  ) {
    details.open = !details.open;
  }
}

beforeEach(() => {
  vi.spyOn(HTMLDetailsElement.prototype, "dispatchEvent").mockImplementation(
    function (this: HTMLDetailsElement, event: Event) {
      return HTMLElement.prototype.dispatchEvent.call(this, event);
    },
  );
  window.addEventListener("click", activate);
});

afterEach(() => {
  cleanup();
  window.removeEventListener("click", activate);
  vi.restoreAllMocks();
});

function summaryOf(name: string): HTMLElement {
  const summary = screen.getByText(name).closest("summary");
  if (summary === null) {
    throw new Error(`no summary reads ${name}`);
  }
  return summary;
}

function detailsOf(name: string): HTMLDetailsElement {
  const details = summaryOf(name).closest("details");
  if (details === null) {
    throw new Error(`no section is named ${name}`);
  }
  return details;
}

it("opens and closes from its summary, and says so on the summary", async () => {
  const user = userEvent.setup();
  const toggled: boolean[] = [];
  render(
    <Disclosure
      summary="Matching rules"
      onToggle={(open) => toggled.push(open)}
    >
      <p>Rule body</p>
    </Disclosure>,
  );
  const summary = summaryOf("Matching rules");
  expect(summary.getAttribute("aria-expanded")).toBe("false");
  const body = document.getElementById(
    summary.getAttribute("aria-controls") ?? "",
  );
  expect(body?.textContent).toBe("Rule body");

  await user.click(summary);
  expect(summary.getAttribute("aria-expanded")).toBe("true");
  expect(detailsOf("Matching rules").open).toBe(true);

  await user.click(summary);
  expect(summary.getAttribute("aria-expanded")).toBe("false");
  expect(detailsOf("Matching rules").open).toBe(false);
  expect(toggled).toEqual([true, false]);
});

it("keeps its content mounted while closed", () => {
  render(
    <Disclosure summary="Import log">
      <input aria-label="Kept draft" defaultValue="half typed" />
    </Disclosure>,
  );
  expect(detailsOf("Import log").open).toBe(false);
  expect(screen.getByLabelText<HTMLInputElement>("Kept draft").value).toBe(
    "half typed",
  );
});

it("follows a caller's open, and lets the reader close a forced section", async () => {
  const user = userEvent.setup();
  const { rerender } = render(
    <Disclosure summary="Run" open>
      <p>Running</p>
    </Disclosure>,
  );
  expect(detailsOf("Run").open).toBe(true);
  await user.click(summaryOf("Run"));
  expect(detailsOf("Run").open).toBe(false);

  rerender(
    <Disclosure summary="Run" open={false}>
      <p>Running</p>
    </Disclosure>,
  );
  rerender(
    <Disclosure summary="Run" open>
      <p>Running</p>
    </Disclosure>,
  );
  expect(detailsOf("Run").open).toBe(true);
  rerender(
    <Disclosure summary="Run" open={undefined}>
      <p>Running</p>
    </Disclosure>,
  );
  expect(detailsOf("Run").open).toBe(false);
});

it("keeps every section open at once outside a group", async () => {
  const user = userEvent.setup();
  render(
    <>
      <Disclosure summary="First">
        <p>One</p>
      </Disclosure>
      <Disclosure summary="Second">
        <p>Two</p>
      </Disclosure>
    </>,
  );
  await user.click(summaryOf("First"));
  await user.click(summaryOf("Second"));
  expect(detailsOf("First").open).toBe(true);
  expect(detailsOf("Second").open).toBe(true);
});

// The rail spreads its sections over several components, so membership is by
// context, not by being a direct child.
function Section({ name }: Readonly<{ name: string }>) {
  return (
    <div>
      <Disclosure summary={name}>
        <p>{name} body</p>
      </Disclosure>
    </div>
  );
}

it("opens one section of a group at a time", async () => {
  const user = userEvent.setup();
  const closed: boolean[] = [];
  render(
    <DisclosureGroup>
      <Disclosure summary="First" onToggle={(open) => closed.push(open)}>
        <p>One</p>
      </Disclosure>
      <Section name="Second" />
    </DisclosureGroup>,
  );
  await user.click(summaryOf("First"));
  await user.click(summaryOf("Second"));
  expect(summaryOf("First").getAttribute("aria-expanded")).toBe("false");
  expect(detailsOf("First").open).toBe(false);
  expect(detailsOf("Second").open).toBe(true);
  expect(closed).toEqual([true, false]);
});

it("never closes a group's section from a section nested inside it", async () => {
  const user = userEvent.setup();
  render(
    <DisclosureGroup>
      <Disclosure summary="Address">
        <Disclosure summary="Postal">
          <p>Street</p>
        </Disclosure>
      </Disclosure>
    </DisclosureGroup>,
  );
  await user.click(summaryOf("Address"));
  await user.click(summaryOf("Postal"));
  expect(detailsOf("Address").open).toBe(true);
  expect(detailsOf("Postal").open).toBe(true);
});

function ForcedPair({ children }: Readonly<{ children?: ReactNode }>) {
  const [running, setRunning] = useState(false);
  return (
    <DisclosureGroup>
      <button type="button" onClick={() => setRunning(true)}>
        Run
      </button>
      <Disclosure summary="Tool" open={running}>
        <p>Output</p>
      </Disclosure>
      <Disclosure summary="Notes">
        <p>Notes body</p>
      </Disclosure>
      {children}
    </DisclosureGroup>
  );
}

it("lets a caller's forced open win over the group's open section", async () => {
  const user = userEvent.setup();
  render(<ForcedPair />);
  await user.click(summaryOf("Notes"));
  await user.click(screen.getByRole("button", { name: "Run" }));
  expect(detailsOf("Tool").open).toBe(true);
  expect(detailsOf("Notes").open).toBe(false);
});

// A stand-in for the browser's height transition, since happy-dom runs no CSS.
function holdMotion(element: HTMLDetailsElement) {
  let end = () => {};
  const finished = new Promise<void>((resolve) => {
    end = resolve;
  });
  const motion = {
    effect: { getComputedTiming: () => ({ iterations: 1 }) },
    finished,
    transitionProperty: "height",
  };
  Object.defineProperty(element, "getAnimations", { value: () => [motion] });
  return end;
}

it("stays open through its close motion and shuts once the motion ends", async () => {
  render(
    <Disclosure summary="Long read" open>
      <p>Paragraphs</p>
    </Disclosure>,
  );
  const end = holdMotion(detailsOf("Long read"));
  const body = () => detailsOf("Long read").querySelector(".disclosure-body");
  expect(body()?.hasAttribute("inert")).toBe(false);
  fireEvent.click(summaryOf("Long read"));
  expect(summaryOf("Long read").getAttribute("aria-expanded")).toBe("false");
  expect(detailsOf("Long read").open).toBe(true);
  expect(detailsOf("Long read").hasAttribute("data-folding")).toBe(true);
  expect(body()?.hasAttribute("inert")).toBe(true);

  await act(async () => end());
  expect(detailsOf("Long read").open).toBe(false);
  expect(body()?.hasAttribute("inert")).toBe(false);
  expect(detailsOf("Long read").hasAttribute("data-folding")).toBe(false);
  expect(detailsOf("Long read").style.height).toBe("");
});

it("closes the open sibling while the new one opens, in one press", async () => {
  render(
    <DisclosureGroup>
      <Disclosure summary="First" open>
        <p>One</p>
      </Disclosure>
      <Disclosure summary="Second">
        <p>Two</p>
      </Disclosure>
    </DisclosureGroup>,
  );
  const end = holdMotion(detailsOf("First"));
  fireEvent.click(summaryOf("Second"));
  expect(summaryOf("First").getAttribute("aria-expanded")).toBe("false");
  expect(detailsOf("First").open).toBe(true);
  expect(detailsOf("Second").open).toBe(true);
  await act(async () => end());
  expect(detailsOf("First").open).toBe(false);
});

it("asks nothing of onToggle on mount, and asks on a press", async () => {
  const user = userEvent.setup();
  const toggled: boolean[] = [];
  render(
    <Disclosure summary="Run" open onToggle={(open) => toggled.push(open)}>
      <p>Running</p>
    </Disclosure>,
  );
  expect(toggled).toEqual([]);
  await user.click(summaryOf("Run"));
  expect(toggled).toEqual([false]);
});

it("shows a caller's open while the caller holds it, and asks on a press", async () => {
  const user = userEvent.setup();
  const asked: boolean[] = [];
  const { rerender } = render(
    <Disclosure summary="Words" open={false} onToggle={(o) => asked.push(o)}>
      <p>Describe the change</p>
    </Disclosure>,
  );
  await user.click(summaryOf("Words"));
  expect(asked).toEqual([true]);
  expect(summaryOf("Words").getAttribute("aria-expanded")).toBe("false");
  expect(detailsOf("Words").open).toBe(false);
  rerender(
    <Disclosure summary="Words" open onToggle={(o) => asked.push(o)}>
      <p>Describe the change</p>
    </Disclosure>,
  );
  expect(detailsOf("Words").open).toBe(true);
  expect(asked).toEqual([true]);
});

function HeldBeside() {
  const [open, setOpen] = useState(true);
  return (
    <DisclosureGroup>
      <Disclosure summary="Held" open={open} onToggle={setOpen}>
        <p>Held body</p>
      </Disclosure>
      <Section name="Free" />
    </DisclosureGroup>
  );
}

it("asks a held section to close when another one in its group opens", async () => {
  const user = userEvent.setup();
  render(<HeldBeside />);
  await user.click(summaryOf("Free"));
  expect(detailsOf("Held").open).toBe(false);
  expect(detailsOf("Free").open).toBe(true);
});

function LateRegistration({ loaded }: Readonly<{ loaded: boolean }>) {
  return (
    <DisclosureGroup>
      {loaded && (
        <Disclosure summary="Registration" open>
          <p>Filed in Berlin</p>
        </Disclosure>
      )}
      <Section name="Team" />
    </DisclosureGroup>
  );
}

// A motion that never ends shows whether a section mounted open, then closed.
it("keeps the section a reader opened when one mounts open later, shut from the start", async () => {
  const user = userEvent.setup();
  const { rerender } = render(<LateRegistration loaded={false} />);
  await user.click(summaryOf("Team"));
  const motion = {
    effect: { getComputedTiming: () => ({ iterations: 1 }) },
    finished: new Promise(() => {}),
    transitionProperty: "height",
  };
  Object.defineProperty(HTMLDetailsElement.prototype, "getAnimations", {
    configurable: true,
    value: () => [motion],
  });
  try {
    rerender(<LateRegistration loaded />);
  } finally {
    Reflect.deleteProperty(HTMLDetailsElement.prototype, "getAnimations");
  }
  expect(detailsOf("Team").open).toBe(true);
  expect(detailsOf("Registration").open).toBe(false);
  expect(detailsOf("Registration").hasAttribute("data-folding")).toBe(false);
});

it("opens a section mounted open when its group has none open", async () => {
  const { rerender } = render(<LateRegistration loaded={false} />);
  rerender(<LateRegistration loaded />);
  expect(detailsOf("Registration").open).toBe(true);
  expect(detailsOf("Team").open).toBe(false);
});

it("leaves the group to the first of two sections mounted open, the second shut from the start", () => {
  const motion = {
    effect: { getComputedTiming: () => ({ iterations: 1 }) },
    finished: new Promise(() => {}),
    transitionProperty: "height",
  };
  Object.defineProperty(HTMLDetailsElement.prototype, "getAnimations", {
    configurable: true,
    value: () => [motion],
  });
  try {
    render(
      <StrictMode>
        <DisclosureGroup>
          <Disclosure summary="First" open>
            <p>One</p>
          </Disclosure>
          <Disclosure summary="Second" open>
            <p>Two</p>
          </Disclosure>
        </DisclosureGroup>
      </StrictMode>,
    );
  } finally {
    Reflect.deleteProperty(HTMLDetailsElement.prototype, "getAnimations");
  }
  expect(detailsOf("First").open).toBe(true);
  expect(detailsOf("Second").open).toBe(false);
  expect(detailsOf("Second").hasAttribute("data-folding")).toBe(false);
});

function PinnedAfterFree() {
  const [open, setOpen] = useState(false);
  const [pinned, setPinned] = useState(false);
  return (
    <DisclosureGroup>
      <Section name="Free" />
      <Disclosure
        summary="Pinned"
        open={open}
        onToggle={(wanted) => pinned || setOpen(wanted)}
      >
        <p>Pinned body</p>
      </Disclosure>
      <button type="button" onClick={() => setPinned(true)}>
        Pin
      </button>
    </DisclosureGroup>
  );
}

it("keeps the section the reader opened beside a held one that refuses to close", async () => {
  const user = userEvent.setup();
  render(
    <StrictMode>
      <PinnedAfterFree />
    </StrictMode>,
  );
  await user.click(summaryOf("Pinned"));
  await user.click(screen.getByRole("button", { name: "Pin" }));
  await user.click(summaryOf("Free"));
  expect(detailsOf("Free").open).toBe(true);
  expect(detailsOf("Pinned").open).toBe(true);
});

function HeldWithAction({ open }: Readonly<{ open: boolean }>) {
  return (
    <Disclosure summary="Words" open={open} onToggle={() => {}}>
      <button type="button">Send</button>
    </Disclosure>
  );
}

it("hands focus to the summary when code closes the section that holds it", () => {
  const { rerender } = render(<HeldWithAction open />);
  screen.getByRole("button", { name: "Send" }).focus();
  rerender(<HeldWithAction open={false} />);
  expect(document.activeElement).toBe(summaryOf("Words"));
});

it("hands focus to the summary of a group's section another one closes", () => {
  render(
    <DisclosureGroup>
      <Disclosure summary="First" open>
        <button type="button">Call</button>
      </Disclosure>
      <Section name="Second" />
    </DisclosureGroup>,
  );
  screen.getByRole("button", { name: "Call" }).focus();
  fireEvent.click(summaryOf("Second"));
  expect(document.activeElement).toBe(summaryOf("First"));
});

it("leaves a press on a link in the summary to the link", async () => {
  const user = userEvent.setup();
  const followed: boolean[] = [];
  const record = (event: Event) => followed.push(event.defaultPrevented);
  window.addEventListener("click", record);
  render(
    <Disclosure summary={<a href="#/deals">Deals</a>}>
      <p>Deal list</p>
    </Disclosure>,
  );
  try {
    await user.click(screen.getByRole("link", { name: "Deals" }));
  } finally {
    window.removeEventListener("click", record);
  }
  expect(followed).toEqual([false]);
  expect(summaryOf("Deals").getAttribute("aria-expanded")).toBe("false");
  expect(detailsOf("Deals").open).toBe(false);
});

it("never toggles on a press into a field in the summary", async () => {
  const user = userEvent.setup();
  render(
    <Disclosure summary={<input aria-label="Rename" defaultValue="Rules" />}>
      <p>Rule body</p>
    </Disclosure>,
  );
  const field = screen.getByLabelText("Rename");
  await user.click(field);
  const summary = field.closest("summary");
  expect(summary?.getAttribute("aria-expanded")).toBe("false");
  expect(summary?.closest("details")?.open).toBe(false);
});

it("refuses the native toggle on a scripted button in the summary", () => {
  render(
    <Disclosure
      summary={
        // biome-ignore lint/a11y/useSemanticElements: the case is a button with no native activation
        <span role="button" tabIndex={0}>
          Chip
        </span>
      }
    >
      <p>Chip body</p>
    </Disclosure>,
  );
  const press = new MouseEvent("click", { bubbles: true, cancelable: true });
  screen.getByRole("button", { name: "Chip" }).dispatchEvent(press);
  expect(press.defaultPrevented).toBe(true);
  expect(summaryOf("Chip").getAttribute("aria-expanded")).toBe("false");
  expect(detailsOf("Chip").open).toBe(false);
});

it.each(["file", "color", "date"])(
  "leaves a press on a %s input in the summary to its picker",
  (type) => {
    render(
      <Disclosure summary={<input type={type} aria-label="Pick" />}>
        <p>Picked</p>
      </Disclosure>,
    );
    const press = new MouseEvent("click", { bubbles: true, cancelable: true });
    screen.getByLabelText("Pick").dispatchEvent(press);
    expect(press.defaultPrevented).toBe(false);
    const summary = screen.getByLabelText("Pick").closest("summary");
    expect(summary?.getAttribute("aria-expanded")).toBe("false");
    expect(summary?.closest("details")?.open).toBe(false);
  },
);

it("shows a held section as its caller passes it, even when its group asks it shut", async () => {
  const user = userEvent.setup();
  const asked: boolean[] = [];
  const held = (
    <Disclosure summary="Held" open onToggle={(open) => asked.push(open)}>
      <p>Held body</p>
    </Disclosure>
  );
  const { rerender } = render(
    <DisclosureGroup>
      <Section name="Free" />
    </DisclosureGroup>,
  );
  await user.click(summaryOf("Free"));
  rerender(
    <DisclosureGroup>
      <Section name="Free" />
      {held}
    </DisclosureGroup>,
  );
  expect(asked).toEqual([false]);
  expect(detailsOf("Held").open).toBe(true);
});
