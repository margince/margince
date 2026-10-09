// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { escapeHtml } from "../format/html";

/**
 * The plain-text rendering of what the editor holds.
 *
 * Not `textContent`: that runs every block together, so three paragraphs arrive
 * as one sentence with no space between them. Block boundaries become newlines
 * and a list item keeps its marker, because the plain part is a real
 * alternative somebody reads rather than a fallback nobody checks.
 */
export function plainTextOf(node: HTMLElement): string {
  const lines: string[] = [];
  // The line being built. Inline formatting must NOT break it: "The <b>deadline
  // </b> is Friday" is one sentence, and a renderer that emitted a line per
  // element would hand the plain reader a column of words.
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
    // The destination is the point of a link, and a text client shows no href —
    // so the URL rides beside the label rather than being lost.
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
 * `value` is not always something a rep typed: an AI draft arrives here, and a
 * model's output is untrusted input however friendly its source. The server
 * filters what LEAVES for a recipient; this filters what ENTERS our own
 * document, and the two protect different contacts.
 *
 * It mirrors the server's allowlist deliberately — the same elements, the same
 * three link schemes — so a rep never sees formatting in the composer that the
 * outbound filter would strip on the way out. Built with the DOM parser rather
 * than a regex: the browser is the thing that decides what markup means, so it
 * is the thing that should parse it.
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
      // Unwrap, exactly as the server does: a sender whose <div> vanished still
      // meant the sentence inside it.
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

// Which item this is within its own list, counting only siblings — so a nested
// list restarts rather than continuing its parent's numbering.
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
 * Plain text as the markup this editor round-trips — the inverse of
 * {@link plainTextOf}, and the way a machine-written draft arrives in a field a
 * human formats from.
 *
 * The drafting endpoints answer in PLAIN text by contract. Handed to the editor
 * unchanged, a three-paragraph mail renders as one run-on block that the rep
 * then has to break up by hand before they can read what was written for them;
 * handed through here it arrives shaped the way the model wrote it. Nothing is
 * INVENTED on the rep's behalf — a blank line is a paragraph and a single
 * newline is a line break, which is what those two characters already mean in
 * the text being converted.
 *
 * It escapes before it wraps. A draft is model output and can carry the three
 * characters that would otherwise close a tag; escaping after wrapping would
 * escape our own markup instead of the words inside it.
 */
export function paragraphsFrom(text: string): string {
  return text
    .split(/\n{2,}/)
    .map((block) => block.trim())
    .filter((block) => block !== "")
    .map((block) => `<p>${escapeHtml(block).replaceAll("\n", "<br>")}</p>`)
    .join("");
}
