// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { Markdown, type MarkdownHighlightOutcome } from "./markdown";

afterEach(cleanup);

/** Everything the reader can actually see, which is what the security
 *  assertions are about: not whether a payload was parsed, but whether it ends
 *  up on the page as text. */
function visibleText(container: HTMLElement): string {
  return container.textContent ?? "";
}

describe("the syntax the shipped handbook uses", () => {
  it("renders ATX headings at their own level", () => {
    render(<Markdown source={"# Title\n\n## Section\n\n### Detail"} />);
    expect(screen.getByRole("heading", { level: 1 })).toHaveTextContent(
      "Title",
    );
    expect(screen.getByRole("heading", { level: 2 })).toHaveTextContent(
      "Section",
    );
    expect(screen.getByRole("heading", { level: 3 })).toHaveTextContent(
      "Detail",
    );
  });

  it("joins a wrapped paragraph and separates a blank-line break", () => {
    const { container } = render(
      <Markdown source={"One line\nand its wrap.\n\nA second paragraph."} />,
    );
    const paragraphs = container.querySelectorAll("p");
    expect(paragraphs).toHaveLength(2);
    expect(paragraphs[0].textContent).toBe("One line\nand its wrap.");
    expect(paragraphs[1].textContent).toBe("A second paragraph.");
  });

  it("renders bold, italic and inline code as their own elements", () => {
    const { container } = render(
      <Markdown source="A **stage** carries a *semantic* and a `win_probability`." />,
    );
    expect(container.querySelector("strong")).toHaveTextContent("stage");
    expect(container.querySelector("em")).toHaveTextContent("semantic");
    expect(container.querySelector("code")).toHaveTextContent(
      "win_probability",
    );
  });

  it("renders a fenced block verbatim, newlines and markup characters kept", () => {
    const { container } = render(
      <Markdown source={"```sh\nmake dev  # **not** bold\n```"} />,
    );
    const pre = container.querySelector("pre");
    expect(pre?.textContent).toBe("make dev  # **not** bold");
    expect(pre?.querySelector("strong")).toBeNull();
  });

  it("renders unordered and ordered lists as their own kind", () => {
    const { container } = render(
      <Markdown source={"- a name\n- a position\n\n1. first\n2. second"} />,
    );
    expect(container.querySelectorAll("ul li")).toHaveLength(2);
    expect(container.querySelectorAll("ol li")).toHaveLength(2);
    expect(container.querySelectorAll("ol li")[1]).toHaveTextContent("second");
  });

  it("carries a wrapped list item into the item above it", () => {
    const { container } = render(
      <Markdown source={"- a **name** — yours\n  to choose\n- a position"} />,
    );
    const items = container.querySelectorAll("li");
    expect(items).toHaveLength(2);
    expect(items[0].textContent).toContain("to choose");
  });

  it("renders a blockquote, with the blocks inside it", () => {
    const { container } = render(
      <Markdown source={"> Only a human can.\n>\n> - never an agent"} />,
    );
    const quote = container.querySelector("blockquote");
    expect(quote?.querySelector("p")).toHaveTextContent("Only a human can.");
    expect(quote?.querySelector("li")).toHaveTextContent("never an agent");
  });

  it("renders a GFM table with its header row", () => {
    render(
      <Markdown
        source={[
          "| Stage | Semantic |",
          "|---|---|",
          "| Qualified | Open |",
          "| Won | Won |",
        ].join("\n")}
      />,
    );
    expect(screen.getAllByRole("columnheader")).toHaveLength(2);
    expect(screen.getAllByRole("row")).toHaveLength(3);
    expect(screen.getByRole("cell", { name: "Qualified" })).toBeInTheDocument();
  });

  it("renders a horizontal rule as a separator", () => {
    const { container } = render(
      <Markdown source={"before\n\n---\n\nafter"} />,
    );
    expect(container.querySelector("hr")).toBeInTheDocument();
  });

  // Not dropped and not raw: the failure this asks about is a document form
  // nobody taught the reader, and the honest answer is the author's own line.
  it("shows syntax it does not support as the plain text it is", () => {
    render(
      <Markdown source={"Setext\n======\n\n- [ ] a task nobody models"} />,
    );
    expect(screen.getByText(/Setext/)).toBeInTheDocument();
    expect(screen.getByText(/a task nobody models/)).toBeInTheDocument();
  });
});

