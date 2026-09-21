/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { plainTextOf, RichText, safeEditorHTML } from "./richtext";

afterEach(cleanup);

const here = dirname(fileURLToPath(import.meta.url));

const LABELS = {
  bold: "Fett",
  italic: "Kursiv",
  bulletList: "Liste",
  numberList: "Nummerierte Liste",
  link: "Link",
  linkPrompt: "Adresse",
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
  it("keeps a frame the reply drawer cannot squeeze to nothing", () => {
    const css = readFileSync(
      join(here, "..", "screens", "composethread.css"),
      "utf8",
    );
    const frame =
      css
        .split(".compose-split > .compose-fields > .richtext {")[1]
        ?.split("}")[0] ?? "";

    expect(frame).toContain("min-block-size");
    expect(frame).not.toContain("min-block-size: 0");
  });
});
