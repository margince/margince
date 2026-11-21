/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, describe, expect, it } from "vitest";
import { Textarea, TextInput } from "./field";

// The server counts characters, so the box does too: 119 letters and one emoji
// are 120 characters and 121 UTF-16 units.

afterEach(cleanup);

const LETTERS = "a".repeat(119);

function Controlled({
  multiline = false,
  start = "",
}: Readonly<{ multiline?: boolean; start?: string }>) {
  const [value, setValue] = useState(start);
  const props = {
    "aria-label": "Name",
    maxLength: 120,
    value,
    onChange: (
      event: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>,
    ) => setValue(event.target.value),
  };
  return (
    <>
      {multiline ? <Textarea {...props} /> : <TextInput {...props} />}
      <output>{value}</output>
    </>
  );
}

const box = () => screen.getByLabelText<HTMLInputElement>("Name");

describe.each([
  ["a text input", false],
  ["a text area", true],
])("the character limit of %s", (_, multiline) => {
  it("takes an emoji as one character", async () => {
    render(<Controlled multiline={multiline} start={LETTERS} />);
    await userEvent.type(box(), "🚀");
    expect(box().value).toBe(`${LETTERS}🚀`);
    expect(screen.getByRole("status").textContent).toBe(`${LETTERS}🚀`);
  });

  it("takes nothing past the limit", async () => {
    render(<Controlled multiline={multiline} start={`${LETTERS}🚀`} />);
    await userEvent.type(box(), "b");
    expect(box().value).toBe(`${LETTERS}🚀`);
  });

  it("sets no native limit, which counts UTF-16 units", () => {
    render(<Controlled multiline={multiline} />);
    expect(box()).not.toHaveAttribute("maxlength");
    expect(box()).toHaveAttribute("data-character-limit", "120");
  });
});

describe("an edit that does not fit", () => {
  it("keeps the start of a paste and the text after the caret", async () => {
    const user = userEvent.setup();
    render(<Controlled start={"a".repeat(116)} />);
    box().focus();
    box().setSelectionRange(58, 58);
    await user.paste("🚀😀xyz");
    expect(box().value).toBe(`${"a".repeat(58)}🚀😀xy${"a".repeat(58)}`);
  });

  // An IME word is still open while it is typed, so the cut waits for it.
  it("cuts a composed word once it is picked, and says so to the form", () => {
    render(<Controlled start={LETTERS} />);
    const element = box();
    const native = Object.getOwnPropertyDescriptor(
      HTMLInputElement.prototype,
      "value",
    );
    native?.set?.call(element, `${LETTERS}việt`);
    fireEvent.input(element, { isComposing: true });
    expect(element.value).toBe(`${LETTERS}việt`);
    fireEvent.compositionEnd(element);
    expect(element.value).toBe(`${LETTERS}v`);
    expect(screen.getByRole("status").textContent).toBe(`${LETTERS}v`);
  });
});