describe("a link is only a link when its scheme is one we allow", () => {
  it("renders an http link, opening it away from the answer", () => {
    render(<Markdown source="See [the docs](https://example.com/a)." />);
    const link = screen.getByRole("link", { name: "the docs" });
    expect(link).toHaveAttribute("href", "https://example.com/a");
    expect(link).toHaveAttribute("target", "_blank");
    expect(link).toHaveAttribute("rel", "noopener noreferrer");
  });

  it("renders a mailto link and a relative one in place", () => {
    render(
      <Markdown source="[write](mailto:a@example.com) or [read](capture.md#filing)" />,
    );
    expect(screen.getByRole("link", { name: "write" })).toHaveAttribute(
      "href",
      "mailto:a@example.com",
    );
    const relative = screen.getByRole("link", { name: "read" });
    expect(relative).toHaveAttribute("href", "capture.md#filing");
    expect(relative).not.toHaveAttribute("target");
  });

  // The defect this primitive exists to make impossible: a corpus document is
  // customer-uploaded, so its hrefs are an attacker's text.
  it.each([
    ["javascript", "javascript:alert(1)"],
    ["data", "data:text/html;base64,PHNjcmlwdD4="],
    ["vbscript", "vbscript:msgbox(1)"],
    ["cased javascript", "JaVaScRiPt:alert(1)"],
    ["null-byte javascript", "java\u0000script:alert(1)"],
    ["protocol-relative", "//evil.example.com"],
  ])("refuses a %s href and leaves inert text", (_name, href) => {
    const { container } = render(<Markdown source={`[press me](${href})`} />);
    expect(screen.queryByRole("link")).toBeNull();
    expect(screen.getByText("press me")).toBeInTheDocument();
    // The payload does not survive as copyable text beside the words it hid
    // behind, either.
    expect(visibleText(container)).not.toContain("alert");
    expect(visibleText(container)).not.toContain("base64");
  });

  it("is not a link at all when whitespace splits the destination", () => {
    // A browser would strip the tab and navigate; no markdown reader makes this
    // a link, so the safest reading is the one that never builds an anchor.
    render(<Markdown source={"[press me](java\tscript:alert)"} />);
    expect(screen.queryByRole("link")).toBeNull();
  });
});

describe("nothing in the source becomes markup", () => {
  it("shows a script tag as text and never as an element", () => {
    const { container } = render(
      <Markdown
        source={"Before\n\n<script>window.stolen = 1;</script>\n\nAfter"}
      />,
    );
    expect(container.querySelector("script")).toBeNull();
    expect(
      screen.getByText(/<script>window\.stolen = 1;<\/script>/),
    ).toBeInTheDocument();
  });

  it("shows an onerror image as text and never as an element", () => {
    const { container } = render(
      <Markdown source={'<img src=x onerror="window.stolen = 1">'} />,
    );
    expect(container.querySelector("img")).toBeNull();
    expect(screen.getByText(/onerror/)).toBeInTheDocument();
  });

  it("shows a raw HTML block as text and never as elements", () => {
    const { container } = render(
      <Markdown source={"<div class='x'><b>bold?</b></div>"} />,
    );
    expect(container.querySelector("div.x")).toBeNull();
    expect(container.querySelector("b")).toBeNull();
    expect(visibleText(container)).toContain("<b>bold?</b>");
  });

  it("shows an html entity as the characters the author typed", () => {
    // Nothing here decodes entities, which is what keeps `&#106;avascript:`
    // from becoming a scheme after the allowlist has already read the href.
    // A JS string, not a JSX attribute: JSX decodes entities in an attribute
    // literal, so the attribute form would hand the component `<script>` and
    // quietly test something else.
    render(<Markdown source={"&lt;script&gt; &#106;avascript:"} />);
    expect(
      screen.getByText("&lt;script&gt; &#106;avascript:"),
    ).toBeInTheDocument();
  });
});

