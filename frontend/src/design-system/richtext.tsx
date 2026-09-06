// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Bold, Italic, Link2, List, ListOrdered } from "lucide-react";
import { useEffect, useId, useRef, useState } from "react";
import "./richtext.css";

/**
 * RichText — the light formatting a business email needs, and nothing more.
 *
 * Bold, italic, links and lists. Not a document editor: the message this writes
 * goes through the server's outbound allowlist
 * (`activities.SanitizeOutboundHTML`), which keeps exactly this set and unwraps
 * everything else, so a toolbar offering headings or colours would offer
 * formatting the recipient never receives.
 *
 * ## Why contentEditable rather than an editor library
 *
 * A library (tiptap, Lexical) buys a document model, collaborative editing and
 * a plugin system — none of which a five-sentence email needs — for a dependency
 * tree this product would then carry forever. What it would genuinely buy is
 * paste normalisation, and the server already does that job for the case that
 * matters: what a recipient receives is what the allowlist admits, whatever the
 * browser put in the DOM.
 *
 * ## The two values
 *
 * Every change reports BOTH renderings: `html` for the markup alternative and
 * `text` for the plain part. They are the same message in two forms and both go
 * on the wire (multipart/alternative), so a caller that kept only one would send
 * a message whose halves disagree — and which half a recipient reads is their
 * client's decision, not ours.
 *
 * ## What it does not do
 *
 * No image button: this product refuses tracking pixels, and a remote image is
 * a read receipt. No colour or font controls: they arrive as inline styles the
 * allowlist drops, so the button would lie.
 */
export function RichText({
  value,
  onChange,
  label,
  labels,
  placeholder,
  hint,
  actions,
  rows = 12,
  id,
  disabled = false,
}: Readonly<{
  /**
   * The markup to show. Read on mount and when it changes from OUTSIDE — an
   * AI draft arriving, a form reset — never on every keystroke, which would
   * fight the caret.
   */
  value: string;
  onChange: (next: { html: string; text: string }) => void;
  label: string;
  /**
   * The toolbar's accessible names. Copy never lives in a primitive: words
   * arrive through props, translated by the caller with t().
   */
  labels: Readonly<{
    bold: string;
    italic: string;
    bulletList: string;
    numberList: string;
    link: string;
    linkPrompt: string;
  }>;
  placeholder?: string;
  /**
   * One line about the control, shown beside the toolbar rather than above or
   * below it. Copy never lives in a primitive: the words arrive translated.
   *
   * It shares the footer with the formatting buttons because the two are the
   * same sentence read twice — this is a box you write in, and here is what you
   * can do to what you wrote. On their own lines they were two claims stacked
   * under one field.
   */
  hint?: string;
  /**
   * Further verbs for the same message, drawn in the footer beside the
   * formatting marks — the composer's paperclip is the one today.
   *
   * A slot rather than a prop per verb: what else you can do to a message is the
   * CALLER's list and grows on their side, and a primitive that enumerated it
   * would have to be edited every time it did.
   */
  actions?: React.ReactNode;
  rows?: number;
  id?: string;
  /**
   * Not now — the surface and its toolbar both refuse.
   *
   * `contentEditable` has no `disabled`, so an editor left editable while a
   * write about its own words is in flight is one a reader can keep typing
   * into: the composer freezes the body while a draft rejection is being
   * recorded, and text typed into that window would be text the returning
   * reference claims to name and never saw. The toolbar goes with it, because a
   * live Bold over a frozen surface is a control that reports success and
   * changes nothing.
   */
  disabled?: boolean;
}>) {
  const generatedId = useId();
  const fieldId = id ?? generatedId;
  const hintId = `${fieldId}-hint`;
  const editor = useRef<HTMLDivElement>(null);
  // What we last handed the caller, or last wrote into the node. Comparing
  // against it tells an outside change (a draft arriving) from the echo of our
  // own keystroke.
  //
  // It starts EMPTY rather than at `value`, and that is the whole point: seeded
  // with `value`, the effect below saw no difference on mount and never wrote
  // the node — so a composer reopened with a message in state rendered blank
  // while Send still carried the invisible text.
  const [ours, setOurs] = useState("");

  useEffect(() => {
    const node = editor.current;
    if (!node || value === ours) {
      return;
    }
    // Filtered before it reaches OUR document, not only before it reaches a
    // recipient's. The server's allowlist governs what goes out; this one
    // governs what a model's draft may put in the page a rep is looking at,
    // which is a different trust boundary with a different victim.
    node.innerHTML = safeEditorHTML(value);
    setOurs(node.innerHTML);
  }, [value, ours]);

  const report = () => {
    const node = editor.current;
    if (!node) {
      return;
    }
    const html = node.innerHTML;
    setOurs(html);
    onChange({ html, text: plainTextOf(node) });
  };

  const apply = (command: string) => {
    if (disabled) {
      return;
    }
    editor.current?.focus();
    // execCommand is deprecated and still the only cross-browser way to apply
    // formatting to a selection without a document model. The alternative is
    // reimplementing selection surgery, which is where hand-rolled editors go
    // wrong; what it produces is bounded by the server's allowlist anyway.
    document.execCommand(command);
    report();
  };

  const addLink = () => {
    if (disabled) {
      return;
    }
    const href = window.prompt(labels.linkPrompt);
    if (href === null) {
      return;
    }
    editor.current?.focus();
    // An empty answer REMOVES the link, which is the only way to undo one from
    // a toolbar with no unlink button.
    document.execCommand(
      href.trim() === "" ? "unlink" : "createLink",
      false,
      href.trim(),
    );
    report();
  };

  return (
    <div className={`richtext${disabled ? " is-disabled" : ""}`}>
      {/* biome-ignore lint/a11y/useSemanticElements: a textarea cannot carry formatting; this is the editable surface the toolbar acts on */}
      <div
        ref={editor}
        id={fieldId}
        role="textbox"
        aria-multiline="true"
        aria-label={label}
        aria-describedby={hint ? hintId : undefined}
        contentEditable={!disabled}
        suppressContentEditableWarning
        // `contentEditable` carries no `disabled`, so the refusal is stated the
        // way a role="textbox" states it — which is also what a checker and a
        // screen reader read.
        aria-disabled={disabled || undefined}
        aria-readonly={disabled || undefined}
        // contentEditable is focusable in every engine, but stating it is what
        // makes the role and the behaviour agree for a checker and a reader.
        tabIndex={0}
        data-placeholder={placeholder}
        className="richtext-input"
        style={{ minHeight: `${rows * 1.5}em` }}
        onInput={report}
        onBlur={report}
      />
      {/* UNDER the words, and quiet. The toolbar led the control for a while —
          a filled band above the box, before a reader had written anything to
          format — which made four glyphs the first thing on a surface whose
          subject is the message. Bold and italic are marks everybody already
          knows; they do not need to announce themselves, and putting them at the
          end of the line the hint occupies costs the field no height at all.

          The buttons keep their names for a screen reader and their tooltips
          for a pointer: quieter is about ink, never about who can use it. */}
      <div className="richtext-foot">
        {hint && (
          <p id={hintId} className="t-caption richtext-hint">
            {hint}
          </p>
        )}
        <div className="richtext-bar" role="toolbar" aria-label={label}>
          <RichTextButton
            onClick={() => apply("bold")}
            title={labels.bold}
            disabled={disabled}
          >
            <Bold size={14} aria-hidden="true" />
          </RichTextButton>
          <RichTextButton
            onClick={() => apply("italic")}
            title={labels.italic}
            disabled={disabled}
          >
            <Italic size={14} aria-hidden="true" />
          </RichTextButton>
          <RichTextButton
            onClick={() => apply("insertUnorderedList")}
            title={labels.bulletList}
            disabled={disabled}
          >
            <List size={14} aria-hidden="true" />
          </RichTextButton>
          <RichTextButton
            onClick={() => apply("insertOrderedList")}
            title={labels.numberList}
            disabled={disabled}
          >
            <ListOrdered size={14} aria-hidden="true" />
          </RichTextButton>
          <RichTextButton
            onClick={addLink}
            title={labels.link}
            disabled={disabled}
          >
            <Link2 size={14} aria-hidden="true" />
          </RichTextButton>
        </div>
        {actions}
      </div>
    </div>
  );
}

