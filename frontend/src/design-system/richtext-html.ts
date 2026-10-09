// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { escapeHtml } from "../format/html";

/**
 * The plain-text rendering of what the editor holds.
 *
 * Not `textContent`, which runs every block together with no space between.
 * Block boundaries become newlines and a list item keeps its marker. The plain
 * part goes on the wire, and a text client shows it.
 */
export function plainTextOf(node: HTMLElement): string {
  const lines: string[] = [];
  // The line being built. Inline formatting does not break it, so
  // "The <b>deadline</b> is Friday" stays one sentence.
  let current = "";
  const flush = () => {
    if (current.trim() !== "") {
      lines.push(current.trim());
    }
    current = "";
  };
  const walkListItem = (element: HTMLElement, prefix: string) => {
    flush();
    // An ordered list numbers its items and an unordered one bullets them.
    // Emitting neither leaves the plain reader a list that is not a list.
    const ordered = element.parentElement?.tagName.toLowerCase() === "ol";
    current = `${prefix}${ordered ? `${itemNumber(element)}. ` : "- "}`;
    walk(element, `${prefix}  `);
    flush();
  };
  const walkLink = (element: HTMLElement, prefix: string) => {
    // A text client shows no href, so the URL follows the label.
    walk(element, prefix);
    const href = element.getAttribute("href") ?? "";
    if (href !== "" && !current.includes(href)) {
      current += ` <${href}>`;
    }
  };
  const walkElement = (element: HTMLElement, prefix: string) => {
    const tag = element.tagName.toLowerCase();
    if (tag === "br") {
      flush();
    } else if (tag === "li") {
      walkListItem(element, prefix);
    } else if (tag === "a") {
      walkLink(element, prefix);
    } else if (isBlock(tag)) {
      flush();
      walk(element, prefix);
      flush();
      lines.push("");
    } else {
      // Inline: the line continues, which keeps a formatted sentence one
      // sentence.
      walk(element, prefix);
    }
  };
  const walk = (parent: Node, prefix: string) => {
    for (const child of Array.from(parent.childNodes)) {
      if (child.nodeType === Node.TEXT_NODE) {
        current += child.textContent ?? "";
      } else if (child instanceof HTMLElement) {
        walkElement(child, prefix);
      }
    }
  };
  walk(node, "");
  flush();
  return lines
    .join("\n")
    .replace(/\n{3,}/g, "\n\n")
    .trim();
}

/**
 * The subset of markup this editor will render.
 *
 * `value` may be an AI draft, which is untrusted input. The server filters what
 * leaves for a recipient; this filters what enters our own page.
 *
 * It mirrors the server's allowlist: the same elements and the same three link
 * schemes. So the composer never shows formatting the outbound filter strips.
 * It parses with the DOM parser, because the browser decides what markup means.
 */
export function safeEditorHTML(
  markup: string,
  allowed: ReadonlySet<string> = EMAIL_TAGS,
): string {
  return cleanMarkup(
    new DOMParser().parseFromString(markup, "text/html").body,
    allowed,
  );
}

/** The elements an outbound email keeps: the server allowlist's set. */
export const EMAIL_TAGS: ReadonlySet<string> = new Set([
  "P",
  "BR",
  "B",
  "STRONG",
  "I",
  "EM",
  "U",
  "UL",
  "OL",
  "LI",
  "A",
  "BLOCKQUOTE",
  "HR",
]);

/**
 * The elements a markdown body can say: the email set without underline, which
 * markdown has no spelling for, plus headings and code.
 * Every one of them has a markdown spelling in `markdownOf`, so nothing the
 * editor shows is lost on save.
 */
export const DOCUMENT_TAGS: ReadonlySet<string> = new Set([
  ...[...EMAIL_TAGS].filter((tag) => tag !== "U"),
  "H1",
  "H2",
  "H3",
  "H4",
  "H5",
  "H6",
  "CODE",
  "PRE",
]);

