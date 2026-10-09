/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { useState } from "react";
import { afterEach, describe, expect, it } from "vitest";
import { RichText } from "./richtext";
import { editorHTMLFromMarkdown, markdownOf } from "./richtext-markdown";

afterEach(cleanup);

const LABELS = {
  bold: "Bold",
  italic: "Italic",
  bulletList: "Bulleted list",
  numberList: "Numbered list",
  link: "Link",
  linkPrompt: "Address",
  heading: "Heading",
};

// The caller's side of the wire: what the form would store as the body.
function Body({ initial = "" }: Readonly<{ initial?: string }>) {
  const [body, setBody] = useState(initial);
  return (
    <>
      <RichText
        format="markdown"
        value={body}
        onChange={(next) => setBody(next.markdown)}
        label="Details"
        labels={LABELS}
      />
      <output data-testid="stored">{body}</output>
    </>
  );
}

const stored = () => screen.getByTestId("stored").textContent;
const surface = () => screen.getByRole("textbox", { name: "Details" });

function editorOf(markdown: string): HTMLElement {
  const node = document.createElement("div");
  node.innerHTML = editorHTMLFromMarkdown(markdown);
  return node;
}

describe("a markdown body in the editor", () => {
  const recap = [
    "## Summary",
    "**Budget** is _approved_, see [the plan](https://gradion.com/plan).",
    "- first point\n- second point",
    "1. call Ida\n2. send the offer",
    "> They want it by Friday.",
    "Price: 5\\*3 for `snake_case` items",
  ].join("\n\n");

  it("loads formatted and saves the same markdown after an edit", async () => {
    render(<Body initial={recap} />);

    await waitFor(() => expect(surface().querySelector("h2")).not.toBeNull());
    expect(surface().querySelector("strong")?.textContent).toBe("Budget");
    expect(surface().querySelectorAll("ul > li")).toHaveLength(2);
    expect(surface().textContent).not.toContain("**");

    const appended = document.createElement("p");
    appended.textContent = "Next: a demo.";
    surface().append(appended);
    fireEvent.input(surface());

    expect(stored()).toBe(`${recap}\n\nNext: a demo.`);
  });

  it("keeps typed syntax characters as the text they were", () => {
    const node = document.createElement("div");
    node.innerHTML =
      "<p># not a heading</p><p>- not a list</p><p>2. not either</p><p>a_b *c*</p>";
    const markdown = markdownOf(node);

    expect(markdownOf(editorOf(markdown))).toBe(markdown);
    expect(editorOf(markdown).querySelector("h1, ul, ol, em")).toBeNull();
    expect(editorOf(markdown).textContent).toContain("# not a heading");
  });

  it("writes italic around bold so the reader reads it back", () => {
    const node = document.createElement("div");
    node.innerHTML = "<p><em><strong>both</strong></em></p>";

    const back = editorOf(markdownOf(node));

    expect(back.querySelector("strong em, em strong")?.textContent).toBe(
      "both",
    );
  });
});

describe("a paste into the markdown editor", () => {
  const paste = (data: Record<string, string>) =>
    fireEvent.paste(surface(), {
      clipboardData: { getData: (type: string) => data[type] ?? "" },
    });

  it("turns pasted markdown into formatting, with no syntax left", () => {
    render(<Body />);

    paste({ "text/plain": "**Summary**\n\n- first point\n- second point" });

    expect(surface().querySelector("strong")?.textContent).toBe("Summary");
    expect(surface().querySelectorAll("li")).toHaveLength(2);
    expect(surface().textContent).not.toMatch(/\*\*|^- /);
    expect(stored()).toBe("**Summary**\n\n- first point\n- second point");
  });

  it("keeps a document's formatting and drops what the body cannot hold", () => {
    render(<Body />);

    paste({
      "text/html":
        '<b style="font-weight:normal" id="docs-internal-guid-1"><h2>Summary</h2><p><span style="font-weight:700">Budget</span> approved</p><ul><li>first</li></ul><script>alert(1)</script></b>',
      "text/plain": "Summary\nBudget approved\nfirst",
    });

    expect(surface().querySelector("script")).toBeNull();
    expect(stored()).toBe("## Summary\n\n**Budget** approved\n\n- first");
  });

  it("reads the plain text as markdown when the markup carries no formatting", () => {
    render(<Body />);

    paste({
      "text/html": '<div><span style="color:#333">**Summary**</span></div>',
      "text/plain": "**Summary**",
    });

    expect(surface().querySelector("strong")?.textContent).toBe("Summary");
    expect(stored()).toBe("**Summary**");
  });
});

describe("the toolbar per mode", () => {
  it("offers a heading only in markdown mode", () => {
    render(
      <>
        <RichText
          value=""
          onChange={() => {}}
          label="Message"
          labels={LABELS}
        />
        <RichText
          format="markdown"
          value=""
          onChange={() => {}}
          label="Details"
          labels={LABELS}
        />
      </>,
    );

    const [email, activity] = screen.getAllByRole("toolbar");

    expect(within(email).queryByRole("button", { name: "Heading" })).toBeNull();
    expect(
      within(activity).getByRole("button", { name: "Heading" }),
    ).toBeTruthy();
  });
});
