/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { plainTextOf, RichText, safeEditorHTML } from "./richtext";

afterEach(cleanup);

const here = dirname(fileURLToPath(import.meta.url));

const LABELS = {
  bold: "Fett",
  italic: "Kursiv",
  bulletList: "Liste",
  numberList: "Nummerierte Liste",
  link: "Link",
};

// `value` is not always something a rep typed — an AI draft arrives here, and a
// model's output is untrusted input however friendly its source. The server
// filters what LEAVES for a recipient; this filters what ENTERS our document,
// and the two protect different contacts.
describe("what may enter the editor", () => {
  it("keeps the formatting the toolbar can produce", () => {
    const markup =
      "<p>The <b>deadline</b> is <em>Friday</em>.</p><ul><li>One</li></ul>";

    expect(safeEditorHTML(markup)).toBe(markup);
  });

  it("drops a script entirely, content included", () => {
    expect(safeEditorHTML("<p>A</p><script>alert(1)</script><p>B</p>")).toBe(
      "<p>A</p><p>B</p>",
    );
  });

  it("drops an image, because a remote one is a read receipt", () => {
    expect(
      safeEditorHTML(`<p>Hi</p><img src="https://track.test/o.gif">`),
    ).toBe("<p>Hi</p>");
  });

  it("keeps a link's text but not a javascript href", () => {
    expect(safeEditorHTML(`<a href="javascript:alert(1)">click</a>`)).toBe(
      "<a>click</a>",
    );
  });

  it("keeps http, https and mailto hrefs", () => {
    for (const href of [
      "https://gradion.com",
      "http://gradion.com",
      "mailto:x@y.test",
    ]) {
      expect(safeEditorHTML(`<a href="${href}">go</a>`)).toBe(
        `<a href="${href}">go</a>`,
      );
    }
  });

  it("unwraps an unknown element rather than losing its words", () => {
    expect(safeEditorHTML("<div>Words <b>kept</b></div>")).toBe(
      "Words <b>kept</b>",
    );
  });

  it("escapes text that looks like markup", () => {
    expect(safeEditorHTML("<p>Use &lt;b&gt; carefully</p>")).toBe(
      "<p>Use &lt;b&gt; carefully</p>",
    );
  });

  it("strips an event handler from an element it keeps", () => {
    expect(safeEditorHTML(`<p onclick="alert(1)">Text</p>`)).toBe(
      "<p>Text</p>",
    );
  });
});

// The plain part is a real alternative somebody reads, not a fallback nobody
// checks: it goes on the wire beside the markup, and a client that prefers text
// shows THIS.
describe("the plain-text rendering", () => {
  const render = (html: string) => {
    const node = document.createElement("div");
    node.innerHTML = html;
    return plainTextOf(node);
  };

  it("separates paragraphs, where textContent would run them together", () => {
    expect(render("<p>First.</p><p>Second.</p>")).toBe("First.\n\nSecond.");
  });

  it("keeps a bulleted list readable as a list", () => {
    expect(render("<ul><li>One</li><li>Two</li></ul>")).toBe("- One\n- Two");
  });

  // Inline formatting must not break a sentence. A renderer that emitted a
  // line per element would hand the plain reader a column of words.
  it("keeps a formatted sentence as one sentence", () => {
    expect(render("<p>The <b>deadline</b> is <em>Friday</em>.</p>")).toBe(
      "The deadline is Friday.",
    );
  });

  it("is empty for an empty editor", () => {
    expect(render("")).toBe("");
    expect(render("<br>")).toBe("");
  });
});

