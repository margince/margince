/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useRef, useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ConfirmModal } from "./confirmmodal";
import { ConfirmPhraseBox, phraseMatches } from "./confirmphrase";

afterEach(cleanup);

describe("phraseMatches", () => {
  it("trims the typed side and nothing else", () => {
    expect(phraseMatches("  Brandt GmbH ", "Brandt GmbH")).toBe(true);
    expect(phraseMatches("Brandt GmbH", " Brandt GmbH")).toBe(false);
  });

  it("is case-sensitive", () => {
    expect(phraseMatches("erase", "ERASE")).toBe(false);
    expect(phraseMatches("ERASE", "ERASE")).toBe(true);
  });

  it("treats a composed and a decomposed letter as the same letter", () => {
    const composed = "Đà Nẵng";
    const decomposed = composed.normalize("NFD");
    expect(decomposed).not.toBe(composed);
    expect(phraseMatches(decomposed, composed)).toBe(true);
    expect(phraseMatches(composed, decomposed)).toBe(true);
  });

  it("never matches an empty phrase", () => {
    expect(phraseMatches("", "")).toBe(false);
    expect(phraseMatches("   ", "")).toBe(false);
  });
});

function Box({ initial = "" }: Readonly<{ initial?: string }>) {
  const [value, setValue] = useState(initial);
  return (
    <ConfirmPhraseBox
      title="This deletes every record"
      impact={["Contacts and deals are deleted.", "Users stay."]}
      prompt="Type this company name:"
      phrase="Brandt GmbH"
      value={value}
      onChange={setValue}
    />
  );
}

describe("ConfirmPhraseBox", () => {
  it("names the field with the prompt and the phrase to type", () => {
    render(<Box />);
    expect(
      screen.getByRole("textbox", {
        name: "Type this company name: Brandt GmbH",
      }),
    ).toBeTruthy();
    expect(screen.getByText("This deletes every record")).toBeTruthy();
    expect(screen.getAllByRole("listitem")).toHaveLength(2);
  });

  it("draws the plate only when it has a title, and no empty body under one", () => {
    const { container, rerender } = render(
      <ConfirmPhraseBox
        prompt="Type this word to confirm:"
        phrase="ERASE"
        value=""
        onChange={() => {}}
      />,
    );
    expect(container.querySelector(".callout")).toBeNull();

    rerender(
      <ConfirmPhraseBox
        title="This erases the contact"
        prompt="Type this word to confirm:"
        phrase="ERASE"
        value=""
        onChange={() => {}}
      />,
    );
    expect(container.querySelector(".callout-danger")).not.toBeNull();
    expect(container.querySelector(".callout-text")).toBeNull();
  });

  it("says the phrase matches only once it does, in a status region", async () => {
    const user = userEvent.setup();
    render(<Box />);
    const status = screen.getByRole("status");
    expect(status.textContent).toBe("");

    await user.type(screen.getByRole("textbox"), "Brandt");
    expect(status.textContent).toBe("");

    await user.type(screen.getByRole("textbox"), " GmbH");
    expect(status.textContent).toBe("Phrase matches");
  });
});

function Dialog({ onConfirm }: Readonly<{ onConfirm: () => void }>) {
  const [open, setOpen] = useState(true);
  const [typed, setTyped] = useState("");
  const inputRef = useRef<HTMLInputElement>(null);
  const close = () => {
    setOpen(false);
    setTyped("");
  };
  return (
    <>
      <button type="button" onClick={() => setOpen(true)}>
        Reset data
      </button>
      <ConfirmModal
        open={open}
        onClose={close}
        title="Reset all data?"
        confirmLabel="Delete all data"
        confirmVariant="danger"
        confirmDisabled={!phraseMatches(typed, "Brandt GmbH")}
        initialFocusTo={() => inputRef.current}
        onConfirm={onConfirm}
      >
        <ConfirmPhraseBox
          prompt="Type this company name:"
          phrase="Brandt GmbH"
          value={typed}
          onChange={setTyped}
          inputRef={inputRef}
        />
      </ConfirmModal>
    </>
  );
}

describe("ConfirmPhraseBox in a ConfirmModal", () => {
  it("holds the confirm until the phrase is typed, and again once it is cleared", async () => {
    const user = userEvent.setup();
    const onConfirm = vi.fn();
    render(<Dialog onConfirm={onConfirm} />);
    const confirm = () =>
      screen.getByRole("button", {
        name: "Delete all data",
      }) as HTMLButtonElement;
    const input = screen.getByRole("textbox");

    expect(confirm().disabled).toBe(true);
    await user.click(confirm());
    expect(onConfirm).not.toHaveBeenCalled();

    await user.type(input, "Brandt GmbH");
    expect(confirm().disabled).toBe(false);
    await user.click(confirm());
    expect(onConfirm).toHaveBeenCalledTimes(1);

    await user.clear(input);
    expect(confirm().disabled).toBe(true);
  });

  it("opens empty again after Escape, because the caller resets the text", async () => {
    const user = userEvent.setup();
    render(<Dialog onConfirm={() => {}} />);

    await user.type(screen.getByRole("textbox"), "Brandt");
    await user.keyboard("{Escape}");
    expect(screen.queryByRole("dialog")).toBeNull();

    await user.click(screen.getByRole("button", { name: "Reset data" }));
    expect((screen.getByRole("textbox") as HTMLInputElement).value).toBe("");
  });
});
