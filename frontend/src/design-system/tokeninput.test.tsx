/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, describe, expect, it } from "vitest";
import { TokenInput, TokenList } from "./tokeninput";

// The `in` operator's value control. Every rule below is one a reader would
// otherwise discover by losing a value they typed.

afterEach(cleanup);

/** Controlled, because a test that never feeds the value back proves only that
 *  the callback fired. */
function Harness({ start = [] }: Readonly<{ start?: readonly string[] }>) {
  const [values, setValues] = useState<readonly string[]>(start);
  return (
    <TokenInput
      values={values}
      onChange={setValues}
      aria-label="Region"
      placeholder="DE, AT"
    />
  );
}

// The query's own type argument, rather than an assertion on its result: the
// value assertions below need the input's `value`, and naming the element type
// here asks the query for it instead of overriding what it answered.
const box = () =>
  screen.getByRole<HTMLInputElement>("textbox", { name: "Region" });

describe("committing a value", () => {
  it("turns typed text into a token on Enter", async () => {
    const user = userEvent.setup();
    render(<Harness />);

    await user.type(box(), "DE{Enter}");

    expect(screen.getByText("DE")).toBeTruthy();
    expect(box().value).toBe("");
  });

  it("commits on comma too, so a pasted list does not become one token", async () => {
    const user = userEvent.setup();
    render(<Harness />);

    await user.type(box(), "DE,AT,");

    expect(screen.getByText("DE")).toBeTruthy();
    expect(screen.getByText("AT")).toBeTruthy();
  });

  it("splits a single commit carrying several values", async () => {
    const user = userEvent.setup();
    render(<Harness />);

    // What a paste looks like once it reaches the box: one value with commas in
    // it, committed at once.
    await user.type(box(), "DE, AT, CH{Enter}");

    for (const region of ["DE", "AT", "CH"]) {
      expect(screen.getByText(region), region).toBeTruthy();
    }
  });

  it("commits on blur, so clicking away does not discard what was typed", async () => {
    const user = userEvent.setup();
    render(<Harness />);

    await user.type(box(), "DE");
    await user.tab();

    expect(screen.getByText("DE")).toBeTruthy();
  });

  it("drops a blank, which would otherwise match the empty string", async () => {
    const user = userEvent.setup();
    render(<Harness />);

    await user.type(box(), "   {Enter}");

    // Nothing was added: the only textbox content is the box itself.
    expect(screen.queryByRole("button")).toBeNull();
  });

  it("drops a duplicate silently, because `in` is a set", async () => {
    const user = userEvent.setup();
    render(<Harness start={["DE"]} />);

    await user.type(box(), "DE{Enter}");

    // One token, and no refusal shown — admitting DE twice would change nothing
    // about what matches, so a message would be noise.
    expect(screen.getAllByRole("button", { name: /^Remove/ })).toHaveLength(1);
  });

  it("drops a value a single commit repeats to itself", async () => {
    const user = userEvent.setup();
    render(<Harness />);

    // The colliding value is not on screen yet, so checking the committed set
    // alone admits both halves. What the reader would then see is two DE tokens
    // sharing one React key, and a remove control that takes away both.
    await user.type(box(), "DE, AT, DE{Enter}");

    expect(screen.getAllByRole("button", { name: /^Remove/ })).toHaveLength(2);
    expect(screen.getByRole("button", { name: "Remove DE" })).toBeTruthy();
  });
});

describe("removing a value", () => {
  it("removes the last token on Backspace in an empty box", async () => {
    const user = userEvent.setup();
    render(<Harness start={["DE", "AT"]} />);

    await user.click(box());
    await user.keyboard("{Backspace}");

    expect(screen.queryByText("AT")).toBeNull();
    expect(screen.getByText("DE")).toBeTruthy();
  });

  it("leaves the tokens alone when Backspace has text to delete instead", async () => {
    const user = userEvent.setup();
    render(<Harness start={["DE"]} />);

    await user.type(box(), "AT{Backspace}");

    // The keystroke edited the typed text, not the committed set — otherwise a
    // typo correction would silently eat a value the reader had finished with.
    expect(screen.getByText("DE")).toBeTruthy();
    expect(box().value).toBe("A");
  });

  it("removes the token its own control names, not the last one", async () => {
    const user = userEvent.setup();
    render(<Harness start={["DE", "AT", "CH"]} />);

    await user.click(screen.getByRole("button", { name: "Remove AT" }));

    expect(screen.queryByText("AT")).toBeNull();
    expect(screen.getByText("DE")).toBeTruthy();
    expect(screen.getByText("CH")).toBeTruthy();
  });
});

