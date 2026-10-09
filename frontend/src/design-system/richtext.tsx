// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Bold, Heading2, Italic, Link2, List, ListOrdered } from "lucide-react";
import { type ClipboardEvent, useEffect, useId, useRef, useState } from "react";
import { plainTextOf, safeEditorHTML } from "./richtext-html";
import {
  editorHTMLFromMarkdown,
  markdownOf,
  pastedMarkup,
} from "./richtext-markdown";
import "./richtext.css";

export { paragraphsFrom, plainTextOf, safeEditorHTML } from "./richtext-html";

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
 * ## Markdown mode
 *
 * `format="markdown"` is the same editor for a body stored as markdown, an
 * activity's: `value` is markdown, every change also reports `markdown`, and
 * the toolbar adds a heading. A paste ends formatted whether the clipboard
 * held a document's markup or markdown source. The marks are the mode's, so
 * the email composer keeps its outbound set and offers no heading.
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
  grow = false,
  format = "html",
  id,
  disabled = false,
}: Readonly<{
  /**
   * The markup to show, or the markdown in markdown mode. Read on mount and when it changes from OUTSIDE — an
   * AI draft arriving, a form reset — never on every keystroke, which would
   * fight the caret.
   */
  value: string;
  onChange: (next: { html: string; text: string; markdown: string }) => void;
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
    /** Shown in markdown mode only, which is the mode that keeps headings. */
    heading?: string;
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
  /**
   * `rows` is the floor rather than the height, and the surface grows with its
   * text and with the room its host gives it: for a host whose body is the
   * field, the Log activity drawer's.
   */
  grow?: boolean;
  format?: "html" | "markdown";
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
  const markdown = format === "markdown";
  // The markup the surface held when we last wrote or reported it. A blur with
  // no edit since then reports nothing, so opening and leaving an unedited
  // body never rewrites it in the editor's own spelling.
  const reported = useRef("");

  useEffect(() => {
    const node = editor.current;
    if (!node || value === ours) {
      return;
    }
    // Filtered before it reaches OUR document, not only before it reaches a
    // recipient's. The server's allowlist governs what goes out; this one
    // governs what a model's draft may put in the page a rep is looking at,
    // which is a different trust boundary with a different victim.
    // A markdown body is rebuilt from the parser's closed tree, so it holds
    // only elements this file names.
    node.innerHTML = markdown
      ? editorHTMLFromMarkdown(value)
      : safeEditorHTML(value);
    reported.current = node.innerHTML;
    setOurs(markdown ? value : node.innerHTML);
  }, [value, ours, markdown]);

  const report = () => {
    const node = editor.current;
    if (!node) {
      return;
    }
    const html = node.innerHTML;
    if (html === reported.current) {
      return;
    }
    reported.current = html;
    const source = markdownOf(node);
    setOurs(markdown ? source : html);
    onChange({ html, text: plainTextOf(node), markdown: source });
  };

  const paste = (event: ClipboardEvent<HTMLDivElement>) => {
    const node = editor.current;
    if (!markdown || disabled || !node) {
      return;
    }
    event.preventDefault();
    insertMarkup(
      node,
      pastedMarkup(
        event.clipboardData.getData("text/html"),
        event.clipboardData.getData("text/plain"),
      ),
    );
    report();
  };

  const apply = (command: string, argument?: string) => {
    if (disabled) {
      return;
    }
    editor.current?.focus();
    // execCommand is deprecated and still the only cross-browser way to apply
    // formatting to a selection without a document model. The alternative is
    // reimplementing selection surgery, which is where hand-rolled editors go
    // wrong; what it produces is bounded by the server's allowlist anyway.
    document.execCommand(command, false, argument);
    report();
  };

  const toggleHeading = () =>
    apply(
      "formatBlock",
      document.queryCommandValue("formatBlock") === "h2" ? "p" : "h2",
    );

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
    <div
      className={`richtext${grow ? " is-growing" : ""}${disabled ? " is-disabled" : ""}`}
    >
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
        style={
          grow
            ? { minHeight: `${rows * 1.5}em` }
            : { height: `${rows * 1.5}em` }
        }
        onInput={report}
        onBlur={report}
        onPaste={paste}
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
          {markdown && labels.heading && (
            <RichTextButton
              onClick={toggleHeading}
              title={labels.heading}
              disabled={disabled}
            >
              <Heading2 size={14} aria-hidden="true" />
            </RichTextButton>
          )}
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
 * Put sanitised markup at the caret, replacing the selection.
 *
 * `insertHTML` first, because it keeps the paste on the browser's undo stack.
 * An engine without it gets the same markup through the selection's range.
 */
function insertMarkup(editor: HTMLElement, markup: string): void {
  editor.focus();
  // The type says execCommand always exists; a test engine (happy-dom) has
  // none, and the range path is the one that works there.
  if (
    typeof document.execCommand === "function" &&
    document.execCommand("insertHTML", false, markup)
  ) {
    return;
  }
  const selection = window.getSelection();
  const current = selection?.rangeCount ? selection.getRangeAt(0) : null;
  const range =
    current && editor.contains(current.commonAncestorContainer)
      ? current
      : endOf(editor);
  range.deleteContents();
  const fragment = range.createContextualFragment(markup);
  const last = fragment.lastChild;
  range.insertNode(fragment);
  if (last) {
    range.setStartAfter(last);
    range.collapse(true);
    selection?.removeAllRanges();
    selection?.addRange(range);
  }
}

function endOf(node: HTMLElement): Range {
  const range = document.createRange();
  range.selectNodeContents(node);
  range.collapse(false);
  return range;
}