// How tall the writing surface is, and what happens to a host that has less
// room than that.
//
// The frame around the surface CLIPS — `overflow: hidden` is what keeps the
// focus ring's rounded corners — so a surface that will not shrink inside a
// shorter frame loses its last lines and the toolbar under them, and nothing
// anywhere scrolls to them: the column above is not overflowing, because the
// frame absorbed the squeeze by shrinking. Both halves below are what keeps
// that from happening, and each fails on its own.
describe("the writing surface inside a host that hands it a height", () => {
  it("is rows lines TALL, not rows lines at least", () => {
    render(
      <RichText
        value=""
        onChange={() => {}}
        label="Nachricht"
        labels={LABELS}
        rows={10}
      />,
    );
    const surface = screen.getByRole("textbox", { name: "Nachricht" });

    // A height follows the frame down and scrolls within itself; a floor is the
    // one thing a shorter frame cannot argue with.
    expect(surface.style.height).toBe("15em");
    expect(surface.style.minHeight).toBe("");
  });

  // Read from the sheet rather than from a rendered box because neither engine
  // the unit suite runs lays anything out: happy-dom and jsdom both answer zero
  // for every height, so a test that measured the drawer would pass with the
  // frame free to shrink to nothing again.
  it("keeps a frame the composer cannot squeeze to nothing", () => {
    const css = readFileSync(
      join(here, "..", "features", "mail", "compose.css"),
      "utf8",
    );
    const frame =
      css.split(".compose-sheet > .richtext {")[1]?.split("}")[0] ?? "";

    expect(frame).toContain("min-block-size");
    expect(frame).not.toContain("min-block-size: 0");
  });
});

describe("a model writing into the surface", () => {
  function surface(thinking: string | null) {
    return (
      <RichText
        value=""
        onChange={() => {}}
        label="Nachricht"
        labels={LABELS}
        thinking={thinking}
      />
    );
  }

  it("holds the place with speck rows and says what is being written", () => {
    const { rerender } = render(surface("Der Entwurf wird geschrieben."));
    expect(screen.getByText("Der Entwurf wird geschrieben.")).toBeTruthy();
    expect(document.querySelector(".richtext .aipending")).not.toBeNull();

    rerender(surface(null));
    expect(document.querySelector(".richtext .aipending")).toBeNull();
  });

  // Read from the sheet: neither engine the unit suite runs matches :empty
  // against typed text the way a browser does.
  it("gives way to the first words a reader types", () => {
    const css = readFileSync(join(here, "richtext.css"), "utf8");
    expect(css).toContain(
      ".richtext-input:not(:empty):not(:has(> br:only-child)) ~ .richtext-thinking",
    );
  });
});

describe("the keys of the writing surface", () => {
  function surface(value = "", onSubmit?: () => void) {
    const onChange = vi.fn();
    render(
      <LocaleProvider initial="en">
        <RichText
          value={value}
          onChange={onChange}
          label="Nachricht"
          labels={LABELS}
          onSubmit={onSubmit}
        />
      </LocaleProvider>,
    );
    return {
      onChange,
      editor: screen.getByRole("textbox", { name: "Nachricht" }),
    };
  }

  it("opens the link dialog on Cmd/Ctrl+K and keeps the key from the page", () => {
    const page = vi.fn();
    globalThis.addEventListener("keydown", page);
    const { editor } = surface();

    fireEvent.keyDown(editor, { key: "k", metaKey: true });
    fireEvent.keyDown(editor, { key: "K", ctrlKey: true });
    globalThis.removeEventListener("keydown", page);

    expect(screen.getByRole("heading", { name: "Add link" })).toBeVisible();
    expect(page).not.toHaveBeenCalled();
  });

  it("offers Remove link when the caret is in a link", async () => {
    const user = userEvent.setup({ delay: null });
    const { editor } = surface(`<p><a href="https://example.com">site</a></p>`);
    const text = editor.querySelector("a")?.firstChild;
    if (!text) throw new Error("no link text");
    window.getSelection()?.collapse(text, 2);

    await user.click(screen.getByRole("button", { name: "Link" }));

    expect(
      screen.getByRole("textbox", { name: "Web or email address" }),
    ).toHaveValue("https://example.com");
    expect(screen.getByRole("button", { name: "Remove link" })).toBeVisible();
  });

  it("sends on Cmd+Enter and Ctrl+Enter, and leaves Enter a new line", () => {
    const onSubmit = vi.fn();
    const { editor } = surface("", onSubmit);

    fireEvent.keyDown(editor, { key: "Enter", ctrlKey: true });
    fireEvent.keyDown(editor, { key: "Enter", metaKey: true });
    const newLine = fireEvent.keyDown(editor, { key: "Enter" });

    expect(onSubmit).toHaveBeenCalledTimes(2);
    expect(newLine).toBe(true);
  });

  it("leaves Cmd/Ctrl+Enter to the browser where the host gave no send", () => {
    const { editor } = surface("");

    expect(fireEvent.keyDown(editor, { key: "Enter", metaKey: true })).toBe(
      true,
    );
  });

  it("closes the link dialog when the words it was on are replaced", () => {
    const view = (value: string) => (
      <LocaleProvider initial="en">
        <RichText
          value={value}
          onChange={() => {}}
          label="Nachricht"
          labels={LABELS}
        />
      </LocaleProvider>
    );
    const { rerender } = render(view("<p>Mine</p>"));
    fireEvent.keyDown(screen.getByRole("textbox", { name: "Nachricht" }), {
      key: "k",
      ctrlKey: true,
    });
    expect(screen.getByRole("heading", { name: "Add link" })).toBeVisible();

    rerender(view("<p>Drafted</p>"));

    expect(screen.queryByRole("heading", { name: "Add link" })).toBeNull();
  });
});

