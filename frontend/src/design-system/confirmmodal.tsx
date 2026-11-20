import type { KeyboardEvent, MouseEvent, ReactNode, Ref } from "react";
import { Children, useId, useRef } from "react";
import { flushSync } from "react-dom";
import { useT } from "../i18n";
import { isParagraphCommit } from "../lib/fieldcommit";
import { isImeCommit } from "../lib/imecommit";
import { Button, Modal } from "./atoms";
import { ErrorLine } from "./errorline";
import { refuseUnreadableField } from "./formvalidity";
import { Heading } from "./heading";
import type { ModalIntent } from "./modal";
import { AutonomyDot } from "./trust";

type DotTier = Parameters<typeof AutonomyDot>[0]["tier"];

const LINE_TYPES = new Set([
  "text",
  "search",
  "email",
  "url",
  "tel",
  "password",
  "number",
]);

// A field in a form of its own submits that form. A combobox's Enter picks a
// row. A popup portalled out of the body holds no field of this dialog.
function isLineCommit(event: KeyboardEvent<HTMLDivElement>): boolean {
  const field = event.target;
  return (
    field instanceof HTMLInputElement &&
    LINE_TYPES.has(field.type) &&
    event.key === "Enter" &&
    !(event.altKey || event.ctrlKey || event.metaKey || event.shiftKey) &&
    !event.defaultPrevented &&
    field.form === null &&
    field.getAttribute("role") !== "combobox" &&
    !field.hasAttribute("aria-autocomplete") &&
    event.currentTarget.contains(field)
  );
}

// The body and its fields are the caller's; the dialog owns their spacing.