describe("the highlight has three outcomes and says which", () => {
  let scrolled: HTMLElement[];

  beforeEach(() => {
    scrolled = [];
    vi.spyOn(HTMLElement.prototype, "scrollIntoView").mockImplementation(
      function scrollIntoView(this: HTMLElement) {
        scrolled.push(this);
      },
    );
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  // The citation is cut from the SOURCE and carries its markup; the page shows
  // the RENDERED text, where those markers are a font weight rather than
  // characters. Without this the common handbook citation — almost every one of
  // which is bold — matched nothing on a page that plainly contained it, and
  // the reader was told the quote could not be pinpointed.
  it("marks a quote that carries the source's own emphasis markers", () => {
    const onHighlight = vi.fn<(outcome: MarkdownHighlightOutcome) => void>();
    const { container } = render(
      <Markdown
        source={"**Full seat.** Can read and change things, subject to role."}
        highlight={{
          quote: "**Full seat.** Can read and change things, subject to role.",
        }}
        onHighlight={onHighlight}
      />,
    );
    expect(onHighlight).toHaveBeenCalledWith("quote");
    expect(container.querySelectorAll("mark").length).toBeGreaterThan(0);
    // The asterisks are not on the page, so they are not in the mark either.
    expect(container.textContent).not.toContain("**");
  });

  it("marks a quote whose emphasis wraps one word inside it", () => {
    const onHighlight = vi.fn<(outcome: MarkdownHighlightOutcome) => void>();
    render(
      <Markdown
        source={"A project is **not** a folder you create when you win."}
        highlight={{ quote: "A project is **not** a folder you create" }}
        onHighlight={onHighlight}
      />,
    );
    expect(onHighlight).toHaveBeenCalledWith("quote");
  });

  it("marks the quote where it is found, and scrolls to it", () => {
    const onHighlight = vi.fn<(outcome: MarkdownHighlightOutcome) => void>();
    const { container } = render(
      <Markdown
        source={"# Stages\n\nA stage carries a win probability from 0 to 100."}
        highlight={{ quote: "a win probability" }}
        onHighlight={onHighlight}
      />,
    );
    const mark = container.querySelector("mark");
    expect(mark).toHaveTextContent("a win probability");
    expect(onHighlight).toHaveBeenCalledWith("quote");
    expect(scrolled).toEqual([mark]);
  });

  // The server locates a claim under collapsed whitespace
  // (claims.CollapseSpace), so a quote the model re-wrapped still matches.
  it("matches a quote whose whitespace does not agree with the document", () => {
    const { container } = render(
      <Markdown
        source={"The move is written\nimmediately   and confirmed."}
        highlight={{ quote: "  written immediately and   confirmed " }}
      />,
    );
    expect(container.querySelector("mark")?.textContent).toBe(
      "written\nimmediately   and confirmed",
    );
  });

  it("keeps the emphasis a quote runs through, marking each part", () => {
    const { container } = render(
      <Markdown
        source="A stage carries a **semantic** and a position."
        highlight={{ quote: "carries a semantic and" }}
      />,
    );
    expect(container.querySelector("strong mark")).toHaveTextContent(
      "semantic",
    );
    expect(container.querySelectorAll("mark")).toHaveLength(3);
    expect(visibleText(container)).toBe(
      "A stage carries a semantic and a position.",
    );
  });

  it("falls back to the line when the quote is not in the document", () => {
    const onHighlight = vi.fn<(outcome: MarkdownHighlightOutcome) => void>();
    const { container } = render(
      <Markdown
        source={"# Stages\n\nFirst paragraph.\n\nSecond paragraph."}
        highlight={{ quote: "a sentence nobody wrote", line: 5 }}
        onHighlight={onHighlight}
      />,
    );
    const marked = container.querySelector("[data-markdown-mark]");
    expect(marked).toHaveTextContent("Second paragraph.");
    expect(container.querySelector("mark")).toBeNull();
    expect(onHighlight).toHaveBeenCalledWith("line");
    expect(scrolled).toEqual([marked]);
  });

  it("reports none, and renders the document plain, when neither lands", () => {
    const onHighlight = vi.fn<(outcome: MarkdownHighlightOutcome) => void>();
    const { container } = render(
      <Markdown
        source={"# Stages\n\nFirst paragraph."}
        highlight={{ quote: "a sentence nobody wrote", line: 900 }}
        onHighlight={onHighlight}
      />,
    );
    expect(onHighlight).toHaveBeenCalledWith("none");
    expect(container.querySelector("mark")).toBeNull();
    expect(container.querySelector("[data-markdown-mark]")).toBeNull();
    expect(scrolled).toEqual([]);
    // Not silently empty: the document is all still there to read.
    expect(screen.getByRole("heading", { level: 1 })).toHaveTextContent(
      "Stages",
    );
    expect(screen.getByText("First paragraph.")).toBeInTheDocument();
  });

  it("reports none for a quote that is only whitespace", () => {
    const onHighlight = vi.fn<(outcome: MarkdownHighlightOutcome) => void>();
    render(
      <Markdown
        source="A paragraph."
        highlight={{ quote: "   \n " }}
        onHighlight={onHighlight}
      />,
    );
    expect(onHighlight).toHaveBeenCalledWith("none");
  });

  it("marks a quote inside a table cell", () => {
    const { container } = render(
      <Markdown
        source={"| Stage | Semantic |\n|---|---|\n| Qualified | Open |"}
        highlight={{ quote: "Qualified" }}
      />,
    );
    expect(container.querySelector("td mark")).toHaveTextContent("Qualified");
  });

  it("says nothing to a caller that asked for no highlight", () => {
    const onHighlight = vi.fn<(outcome: MarkdownHighlightOutcome) => void>();
    render(<Markdown source="A paragraph." onHighlight={onHighlight} />);
    expect(onHighlight).toHaveBeenCalledWith("none");
    expect(scrolled).toEqual([]);
  });
});
