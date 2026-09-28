import {
  type ComponentPropsWithoutRef,
  forwardRef,
  useEffect,
  useId,
  useRef,
  useState,
} from "react";
import { useT } from "../i18n";
import { problemMessageOf } from "../screens/common";
import { BusyMark, Textarea, TextInput } from "./atoms";
import { ErrorLine } from "./errorline";
import "./inlinechoice.css";

// Free-text editing follows the same save/refusal contract as choices.
// description, edited where it is read rather than inside a form that also
// asks about legal names and size bands.
//
// It keeps the same rules as InlineChoice above — a viewer who may not edit
// sees the value with no hover affordance, a failed save keeps the typed
// text and shows the refusal beside the field, Escape reverts — and adds the
// two a text field needs that a chooser does not: an explicit MOMENT of
// commit (a chooser commits the instant something is picked; typing has no
// such moment, so Enter or losing focus stands in for it), and something to
// press when the value is empty, since there is no text to click on.
// One control, two elements. The edit session's rules — focus on open, Escape
// reverts, blur commits, `readOnly` while saving — are identical for a line and
// for a paragraph, so they are written once above and this only decides which
// element receives them. Written as a forwarding component rather than a
// ternary at the call site so the whole prop set cannot drift between the two
// branches, which is exactly how one of them would quietly lose `aria-invalid`.
const InlineTextControl = forwardRef<
  HTMLInputElement & HTMLTextAreaElement,
  { multiline?: boolean } & ComponentPropsWithoutRef<"input"> &
    ComponentPropsWithoutRef<"textarea">
>(function InlineTextControl({ multiline, ...props }, ref) {
  if (multiline) {
    return <Textarea ref={ref} rows={4} {...props} />;
  }
  return <TextInput ref={ref} {...props} />;
});

// The field for a reader who cannot edit it. `placeholder` is written for
// someone about to press it ("Add legal name"): shown plain to a viewer who
// cannot, it reads as an instruction aimed at them. A suggested value stands
// in, muted; failing that, `field.unset` is the neutral fact, the same
// fallback the grid's own read-only rows (owner, domain, address) use, so an
// empty field never reads as an invitation this viewer cannot act on or a
// blank the row forgot to fill.
function ReadOnlyText({
  value,
  suggested,
  readOnlyReason,
}: Readonly<{
  value: string;
  suggested?: string;
  readOnlyReason?: string;
}>) {
  const t = useT();
  return (
    <span
      className={
        !value && suggested ? "inlinetext inlinetext-suggested" : "inlinetext"
      }
      title={readOnlyReason}
    >
      {value || suggested || t("field.unset")}
    </span>
  );
}