describe("what the editor reports", () => {
  it("never reports a link the link dialog would refuse", () => {
    const onChange = vi.fn();
    render(
      <RichText
        value=""
        onChange={onChange}
        label="Nachricht"
        labels={LABELS}
      />,
    );
    const editor = screen.getByRole("textbox", { name: "Nachricht" });

    editor.innerHTML = `<p><a href="javascript:alert(1)">pasted</a> <a href="http://">empty</a> <a href="mailto:a@example.com?cc=b@example.com">copy</a> <a href="https://example.com">kept</a></p>`;
    fireEvent.input(editor);

    expect(onChange).toHaveBeenCalledOnce();
    const { html } = onChange.mock.calls[0]?.[0] ?? {};
    expect(html).not.toContain("javascript:");
    expect(html).not.toContain('href="http://"');
    expect(html).not.toContain("mailto:");
    expect(html).toContain(`<a href="https://example.com">kept</a>`);
  });
});

describe("which outside values the reader can undo", () => {
  function surface(value: string, revision?: number) {
    return (
      <RichText
        value={value}
        onChange={() => {}}
        label="Nachricht"
        labels={LABELS}
        undoableRevision={revision}
      />
    );
  }

  // happy-dom has neither the command nor layout; the browser has both.
  function withEditingCommand() {
    const command = vi.fn(() => false);
    Object.defineProperty(document, "execCommand", {
      value: command,
      configurable: true,
    });
    vi.spyOn(HTMLElement.prototype, "getClientRects").mockReturnValue([
      new DOMRect(0, 0, 10, 10),
    ] as unknown as DOMRectList);
    return command;
  }

  afterEach(() => {
    Reflect.deleteProperty(document, "execCommand");
    vi.restoreAllMocks();
  });

  it("puts a value with a new revision through the browser's command", () => {
    const command = withEditingCommand();
    const { rerender } = render(surface("<p>Mine</p>", 1));

    rerender(surface("<p>Drafted</p>", 2));

    expect(command).toHaveBeenCalledWith("insertHTML", false, "<p>Drafted</p>");
    expect(screen.getByRole("textbox").innerHTML).toBe("<p>Drafted</p>");
  });

  it("spends a revision that brought the words already on screen", () => {
    const command = withEditingCommand();
    const { rerender } = render(surface("<p>Mine</p>", 1));

    rerender(surface("<p>Mine</p>", 2));
    rerender(surface("<p>Another conversation</p>", 2));

    expect(command).not.toHaveBeenCalled();
    expect(screen.getByRole("textbox").innerHTML).toBe(
      "<p>Another conversation</p>",
    );
  });

  it("writes any other outside value straight in, where undo cannot reach", () => {
    const command = withEditingCommand();
    const { rerender } = render(surface("<p>Mine</p>", 1));

    rerender(surface("<p>Another conversation</p>", 1));
    rerender(surface("<p>Reset</p>"));

    expect(command).not.toHaveBeenCalled();
    expect(screen.getByRole("textbox").innerHTML).toBe("<p>Reset</p>");
  });
});
