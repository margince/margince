import {
  type ComponentPropsWithoutRef,
  forwardRef,
  type Ref,
  useEffect,
  useRef,
  useState,
} from "react";
import { useT } from "../i18n";
import { isImeCommit } from "../lib/imecommit";
import { problemMessageOf } from "../lib/problem";
import { BusyMark, Button, Field, Textarea, TextInput } from "./atoms";
import { DateInput, type DateInputProps, isoOrBlank } from "./dateinput";
import { panelsOpenedFrom } from "./dialogfocus";
import "./inlinechoice.css";

// Edits a value where it is read, under InlineChoice's save and refusal rules.
// Typing has no moment of choice, so Enter or leaving the field commits it.
// One forwarding control holds the line, the day and the paragraph, so their
// shared props cannot drift apart and lose `aria-invalid` on one branch.
const InlineTextControl = forwardRef<
  HTMLInputElement & HTMLTextAreaElement,
  Readonly<{
    multiline?: boolean;
    value: string;
    onDraft: (next: string) => void;
    onSettle: (next: string) => void;
    onLeave: (typed?: string) => void;
  }> &
    Omit<
      ComponentPropsWithoutRef<"input"> & ComponentPropsWithoutRef<"textarea">,
      "value" | "defaultValue" | "onChange" | "onBlur" | "min" | "max"
    >
>(function InlineTextControl(
  { multiline, type, onDraft, onSettle, onLeave, ...props },
  ref,
) {
  if (multiline) {
    return (
      <Textarea
        ref={ref}
        rows={4}
        {...props}
        onChange={(event) => onDraft(event.target.value)}
        onBlur={() => onLeave()}
      />
    );
  }
  if (type === "date") {
    return (
      <InlineDate
        ref={ref}
        {...props}
        onDraft={onDraft}
        onSettle={onSettle}
        onLeave={onLeave}
      />
    );
  }
  return (
    <TextInput
      ref={ref}
      type={type}
      {...props}
      onChange={(event) => onDraft(event.target.value)}
      onBlur={() => onLeave()}
    />
  );
});

// Every day DateInput reports is settled, so it saves at once. A day settled by
// the blur that moves to the calendar button is only drafted.
function InlineDate({
  value,
  onDraft,
  onSettle,
  onLeave,
  onKeyDown,
  ...box
}: Readonly<{
  value: string;
  onDraft: (next: string) => void;
  onSettle: (next: string) => void;
  onLeave: (typed?: string) => void;
}> &
  Omit<DateInputProps, "value" | "onChange" | "onBlur">) {
  const shell = useRef<HTMLSpanElement>(null);
  const blurring = useRef(false);
  const settled = useRef<string | null>(null);
  // While the calendar is open: a press in it that focuses nothing blurs to null.
  const stays = (next: Element | null) =>
    shell.current?.contains(next) || panelsOpenedFrom(shell.current).length > 0;
  return (
    // biome-ignore lint/a11y/noStaticElementInteractions: not a control — it hears focus leave the text box and calendar button inside it
    <span
      ref={shell}
      className="inlinetext-date"
      onKeyDownCapture={() => {
        settled.current = null;
      }}
      onBlurCapture={() => {
        settled.current = null;
        blurring.current = true;
      }}
      onBlur={(event) => {
        const next = settled.current;
        blurring.current = false;
        settled.current = null;
        if (!stays(event.relatedTarget)) {
          onLeave(next ?? undefined);
        } else if (next !== null) {
          onDraft(next);
        }
      }}
    >
      <DateInput
        {...box}
        value={isoOrBlank(value)}
        onChange={(event) => {
          settled.current = event.target.value;
          if (!blurring.current) {
            onSettle(event.target.value);
          }
        }}
        onKeyDown={(event) => {
          if (event.key !== "Enter" || isImeCommit(event)) {
            onKeyDown?.(event);
            return;
          }
          event.preventDefault();
          if (settled.current === null) {
            onSettle(value);
          }
        }}
      />
    </span>
  );
}

// The field for a reader who cannot edit it. `placeholder` is written for
// someone about to press it ("Add legal name"): shown plain to a viewer who
// cannot, it reads as an instruction aimed at them. A suggested value stands
// in, muted; failing that, `field.unset` is the neutral fact, the same
// fallback the grid's own read-only rows (owner, domain, address) use, so an
// empty field never reads as an invitation this viewer cannot act on or a
// blank the row forgot to fill.
function ReadOnlyText({
  value,
  display,
  suggested,
  readOnlyReason,
}: Readonly<{
  value: string;
  display?: string;
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
      {(value && display) || value || suggested || t("field.unset")}
    </span>
  );
}

