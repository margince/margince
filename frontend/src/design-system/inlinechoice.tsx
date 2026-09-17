import { ChevronDown } from "lucide-react";
import {
  type ReactNode,
  type RefObject,
  useEffect,
  useId,
  useRef,
  useState,
} from "react";
import { useT } from "../i18n";
import { problemMessageOf } from "../screens/common";
import { BusyMark } from "./atoms";
import "./inlinechoice.css";
import { Select, type SelectOption } from "./select";

// One value a reader can change without leaving the page they are reading.
//
// The interaction is edit-in-place, not a form: at rest the value reads as
// plain text — no box, no accent, nothing saying "control" — and only a hover
// or a keyboard focus reveals the affordance (an underline, and for a chooser
// a caret) that this can be changed. A click turns the value itself into the
// live control in the same spot; there is no separate Save — a chooser
// commits on picking, a text field commits on Enter or on losing focus.
//
// The rules it keeps, all failure modes rather than polish:
//
//   - A viewer who may NOT change the value sees the VALUE, with no hover
//     affordance at all. A control that looks editable and then refuses is
//     a defect already fixed once; plain text says what is true.
//   - A save that fails leaves the control open on what the user chose,
//     the refusal shown right beside it. Snapping back to the old value on a
//     version conflict would discard their answer and tell them nothing.
//   - Escape reverts and closes, so a reader who opened it to LOOK can get
//     out without changing anything, whether or not a save is failing.
//   - Choosing or retyping the value already stored is not an edit: no
//     audit row for a change that did not happen, however often blur fires.

export function InlineChoice({
  label,
  hideLabel,
  value,
  options,
  canEdit,
  readOnlyReason,
  render,
  onSave,
  onEditingChange,
  onDirtyChange,
}: Readonly<{
  // Names the field, for the reader and for assistive tech. A bare value in a
  // header row reads as one more fact among many.
  label: string;
  // Suppresses the VISIBLE "label: " prefix without touching the accessible
  // name: `label` still drives the change button's aria-label and the edit
  // form's own label, both read to assistive tech, sr-only rather than
  // dropped. For a caller whose surrounding layout already prints the field's
  // name once (FieldGrid's own label column) — printing it a second time here
  // is the field naming itself twice, not a second fact.
  hideLabel?: boolean;
  onEditingChange?: (editing: boolean) => void;
  onDirtyChange?: (dirty: boolean) => void;
  value: string;
  options: readonly SelectOption[];
  canEdit: boolean;
  readOnlyReason?: string;
  render: (value: string) => ReactNode;
  // Refused saves keep the draft and render a translated problem.
  onSave: (next: string) => Promise<void>;
}>) {
  const t = useT();
  const [editing, setEditing] = useState(false);
  useEffect(() => {
    onEditingChange?.(editing);
  }, [editing, onEditingChange]);
  const [pending, setPending] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  useEffect(() => {
    onDirtyChange?.(editing && (saving || pending !== value));
  }, [editing, saving, pending, value, onDirtyChange]);
  const [failure, setFailure] = useState<string | null>(null);
  const container = useRef<HTMLSpanElement>(null);
  const trigger = useRef<HTMLButtonElement>(null);
  // Set right before a close that should return focus to the resting
  // trigger, read once that trigger has actually remounted (see the effect
  // below). Every exit from the editing view sets it — a value picked and a
  // value abandoned both put the reader back where they started — because
  // what the rule is about is leaving the view, not which answer they left
  // with. Neither path can focus the trigger directly: at the moment they
  // run, `editing` is still true, so the resting button — only rendered in
  // the `!editing` branch — does not exist yet and `trigger.current` is null.
  const restoreFocus = useRef(false);
  const fieldId = useId();
  const errorId = useId();

  const close = () => {
    setEditing(false);
    setPending(null);
    setFailure(null);
  };

  const revert = () => {
    // The a11y counterpart of the value snapping back into plain text: a
    // keyboard user who opened this and backed out lands on the same trigger
    // they pressed, not dropped to the document body.
    restoreFocus.current = true;
    close();
  };

  // Runs after the resting trigger has actually mounted, so the focus call
  // lands on a live node instead of the null ref `revert` would have hit.
  useEffect(() => {
    if (!editing && restoreFocus.current) {
      restoreFocus.current = false;
      trigger.current?.focus();
    }
  }, [editing]);

  if (!canEdit || !editing) {
    return (
      <span>
        {!hideLabel && <>{label}: </>}
        <ChoiceReading
          canEdit={canEdit}
          label={label}
          value={value}
          render={render}
          readOnlyReason={readOnlyReason}
          triggerRef={trigger}
          onOpen={() => {
            setPending(value);
            setFailure(null);
            setEditing(true);
          }}
        />
      </span>
    );
  }

  const chosen = pending ?? value;
  const commit = async (next: string) => {
    // Choosing what is already set is not an edit. Sending it would write an
    // audit row for a change that did not happen.
    if (next === value) {
      restoreFocus.current = true;
      setEditing(false);
      return;
    }
    setSaving(true);
    setFailure(null);
    try {
      await onSave(next);
      // Picking a value leaves the editing view exactly as backing out of it
      // does, so it lands the reader on the same trigger: the rule is about
      // leaving the view at all, not about which answer they left with.
      restoreFocus.current = true;
      setEditing(false);
    } catch (err) {
      // The draft survives: `pending` still holds what they chose, and the
      // control stays open on it. A save that fails must not also lose the
      // answer the user gave.
      setFailure(problemMessageOf(err, t));
    } finally {
      setSaving(false);
    }
  };

  return (
    // Escape only reaches this handler once the popup itself is closed — an
    // OPEN popup's own keydown claims and stops the Escape press, which is
    // exactly the case (a picker left open on a failed save) that this
    // control has no other way to back out of.
    // biome-ignore lint/a11y/noStaticElementInteractions: keydown here only ever catches an Escape the Select below already declined to claim; the interactive element is that Select's own trigger.
    <span
      ref={container}
      className="inlinechoice-edit"
      onKeyDown={(event) => {
        if (event.key === "Escape") {
          revert();
        }
      }}
    >
      <label className={hideLabel ? "sr-only" : undefined} htmlFor={fieldId}>
        {label}
        {!hideLabel && ": "}
      </label>
      <Select
        id={fieldId}
        value={chosen}
        options={options}
        // Disabled AND busy, which are not the same claim. `disabled` is what
        // stops a second choice landing on top of a write that has not answered
        // yet; `aria-busy` is what says the control is working rather than
        // refused, and it is what the stylesheet keys the paint off — a write in
        // flight keeps its full ink and takes the waiting cursor, exactly as
        // Switch has since it was written.
        disabled={saving}
        aria-busy={saving || undefined}
        aria-invalid={failure ? true : undefined}
        aria-describedby={failure ? errorId : undefined}
        // The click that started editing already meant "show me the
        // options" — opening on mount spends that same click rather than
        // asking for a second one.
        openOnMount
        // Closing the popup without picking anything (a press outside, the
        // trigger scrolling away) is the one closed transition that is not
        // also a commit — Select's own `commit` never routes through this,
        // only `cancel`/an outside dismissal do. Tab is deliberately routed
        // to `onLeave`, not here: the reader already moved forward, and
        // refocusing this trigger would drag them back to where they left.
        onCancel={revert}
        onLeave={close}
        onChange={(next) => {
          setPending(next);
          void commit(next);
        }}
      />
      {saving && <BusyMark />}
      {failure && (
        <span id={errorId} role="alert" className="form-error">
          {failure}
        </span>
      )}
    </span>
  );
}