const DROPPED = new Set([
  "SCRIPT",
  "STYLE",
  "IFRAME",
  "OBJECT",
  "EMBED",
  "FORM",
  "INPUT",
  "BUTTON",
  "SELECT",
  "TEXTAREA",
  "TEMPLATE",
  "NOSCRIPT",
  "TITLE",
  "LINK",
  "META",
  "IMG",
]);

// Containers a document wraps its lines in. Unwrapped with `blockBreaks`,
// each stays a paragraph, so two pasted lines do not run together.
const BLOCKS =
  "p, div, section, article, ul, ol, li, h1, h2, h3, h4, h5, h6, pre, blockquote, hr, table, tr";

const CONTAINERS = new Set([
  "DIV",
  "SECTION",
  "ARTICLE",
  "HEADER",
  "FOOTER",
  "MAIN",
  "ASIDE",
  "FIGURE",
  "ADDRESS",
  "TR",
  "DT",
  "DD",
]);

/**
 * The children of `root` as markup holding only `allowed` elements.
 *
 * `blockBreaks` is for a markdown body, where a lost line break merges two
 * paragraphs into one sentence. The email path keeps the server's plain unwrap.
 */
export function cleanMarkup(
  root: Node,
  allowed: ReadonlySet<string>,
  { blockBreaks = false }: Readonly<{ blockBreaks?: boolean }> = {},
): string {
  const unwrap = (element: Element): string => {
    const inner = clean(element);
    if (!blockBreaks || !CONTAINERS.has(element.tagName)) {
      return inner;
    }
    if (element.querySelector(BLOCKS)) {
      // Its own blocks already break; loose text beside them gets a paragraph.
      return inner;
    }
    return inner.trim() === "" ? "" : `<p>${inner}</p>`;
  };
  const cleanElement = (element: Element): string => {
    const tag = element.tagName;
    if (DROPPED.has(tag)) {
      return "";
    }
    if (!allowed.has(tag)) {
      // Unwrap as the server does, keeping the words inside.
      return unwrap(element);
    }
    const lower = tag.toLowerCase();
    if (lower === "br" || lower === "hr") {
      return `<${lower}>`;
    }
    const href = tag === "A" ? safeHref(element.getAttribute("href")) : "";
    const attr = href ? ` href="${escapeHtml(href)}"` : "";
    return `<${lower}${attr}>${clean(element)}</${lower}>`;
  };
  const clean = (parent: Node): string => {
    let out = "";
    for (const child of Array.from(parent.childNodes)) {
      if (child.nodeType === Node.TEXT_NODE) {
        out += escapeHtml(child.textContent ?? "");
      } else if (child instanceof Element) {
        out += cleanElement(child);
      }
    }
    return out;
  };
  return clean(root);
}

// The item's position among its own siblings, so a nested list restarts.
function itemNumber(item: HTMLElement): number {
  let n = 1;
  for (
    let prev = item.previousElementSibling;
    prev !== null;
    prev = prev.previousElementSibling
  ) {
    if (prev.tagName.toLowerCase() === "li") {
      n += 1;
    }
  }
  return n;
}

// A block element ends the line it was on; an inline one continues it.
function isBlock(tag: string): boolean {
  return (
    tag === "p" ||
    tag === "div" ||
    tag === "ul" ||
    tag === "ol" ||
    tag === "blockquote"
  );
}

export function safeHref(href: string | null): string {
  const trimmed = (href ?? "").trim();
  const lowered = trimmed.toLowerCase();
  const ok = ["http://", "https://", "mailto:"].some((scheme) =>
    lowered.startsWith(scheme),
  );
  return ok ? trimmed : "";
}

/**
 * Plain text as editor markup: the inverse of {@link plainTextOf}.
 *
 * The drafting endpoints answer in plain text. Without this, a three-paragraph
 * draft shows as one block. A blank line becomes a paragraph and a single
 * newline a line break.
 *
 * It escapes before it wraps, because a draft is model output and may hold
 * characters that close a tag.
 */
export function paragraphsFrom(text: string): string {
  return text
    .split(/\n{2,}/)
    .map((block) => block.trim())
    .filter((block) => block !== "")
    .map((block) => `<p>${escapeHtml(block).replaceAll("\n", "<br>")}</p>`)
    .join("");
}
