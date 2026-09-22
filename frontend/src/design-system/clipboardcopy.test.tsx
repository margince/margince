/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, describe, expect, it } from "vitest";
import { LocaleProvider } from "../i18n";
import { type ClipboardStub, stubClipboard } from "./clipboard-testing";
import { useClipboardCopy } from "./clipboardcopy";

const LABELS = {
  copy: "Copy link",
  copied: "Copied",
  remedy: "Select it in the field and copy it by hand.",
} as const;

let clipboard: ClipboardStub | null = null;

afterEach(() => {
  cleanup();
  clipboard?.restore();
  clipboard = null;
});

/**
 * A caller, wired the way the real ones are: the label on the button, the
 * notice wherever the screen keeps its notices.
 *
 * The text is state rather than a prop so a test can change what is on offer
 * WITHOUT remounting — a remount resets the hook and would prove nothing about
 * a claim going stale.
 */
function CopyProbe({ initial }: Readonly<{ initial: string }>) {
  const [text, setText] = useState(initial);
  const copy = useClipboardCopy(text, LABELS);
  return (
    <>
      <button type="button" onClick={copy.copy}>
        {copy.label}
      </button>
      <button type="button" onClick={() => setText(`${text}!`)}>
        Edit the text
      </button>
      {copy.notice}
    </>
  );
}

function renderProbe(initial: string) {
  render(
    <LocaleProvider initial="en">
      <CopyProbe initial={initial} />
    </LocaleProvider>,
  );
}

describe("useClipboardCopy", () => {
  it("hands the text over and then says it has been copied", async () => {
    const user = userEvent.setup();
    clipboard = stubClipboard("accepts");
    renderProbe("the-link");

    await user.click(screen.getByRole("button", { name: "Copy link" }));

    expect(await screen.findByRole("button", { name: "Copied" })).toBeTruthy();
    expect(clipboard.written).toEqual(["the-link"]);
  });

  it("says so rather than throwing where the browser offers no clipboard", async () => {
    // The deployment this case is about is a plain-http installation, where
    // `navigator.clipboard` is undefined and asking it for `writeText` throws
    // before any rejection handler could run.
    const user = userEvent.setup();
    clipboard = stubClipboard("absent");
    renderProbe("the-link");

    await user.click(screen.getByRole("button", { name: "Copy link" }));

    expect(
      await screen.findByText(/this browser refused the clipboard/i),
    ).toBeTruthy();
    expect(screen.getByText(LABELS.remedy)).toBeTruthy();
    // Still offering the verb: the button that did nothing must not go on to
    // claim it worked.
    expect(screen.getByRole("button", { name: "Copy link" })).toBeTruthy();
  });

  it("says so where the clipboard is there and refuses the write", async () => {
    const user = userEvent.setup();
    clipboard = stubClipboard("refuses");
    renderProbe("the-link");

    await user.click(screen.getByRole("button", { name: "Copy link" }));

    expect(
      await screen.findByText(/this browser refused the clipboard/i),
    ).toBeTruthy();
    expect(screen.getByRole("button", { name: "Copy link" })).toBeTruthy();
  });

  it("stops claiming Copied once the text on screen is no longer the text copied", async () => {
    // The clipboard still holds the OLD text. A button that went on saying
    // Copied would be describing something the reader can no longer paste.
    const user = userEvent.setup();
    clipboard = stubClipboard("accepts");
    renderProbe("the-link");

    await user.click(screen.getByRole("button", { name: "Copy link" }));
    await screen.findByRole("button", { name: "Copied" });
    await user.click(screen.getByRole("button", { name: "Edit the text" }));

    expect(screen.getByRole("button", { name: "Copy link" })).toBeTruthy();
  });

  it("takes the notice away once a later attempt lands", async () => {
    const user = userEvent.setup();
    clipboard = stubClipboard("refuses");
    renderProbe("the-link");
    await user.click(screen.getByRole("button", { name: "Copy link" }));
    await screen.findByText(/this browser refused the clipboard/i);

    clipboard.restore();
    clipboard = stubClipboard("accepts");
    await user.click(screen.getByRole("button", { name: "Copy link" }));

    expect(await screen.findByRole("button", { name: "Copied" })).toBeTruthy();
    expect(
      screen.queryByText(/this browser refused the clipboard/i),
    ).toBeNull();
  });
});
