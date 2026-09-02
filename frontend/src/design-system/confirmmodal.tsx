import type { ReactNode } from "react";
import { useId } from "react";
import { useT } from "../i18n";
import { Button, Modal } from "./atoms";
import { AutonomyDot } from "./trust";

// The shared confirm-dialog chrome: this used to live duplicated,
// near-identically, inline in the deals.tsx terminal-stage
// advance confirm and archive.tsx's ArchiveAction. Both wire a Modal, a
// title (deals.tsx's carries an autonomy dot, archive.tsx's doesn't), an
// inline mutation error, and a Cancel/Confirm pair that both refuse the press
// while a mutation is in flight — Cancel by going unavailable, Confirm by
// going busy, since only one of them started anything. The caller owns the body copy and any extra
// fields (e.g. the lost-reason input) via children — this atom only owns
// the modal chrome and the actions.

export function ConfirmModal({
  open,
  onClose,
  title,
  tier,
  confirmLabel,
  confirmVariant = "primary",
  confirmDisabled = false,
  actionsLead,
  confirmMenu,
  confirmReason,
  onConfirm,
  pending,
  error,
  size,
  placement,
  returnFocusTo,
  children,
}: Readonly<{
  open: boolean;
  onClose: () => void;
  title: string;
  tier?: "confirm";
  confirmLabel: string;
  // Passed through to Modal. A confirm whose body is a form the user has to
  // READ before an irreversible act — an email about to leave — needs more
  // than the compact width every yes/no confirm uses. "split" is the two-column
  // drawer: the reply beside the conversation it answers.
  size?: "default" | "wide" | "split";
  // Passed through to Modal. "right" is the drawer form: the record the
  // confirm is about stays visible beside it as context, which a centred box
  // covers. The composer uses it so a rep can read the account while writing
  // to it.
  placement?: "center" | "right";
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
  // Passed through to Modal. A confirm whose action destroys its own trigger —
  // deactivating a member, ending a connection, closing a request — names the
  // place focus should land instead, since the trigger will not be there to
  // take it back.
  returnFocusTo?: () => HTMLElement | null;
  children: ReactNode;
}>) {
  const t = useT();
  const headingId = useId();
  return (
    <Modal
      open={open}
      onClose={onClose}
      labelledBy={headingId}
      size={size}
      placement={placement}
      returnFocusTo={returnFocusTo}
    >
      <h2 id={headingId} className="t-h2" style={{ marginBottom: 12 }}>
        {tier && (
          <>
            <AutonomyDot tier={tier} />{" "}
          </>
        )}
        {title}
      </h2>
      {children}
      {error && (
        // role="alert" (assertive live region) so a screen reader announces the
        // mutation failure when it appears — e.g. a rejected reset confirmation.
        <p
          className="t-caption"
          role="alert"
          style={{ color: "var(--danger)" }}
        >
          {error}
        </p>
      )}
      <div className="actions">
        {actionsLead && <span className="actions-lead">{actionsLead}</span>}
        {/* Cancel is `disabled`, not `pending`, and the difference is real: it
            is not the control that started the write, so it is genuinely
            unavailable rather than busy. Backing out of an act that is already
            on its way to the server would leave the reader believing they
            stopped something they did not. */}
        <Button onClick={onClose} disabled={pending}>
          {t("create.cancel")}
        </Button>
        {/* The compound this dialog used to carry — `pending || confirmDisabled`
            — folded two unrelated facts into one attribute: a write in flight
            and a precondition the caller has not met. Split, each is drawn as
            what it is, and the twenty-eight surfaces built on this dialog get
            it without changing a line. */}
        {confirmMenu ? (
          <span className="actions-split">
            <Button
              variant={confirmVariant}
              onClick={onConfirm}
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
            variant={confirmVariant}
            onClick={onConfirm}
            pending={pending}
            disabled={confirmDisabled}
            reason={confirmReason}
          >
            {confirmLabel}
          </Button>
        )}
      </div>
    </Modal>
  );
}