// The same token without the text box: a set built somewhere else, drawn as a
// set. What matters is that each remove control names its OWN token and that a
// reader who may not change the set is not shown controls that refuse.
describe("TokenList", () => {
  const CONTACTS = [
    { id: "8801", label: "Chi Mai" },
    { id: "8802", label: "Anh Tuan" },
  ];

  it("draws a list, and removes the token its own control names", async () => {
    const user = userEvent.setup();
    const removed: string[] = [];
    render(
      <TokenList
        items={CONTACTS}
        removeLabel={(item) => `Take ${item.label} off the list`}
        onRemove={(id) => removed.push(id)}
      />,
    );

    // A list, so a screen reader can say how many before walking them.
    expect(screen.getAllByRole("listitem")).toHaveLength(2);
    await user.click(
      screen.getByRole("button", { name: "Take Anh Tuan off the list" }),
    );
    expect(removed).toEqual(["8802"]);
  });

  it("offers no remove control at all when the set is read-only", () => {
    render(
      <TokenList
        items={CONTACTS}
        removeLabel={(item) => `Take ${item.label} off the list`}
      />,
    );

    // Absent rather than disabled: a viewer who may not change a set is not one
    // whose controls are temporarily unavailable.
    expect(screen.queryAllByRole("button")).toHaveLength(0);
    expect(screen.getByText("Chi Mai")).toBeTruthy();
  });

  it("refuses removal while the caller is busy, without hiding the set", () => {
    render(
      <TokenList
        items={CONTACTS}
        disabled
        removeLabel={(item) => `Take ${item.label} off the list`}
        onRemove={() => {}}
      />,
    );

    for (const button of screen.getAllByRole("button")) {
      expect(button.hasAttribute("disabled")).toBe(true);
    }
    expect(screen.getAllByRole("listitem")).toHaveLength(2);
  });
});

// The optional half: a vocabulary the field offers while the reader types. Every
// rule below is one that decides whether the list is HELP or a constraint.

const CONTACTS = [
  { value: "dana@nordwand.example", label: "Dana Ellwanger", hint: "Nordwand" },
  { value: "milo@nordwand.example", label: "Milo Fenn", hint: "Nordwand" },
] as const;

function Offering({ start = [] }: Readonly<{ start?: readonly string[] }>) {
  const [values, setValues] = useState<readonly string[]>(start);
  return (
    <TokenInput
      values={values}
      onChange={setValues}
      suggestions={CONTACTS}
      aria-label="To"
      placeholder="name@example.com"
    />
  );
}

const toBox = () =>
  screen.getByRole<HTMLInputElement>("combobox", { name: "To" });

describe("offering a vocabulary", () => {
  // A `combobox` that never opens tells a reader to press a key that does
  // nothing, so a field with no vocabulary must not claim the role. `box()`
  // resolving at all is the textbox half; the absence of a combobox is the other.
  it("stays a plain text box when the call site declared no list", () => {
    render(<Harness />);
    expect(box()).toBeTruthy();
    expect(screen.queryByRole("combobox")).toBeNull();
  });

  it("offers the rows on focus and commits the one that is picked", async () => {
    const user = userEvent.setup();
    render(<Offering />);
    await user.click(toBox());
    await user.click(screen.getByRole("option", { name: /Dana Ellwanger/ }));
    expect(screen.getByText("dana@nordwand.example")).toBeTruthy();
    // The box empties on a pick exactly as it does on a typed commit: what was
    // being typed became the token, so leaving it behind would offer the reader
    // a second copy of what they just chose.
    expect(toBox().value).toBe("");
  });

  it("finds a row by the contact's name, not only by the address", async () => {
    const user = userEvent.setup();
    render(<Offering />);
    await user.type(toBox(), "Fenn");
    expect(screen.getByRole("option", { name: /Milo Fenn/ })).toBeTruthy();
    expect(screen.queryByRole("option", { name: /Dana/ })).toBeNull();
  });

  it("stops offering a value the field already holds", async () => {
    const user = userEvent.setup();
    render(<Offering start={["dana@nordwand.example"]} />);
    await user.click(toBox());
    expect(screen.queryByRole("option", { name: /Dana/ })).toBeNull();
    expect(screen.getByRole("option", { name: /Milo/ })).toBeTruthy();
  });

  // The list is help, never a constraint: an address nobody has on file is the
  // ordinary case for a first message, and a field that refused it would be
  // worse than the plain box it replaced.
  it("commits a typed value that is on no list", async () => {
    const user = userEvent.setup();
    render(<Offering />);
    await user.type(toBox(), "stranger@example.com{Enter}");
    expect(screen.getByText("stranger@example.com")).toBeTruthy();
  });

  it("takes the highlighted row on Enter rather than the typed text", async () => {
    const user = userEvent.setup();
    render(<Offering />);
    await user.type(toBox(), "dan");
    await user.keyboard("{ArrowDown}{Enter}");
    expect(screen.getByText("dana@nordwand.example")).toBeTruthy();
    expect(screen.queryByText("dan")).toBeNull();
  });
});
