// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { escapeHtml } from "../format/html";
import {
  type Block,
  type Inline,
  type Run,
  readMarkdown,
} from "./markdown-parse";
import { cleanMarkup, DOCUMENT_TAGS, safeHref } from "./richtext-html";

/**
 * The two directions of `RichText`'s markdown mode: a stored body becomes
 * editor markup, and the editor's DOM becomes the body again.
 *
 * Reading goes through `markdown-parse`, the parser `<Markdown>` renders with,
 * so the editor and the timeline cannot disagree about what a star means.
 * Writing emits only the spellings that parser reads back, which is what makes
 * load, edit and save a round trip.
 */
export function editorHTMLFromMarkdown(source: string): string {
  return readMarkdown(source).blocks.map(blockHTML).join("");
}

function blockHTML(block: Block): string {
  switch (block.kind) {
    case "heading": {
      const tag = `h${Math.min(Math.max(block.level, 1), 6)}`;
      return `<${tag}>${runHTML(block.run)}</${tag}>`;
    }
    case "paragraph":
      return `<p>${runHTML(block.run)}</p>`;
    case "code":
      return `<pre>${escapeHtml(runPlain(block.run))}</pre>`;
    case "rule":
      return "<hr>";
    case "list": {
      const items = block.items.map((item) => `<li>${runHTML(item)}</li>`);
      if (!block.ordered) return `<ul>${items.join("")}</ul>`;
      const start = block.start === 1 ? "" : ` start="${block.start}"`;
      return `<ol${start}>${items.join("")}</ol>`;
    }
    case "quote":
      return `<blockquote>${block.blocks.map(blockHTML).join("")}</blockquote>`;
    case "table": {
      // The editor has no table to offer, so a table stays its own pipe rows,
      // which `markdownOf` writes back as the table they were.
      const row = (cells: readonly Run[]) =>
        `| ${cells.map(runHTML).join(" | ")} |`;
      const rule = `| ${block.header.map(() => "---").join(" | ")} |`;
      return `<p>${[row(block.header), rule, ...block.rows.map(row)].join("<br>")}</p>`;
    }
  }
}

function runHTML(run: Run): string {
  return run.nodes.map(inlineHTML).join("");
}

function inlineHTML(node: Inline): string {
  switch (node.kind) {
    case "text":
      return escapeHtml(node.text).replaceAll("\n", "<br>");
    case "code":
      return `<code>${escapeHtml(node.text)}</code>`;
    case "strong":
      return `<strong>${node.children.map(inlineHTML).join("")}</strong>`;
    case "em":
      return `<em>${node.children.map(inlineHTML).join("")}</em>`;
    case "mark":
      return node.children.map(inlineHTML).join("");
    case "link":
      return `<a href="${escapeHtml(node.href)}">${node.children.map(inlineHTML).join("")}</a>`;
  }
}

function runPlain(run: Run): string {
  return run.nodes
    .map((node) => (node.kind === "text" ? node.text : ""))
    .join("");
}

/** The markdown body the editor's DOM spells. */
export function markdownOf(root: HTMLElement): string {
  return blocksOf(root)
    .filter((block) => block.trim() !== "")
    .join("\n\n")
    .trim();
}

const HEADING_TAGS = ["H1", "H2", "H3", "H4", "H5", "H6"];
const BLOCK_TAGS = new Set([
  ...HEADING_TAGS,
  "P",
  "DIV",
  "UL",
  "OL",
  "BLOCKQUOTE",
  "PRE",
  "HR",
]);

// Children of one container as markdown blocks. Loose inline content between
// blocks is a paragraph of its own. A browser leaves it when the first line is
// typed into an empty editor.
function blocksOf(parent: Node): string[] {
  const blocks: string[] = [];
  let loose: Node[] = [];
  const flush = () => {
    if (loose.length > 0) {
      blocks.push(lineStarts(inlineOf(loose)));
    }
    loose = [];
  };
  const tidy = (block: string) => block.replace(/^\n+/, "").trimEnd();
  for (const child of Array.from(parent.childNodes)) {
    if (child instanceof HTMLElement && BLOCK_TAGS.has(child.tagName)) {
      flush();
      blocks.push(blockOf(child));
    } else {
      loose.push(child);
    }
  }
  flush();
  return blocks.map(tidy);
}

