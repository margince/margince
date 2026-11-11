// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Bold, Heading2, Italic, Link2, List, ListOrdered } from "lucide-react";
import {
  type ClipboardEvent,
  type KeyboardEvent,
  useEffect,
  useId,
  useRef,
  useState,
} from "react";
import { createPortal } from "react-dom";
import { escapeHtml } from "../format/html";
import { isChordOf } from "../lib/chord";
import { isImeCommit } from "../lib/imecommit";
import { AiPending } from "./aipending";
import { useAiReveal } from "./aireveal";
import { isParagraphCommit } from "./fieldeditdialog";
import { LinkDialog } from "./linkdialog";
import {
  dropRefusedHrefs,
  endOf,
  insertMarkup,
  linkAround,
  plainTextOf,
  replaceUndoably,
  safeEditorHTML,
  selectionIn,
} from "./richtext-html";
import {
  editorHTMLFromMarkdown,
  markdownOf,
  pastedMarkup,
} from "./richtext-markdown";
import "./richtext.css";

// The palette's key too: in the editor it opens the link instead.
const LINK_KEY = "k";

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
  footIn,
  revealText,
  thinking,
  onSubmit,
  undoableRevision,
}: Readonly<{
  /**
   * The markup to show, or the markdown in markdown mode. Read on mount and
   * when it changes from outside (an AI draft, a form reset). Never on every
   * keystroke, which would fight the caret.
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
   * `rows` is the floor rather than the height. The surface grows with its text
   * and with the room its host gives it, as in the Log activity drawer.
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
  /**
   * Where the footer draws instead of under the surface: the composer keeps
   * the marks in its own foot, beside Send. Null while that place is not
   * mounted yet; absent draws the footer in the frame.
   */
  footIn?: Element | null;
  /**
   * The text a model just wrote into the surface. It arrives the way every
   * machine-written answer does (`AiReveal`): once, the first time it is on
   * screen. Unset for text a reader typed.
   */
  revealText?: string | null;
  /**
   * What a model is writing into the surface right now, as the sentence
   * `AiPending` shows. While the surface is empty its speck rows stand where
   * the words will land; the first key a reader types takes them away.
   */
  thinking?: string | null;
  /** Cmd/Ctrl+Enter in the surface: the host's own send. */
  onSubmit?: () => void;
  /** A new number with a new `value` marks a replacement the reader may undo,
   * such as a model's draft. Absent, no outside value is undoable. */
  undoableRevision?: number;
}>) {
  const generatedId = useId();
  const fieldId = id ?? generatedId;
  const hintId = `${fieldId}-hint`;
  const editor = useRef<HTMLDivElement>(null);
  useAiReveal(editor, revealText);
  // What we last handed the caller, or last wrote into the node. Comparing
  // against it tells an outside change (a draft arriving) from the echo of our
  // own keystroke. It starts empty, so a value already in state is written on
  // mount: a body reopened from state never renders blank.
  const [ours, setOurs] = useState("");
  const markdown = format === "markdown";
  // The markup the surface held when we last wrote or reported it. A blur with
  // no edit since then reports nothing, so opening and leaving an unedited
  // body never rewrites it in the editor's own spelling.
  const reported = useRef("");
  const seenRevision = useRef(undoableRevision);
  // While our own command writes a value from outside, its input event is no
  // edit of the reader's.
  const replacing = useRef(false);
  const [linking, setLinking] = useState<{
    href: string;
    range: Range;
  } | null>(null);

  useEffect(() => {
    const node = editor.current;
    // Only the host knows which values the reader may undo: undoing a switch of
    // conversation or a reset would put one message's words into another.
    const undoable =
      undoableRevision !== undefined &&
      undoableRevision !== seenRevision.current;
    seenRevision.current = undoableRevision;
    if (!node || value === ours) {
      return;
    }
    // Filtered before it reaches OUR document, not only before it reaches a
    // recipient's. The server's allowlist governs what goes out; this one
    // governs what a model's draft may put in the page a rep is looking at,
    // which is a different trust boundary with a different victim.
    // A markdown body is rebuilt from the parser's closed tree, so it holds
    // only elements this file names.
    const markup = markdown
      ? editorHTMLFromMarkdown(value)
      : safeEditorHTML(value);
    if (
      node.innerHTML !== markup &&
      !(undoable && replaceUndoably(node, markup, replacing))
    ) {
      node.innerHTML = markup;
    }
    reported.current = node.innerHTML;
    setOurs(value);
    // A link asked for over words that are gone would land at the top.
    setLinking(null);
  }, [value, ours, markdown, undoableRevision]);

  const report = () => {
    const node = editor.current;
    if (!node || replacing.current) {
      return;
    }
    dropRefusedHrefs(node);
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

  const openLink = () => {
    const node = editor.current;
    if (disabled || !node) {
      return;
    }
    const range = selectionIn(node) ?? endOf(node);
    const anchor = linkAround(node, range);
    setLinking({ href: anchor?.getAttribute("href") ?? "", range });
  };

  // The dialog took the caret; the words it was on get it back. A caret in a
  // link stands for the whole link when the link is what changes.
  const backToText = (wholeLink: boolean) => {
    const node = editor.current;
    if (!node || !linking) {
      return null;
    }
    node.focus();
    const selection = window.getSelection();
    selection?.removeAllRanges();
    selection?.addRange(linking.range);
    const anchor = linkAround(node, linking.range);
    if (wholeLink && anchor && linking.range.collapsed) {
      selection?.selectAllChildren(anchor);
    }
    return { node, anchor, collapsed: linking.range.collapsed };
  };

  const closeLink = () => {
    backToText(false);
    setLinking(null);
  };

  const applyLink = (href: string) => {
    const back = backToText(true);
    setLinking(null);
    if (back && !back.anchor && back.collapsed) {
      const words = escapeHtml(href.replace(/^mailto:/i, ""));
      insertMarkup(back.node, `<a href="${escapeHtml(href)}">${words}</a>`);
    } else if (back) {
      document.execCommand("createLink", false, href);
    }
    report();
  };

  const removeLink = () => {
    backToText(true);
    setLinking(null);
    document.execCommand("unlink");
    report();
  };

  const keyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    if (isImeCommit(event)) {
      return;
    }
    if (isChordOf(event, LINK_KEY)) {
      // Stopped here, the palette's listener on the window never hears it.
      event.preventDefault();
      event.stopPropagation();
      openLink();
    } else if (onSubmit && isParagraphCommit(event)) {
      event.preventDefault();
      onSubmit();
    }
  };

  const foot = (
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
          onClick={openLink}
          title={labels.link}
          disabled={disabled}
        >
          <Link2 size={14} aria-hidden="true" />
        </RichTextButton>
      </div>
      {actions}
    </div>
  );
  return (
    <div
      className={`richtext${grow ? " is-growing" : ""}${disabled ? " is-disabled" : ""}${thinking ? " is-thinking" : ""}`}
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
        onKeyDown={keyDown}
      />
      {thinking && (
        <div className="richtext-thinking">
          <AiPending label={thinking} lines={3} />
        </div>
      )}
      {/* Under the words and quiet, so the message comes first; the buttons
          keep their names and tooltips, since quieter is about ink only. */}
      {footIn === undefined ? foot : footIn && createPortal(foot, footIn)}
      <LinkDialog
        open={linking !== null}
        href={linking?.href ?? ""}
        onApply={applyLink}
        onRemove={linking?.href ? removeLink : undefined}
        onClose={closeLink}
        returnFocusTo={() => editor.current}
      />
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
