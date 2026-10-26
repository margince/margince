import { LoaderCircle } from "lucide-react";
import { type ComponentPropsWithRef, type ReactNode, useId } from "react";
import { describedByIds } from "./describedby";
import { swallowWhileBusy, useSinglePress } from "./presslatch";

// `federated` is the door into another company's sign-in: full-width,
// unfilled, and carrying that company's own mark. It is a variant rather than a
// screen's own control because the alternative was tried. The sign-in surface
// hand-rolled a button with its own border, fill, radius, weight, padding,
// hover, focus and two dim states. A control that redeclares every one of a
// variant's properties has left the design system rather than reused it.
/**
 * How loud a control is, and on whose behalf it acts.
 *
 * `ai` is the one that is not a volume. It says the press starts a model: it
 * drafts the mail, reads the site, writes the summary. It is indigo everywhere
 * for the same reason the bands and the citation rules are. A reader learns
 * one colour for "Margince does this now" rather than one per surface. It never
 * marks importance: a destructive verb an agent performs is still danger.
 * Accepting a proposal is `primary` and only opening something is `ghost`.
 *
 * `link` is the quietest rung: the text affordance `.link-button` already draws
 * for an `<a>` or a hand-rolled `<button>`. A verb reaches it through this
 * component when it also needs what only this component gives: the refusal
 * contract and the `pending` one. It wears that same class rather than a look
 * of its own, so the two spellings of a link affordance cannot drift apart.
 * `iconOnly` says nothing here: the class has no fill, no width floor and no
 * control height for it to shrink.
 */
export type ButtonVariant =
  | "primary"
  | "ghost"
  | "danger"
  | "federated"
  | "ai"
  | "aiQuiet"
  | "link";

/**
 * The turning mark a control shows while a write it started is in flight.
 *
 * Exported because `Switch` carries the same state and must draw it the same
 * way. It is sized by whatever control hosts it (`.btn svg` draws
 * --controlIcon), so it takes no size prop. It is decorative: `aria-busy` on
 * the control is the fact, and a glyph that announced itself would say it
 * twice.
 */
export function BusyMark({ className }: Readonly<{ className?: string }>) {
  return (
    <LoaderCircle
      className={["busy-mark", className ?? ""].filter(Boolean).join(" ")}
      aria-hidden="true"
    />
  );
}

/**
 * `bare` is the catalog's `.iconbtn` reached through this component: no fill and
 * no edge, for a glyph among glyphs such as the top strip's. With no edge to
 * hold a word it compiles only as a square glyph button.
 */
type ButtonShape =
  | {
      variant: "bare";
      /** The whole label is the glyph; see the other arm. */
      iconOnly: true;
    }
  | {
      variant?: ButtonVariant;
      /**
       * This button's whole label is its icon, so it drops the width floor that
       * keeps a short word readable and becomes square. The caller still owes
       * it an accessible name (`aria-label` or a visually-hidden child),
       * because a glyph announces as nothing.
       */
      iconOnly?: boolean;
    };

type ButtonBase = ComponentPropsWithRef<"button"> & ButtonShape;

