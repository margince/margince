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
import { type ReactNode, useState } from "react";
import { afterEach, expect, it, vi } from "vitest";
import { Disclosure, DisclosureGroup } from "./disclosure";

afterEach(() => {
  cleanup();
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
// happy-dom also toggles a `<details>` before React's listener can refuse it.
// So the summary refuses the native toggle itself, as a browser lets it.
function holdMotion(element: HTMLDetailsElement) {
  element
    .querySelector("summary")
    ?.addEventListener("click", (event) => event.preventDefault());
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
  fireEvent.click(summaryOf("Long read"));
  expect(summaryOf("Long read").getAttribute("aria-expanded")).toBe("false");
  expect(detailsOf("Long read").open).toBe(true);
  expect(detailsOf("Long read").hasAttribute("data-folding")).toBe(true);

  await act(async () => end());
  expect(detailsOf("Long read").open).toBe(false);
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