// The edit verb beside a value its caller draws as something to follow.
// `text` replaces "Change {field}" where the editor does more than change one
// value, as a list's does.
export function InlineEditVerb({
  label,
  text,
  disabled,
  onClick,
  ref,
}: Readonly<{
  label: string;
  text?: string;
  disabled?: boolean;
  onClick: () => void;
  ref?: Ref<HTMLButtonElement>;
}>) {
  const t = useT();
  return (
    <Button
      ref={ref}
      variant="link"
      className="inline-editable"
      disabled={disabled}
      onClick={onClick}
    >
      {text ?? t("inlineChoice.change", { field: label })}
    </Button>
  );
}

export function InlineText({
  label,
  value,
  display,
  verb,
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
  // The resting reading of `value` when it is not the text the editor edits:
  // a currency in its own code over the bare major-unit amount.
  display?: string;
  // The caller draws the value beside this as a link, so the resting trigger is
  // the edit verb (disabled while `canEdit` is false): a link inside the trigger
  // would be two controls in one.
  verb?: boolean;
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
  // Not a dependency below: a parent that re-renders on dirty would take focus back.
  const editingHeard = useRef(onEditingChange);
  useEffect(() => {
    editingHeard.current = onEditingChange;
  }, [onEditingChange]);

  // The click that opened this asked to TYPE here, so the caret belongs in the
  // field without a second click. It is also what makes every exit rule below
  // hold at all: Escape, Enter and the blur-commit are all keyboard and focus
  // events on this input, and an input nobody ever focused receives none of
  // them — the box would then sit open until it was clicked into, including
  // after the reader had moved on to another field.
  useEffect(() => {
    editingHeard.current?.(editing);
    if (editing) {
      field.current?.focus();
      return;
    }
    if (restoreFocus.current) {
      restoreFocus.current = false;
      trigger.current?.focus();
    }
  }, [editing]);

  if (!canEdit || !editing) {
    const shown = (value && display) || value || suggested || placeholder;
    const change = t("inlineChoice.change", { field: label });
    const open = () => {
      setDraft(value);
      setFailure(null);
      // A previous Escape can leave this set if the browser never
      // delivered the unmount's blur to this node's React handler — the
      // one place `onBlur` below clears it. Cleared here too, the one
      // path every new edit session always runs, so a stale flag cannot
      // silently swallow this session's first blur commit.
      cancelling.current = false;
      setEditing(true);
    };
    if (verb)
      return (
        <InlineEditVerb
          ref={trigger}
          label={label}
          disabled={!canEdit}
          onClick={open}
        />
      );
    if (!canEdit) {
      return (
        <ReadOnlyText
          value={value}
          display={display}
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
        aria-label={change}
        title={change}
        onClick={open}
      >
        {shown}
      </button>
    );
  }

  // `byKeyboard` is what tells the two exits apart: an Enter press ends the
  // edit without moving the reader anywhere, so focus goes back to the value
  // they were on; a blur means they have already moved, and is left alone.
  const commit = async (byKeyboard = false, typed = draft) => {
    // A commit already in flight owns the next state transition; a second
    // one racing in behind it (Enter, then the blur disabling the input for
    // `saving` fires synchronously) would double-send the same edit.
    if (saving) {
      return;
    }
    if (field.current && !field.current.checkValidity()) {
      // Not for a day: DateInput draws its own refusal under the box.
      setFailure(type === "date" ? null : field.current.validationMessage);
      field.current.focus();
      return;
    }
    const next = multiline ? typed : typed.trim();
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
    <Field
      label={label}
      labelHidden
      error={failure ?? undefined}
      className="inlinetext-edit"
    >
      {(control) => (
        <>
          <InlineTextControl
            {...control}
            multiline={multiline}
            type={type}
            step={step}
            ref={field}
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
            onDraft={setDraft}
            onSettle={(settled) => {
              setDraft(settled);
              void commit(true, settled);
            }}
            onKeyDown={(event) => {
              // In a paragraph Enter is a newline and the commit moves to
              // Cmd/Ctrl+Enter; in a line Enter is still the commit.
              if (
                event.key === "Enter" &&
                !isImeCommit(event) &&
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
            onLeave={(typed) => {
              if (cancelling.current) {
                cancelling.current = false;
                return;
              }
              if (typed !== undefined) {
                setDraft(typed);
              }
              void commit(false, typed);
            }}
          />
          {saving && <BusyMark />}
        </>
      )}
    </Field>
  );
}