export function Button({
  variant = "ghost",
  iconOnly,
  className,
  reason,
  reasonId,
  unavailable,
  pending,
  busyLabel,
  disabled,
  ...rest
  // `ComponentPropsWithRef`, not the bare attribute set, for the same reason
  // `TextInput` takes it. A caller that has to move focus to this control
  // needs the node. So does one restoring focus here after a dialog its own
  // mutation removed the opener of. React 19 passes `ref` as an ordinary prop
  // to a function component, so this costs nothing but the type.
}: ButtonBase & {
  /**
   * Why this action is unavailable. Passing it disables the button and points
   * the control at the explanation with `aria-describedby`. No screen reader
   * announces a `title` on a disabled button, and a disabled button cannot be
   * focused. So a reason living only in `title` reaches nobody who needed it.
   * `Switch.reason` carries the same contract.
   *
   * STATE-4a decides when to use it. A control blocked by state rather than
   * permission (an archived record, a frozen setting) stays visible and says
   * why. The reason is the information, and it can change.
   */
  reason?: string;
  /**
   * The id of an element already on the page carrying that explanation, for
   * a surface where several controls are refused by one fact. Printing the
   * same sentence beside every control states it as many times as there are
   * buttons. Naming it once and pointing every control at it says it once
   * and still reaches a screen reader from each of them.
   */
  reasonId?: string;
  /**
   * A control the installation advertises and cannot complete.
   *
   * This is the resting refusal. What separates it from the other two is how
   * long it lasts. `disabled` is a precondition that clears, and the control
   * comes back. `pending` is a wait measured in seconds. This is a door drawn
   * because the installation offers it with nothing behind it yet. It refuses
   * the press by itself, the way `reason` does, so the dead treatment cannot
   * end up drawn over a live control.
   *
   * It carries no sentence of its own; a caller that has one passes `reason`
   * as well and gets both. The surface that needs this state may have nothing
   * it is allowed to say. The federated sign-in button is named by the
   * installation's own label, and a provider it advertises is not ours to
   * explain. So the drawing is the whole claim, which is why it is a deeper
   * fade than `disabled` rather than the same one.
   */
  unavailable?: boolean;
  /**
   * Whether a write this button started is still in flight.
   *
   * It does not set `disabled`, and that is the whole contract. Disabling the
   * control the reader has just pressed detaches focus (Chrome and Safari drop
   * it to `<body>`). The reader then loses their place at the moment the app
   * has something to tell them. An `aria-busy` announcing the wait lands on a
   * control nobody is on. `aria-disabled` refuses the second press while
   * keeping the button focusable. Focus stays where the reader put it, and the
   * state is announced from there. The press itself is swallowed in the
   * handler, since `aria-disabled` is a promise to assistive technology and
   * not something the browser enforces.
   *
   * The label does not change while this is set. The mark and `aria-busy`
   * already say "working". Swapping "Save" for "Saving…" says it a second time
   * and renames a focused control mid-press, which a screen reader re-reads.
   * So a caller passes one label and lets the state carry the rest.
   *
   * `reason` and `disabled` both outrank it. A control nobody may press cannot
   * also be mid-press, and drawing the mark there would claim a write nobody
   * started. That precedence is not cosmetic: a button carrying both
   * `disabled` and `pending` would be natively disabled. That drops the focus
   * this whole prop exists to keep, while it announces itself busy to a reader
   * who is no longer on it.
   */
  pending?: boolean;
  /**
   * What is happening, for a reader who cannot see the mark.
   *
   * `aria-busy` is the machine-readable state, and this makes no claim that it
   * is spoken. ARIA defines it as "the element is being modified, and assistive
   * technology may want to wait before exposing the change". That is
   * permission to defer, not an instruction to announce. Nothing obliges a
   * screen reader to say anything about it.
   *
   * So a screen that has something worth saying passes the sentence here. It
   * is added to `aria-describedby` while the write is out. Focus is on this
   * control, which is the premise of the whole prop. A description arriving on
   * the focused element is announced, where a changed name would make the
   * reader re-hear the button itself. Copy arrives translated, like everything
   * else here.
   *
   * Optional because most buttons have nothing to add beyond "working". Not on
   * `Switch` yet: no surface has needed it there, and a prop with no caller is
   * an API nobody has tested.
   */
  busyLabel?: string;
}) {
  const ownReasonId = useId();
  const busyLabelId = useId();
  const singlePress = useSinglePress(pending);
  const classes = [
    "btn",
    `btn-${variant}`,
    iconOnly ? "btn-icon" : "",
    unavailable ? "btn-unavailable" : "",
    className ?? "",
  ]
    .filter(Boolean)
    .join(" ");
  const refused = reason !== undefined || reasonId !== undefined;
  // Every way this control can be barred, in one value, because `disabled` and
  // `busy` both have to agree about it. Refusal beats busy in all its
  // spellings, `disabled` included. Otherwise a caller passing both gets a
  // natively disabled button with focus gone. It announces itself busy and
  // draws the dimmed refused chrome with a spinner turning inside it, the
  // failure `pending` exists to prevent. `unavailable` joins them for the same
  // reason: a door with nothing behind it cannot also be mid-press. `Switch`
  // reads the same way.
  const barred = refused || disabled === true || unavailable === true;
  const busy = pending === true && !barred;
  // Everything this component computes for itself is destructured out of
  // `rest`, so a caller's props cannot land on top of it. That is not tidiness.
  // A `disabled={false}` passed alongside `reason` would re-enable a control
  // the reason contract promises is refused. A caller's `aria-describedby`
  // would drop the pointer to the sentence saying why. An `onClick` surviving
  // `pending` would let a second press through a button that is already
  // writing. `aria-busy` and `aria-disabled` join them because this component
  // owns that state. A caller setting either by hand describes something
  // Button already describes, and the two can only disagree.
  const {
    "aria-describedby": callerDescribedBy,
    "aria-busy": _callerBusy,
    "aria-disabled": _callerAriaDisabled,
    onClick,
    children,
    ...attrs
  } = rest;
  const describedBy = describedByFor({
    callerDescribedBy,
    reason,
    reasonId,
    ownReasonId,
    busyLabelId: busy && busyLabel !== undefined ? busyLabelId : undefined,
  });
  const button = (
    <button
      type="button"
      {...attrs}
      className={classes}
      disabled={barred}
      aria-disabled={busy || undefined}
      aria-busy={busy || undefined}
      aria-describedby={describedBy}
      onClick={busy ? swallowWhileBusy : singlePress(onClick)}
    >
      {busy && <BusyMark />}
      {/* The children stay, always. An icon-only control has no room for two
          16px marks side by side, so the glyph is hidden, but in CSS, by the
          `.btn-icon` rule for a busy button. Dropping the children here would also
          drop the visually-hidden text that `iconOnly` documents as one of
          the two ways to name such a button. That would leave a focusable
          control with no accessible name at all. */}
      {children}
    </button>
  );
  return (
    <ButtonSentences
      reason={reason}
      reasonId={ownReasonId}
      busyLabel={busyLabel}
      busyLabelId={busyLabelId}
      busy={busy}
    >
      {button}
    </ButtonSentences>
  );
}