/**
 * The choice at rest: the value, as a trigger where the reader may change it
 * and as words where they may not.
 *
 * Its own component rather than a branch inside the control: the editing form
 * below carries the state, the guards and the write, and folding the resting
 * pair in beside them put four unrelated decisions in one function.
 */
function ChoiceReading({
  canEdit,
  label,
  value,
  render,
  readOnlyReason,
  triggerRef,
  onOpen,
}: Readonly<{
  canEdit: boolean;
  label: string;
  value: string;
  render: (value: string) => ReactNode;
  readOnlyReason?: string;
  triggerRef: RefObject<HTMLButtonElement | null>;
  onOpen: () => void;
}>) {
  const t = useT();
  // A choice nobody has made still says so. Rendered blank, the row was a
  // caret with nothing in front of it, and a reader could not tell an unset
  // owner from a value that failed to load.
  const shown = value ? render(value) : t("field.unset");
  if (!canEdit) {
    return (
      <span
        className={value ? undefined : "inlinechoice-unset"}
        title={readOnlyReason}
      >
        {shown}
      </span>
    );
  }
  return (
    <button
      ref={triggerRef}
      type="button"
      className="inline-editable inline-editable-choice"
      data-empty={!value}
      // aria-label, not title: the button's content is the VALUE, so without
      // this a screen reader announces "Not assessed, button", the state, with
      // no hint that pressing it changes anything. title does not override
      // content for the accessible name; aria-label does, and stays as the
      // tooltip for a pointer. Carried regardless of `hideLabel`: the visible
      // prefix is what a sighted reader does not need twice, not the
      // accessible name a screen reader needs at all.
      aria-label={t("inlineChoice.change", { field: label })}
      title={t("inlineChoice.change", { field: label })}
      onClick={onOpen}
    >
      {shown}
      <ChevronDown
        className="inline-editable-caret"
        size={12}
        aria-hidden="true"
      />
    </button>
  );
}