function blockOf(element: HTMLElement): string {
  const tag = element.tagName;
  const level = HEADING_TAGS.indexOf(tag) + 1;
  if (level > 0) {
    const text = inlineOf(Array.from(element.childNodes)).replaceAll("\n", " ");
    return `${"#".repeat(level)} ${text.trim()}`;
  }
  if (tag === "UL" || tag === "OL") return listOf(element, "");
  if (tag === "BLOCKQUOTE") {
    return blocksOf(element)
      .filter((block) => block.trim() !== "")
      .join("\n\n")
      .split("\n")
      .map((line) => `> ${line}`.trimEnd())
      .join("\n");
  }
  if (tag === "PRE") {
    const body = element.textContent ?? "";
    const fence = body.includes("```") ? "~~~" : "```";
    return `${fence}\n${body.replace(/\n$/, "")}\n${fence}`;
  }
  if (tag === "HR") return "---";
  // A browser wraps each typed line in a div, and a pasted document nests its
  // paragraphs in one. Either way the blocks inside are what count.
  if (
    Array.from(element.children).some((child) => BLOCK_TAGS.has(child.tagName))
  ) {
    return blocksOf(element)
      .filter((block) => block.trim() !== "")
      .join("\n\n");
  }
  return lineStarts(inlineOf(Array.from(element.childNodes)));
}

// The reader parses no nested level. A nested list's items are written as
// indented items of the list around them.
function listOf(list: HTMLElement, indent: string): string {
  const ordered = list.tagName === "OL";
  const lines: string[] = [];
  const start = Number.parseInt(list.getAttribute("start") ?? "", 10);
  let n = ordered && Number.isFinite(start) ? start - 1 : 0;
  for (const item of Array.from(list.children)) {
    if (!(item instanceof HTMLElement)) continue;
    if (isList(item)) {
      lines.push(listOf(item, `${indent}  `));
    } else if (item.tagName === "LI") {
      n += 1;
      lines.push(itemOf(item, ordered ? `${n}. ` : "- ", indent));
    }
  }
  return lines.join("\n");
}

function itemOf(item: HTMLElement, marker: string, indent: string): string {
  const own: Node[] = [];
  const nested: string[] = [];
  for (const child of Array.from(item.childNodes)) {
    if (child instanceof HTMLElement && isList(child)) {
      nested.push(listOf(child, `${indent}  `));
    } else {
      own.push(child);
    }
  }
  const text = lineStarts(inlineOf(own).trim()).replaceAll(
    "\n",
    `\n${indent}  `,
  );
  return [`${indent}${marker}${text}`, ...nested].join("\n");
}

function isList(element: HTMLElement): boolean {
  return element.tagName === "UL" || element.tagName === "OL";
}

function inlineOf(nodes: readonly Node[]): string {
  return nodes.map(inlineNode).join("");
}

function inlineNode(node: Node): string {
  if (node.nodeType === Node.TEXT_NODE) {
    return escapeMarkdown(node.textContent ?? "");
  }
  if (!(node instanceof HTMLElement)) return "";
  const inner = () => inlineOf(Array.from(node.childNodes));
  switch (node.tagName) {
    case "BR":
      return "\n";
    case "B":
    case "STRONG":
      return wrap("**", inner());
    case "I":
    case "EM":
      return emphasis(node, inner);
    case "CODE":
      return codeSpan(node.textContent ?? "");
    case "A": {
      const label = inner();
      const href = safeHref(node.getAttribute("href"));
      return href === "" || label.trim() === ""
        ? label
        : `[${label}](${href.replaceAll(" ", "%20")})`;
    }
    default:
      // A block inside a line (a div a browser put in a list item) breaks it.
      return BLOCK_TAGS.has(node.tagName) ? `\n${inner()}\n` : inner();
  }
}