export function InlineText({
  label,
  value,
  placeholder,
  suggested,
  maxLength,
  multiline,
  type,
  step,
  onEditingChange,
  onDirtyChange,
  canEdit,
  readOnlyReason,
  onSave,
}: Readonly<{
  label: string;
  value: string;
  placeholder: string;
  // A value the record carries elsewhere that stands in for this field until
  // one is written here (a contact's title, for the role at their current
  // employer). Shown muted, in read and in edit alike: an editor sees it as
  // the placeholder they can confirm, a reader sees it instead of "Unset",
  // which would deny a fact the record does hold.
  suggested?: string;
  maxLength?: number;
  type?: "text" | "email" | "number" | "date";
  step?: string;
  // A value that is a PARAGRAPH rather than a line: the account's own story
  // fields run to several sentences, and a single-line input shows a reader
  // one sentence of what they are editing. Enter then inserts a newline
  // instead of committing, because a paragraph needs both — so the commit key
  // becomes Cmd/Ctrl+Enter and blur still commits exactly as it does for a
  // line.
  multiline?: boolean;
  // Fires when the reader opens or closes the editor. The row beside this
  // control may need to know: an evidence receipt describes the STORED value,
  // and standing beside a draft it would attribute the reader's own words to
  // whatever produced the old one.
  onEditingChange?: (editing: boolean) => void;
  onDirtyChange?: (dirty: boolean) => void;
  canEdit: boolean;
  readOnlyReason?: string;
  // Returns nothing on success and throws on failure. What the reader is shown
  // is `problemMessageOf`'s reading of the throw, on the same terms as
  // InlineChoice above.
  onSave: (next: string) => Promise<void>;
}>) {
  const t = useT();
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState(value);
  const [saving, setSaving] = useState(false);
  const [failure, setFailure] = useState<string | null>(null);
  // Escape unmounts this input, which the browser reads as focus leaving it —
  // a blur this control did not ask to commit. Set true for exactly the tick
  // between the Escape keydown and that blur, so the blur handler below can
  // tell "the reader cancelled" from "the reader tabbed away" and skip the
  // commit only for the former.
  useEffect(() => {
    onDirtyChange?.(
      editing && (saving || (multiline ? draft : draft.trim()) !== value),
    );
  }, [editing, saving, draft, multiline, value, onDirtyChange]);
  const cancelling = useRef(false);
  const field = useRef<HTMLInputElement & HTMLTextAreaElement>(null);
  const trigger = useRef<HTMLButtonElement>(null);
  // Set by the two exits the reader takes without leaving this field —
  // Escape, and Enter on a save that succeeds — and read once the resting
  // trigger has remounted, exactly as InlineChoice does. A blur-commit
  // deliberately does NOT set it: the reader is already somewhere else, and
  // dragging focus back here would undo the move they just made.
  const restoreFocus = useRef(false);
  const fieldId = useId();
  const errorId = useId();

  // The click that opened this asked to TYPE here, so the caret belongs in the
  // field without a second click. It is also what makes every exit rule below
  // hold at all: Escape, Enter and the blur-commit are all keyboard and focus
  // events on this input, and an input nobody ever focused receives none of
  // them — the box would then sit open until it was clicked into, including
  // after the reader had moved on to another field.
  useEffect(() => {
    onEditingChange?.(editing);
    if (editing) {
      field.current?.focus();
      return;
    }
    if (restoreFocus.current) {
      restoreFocus.current = false;
      trigger.current?.focus();
    }
  }, [editing, onEditingChange]);

  if (!canEdit || !editing) {
    const shown = value || suggested || placeholder;
    if (!canEdit) {
      return (
        <ReadOnlyText
          value={value}
          suggested={suggested}
          readOnlyReason={readOnlyReason}
        />
      );
    }
    return (
      <button
        ref={trigger}
        type="button"
        className="inline-editable"
        // An empty field is an invitation to fill it and reads as one; a field
        // with a value is a value, and dressing the fact as a link would say
        // it is a place to go.
        data-empty={value ? undefined : "true"}
        aria-label={t("inlineChoice.change", { field: label })}
        title={t("inlineChoice.change", { field: label })}
        onClick={() => {
          setDraft(value);
          setFailure(null);
          // A previous Escape can leave this set if the browser never
          // delivered the unmount's blur to this node's React handler — the
          // one place `onBlur` below clears it. Cleared here too, the one
          // path every new edit session always runs, so a stale flag cannot
          // silently swallow this session's first blur commit.
          cancelling.current = false;
          setEditing(true);
        }}
      >
        {shown}
      </button>
    );
  }

  // `byKeyboard` is what tells the two exits apart: an Enter press ends the
  // edit without moving the reader anywhere, so focus goes back to the value
  // they were on; a blur means they have already moved, and is left alone.
  const commit = async (byKeyboard = false) => {
    // A commit already in flight owns the next state transition; a second
    // one racing in behind it (Enter, then the blur disabling the input for
    // `saving` fires synchronously) would double-send the same edit.
    if (saving) {
      return;
    }
    if (field.current && !field.current.checkValidity()) {
      setFailure(field.current.validationMessage);
      field.current.focus();
      return;
    }
    const next = multiline ? draft : draft.trim();
    // Saving what is already stored writes an audit row for a change that did
    // not happen. Blur fires on every exit now, so this guard is what keeps
    // "clicked in, typed nothing, clicked out" silent.
    if (next === value) {
      restoreFocus.current = byKeyboard;
      setEditing(false);
      return;
    }
    setSaving(true);
    setFailure(null);
    try {
      await onSave(next);
      restoreFocus.current = byKeyboard;
      setEditing(false);
    } catch (err) {
      // The draft survives and the input stays mounted right where the
      // reader left it — pulling focus back after a failed blur-commit would
      // be a second surprise on top of the refusal.
      setFailure(problemMessageOf(err, t));
    } finally {
      setSaving(false);
    }
  };

  return (
    <span className="inlinetext-edit">
      <label className="sr-only" htmlFor={fieldId}>
        {label}
      </label>
      <InlineTextControl
        multiline={multiline}
        type={type}
        step={step}
        ref={field}
        id={fieldId}
        value={draft}
        maxLength={maxLength}
        // `readOnly`, not `disabled`, and this is the one that had to change.
        // A disabled field leaves the tab order, so a reader who pressed Enter
        // and then Tab was thrown to the far side of the form for as long as
        // the write took. Read-only holds the field, holds the caret, and
        // refuses the keystroke — which is the whole of what a write in flight
        // needs. `aria-busy` carries the reason.
        readOnly={saving}
        aria-busy={saving || undefined}
        aria-invalid={failure ? true : undefined}
        aria-describedby={failure ? errorId : undefined}
        onChange={(event) => setDraft(event.target.value)}
        onKeyDown={(event) => {
          // In a paragraph Enter is a newline and the commit moves to
          // Cmd/Ctrl+Enter; in a line Enter is still the commit.
          if (
            event.key === "Enter" &&
            (!multiline || event.metaKey || event.ctrlKey)
          ) {
            event.preventDefault();
            void commit(true);
          }
          if (event.key === "Escape") {
            cancelling.current = true;
            restoreFocus.current = true;
            setDraft(value);
            setEditing(false);
          }
        }}
        onBlur={() => {
          if (cancelling.current) {
            cancelling.current = false;
            return;
          }
          void commit();
        }}
      />
      {saving && <BusyMark />}
      {failure && (
        <ErrorLine inline id={errorId}>
          {failure}
        </ErrorLine>
      )}
    </span>
  );
}