function RichTextButton({
  onClick,
  title,
  disabled,
  children,
}: Readonly<{
  onClick: () => void;
  title: string;
  disabled?: boolean;
  children: React.ReactNode;
}>) {
  return (
    <button
      type="button"
      className="richtext-btn"
      disabled={disabled}
      // The pointer-down default is what steals the selection the command is
      // about to act on, so the button never takes focus from the text.
      onMouseDown={(event) => event.preventDefault()}
      onClick={onClick}
      title={title}
      aria-label={title}
    >
      {children}
    </button>
  );
}

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
 * document, and the two protect different people.
 *
 * It mirrors the server's allowlist deliberately — the same elements, the same
 * three link schemes — so a rep never sees formatting in the composer that the
 * outbound filter would strip on the way out. Built with the DOM parser rather
 * than a regex: the browser is the thing that decides what markup means, so it
 * is the thing that should parse it.
 */
export function safeEditorHTML(markup: string): string {
  const allowed = new Set([
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
  const dropped = new Set([
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
  const parsed = new DOMParser().parseFromString(markup, "text/html");
  const cleanElement = (element: Element): string => {
    const tag = element.tagName;
    if (dropped.has(tag)) {
      return "";
    }
    if (!allowed.has(tag)) {
      // Unwrap, exactly as the server does: a sender whose <div> vanished still
      // meant the sentence inside it.
      return clean(element);
    }
    const lower = tag.toLowerCase();
    if (lower === "br" || lower === "hr") {
      return `<${lower}>`;
    }
    const href = tag === "A" ? safeHref(element.getAttribute("href")) : "";
    const attr = href ? ` href="${escapeText(href)}"` : "";
    return `<${lower}${attr}>${clean(element)}</${lower}>`;
  };
  const clean = (parent: Node): string => {
    let out = "";
    for (const child of Array.from(parent.childNodes)) {
      if (child.nodeType === Node.TEXT_NODE) {
        out += escapeText(child.textContent ?? "");
      } else if (child instanceof Element) {
        out += cleanElement(child);
      }
    }
    return out;
  };
  return clean(parsed.body);
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

function safeHref(href: string | null): string {
  const trimmed = (href ?? "").trim();
  const lowered = trimmed.toLowerCase();
  const ok = ["http://", "https://", "mailto:"].some((scheme) =>
    lowered.startsWith(scheme),
  );
  return ok ? trimmed : "";
}

function escapeText(text: string): string {
  return text
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;");
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
  const escaped = (line: string) =>
    line
      .replaceAll("&", "&amp;")
      .replaceAll("<", "&lt;")
      .replaceAll(">", "&gt;");
  return text
    .split(/\n{2,}/)
    .map((block) => block.trim())
    .filter((block) => block !== "")
    .map((block) => `<p>${escaped(block).replaceAll("\n", "<br>")}</p>`)
    .join("");
}