// Italic is `_` and bold `**`. Italic around bold is written as bold around
// italic, because the reader cannot read `_**x**_` or `***x***`.
function emphasis(node: HTMLElement, inner: () => string): string {
  const only = node.childNodes.length === 1 ? node.firstChild : null;
  if (
    only instanceof HTMLElement &&
    (only.tagName === "B" || only.tagName === "STRONG")
  ) {
    return wrap("**", wrap("_", inlineOf(Array.from(only.childNodes))));
  }
  return wrap("_", inner());
}

// The mark hugs the words: `** bold**` is not bold to the reader, so the
// spaces a selection caught stand outside it.
function wrap(mark: string, text: string): string {
  const match = /^(\s*)([\s\S]*?)(\s*)$/.exec(text);
  if (match === null || match[2] === "") return text;
  return `${match[1]}${mark}${match[2]}${mark}${match[3]}`;
}

function codeSpan(text: string): string {
  const longest = Math.max(
    0,
    ...(text.match(/`+/g) ?? []).map((run) => run.length),
  );
  const fence = "`".repeat(longest + 1);
  const pad = text.startsWith("`") || text.endsWith("`") ? " " : "";
  return `${fence}${pad}${text}${pad}${fence}`;
}

// The characters the inline reader would take as syntax, so a typed `5*3` or
// `snake_case` comes back as the text it was.
function escapeMarkdown(text: string): string {
  return text.replace(/[\\`*_[]/g, (character) => `\\${character}`);
}

// A line the block reader would open a heading, list, quote, fence or rule
// with is escaped, so typed text stays the paragraph it was.
const OPENS_BLOCK =
  /^\s*(#{1,6}(\s|$)|>|[-+](\s|$)|\d{1,9}[.)](\s|$)|~~~|-(\s*-){2,}\s*$)/;

function lineStarts(text: string): string {
  return text
    .split("\n")
    .map((line) => {
      if (!OPENS_BLOCK.test(line)) return line;
      // A number keeps its digits and escapes the dot after them.
      return /^\s*\d/.test(line)
        ? line.replace(/^(\s*\d+)/, "$1\\")
        : line.replace(/^(\s*)/, "$1\\");
    })
    .join("\n");
}

/**
 * What a paste becomes in markdown mode: formatted either way.
 *
 * Markup from a document or a mail keeps its formatting when it has some.
 * Otherwise the plain text is read as markdown. A notes tool or an assistant
 * puts markdown on the clipboard. A code editor's HTML copy of markdown source
 * is styled spans with the asterisks still in them.
 */
export function pastedMarkup(html: string, text: string): string {
  if (html.trim() !== "") {
    const body = new DOMParser().parseFromString(html, "text/html").body;
    markStyledRuns(body);
    if (
      body.querySelector(
        "b, strong, i, em, ul, ol, h1, h2, h3, h4, h5, h6, a[href], pre, code, blockquote, hr",
      )
    ) {
      return cleanMarkup(body, DOCUMENT_TAGS, { blockBreaks: true });
    }
  }
  return editorHTMLFromMarkdown(text);
}

// A word processor marks bold and italic with inline styles on spans, and
// wraps a whole copy in a `<b style="font-weight:normal">`. Both are turned
// into the elements they mean before the allowlist reads the tree.
function markStyledRuns(root: HTMLElement): void {
  for (const element of Array.from(
    root.querySelectorAll<HTMLElement>("[style]"),
  )) {
    const weight = element.style.fontWeight;
    const bold = weight === "bold" || Number(weight) >= 600;
    const italic = element.style.fontStyle === "italic";
    if (
      (element.tagName === "B" || element.tagName === "STRONG") &&
      weight !== "" &&
      !bold
    ) {
      element.replaceWith(...Array.from(element.childNodes));
      continue;
    }
    if (element.tagName !== "SPAN") continue;
    let inner = element;
    if (bold) inner = wrapChildren(inner, "strong");
    if (italic) wrapChildren(inner, "em");
  }
}

function wrapChildren(element: HTMLElement, tag: "strong" | "em"): HTMLElement {
  const mark = element.ownerDocument.createElement(tag);
  mark.append(...Array.from(element.childNodes));
  element.append(mark);
  return mark;
}