export function ConfirmModal({
  open,
  onClose,
  title,
  tier,
  confirmLabel,
  cancelLabel,
  confirmVariant = "primary",
  confirmDisabled = false,
  actionsLead,
  confirmMenu,
  confirmReason,
  onConfirm,
  pending,
  error,
  intent = "confirm",
  enterConfirms = intent === "form",
  returnFocusTo,
  initialFocusTo,
  unsaved,
  unsavedBody,
  bodyRef,
  children,
}: Readonly<{
  open: boolean;
  onClose: () => void;
  title: string;
  tier?: DotTier;
  confirmLabel: string;
  // The way back, named for what it keeps ("Keep it on") where a bare Cancel
  // would not say what staying put means. Defaults to Cancel.
  cancelLabel?: string;
  // The confirm button's tone. Defaults to "primary" (backward-compatible);
  // a destructive confirm (e.g. reject-with-reason) passes "danger" so it
  // doesn't read green like an approve.
  confirmVariant?: "primary" | "danger";
  // Lets the caller gate its own precondition (e.g. a typed-confirmation
  // input in children) without teaching this atom what the action means.
  // Defaults false. It is NOT where a caller puts its in-flight state: that is
  // `pending`, and folding the two together here gives the confirm both a
  // native `disabled` and a busy announcement — a control that has lost focus
  // telling a reader who is no longer on it that their write is going.
  confirmDisabled?: boolean;
  // A verb that belongs to the dialog but is not its confirm and not its
  // cancel — discarding a draft the machine wrote, where cancel closes the
  // dialog and this throws the words away. It sits at the START of the action
  // row, away from the two controls on the right, because it is neither of
  // them and a third button beside them would be pressed as one.
  actionsLead?: ReactNode;
  // A control seated against the confirm button's trailing edge, sharing its
  // fill — the caret half of a split button. What it opens is the caller's:
  // this dialog only promises the seam is drawn as one control.
  confirmMenu?: ReactNode;
  // Why the confirm is refused, when the reason is a sentence rather than a
  // state. `Button`'s own `reason` does the work — it disables the control,
  // prints the sentence beside it and points `aria-describedby` at it — which
  // `confirmDisabled` alone cannot: a dialog whose confirm is dead with no
  // explanation is a dead end, and a `title` on a disabled button reaches
  // nobody. Pass this instead of `confirmDisabled`, not as well.
  confirmReason?: string;
  onConfirm: () => void;
  pending?: boolean;
  error?: string | null;
  intent?: Exclude<ModalIntent, "full">;
  /** Enter in a single-line field presses Confirm; on by default in the form shape. */
  enterConfirms?: boolean;
  // Passed through to Modal. A confirm whose action destroys its own trigger —
  // deactivating a member, ending a connection, closing a request — names the
  // place focus should land instead, since the trigger will not be there to
  // take it back.
  returnFocusTo?: () => HTMLElement | null;
  /** The writing field can take focus before supporting context controls. */
  initialFocusTo?: () => HTMLElement | null;
  /** The caller holds a draft, so a close other than the confirm asks first. */
  unsaved?: boolean;
  unsavedBody?: string;
  /** The body around the fields, where a form looks for its first refusal. */
  bodyRef?: Ref<HTMLDivElement>;
  children: ReactNode;
}>) {
  const t = useT();
  const titleId = useId();
  const body = Children.toArray(children).length > 0 || Boolean(error);
  const confirm = (event: MouseEvent<HTMLButtonElement>) => {
    if (!refuseUnreadableField(event.currentTarget)) onConfirm();
  };
  const confirmButton = useRef<HTMLButtonElement>(null);
  // Enter is a new line in a note, so ⌘/Ctrl+Enter there presses Confirm.
  const confirmFromKey = (event: KeyboardEvent<HTMLDivElement>) => {
    const commit =
      event.target instanceof HTMLTextAreaElement
        ? isParagraphCommit(event)
        : enterConfirms && isLineCommit(event);
    if (!commit || isImeCommit(event)) {
      return;
    }
    event.preventDefault();
    // A field that settles on Enter (DateInput) hands up its value in this
    // keystroke. The press waits for that render, and refuses as a click does.
    flushSync(() => undefined);
    confirmButton.current?.click();
  };
  return (
    <Modal
      open={open}
      onClose={onClose}
      // The title alone names the dialog; the dot's word stays in the heading.
      labelledBy={titleId}
      intent={intent}
      closeDisabled={pending}
      returnFocusTo={returnFocusTo}
      initialFocusTo={initialFocusTo}
      unsaved={unsaved}
      unsavedBody={unsavedBody}
    >
      <Heading size="large" className="t-h2 modal-title">
        {tier && (
          <>
            <AutonomyDot tier={tier} />{" "}
          </>
        )}
        <span id={titleId}>{title}</span>
      </Heading>
      {body && (
        // biome-ignore lint/a11y/noStaticElementInteractions: not a control — it hears the Enter that commits a field inside it; Confirm stays the control
        <div ref={bodyRef} className="form-stack" onKeyDown={confirmFromKey}>
          {children}
          {error && <ErrorLine>{error}</ErrorLine>}
        </div>
      )}
      <div className="actions">
        {actionsLead && <span className="actions-lead">{actionsLead}</span>}
        {/* Cancel is `disabled`, not `pending`, and the difference is real: it
            is not the control that started the write, so it is genuinely
            unavailable rather than busy. Backing out of an act that is already
            on its way to the server would leave the reader believing they
            stopped something they did not. */}
        <span className="actions-pair">
          <Button onClick={onClose} disabled={pending}>
            {cancelLabel ?? t("create.cancel")}
          </Button>
          {/* The compound this dialog used to carry — `pending || confirmDisabled`
            — folded two unrelated facts into one attribute: a write in flight
            and a precondition the caller has not met. Split, each is drawn as
            what it is, and the twenty-eight surfaces built on this dialog get
            it without changing a line. */}
          {confirmMenu ? (
            <span className="actions-split">
              <Button
                ref={confirmButton}
                variant={confirmVariant}
                onClick={confirm}
                pending={pending}
                disabled={confirmDisabled}
                reason={confirmReason}
              >
                {confirmLabel}
              </Button>
              {confirmMenu}
            </span>
          ) : (
            <Button
              ref={confirmButton}
              variant={confirmVariant}
              onClick={confirm}
              pending={pending}
              disabled={confirmDisabled}
              reason={confirmReason}
            >
              {confirmLabel}
            </Button>
          )}
        </span>
      </div>
    </Modal>
  );
}