/**
 * What a Button's `aria-describedby` points at. It lives in one place because
 * three sources compete for it, and the precedence between them is the
 * contract. `reasonId` names an element the page already owns. `reason`
 * renders its own and outranks whatever the caller passed. A caller's own
 * description survives only when nothing is refused. `busyLabel` is additive:
 * a refused control is never busy, so in practice it joins a caller's
 * description or stands alone.
 */
function describedByFor({
  callerDescribedBy,
  reason,
  reasonId,
  ownReasonId,
  busyLabelId,
}: Readonly<{
  callerDescribedBy?: string;
  reason?: string;
  reasonId?: string;
  ownReasonId: string;
  busyLabelId?: string;
}>): string | undefined {
  const refusal =
    reasonId ?? (reason === undefined ? callerDescribedBy : ownReasonId);
  return describedByIds(refusal, busyLabelId);
}

/**
 * The sentences that belong to a button but may not live inside it.
 *
 * Anything rendered within a `<button>` joins its accessible name. A
 * description of the wait placed there would rename the control mid-press,
 * which is what holding the label steady prevents. Both sentences are
 * siblings instead. The wrapper is `display: contents` when there is nothing
 * visible to stack, so a button that opts into `busyLabel` lays out as it did
 * before.
 */
function ButtonSentences({
  reason,
  reasonId,
  busyLabel,
  busyLabelId,
  busy,
  children,
}: Readonly<{
  reason?: string;
  reasonId: string;
  busyLabel?: string;
  busyLabelId: string;
  busy: boolean;
  children: ReactNode;
}>) {
  if (reason === undefined && busyLabel === undefined) {
    return children;
  }
  return (
    <span className={reason === undefined ? "btn-shell" : "btn-with-reason"}>
      {children}
      {/* `t-caption` is the supporting line's role: one size and one ink for
          every refusal, and the class atoms.css's dialog rule selects. */}
      {reason !== undefined && (
        <span id={reasonId} className="t-caption">
          {reason}
        </span>
      )}
      {/* Rendered whether or not the write is out, and emptied rather than
          removed. A description that arrives together with the element holding
          it is often missed. One that is already there and changes is what a
          screen reader on the focused control reads. */}
      {busyLabel !== undefined && (
        <span id={busyLabelId} className="sr-only">
          {busy ? busyLabel : ""}
        </span>
      )}
    </span>
  );
}
