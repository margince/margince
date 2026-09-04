import {
  ChevronRight,
  LoaderCircle,
  MoreHorizontal,
  Search,
} from "lucide-react";
import {
  type ComponentPropsWithRef,
  type CSSProperties,
  type ElementType,
  type FormEventHandler,
  type InputHTMLAttributes,
  type MouseEvent as ReactMouseEvent,
  type ReactNode,
  type RefObject,
  useEffect,
  useId,
  useLayoutEffect,
  useRef,
  useState,
} from "react";
import { createPortal } from "react-dom";
import { formatNumber } from "../format/format";
import { useLocale } from "../i18n";
import { useAnchoredToTrigger } from "./anchored";
import { useDialogFocus } from "./dialogfocus";
import { Popover } from "./popover";
import "./atoms.css";

// The Margince atom library (B-EP09.2, re-scoped to our own
// system, no gw-ui port; atoms are added as screens need them). Copy always
// arrives through props — callers translate with t(); atoms never hard-code
// user-facing words.

// `federated` is the door into another company's sign-in: full-width,
// unfilled, and carrying that company's own mark. It is a variant rather than a
// screen's own control because the alternative was tried — the sign-in surface
// hand-rolled a button with its own border, fill, radius, weight, padding,
// hover, focus and two dim states, and a control that redeclares every one of a
// variant's properties has left the design system rather than reused it.
/**
 * How loud a control is, and on whose behalf it acts.
 *
 * `ai` is the one that is not a volume: it says a MACHINE does the work behind
 * the click — drafts the mail, reads the site, writes the summary. It is
 * indigo everywhere for the same reason the bands and the citation rules are,
 * so a reader learns one colour for "Margince did this" rather than one per
 * surface, and it never marks importance: a destructive verb an agent performs
 * is still danger.
 */
export type ButtonVariant =
  | "primary"
  | "ghost"
  | "danger"
  | "federated"
  | "ai"
  | "aiQuiet";

/**
 * The turning mark a control shows while a write it started is in flight.
 *
 * Exported because `Switch` carries the same state and must draw it the same
 * way; sized by whatever control hosts it (`.btn svg` is 16px, 14px small), so
 * it takes no size prop. Decorative — `aria-busy` on the control is the fact,
 * and a glyph that announced itself would say it twice.
 */
export function BusyMark({ className }: Readonly<{ className?: string }>) {
  return (
    <LoaderCircle
      className={["busy-mark", className ?? ""].filter(Boolean).join(" ")}
      aria-hidden="true"
    />
  );
}

// A press that lands on a control already waiting for its own answer. Both
// halves are load bearing: `preventDefault` is what stops a `type="submit"`
// button posting the form a second time (a plain early return does not — the
// browser submits on the click, not on the handler), and `stopPropagation`
// stops a clickable row underneath treating the press as a click on itself.
//
// Aliased on import because this file also uses the DOM's own `MouseEvent`,
// for the document-level listener `OverflowMenu` attaches; the unaliased React
// type shadows it and that listener stops compiling.
function swallowWhileBusy(event: ReactMouseEvent<HTMLButtonElement>) {
  event.preventDefault();
  event.stopPropagation();
}

export function Button({
  variant = "ghost",
  small,
  iconOnly,
  className,
  reason,
  reasonId,
  unavailable,
  pending,
  busyLabel,
  disabled,
  ...rest
  // `ComponentPropsWithRef`, not the bare attribute set — the same reason
  // `TextInput` takes it: a caller that has to move focus to this control, or
  // restore it here after a dialog its own mutation removed the opener of,
  // needs the node. React 19 passes `ref` as an ordinary prop to a function
  // component, so this costs nothing but the type.
}: ComponentPropsWithRef<"button"> & {
  variant?: ButtonVariant;
  small?: boolean;
  /**
   * This button's whole label IS its icon, so it drops the width floor that
   * keeps a short word readable and becomes square. The caller still owes it an
   * accessible name — `aria-label` or a visually-hidden child — because a
   * glyph announces as nothing.
   */
  iconOnly?: boolean;
  /**
   * Why this action is unavailable. Passing it DISABLES the button and points
   * the control at the explanation with `aria-describedby` — a `title` on a
   * disabled button is announced by no screen reader, and a disabled button
   * cannot be focused, so a reason living only in `title` reaches nobody who
   * needed it. `Switch.reason` carries the same contract.
   *
   * STATE-4a decides WHEN to use it: a control blocked by state rather than
   * permission — an archived record, a frozen setting — stays visible and
   * says why, because the reason is the information and it can change.
   */
  reason?: string;
  /**
   * The id of an element ALREADY on the page carrying that explanation, for
   * a surface where several controls are refused by ONE fact. Printing the
   * same sentence beside every control states it as many times as there are
   * buttons; naming it once and pointing every control at it says it once
   * and still reaches a screen reader from each of them.
   */
  reasonId?: string;
  /**
   * A control the installation ADVERTISES and cannot complete.
   *
   * The RESTING refusal, and what separates it from the other two is how long
   * it lasts: `disabled` is a precondition that clears and the control comes
   * back, `pending` is a wait measured in seconds, and this is a door drawn
   * because the installation offers it with nothing behind it yet. It refuses
   * the press by itself, the way `reason` does, so the dead treatment cannot
   * end up drawn over a live control.
   *
   * It carries no sentence of its own — a caller that has one passes `reason`
   * as well and gets both — because the surface that needs this state may have
   * nothing it is allowed to say: the federated sign-in button is named by the
   * installation's own label, and a provider it advertises is not ours to
   * explain. So the drawing IS the whole claim, which is why it is a deeper
   * fade than `disabled` rather than the same one.
   */
  unavailable?: boolean;
  /**
   * Whether a write this button started is still in flight.
   *
   * It deliberately does NOT set `disabled`, and that is the whole contract.
   * Disabling the control the reader has just pressed detaches focus — Chrome
   * and Safari drop it to `<body>` — so the reader loses their place at the
   * exact moment the app has something to tell them, and an `aria-busy`
   * announcing the wait lands on a control nobody is on. `aria-disabled`
   * refuses the second press while keeping the button focusable, so focus
   * stays where the reader put it and the state is announced from there. The
   * press itself is swallowed in the handler, since `aria-disabled` is a
   * promise to assistive technology and not something the browser enforces.
   *
   * The LABEL does not change while this is set. The mark and `aria-busy`
   * already say "working"; swapping "Save" for "Saving…" says it a second time
   * and renames a focused control mid-press, which a screen reader re-reads —
   * so a caller passes one label and lets the state carry the rest.
   *
   * `reason` and `disabled` both outrank it: a control nobody may press cannot
   * also be mid-press, and drawing the mark there would claim a write nobody
   * started. That precedence is not cosmetic — a button carrying `disabled`
   * AND `pending` would be natively disabled, which drops the focus this whole
   * prop exists to keep, while announcing itself busy to a reader who is no
   * longer on it.
   */
  pending?: boolean;
  /**
   * What is happening, for a reader who cannot see the mark.
   *
   * `aria-busy` is the honest machine-readable state and this is deliberately
   * not a claim that it is spoken: ARIA defines it as "the element is being
   * modified, and assistive technology may want to wait before exposing the
   * change" — permission to DEFER, not an instruction to announce. Nothing
   * obliges a screen reader to say anything about it.
   *
   * So a screen that had something worth saying passes the sentence here and it
   * is added to `aria-describedby` while the write is out. Focus is on this
   * control — that is the premise of the whole prop — and a description arriving
   * on the focused element is announced, where a changed NAME would make the
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
  const classes = [
    "btn",
    `btn-${variant}`,
    small ? "btn-sm" : "",
    iconOnly ? "btn-icon" : "",
    unavailable ? "btn-unavailable" : "",
    className ?? "",
  ]
    .filter(Boolean)
    .join(" ");
  const refused = reason !== undefined || reasonId !== undefined;
  // Every way this control can be barred, in one value, because `disabled` and
  // `busy` both have to agree about it. Refusal beats busy in ALL its
  // spellings: `disabled` used to be missing from this test, and the result was
  // the exact failure `pending` exists to prevent — a caller passing both got a
  // natively disabled button, focus gone, that announced itself busy and drew
  // the dimmed refused chrome with a spinner turning inside it. `unavailable`
  // joins them for the same reason: a door with nothing behind it cannot also
  // be mid-press. `Switch` reads the same way.
  const barred = refused || disabled === true || unavailable === true;
  const busy = pending === true && !barred;
  // Everything this component computes for itself is destructured out of
  // `rest`, so a caller's props cannot land on top of it. That is not tidiness:
  // a `disabled={false}` passed alongside `reason` re-enabled a control the
  // reason contract promises is refused; a caller's `aria-describedby` dropped
  // the pointer to the sentence saying why; an `onClick` surviving `pending`
  // let a second press through a button that is already writing. `aria-busy`
  // and `aria-disabled` join them because this component owns that state now —
  // a caller setting either by hand is describing something Button is already
  // describing, and the two can only disagree.
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
      onClick={busy ? swallowWhileBusy : onClick}
    >
      {busy && <BusyMark />}
      {/* The children stay, ALWAYS. An icon-only control has no room for two
          16px marks side by side, so the glyph is hidden — but in CSS, by
          `.btn-icon[aria-busy="true"]`, because dropping the children here also
          dropped the visually-hidden text that `iconOnly` documents as one of
          the two ways to name such a button, leaving a focusable control with
          no accessible name at all. */}
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
 * What a Button's `aria-describedby` points at, in one place because three
 * sources compete for it and the precedence between them is the contract:
 * `reasonId` names an element the page already owns, `reason` renders its own
 * and outranks whatever the caller passed, and a caller's own description
 * survives only when nothing is refused. `busyLabel` is additive — a refused
 * control is never busy, so in practice it joins a caller's description or
 * stands alone.
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
  return [refusal, busyLabelId].filter(Boolean).join(" ") || undefined;
}

/**
 * The sentences that belong to a button but may not live inside it.
 *
 * Anything rendered within a `<button>` joins its accessible NAME, so a
 * description of the wait placed there would rename the control mid-press —
 * the exact thing holding the label steady was for. Both sentences are
 * siblings instead, and the wrapper is `display: contents` when there is
 * nothing visible to stack, so a button that opts into `busyLabel` lays out
 * exactly as it did before.
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
      {reason !== undefined && (
        <span id={reasonId} className="t-caption">
          {reason}
        </span>
      )}
      {/* Rendered whether or not the write is out, and emptied rather than
          removed. A description that arrives together with the element holding
          it is frequently missed; one that is already there and CHANGES is what
          a screen reader on the focused control actually reads. */}
      {busyLabel !== undefined && (
        <span id={busyLabelId} className="sr-only">
          {busy ? busyLabel : ""}
        </span>
      )}
    </span>
  );
}

export function Badge({
  tone,
  children,
  quiet,
}: Readonly<{
  tone?: "success" | "warn" | "danger" | "ai" | "accent";
  children: ReactNode;
  // The same status in a column of them. A pill states one status against
  // surrounding prose; a table row states one per row, and a stack of filled
  // pills reads as decoration a reader learns to skip. `quiet` keeps the tone
  // and drops the fill: a dot in the tone's colour, and the label as plain
  // text. Same vocabulary, so a status cannot be worded one way in a list and
  // another on the record the list opens.
  quiet?: boolean;
}>) {
  const classes = ["badge"];
  if (quiet) {
    classes.push("badge-quiet");
  }
  if (tone) {
    classes.push(`badge-${tone}`);
  }
  return <span className={classes.join(" ")}>{children}</span>;
}

// AVATAR_TONES are the monogram backgrounds, all token-driven. The colour
// is picked from the record, not stored, so the same record looks the same on
// every screen and in every session without a round trip.
const AVATAR_TONES = 6;

/**
 * The initials a chip falls back to.
 *
 * Split on whitespace AND on the punctuation an address uses, because the
 * signed-in reader is frequently known to the product only by their address:
 * `jane.doe@example.com` reads as "JD" here, where a whitespace-only split
 * gives the single letter "J" and every colleague whose address starts with a
 * J gets the same chip. Two letters at most — a third stops being a monogram
 * and starts being text set too small to read.
 */
function monogramOf(name: string): string {
  return name
    .split(/[\s@._-]+/)
    .filter(Boolean)
    .slice(0, 2)
    .map((part) => [...part][0]?.toUpperCase() ?? "")
    .join("");
}

export function Avatar({
  name,
  identity,
  src,
  size = "sm",
  shape = "person",
}: Readonly<{
  name: string;
  /**
   * What the tint is derived FROM, when that is not the displayed name — a
   * record id, an address, anything stable for the life of the record. The
   * name is the fallback and it is a poor key: renaming a person or a company
   * silently moves them to a different colour on every screen at once, which
   * reads as a different record rather than as a rename.
   */
  identity?: string;
  // A resolved logo to render instead of the monogram. The monogram is the
  // floor, not the fallback of last resort: it is what shows while the image
  // loads, if it fails to load, and whenever no logo resolved — so a company
  // is never a broken image or an empty slot.
  src?: string | null;
  /**
   * The four sizes this chip is drawn at, which used to be four numbers in
   * four different stylesheets for a prop that admitted two: `xs` in a dense
   * table, `sm` in every list row and beside every name, `md` on a record
   * header, `lg` on a wide one.
   */
  size?: "xs" | "sm" | "md" | "lg" | "xl";
  /**
   * What KIND of thing this chip stands for, which decides its shape.
   *
   * A person is round, the way a face is drawn everywhere; an organization is
   * a rounded square, the way a logo is. The distinction is not decoration —
   * on a page carrying both, the shape is what tells a reader whether a chip
   * is a company or somebody at it before they have read a word of it.
   */
  shape?: "person" | "organization";
}>) {
  // An image that fails to load falls back to the monogram for the rest of
  // this mount. Keyed by src so a record whose logo changes gets a fresh try
  // rather than inheriting the previous one's failure.
  const [brokenSrc, setBrokenSrc] = useState<string | null>(null);
  const broken = Boolean(src) && brokenSrc === src;
  const setBroken = () => setBrokenSrc(src ?? null);
  // The monogram is the floor UNDER the mark, so it has to stop being drawn the
  // moment the mark is actually on screen: a logo with transparency would
  // otherwise show the initials through it. Tracked by src for the same reason
  // as the failure above.
  const [paintedSrc, setPaintedSrc] = useState<string | null>(null);
  // A mark that painted once and then failed on a later load is no longer on
  // screen, so it stops holding the monogram down: without the `!broken` the
  // image is removed while the fallback stays suppressed, and the avatar is
  // simply empty.
  const painted = Boolean(src) && paintedSrc === src && !broken;
  const initials = monogramOf(name);
  // A small sum over the code points: stable across sessions and locales, and
  // the spread only has to be even enough that neighbouring records in a list
  // rarely collide.
  //
  // The tint is UNCONDITIONAL. It used to be opt-in, and the result was that a
  // company was tinted in the list it was found in and a neutral accent chip on
  // the record page that list opened — the same company, two colours, one
  // click apart. A chip that identifies a record on one screen and not on the
  // next identifies nothing.
  let tone = 0;
  for (const char of identity ?? name) {
    tone = (tone + (char.codePointAt(0) ?? 0)) % AVATAR_TONES;
  }
  const classes = ["avatar", `avatar-t${tone}`, `avatar-${size}`];
  if (shape === "organization") classes.push("avatar-org");
  if (src && !broken) classes.push("avatar-has-logo");
  if (painted) classes.push("avatar-painted");
  return (
    <span className={classes.join(" ")}>
      {src && !broken ? (
        // The monogram stays underneath: it is what the chip shows until the
        // image paints, and what is left if the image never does.
        <img
          className="avatar-img"
          src={src}
          alt=""
          loading="lazy"
          onError={setBroken}
          onLoad={() => setPaintedSrc(src ?? null)}
        />
      ) : null}
      {!painted && initials}
    </span>
  );
}

// `ComponentPropsWithRef`, not the bare attribute set: a caller that opens
// this field itself — an edit-in-place value that has to put the caret where
// the reader just clicked — needs the node, and React 19 passes `ref` as an
// ordinary prop to a function component.
export function TextInput(props: ComponentPropsWithRef<"input">) {
  return (
    <input {...props} className={`input ${props.className ?? ""}`.trim()} />
  );
}

/**
 * The one search field: a text input that announces itself as a search box and
 * carries the magnifier.
 *
 * `flush` is for a field whose CONTAINER already draws the chrome — the ⌘K
 * palette's own bar, which has its own ground, its own inset and its own
 * bottom rule. Nested, the ordinary field's border and radius read as a box
 * inside a box, and the field's own padding pushes the caret off the text
 * column the results below it stand on. It is a variant rather than a caller
 * overriding `.input` from outside, because a call site that reaches in to
 * cancel a primitive's chrome is how the next surface grows a second search
 * field nobody can find.
 */
export function SearchField({
  flush,
  ...props
}: InputHTMLAttributes<HTMLInputElement> & Readonly<{ flush?: boolean }>) {
  return (
    <span className={flush ? "input-icon input-icon-flush" : "input-icon"}>
      <Search aria-hidden />
      <input
        type="search"
        {...props}
        className={`input ${props.className ?? ""}`.trim()}
      />
    </span>
  );
}

/**
 * Textarea carries no label of its own, exactly like TextInput: the label is
 * composed outside it, by the `.field` wrapper a form uses or by a screen's own
 * richer shell. What it owns is the ONE spelling of the control's surface, so a
 * note field in a create form and one in settings cannot drift.
 *
 * The dropdown is NOT here: `Select` in select.tsx is a button and a portalled
 * listbox, because a native `<select>` draws its own option list in the
 * platform's idiom and no CSS reaches inside it. It still reads `.input` for its
 * closed face — a dropdown and a text input are the same field on screen.
 *
 * `ComponentPropsWithRef`, for the same reason `TextInput` takes it: a caller
 * that has to move focus HERE needs the node, and a dialog whose fields are
 * mostly prose has no single-line input to land on instead.
 */
export function Textarea(props: ComponentPropsWithRef<"textarea">) {
  return (
    <textarea
      {...props}
      className={`textarea ${props.className ?? ""}`.trim()}
    />
  );
}

/**
 * Checkbox and Radio DO carry their label, and that is the difference from the
 * fields above: for a tick the label is not a caption sitting nearby, it is the
 * other half of the click target. Wrapping the input is what makes the words
 * clickable and what gives the control its accessible name without an `id` to
 * thread — which is why seventeen of the twenty hand-rolled sites already wrote
 * this shape, each with its own wrapper class and its own idea of the gap.
 *
 * `label` is a ReactNode, not a string: a consent line carries emphasis and a
 * settings toggle carries a help line under the name.
 *
 * `className` lands on the LABEL, not the input, because that is where every
 * existing call site puts its layout — a row that needs `align-items:flex-start`
 * for a two-line label says so there.
 */
type ToggleProps = Omit<InputHTMLAttributes<HTMLInputElement>, "type"> & {
  label: ReactNode;
};

function Toggle({
  kind,
  label,
  className,
  ...rest
}: ToggleProps & { kind: "checkbox" | "radio" }) {
  return (
    <label
      className={["checkfield", className ?? ""].filter(Boolean).join(" ")}
    >
      <input type={kind} {...rest} />
      <span>{label}</span>
    </label>
  );
}

export function Checkbox(props: ToggleProps) {
  return <Toggle kind="checkbox" {...props} />;
}

export function Radio(props: ToggleProps) {
  return <Toggle kind="radio" {...props} />;
}

/**
 * What a Field hands its control: the id its label points at, the required
 * state, and the hint to describe it by. Callers spread it whole rather than
 * picking pieces, so a field that later grows a hint wires it up without the
 * call site changing.
 */
export type FieldControl = Readonly<{
  id: string;
  required?: boolean;
  "aria-describedby"?: string;
  /**
   * Whether the value currently in the control was refused. Set from `error`,
   * so a caller that spreads the control whole announces the refusal from the
   * control itself rather than only printing it underneath.
   */
  "aria-invalid"?: boolean;
}>;

/**
 * Field is the label-above-control row every form is built from.
 *
 * It owns the id. Before this, each call site minted its own — `${formId}-role`,
 * `${headingId}-expiry`, a hardcoded "overlay-region" — and had to remember to
 * repeat it in two places; a typo in either half silently unlabels the control,
 * and nothing fails. `useId` removes the chance to get it wrong.
 *
 * The label is a real `<label>` with `htmlFor`, which is the other reason this
 * exists: eleven call sites drew the same row with a `<span>` and pointed at it
 * with `aria-labelledby`. That announces correctly but is not a label — clicking
 * the words does not focus the control, and the browser's own form semantics
 * never engage.
 *
 * The hint sits OUTSIDE the label deliberately. Inside, it would be swallowed
 * into the control's accessible name, so a reader would hear the entire help
 * text every time focus lands.
 *
 * `required` marks the label and the control from one prop. The asterisk is
 * `aria-hidden` because the control's own `required` already announces the
 * state — spelling it twice is how a field ends up read as "Role star required".
 */
export function Field({
  label,
  labelEnd,
  hint,
  hintLive,
  error,
  icon,
  trailing,
  required,
  className,
  children,
}: Readonly<{
  // A node, not a string: a label is usually words, but a field whose value was
  // read from somewhere carries its provenance in the label row — a confidence
  // meter and a source chip beside the name.
  label: ReactNode;
  /**
   * What sits at the far end of the label's own line — the "Forgot?" link
   * beside a password, a unit beside an amount. It belongs to the label ROW
   * rather than to the label, so it is not swallowed into the control's
   * accessible name.
   */
  labelEnd?: ReactNode;
  hint?: string;
  /**
   * Announce the hint when it CHANGES, not only when focus reaches the field.
   *
   * Off by default, because a rule that is always true is a description and a
   * description read on every focus is noise. On for a hint that appears in
   * response to something the reader just did — the caps-lock warning under a
   * password is the case this exists for, and the reason is in its timing: caps
   * lock gets pressed while typing, so a warning a reader only hears if they
   * leave the field and come back has arrived after the password it was about.
   */
  hintLive?: boolean;
  /**
   * Why the value in this field was refused.
   *
   * A separate slot from `hint` because the two say different things and were
   * being spelled the same way: the password forms put "too short" and "the
   * passwords do not match" through `hint`, so a refusal rendered in the same
   * meta-grey as neutral helper text, and on one screen in the same grey as the
   * SUCCESS line four elements above it. This one announces, marks the control
   * `aria-invalid`, and reads in the danger tone — and the hint stays visible
   * beside it, because what the field wants is still true while it is wrong.
   */
  error?: string;
  /**
   * A leading affordance INSIDE the control's outline — a mail glyph on an
   * address field. Decorative: the label names the field.
   */
  icon?: ReactNode;
  /**
   * A control inside the outline at the trailing end — the password reveal.
   *
   * This is what `auth.tsx` forked its own `Field` for. `.input-icon` could
   * carry a leading glyph and nothing else, so a button that has to sit inside
   * the focus ring had no way to get there, and a second field component grew
   * on the sign-in screens with its own label size, its own gap and its own
   * hint that was never wired to `aria-describedby`.
   */
  trailing?: ReactNode;
  required?: boolean;
  // Layout the surrounding form owns — a width, a grid span, a screen's own
  // field modifier. It lands on the wrapper, which is the only element a
  // caller has any business positioning.
  className?: string;
  children: (control: FieldControl) => ReactNode;
}>) {
  const id = useId();
  const hintId = hint ? `${id}-hint` : undefined;
  const errorId = error ? `${id}-error` : undefined;
  // Both, when both are on screen. A field that is refused AND still carrying
  // its rule has two things to say, and naming only one of them in
  // `aria-describedby` picks which sighted and non-sighted readers get.
  const describedBy = [errorId, hintId].filter(Boolean).join(" ") || undefined;
  const control = children({
    id,
    required,
    "aria-describedby": describedBy,
    "aria-invalid": error ? true : undefined,
  });
  return (
    <div className={["field", className ?? ""].filter(Boolean).join(" ")}>
      {labelEnd ? (
        <span className="field-label-row">
          <label className="t-label" htmlFor={id}>
            {label}
            {required && <span aria-hidden> *</span>}
          </label>
          {labelEnd}
        </span>
      ) : (
        <label className="t-label" htmlFor={id}>
          {label}
          {required && <span aria-hidden> *</span>}
        </label>
      )}
      {/* The shell exists ONLY when something has to sit inside the outline. A
          field with neither affordance emits exactly the markup it always did,
          which is what keeps the two hundred existing call sites unchanged —
          the border and the focus ring stay on the input there, and move to the
          shell only where an icon or a control would otherwise sit beside the
          outline rather than within it. */}
      {icon || trailing ? (
        <span className="field-shell">
          {icon && (
            <span className="field-shell-icon" aria-hidden>
              {icon}
            </span>
          )}
          {control}
          {trailing}
        </span>
      ) : (
        control
      )}
      {error && (
        <p className="field-error" id={errorId} role="alert">
          {error}
        </p>
      )}
      {hint && (
        <p
          className="t-caption"
          id={hintId}
          role={hintLive ? "status" : undefined}
        >
          {hint}
        </p>
      )}
    </div>
  );
}

/**
 * StatCard is one reading at the top of a record: a label, the reading itself,
 * and one line of detail saying what it is drawn from.
 *
 * The detail line is not decoration. A reading with no basis stated is a number
 * a reader has to trust, and this surface exists because a number nobody could
 * scale — "Relationship 2/100" — was doing exactly that.
 *
 * `tone` colours the value, never the whole tile: a strip of coloured boxes
 * reads as a dashboard, and the reader is meant to see three facts, not a
 * traffic light.
 *
 * `alert` is the one exception, for the one slot whose reading is bad news
 * simply by being present (an overdue balance, a lapsed renewal) rather than
 * by its number — those tint the whole tile, because there is no value to
 * colour that says the same thing on its own. Wire it only there; a reading
 * that could be read either way stays plain and lets its own words carry the
 * judgement.
 *
 * `basis` is the reading's receipt: the rows it was computed from, folded away
 * until a reader asks for them. It is a `Popover` that opens to a settled
 * pointer AND to a click or Enter — a row of readings is compared by moving
 * across it, and asking for a click at every stop makes the comparison cost
 * five presses, while a receipt only a pointer could open would be a reading
 * half the readers do not have. It sits OVER the card rather than expanding
 * it, so opening one does not move the readings beside it.
 *
 * `detail` still carries the one-line basis either way: a reader who never
 * opens this must not meet a number with nothing behind it.
 */

// The bar under a reading: segments a reader would count, or one track they
// would not.
//
// Six is the line, and it is about counting rather than about width: "one of
// three signals" is a set a reader checks off, and "two of ten people" is a
// share they read as a length. Drawn the other way round, three segments of a
// hundred are invisible and a tenth of one track says nothing about which
// signal is out.
const COUNTABLE = 6;

// The segments' own names. A position in a bar has no identity of its own —
// nothing distinguishes the second segment from the third except where it
// sits — so they are named here rather than keyed on the loop index, which is
// the same claim in the spelling a linter cannot tell apart from a keyed list
// of records.
const SEGMENTS = ["first", "second", "third", "fourth", "fifth", "sixth"];

function Meter({ filled, total }: Readonly<{ filled: number; total: number }>) {
  const held = Math.min(Math.max(filled, 0), total);
  if (total <= COUNTABLE) {
    return (
      <span
        className="stat-card-meter stat-card-meter-segments"
        aria-hidden="true"
      >
        {SEGMENTS.slice(0, total).map((segment, index) => (
          <span
            key={segment}
            className={
              index < held
                ? "stat-card-meter-fill"
                : "stat-card-meter-fill stat-card-meter-empty"
            }
          />
        ))}
      </span>
    );
  }
  return (
    <span className="stat-card-meter" aria-hidden="true">
      <span
        className="stat-card-meter-fill"
        style={{ inlineSize: `${Math.round((held / total) * 100)}%` }}
      />
    </span>
  );
}

export function StatCard({
  label,
  value,
  detail,
  basis,
  basisLabel,
  tone,
  source,
  alert,
  dot,
  numeric,
  openLabel,
  onOpen,
  meter,
}: Readonly<{
  label: string;
  value: string;
  // The line under the figure: what it rests on, in the reader's words. A node
  // rather than a string, because a reading whose detail is two facts — how
  // much is failing, and why — says them on two lines rather than in one
  // sentence a reader has to parse.
  detail?: ReactNode;
  // The way OUT of the reading: the tab that holds what it was read from.
  // Both or neither, like `basis` — a labelled door with nothing behind it is
  // worse than no door. A LINK at the card's foot rather than a pressable
  // card, because the card already holds a control (the basis) and a control
  // inside a control is a press whose target the reader has to guess at.
  openLabel?: string;
  onOpen?: () => void;
  // How far along this reading is, as the two numbers it is made of. Drawn as
  // separate segments when there are few enough to count (a verdict made of
  // three signals) and as one filled track when there are not (two of ten
  // people replying) — the difference is whether a reader would count them.
  //
  // Only for a reading that HAS a denominator. A figure with nothing to be out
  // of gets no bar rather than a bar with an invented one.
  meter?: { filled: number; total: number };
  // What the reading rests on, and the words that name it. Both or neither —
  // an unlabelled disclosure asks a reader to open it to find out whether they
  // wanted it. The copy belongs to the caller, because no copy lives in a
  // primitive.
  basis?: ReactNode;
  basisLabel?: string;
  // `good` is not "no tone": a slot whose reading is a VERDICT says so in both
  // directions, and a verdict that is fine reads as fine rather than as one
  // nobody has judged yet.
  tone?: "good" | "warn" | "danger";
  // Where the figure came from, named on the card that shows it. A money
  // reading a reader cannot trace is one they have to go and verify
  // elsewhere, which is the trip the badge saves them.
  source?: ReactNode;
  // Tints the whole tile. See the docblock above — this is not `tone` at
  // stronger volume, it is a different judgement (the slot itself is bad
  // news, not just its figure).
  alert?: boolean;
  // A small coloured mark before the value, for the one slot whose reading
  // is a VERDICT rather than a figure — a glance a reader can catch without
  // reading the word. Gated on `tone` as well as this flag, never on its
  // own: the colour and the decision to show it at all come from the same
  // judgement, so a fine verdict can never carry a leftover dot.
  dot?: boolean;
  // The reading is a FIGURE — money, a count, a duration — so it draws in the
  // mono face, where digits share one width and a column of readings lines up
  // instead of shifting slot to slot with every comma.
  //
  // A flag rather than a `ReactNode` value: the value stays a string this
  // component owns the type of. Widened to a node, the face would be the
  // caller's to spell, and a screen that spells type is the second author of a
  // scale this tier owns — which is also markup arriving at a slot the copy and
  // colour gates read as a string.
  numeric?: boolean;
}>) {
  // No `t-h3`: the card owns the figure's face and size (atoms.css), because
  // a reading is compared across a row and the row is the thing that has to
  // agree. Sharing the page's heading class made the figure change size with a
  // scale that answers a different question.
  const valueClass = [
    "stat-card-value",
    numeric ? "t-mono" : "",
    tone ? `stat-card-${tone}` : "",
  ]
    .filter(Boolean)
    .join(" ");
  return (
    <section className={alert ? "stat-card stat-card-alert" : "stat-card"}>
      <span className="stat-card-label t-eyebrow">
        {label}
        {source && <span className="stat-card-source">{source}</span>}
      </span>
      <span className={valueClass}>
        {dot && tone && (
          <span
            className={`stat-card-dot stat-card-dot-${tone}`}
            aria-hidden="true"
          />
        )}
        {value}
      </span>
      {detail && <span className="stat-card-detail t-caption">{detail}</span>}
      {/* The proportion under the words that state it. A bar rather than a
          second figure: the reader has the number above it, and what a bar
          adds is the SHARE at a glance, which is what a row of readings is
          compared on. Hidden from a screen reader — the figure and its detail
          line already say it in words, and a bar announced as well is the
          same fact twice. */}
      {meter && meter.total > 0 && <Meter {...meter} />}
      {/* THE CARD'S FOOT: the receipt on the left, the way out on the right.
          Both are places to GO rather than parts of the reading, which is why
          they share a row under it — and why the door is at the end of the
          card a reader finishes on rather than up beside the reading's name,
          where it competed with the label for the first glance. */}
      <span className="stat-card-foot">
        {basis && basisLabel && (
          <Popover
            className="stat-card-basis"
            onHover
            label={
              <>
                <ChevronRight aria-hidden="true" size={14} />
                {basisLabel}
              </>
            }
          >
            {/* The receipt names itself inside the panel, because the trigger is
              gone from under the reader's eye the moment the panel is over it
              — and an unnamed list of facts beside a figure is a reader
              guessing which question it answers. */}
            <span className="stat-card-basis-head t-eyebrow">{basisLabel}</span>
            {basis}
            {/* The same door as the reading's own, repeated where the reader has
              just finished reading the working. Going to the tab is what most
              readers do next, and sending them back up to the card's name line
              to find it is a trip the panel can save. */}
            {onOpen && openLabel && (
              <button type="button" className="stat-card-open" onClick={onOpen}>
                {openLabel}
                <span aria-hidden="true">{" \u2192"}</span>
              </button>
            )}
          </Popover>
        )}
        {onOpen && openLabel && (
          <button type="button" className="stat-card-open" onClick={onOpen}>
            {openLabel}
            <span aria-hidden="true">{" \u2192"}</span>
          </button>
        )}
      </span>
    </section>
  );
}

// The element a card wraps its content in. A card is a section of the page by
// default; the other four exist because a card sometimes IS the form you submit,
// the item in a list, or a plain grouping box that must not add a section to the
// document outline.
type CardElement = "section" | "div" | "article" | "form" | "li";

/**
 * The one card in the product: elevated ground, subtle border, 12px radius, one
 * padding. Every surface that reads as a card comes from here — a hand-rolled
 * `<div className="card">` drifts the moment one of those five values changes.
 *
 * `title`/`sub`/`actions` render the card's SectionHeader, so the header sits at
 * the top of the card's own padding without the caller re-deriving that; a card
 * whose head is genuinely bespoke passes children only.
 */
export function Card({
  as = "section",
  inset,
  title,
  sub,
  actions,
  level,
  children,
  className,
  style,
  id,
  ariaLabel,
  role,
  testId,
  onSubmit,
}: Readonly<{
  as?: CardElement;
  inset?: boolean;
  title?: string;
  sub?: string;
  actions?: ReactNode;
  // Passed straight to the card's SectionHeader. A card nested inside a
  // section that already has an h2 passes 3, so the outline says "inside"
  // rather than "beside" — see SectionHeader's own note.
  level?: 1 | 2 | 3;
  children?: ReactNode;
  className?: string;
  style?: CSSProperties;
  id?: string;
  // Naming the card makes it a region a screen reader can land on and list;
  // spelled out rather than spread so the prop reads the same at every call.
  ariaLabel?: string;
  // A card that ANNOUNCES itself: an advisory the app raises while the reader is
  // elsewhere on the page has to reach a screen reader without stealing focus,
  // and that is a live region on the card itself — wrapping it in one would add
  // a node that exists only to satisfy this component.
  role?: "status";
  testId?: string;
  // Only meaningful with `as="form"` — a card that is the form it submits.
  onSubmit?: FormEventHandler<HTMLElement>;
}>) {
  const Tag: ElementType = as;
  return (
    <Tag
      className={["card", inset ? "card-inset" : "", className ?? ""]
        .filter(Boolean)
        .join(" ")}
      style={style}
      id={id}
      aria-label={ariaLabel}
      role={role}
      data-testid={testId}
      onSubmit={onSubmit}
    >
      {title !== undefined && (
        <SectionHeader
          title={title}
          sub={sub}
          actions={actions}
          level={level}
        />
      )}
      {children}
    </Tag>
  );
}

export function Skeleton({
  width,
  height = 14,
}: Readonly<{
  width: number | string;
  height?: number;
}>) {
  return <div className="skeleton" style={{ width, height }} />;
}

// The placeholder lines, named rather than counted: a line's identity IS its
// position, and naming them gives the list a stable key without reaching for
// the array index. Eight is the ceiling on purpose — a wait that needs more room
// than eight lines of text is a shape (a table, a chart, a form), and more bars
// is the wrong answer to it.
const PENDING_LINES = [
  "first",
  "second",
  "third",
  "fourth",
  "fifth",
  "sixth",
  "seventh",
  "eighth",
] as const;

/**
 * The pending state of a surface — the ONE spelling of it in this product.
 *
 * Four grew before this: `QueryStates`' three inline-styled bars, `SurfaceState`'s
 * single silent 32px bar, `ListTable`'s five unanimated bone rows, and a page's
 * worth of hand-rolled bars and "Loading…" lines. They disagreed about the shape,
 * about the height, about whether the pulse ran at all, and — the part that
 * mattered — about whether a reader who cannot see the bars is told anything.
 * Three call sites had already bolted their own `sr-only` line beside one of
 * these, which is the tell that the primitive was missing something rather than
 * that those screens were special.
 *
 * `label` is REQUIRED and not defaulted. A placeholder carries no text, so the
 * spoken line is the only thing a screen reader has; making it a required prop
 * is what stops the next pending state from being silent. It is also the caller
 * who knows what is being waited for, and "Loading the review queue…" is worth
 * more than "Loading…" to someone who cannot see which part of the page went
 * grey.
 *
 * `lines` is a HEIGHT RESERVATION, not decoration: it is how many rows of
 * content will stand here once the read answers. Under-reserving is the reason
 * a card jumps when its body arrives, and the jump is worse than the wait —
 * a reader who has started reading loses their place. Over-reserving costs a
 * collapse in the other direction, so the honest number is the content's own
 * usual size, and 3 is the default only because it is the commonest.
 *
 * `visible` shows the label above the bars instead of only speaking it, for a
 * wait long enough that a mute grey block reads as broken rather than as
 * working — a first assessment that includes a model call is upwards of twenty
 * seconds cold. It is a flag rather than a second string because the sentence is
 * the same sentence: two of them is how a screen reader ends up hearing the wait
 * announced twice.
 *
 * `delayMs` holds the whole thing back until the wait has actually been long
 * enough to be worth reporting. It is for a surface that re-reads as a reader
 * types, where the usual answer arrives faster than a person can perceive: a
 * placeholder that flashes on every keystroke is noise, and it reports work
 * that was already done. Nothing renders before the delay elapses — the spoken
 * line included, deliberately, because announcing a wait that is about to end
 * is the same interruption in the accessibility tree that the flash is on
 * screen. Unset, the pending state shows immediately, which is right for a
 * surface a reader opened rather than one they are typing into.
 */
export function PendingBody({
  label,
  lines = 3,
  visible,
  delayMs,
}: Readonly<{
  label: string;
  lines?: number;
  visible?: boolean;
  delayMs?: number;
}>) {
  const [waited, setWaited] = useState(delayMs === undefined);
  useEffect(() => {
    if (delayMs === undefined) {
      return;
    }
    // Re-armed per mount, so a surface that swaps one pending body for another
    // (a new query key on a new keystroke) starts the clock again rather than
    // inheriting a window the previous read already spent.
    setWaited(false);
    const timer = setTimeout(() => setWaited(true), delayMs);
    return () => clearTimeout(timer);
  }, [delayMs]);
  if (!waited) {
    return null;
  }
  return (
    <div className="pending" role="status" aria-busy="true">
      {/* The label lands EITHER on the page or in the accessibility tree alone,
          never both: this is a live region, and the same sentence twice inside
          it is announced twice. */}
      {visible ? (
        <p className="t-small pending-note">{label}</p>
      ) : (
        <span className="sr-only">{label}</span>
      )}
      {PENDING_LINES.slice(0, lines).map((line) => (
        <div key={line} className="skeleton pending-line" />
      ))}
    </div>
  );
}

/**
 * EmptyState is the one "nothing here" plate.
 *
 * Bare, it is a one-liner: the caller's sentence, centred, in the meta tone —
 * the shape a filtered list or a section with no rows takes. With `title` it
 * becomes the INSTRUCTIONAL variant a first-run surface needs: a heading that
 * names what the page holds, the caller's paragraph saying how a record of
 * this kind comes to exist, and the one primary `action` that makes the first
 * one. The two are one component rather than two because they are the same
 * plate with more or less on it, and a second spelling of the plate is how a
 * page's first-run state came to look like a different product from its
 * filtered-empty state.
 *
 * The words stay the caller's, translated with `t()`; nothing here knows what
 * kind of record is missing.
 */
export function EmptyState({
  title,
  action,
  plate,
  children,
}: Readonly<{
  // The instructional variant's heading. Present, the children render as the
  // explanatory paragraph under it rather than as the whole plate.
  title?: string;
  // The one verb that ends the empty state — a create button. Rendered only
  // with `title`: a bare one-liner that offered a verb would be a filtered
  // list inviting the reader to create what the filter hid.
  action?: ReactNode;
  // An empty GROUP inside a pane, as a dashed plate rather than a sentence:
  // the title says there is none of this kind of thing, the children say what
  // the kind is for. The dashed edge is what says the space is WAITING rather
  // than broken — a solid card holding one grey line reads as a section whose
  // content failed to arrive. No verb in here: the group's own head carries
  // it, so a reader who has just pressed one finds the next where the last
  // one was. Needs `title`.
  plate?: boolean;
  children: ReactNode;
}>) {
  if (plate && title !== undefined) {
    return (
      <div className="empty empty-plate">
        <p className="empty-plate-title">{title}</p>
        <p className="empty-plate-note">{children}</p>
      </div>
    );
  }
  if (title === undefined) {
    return (
      <Card as="div" inset className="empty">
        {children}
      </Card>
    );
  }
  return (
    <Card as="div" inset className="empty empty-instructional">
      <h2 className="t-h2 empty-title">{title}</h2>
      <div className="empty-body">{children}</div>
      {action && <div className="empty-action">{action}</div>}
    </Card>
  );
}

export function SectionHeader({
  title,
  sub,
  actions,
  level = 2,
}: Readonly<{
  title: string;
  sub?: string;
  // Controls that act on this section, placed beside the title stack rather
  // than under it. A caller that needs them anywhere else lays them out itself.
  actions?: ReactNode;
  // A section heading by default. `1` is for the one header on a page that IS
  // the page's name — a record surface the app shell deliberately yields to,
  // where this title is the only thing naming the page. Every other header on
  // that page stays at level 2, so a document never carries two page titles.
  //
  // `3` is a section INSIDE a section: a group of fields under a settings
  // page's own h2, an "add a connection" block inside the connectors card.
  // Without it those headers were h2s nested in an h2, which tells a screen
  // reader the inner block is a sibling of the page's own section — the
  // outline says the group is as important as the page it sits in, and a
  // reader navigating by heading cannot tell where they are. The type follows
  // the level down with it: an inner heading that is the same size as its
  // parent is the same defect drawn instead of announced.
  level?: 1 | 2 | 3;
}>) {
  return (
    <div className="section-header">
      <div className="section-header-text">
        {level === 1 && <h1>{title}</h1>}
        {level === 2 && <h2>{title}</h2>}
        {level === 3 && <h3>{title}</h3>}
        {sub && <span className="sub">{sub}</span>}
      </div>
      {actions && <div className="section-header-actions">{actions}</div>}
    </div>
  );
}

/**
 * The figure beside an option's name, in a strip that counts what is behind
 * each one.
 *
 * One component because the count is four decisions, not a number: the mono
 * face so a column of them lines up, the reader's own number format, the host's
 * class, and the SEPARATOR — which is the one that was missing. Both strips
 * rendered `{label}{count}` as adjacent nodes, so the accessible name a screen
 * reader speaks was "People2", "Deals0", "Tasks0". The comma is
 * visually hidden because the gap between them is already drawn in CSS; what it
 * fixes is the spoken name, where there was nothing between the two at all.
 *
 * It renders INSIDE the option's button, which is what puts the figure in that
 * option's accessible name rather than leaving it to a sighted reader alone.
 */
export function OptionCount({
  count,
  className,
}: Readonly<{ count: number; className: string }>) {
  const { locale } = useLocale();
  return (
    <>
      <span className="sr-only">, </span>
      <span className={`${className} t-mono`}>
        {formatNumber(count, locale)}
      </span>
    </>
  );
}

export function SegmentedControl<Option extends string>({
  options,
  value,
  onChange,
  labels,
  counts,
  label,
  marks,
}: Readonly<{
  options: readonly Option[];
  value: Option;
  onChange: (next: Option) => void;
  labels: Record<Option, string>;
  // How much is behind each option, for a strip that chooses between bodies of
  // a record rather than between settings. Partial and per-option on purpose:
  // an option whose count is absent draws none, which is what a section that
  // is not a list of things (an overview, a form) needs — and it is NOT the
  // same as a zero. A zero is a fact about the account and prints; a missing
  // count is a fact about the section and does not.
  //
  // Inside the button, so the count joins the option's accessible name and a
  // screen reader announces "People 6" rather than leaving the figure to a
  // sighted reader alone.
  counts?: Partial<Record<Option, number>>;
  // Accessible name for the control as a whole (the `fieldset` group); a
  // screen reader announces it alongside each option so the buttons aren't
  // read out of context. Optional so existing callers are unaffected.
  label?: string;
  // Options carrying a dot: something waits behind that option. Decorative by
  // construction — the dot is `aria-hidden` and the fact it hints at must be
  // stated in words on the surface the option opens, because a mark is the one
  // thing a screen reader cannot read out and a colour-blind reader may not
  // see. It draws attention; it never carries the meaning alone.
  marks?: Partial<Record<Option, boolean>>;
}>) {
  return (
    <fieldset className="segmented" aria-label={label}>
      {options.map((option) => {
        const count = counts?.[option];
        return (
          <button
            key={option}
            type="button"
            aria-pressed={option === value}
            onClick={() => onChange(option)}
          >
            {labels[option]}
            {count !== undefined && (
              <OptionCount count={count} className="segmented-count" />
            )}
            {marks?.[option] && <span className="segmented-mark" aria-hidden />}
          </button>
        );
      })}
    </fieldset>
  );
}

export function Kbd({ children }: Readonly<{ children: ReactNode }>) {
  return <kbd className="kbd">{children}</kbd>;
}

export function Modal({
  open,
  onClose,
  labelledBy,
  size = "default",
  placement = "center",
  returnFocusTo,
  children,
}: Readonly<{
  open: boolean;
  onClose: () => void;
  labelledBy: string;
  // "wide" roomier variant for content-dense dialogs (code/YAML previews);
  // "default" keeps the compact form width every confirm/create modal uses.
  // "split" is a drawer holding TWO columns rather than one — the conversation
  // being answered beside the reply being written. It is a width because that
  // is what a second column costs; a drawer at the wide clamp splits into two
  // unreadable halves.
  size?: "default" | "wide" | "split";
  // "right" anchors the dialog to the right edge, full height — the drawer
  // form the composer and the evidence receipt use, where the record behind
  // stays visible as context rather than being covered by a centred box.
  // With size="wide" it takes the roomier clamp and a sticky header/footer,
  // for the surfaces a rep works IN rather than glances at.
  placement?: "center" | "right";
  // Where focus should land instead of the opener, for a dialog whose OWN
  // mutation removes the control that opened it — a Deactivate button that
  // becomes Reactivate, a row the delete drops from the list.
  //
  // A callback rather than a ref because the resting place frequently does not
  // exist while the dialog is open: it is produced by the very mutation the
  // dialog performs, and the opener is detached by the time anything could ask
  // about it. Resolving at restore time is the only moment the answer is known.
  // A caller that does hold a ref passes `() => ref.current`, so the ref form
  // is a subset of this one rather than a second API.
  returnFocusTo?: () => HTMLElement | null;
  children: ReactNode;
}>) {
  const dialog = useRef<HTMLDivElement | null>(null);
  // Escape, the Tab trap and focus in-and-back live in `dialogfocus.ts`,
  // because this is not the only dialog in the product: the ⌘K palette draws
  // its own box and had grown its own, weaker, answer to the same three
  // questions. The chrome below stays this component's; the keyboard does not.
  useDialogFocus({ open, onClose, container: dialog, returnFocusTo });

  if (!open) {
    return null;
  }
  // Portalled to the document body rather than rendered in place: a dialog
  // opened from inside a collapsed container — the record header's overflow
  // menu — would otherwise be hidden along with it, and the click that opened
  // the dialog is the same click that collapses the menu.
  return createPortal(
    // biome-ignore lint/a11y/noStaticElementInteractions: backdrop dismiss is a convention; Esc is the keyboard path
    // biome-ignore lint/a11y/useKeyWithClickEvents: Esc handles the keyboard path above
    <div // NOSONAR: backdrop dismiss only; keyboard path (Esc) handled by the effect above
      className={placement === "right" ? "overlay overlay-right" : "overlay"}
      onClick={(event) => {
        if (event.target === event.currentTarget) {
          onClose();
        }
      }}
    >
      <div
        // NOSONAR: styled modal overlay driven by React state, not a native <dialog>; conversion would change focus/backdrop behavior
        role="dialog"
        aria-modal="true"
        aria-labelledby={labelledBy}
        className={modalClass(size, placement)}
        ref={dialog}
        // Focusable so a dialog whose body is pure text still receives focus
        // when it opens, rather than leaving it on the page behind.
        tabIndex={-1}
      >
        {children}
      </div>
    </div>,
    document.body,
  );
}

// A right-anchored dialog draws its width from the viewport, so the `size`
// variants — which exist to widen a centred box — do not apply to it.
function modalClass(
  size: "default" | "wide" | "split",
  placement: "center" | "right",
) {
  if (placement === "right") {
    // A drawer's width normally comes from the viewport, but a surface a rep
    // WORKS in — a numbered claim list, a message being written — wraps into an
    // unreadable column at the default clamp. `size` is what asks for the
    // roomier one, and it brings sticky header and footer with it.
    //
    // A split drawer is the wide one plus the room its second column needs, so
    // it keeps the wide band behaviour rather than restating it.
    if (size === "split") {
      return "modal modal-drawer modal-drawer-wide modal-drawer-split";
    }
    return size === "wide"
      ? "modal modal-drawer modal-drawer-wide"
      : "modal modal-drawer";
  }
  // Centred, a split has no second column to hold — the layout that earns the
  // extra width is the drawer's — so it falls back to the roomy box rather
  // than to a width nothing on screen uses.
  return size === "default" ? "modal" : "modal modal-wide";
}

/** Whether a box is holding more width than it is showing. */
function overflowsSideways(element: HTMLElement | null): boolean {
  return element !== null && element.scrollWidth - element.clientWidth > 1;
}

/** Spread onto the scrolling box. Empty while it has nothing hidden to reach. */
type ScrollRegion = Readonly<{
  tabIndex?: 0;
  role?: "region";
  "aria-label"?: string;
}>;

/**
 * Make a box that scrolls sideways reachable, and only then.
 *
 * A region holding content past its right edge is content pointer users can
 * drag to and keyboard users cannot reach at all, so it takes a tab stop and
 * announces itself by name. It takes neither while it fits: a tab stop in front
 * of every table in the product, most of which fit, is a cost every keyboard
 * reader pays for the few that do not. That is the same bargain
 * `useTruncationTooltip` strikes for a string that fits its row.
 *
 * Both spellings of a scrolling table body use this — `TableScroll` below, and
 * the list surface's own `.lt-scroll` (listtable.tsx) — so a reader meets the
 * same behaviour whichever table they land in.
 */
export function useScrollRegion(
  box: RefObject<HTMLElement | null>,
  label: string,
): ScrollRegion {
  const [scrolls, setScrolls] = useState(false);
  const [watched, setWatched] = useState<HTMLElement | null>(null);
  // Measured after every render rather than when the rows change: the answer
  // moves for reasons this hook never sees — a column the reader dragged, a
  // cell whose badge arrived — and re-reading it is two property reads. Setting
  // either answer twice is a no-op, so this cannot loop. The element goes into
  // state as well, so a box that unmounts and comes back (a list switching
  // between a board and a table) is re-watched rather than leaving the observer
  // below holding a node that is no longer on the page.
  useLayoutEffect(() => {
    setScrolls(overflowsSideways(box.current));
    setWatched(box.current);
  });
  // A window resize is only one of the ways the box changes size, and the least
  // interesting one: the sidebar collapsing, a rail opening beside the table,
  // a settings card that is 720px on one route and full width on the next all
  // move the edge without the window moving at all. So the BOX is watched, and
  // the table inside it too — a table that grew is the other half of the same
  // question.
  useEffect(() => {
    // Measured once wherever the observer is unavailable (jsdom): the answer is
    // still right for the render that just happened, it simply stops following
    // a resize.
    if (!watched || typeof ResizeObserver === "undefined") {
      return;
    }
    const observer = new ResizeObserver(() =>
      setScrolls(overflowsSideways(watched)),
    );
    observer.observe(watched);
    const content = watched.firstElementChild;
    if (content) {
      observer.observe(content);
    }
    return () => observer.disconnect();
  }, [watched]);
  return scrolls ? { tabIndex: 0, role: "region", "aria-label": label } : {};
}

/**
 * The box a table too wide for its column scrolls sideways INSIDE.
 *
 * The one spelling of `.table-scroll`. A settings page is 720px wide and a
 * record's finance table is eight columns, so the overflow is a property of the
 * TABLE rather than a knob each page answers for — and the four screens that
 * had each written this wrapper by hand were four chances to forget the part
 * below.
 *
 * Reachability is `useScrollRegion`'s, above: the tab stop and the name arrive
 * only while the box is actually holding something past its right edge.
 *
 * `label` is what the region is called ("Recent invoices", "Spend by task") and
 * is the caller's to translate. It is required rather than defaulted because a
 * region announced as "region" tells a reader nothing about which of the page's
 * tables they have just landed in.
 */
export function TableScroll({
  label,
  className,
  children,
}: Readonly<{ label: string; className?: string; children: ReactNode }>) {
  const box = useRef<HTMLDivElement | null>(null);
  const region = useScrollRegion(box, label);
  return (
    <div
      ref={box}
      className={["table-scroll", className ?? ""].filter(Boolean).join(" ")}
      {...region}
    >
      {children}
    </div>
  );
}

export function DataTable<Row>({
  columns,
  rows,
  rowKey,
  onRowClick,
  label,
}: Readonly<{
  columns: { key: string; header: string; render: (row: Row) => ReactNode }[];
  rows: Row[];
  rowKey: (row: Row) => string;
  onRowClick?: (row: Row) => void;
  /** What the scroll region is called once the table is wider than its box. */
  label: string;
}>) {
  return (
    <TableScroll label={label}>
      <table className="table">
        <thead>
          <tr>
            {columns.map((column) => (
              <th key={column.key}>{column.header}</th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map((row) => (
            <tr
              key={rowKey(row)}
              className={onRowClick ? "rowlink" : undefined}
              onClick={onRowClick ? () => onRowClick(row) : undefined}
            >
              {columns.map((column) => (
                <td key={column.key}>{column.render(row)}</td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </TableScroll>
  );
}

/**
 * Disclosure is a section the reader opens when they want it.
 *
 * For the surfaces a record page carries but does not lead with — one-off
 * tools, configuration, the occasional deep read. Kept as a standing card
 * each of those competes for the eye with the facts a reader came for; kept
 * behind a summary they cost one line until asked for.
 *
 * `open` forces it open for a state the reader must not miss (a tool that is
 * running, a result that just arrived); left undefined the reader decides.
 *
 * `summary` is a node rather than a string because a summary is a ROW, and
 * some of them carry more than a label — a count beside the name, a status
 * chip. Passing a string stays the ordinary case and reads identically; the
 * alternative was a second `<details>` implementation living beside this one,
 * which is how two disclosures on one screen end up disagreeing about their
 * own caret. `className` is the same bargain for the row's chrome.
 */
// Whether an item SETS something rather than doing something — a toggle, or a
// control that draws a region open — which is the one shape of item a menu must
// not close under.
//
// A verb is finished when it has run, and a menu still standing over the page
// after it reads as a control that never took the press. A switch is not: the
// reader came here to set two of them, and the control that opened a region is
// also the only control that closes it again. The button says which it is —
// `aria-pressed` and `aria-expanded` are exactly that claim — so nothing has to
// be declared at the call site, and an item that grows a toggle later carries
// the right behaviour the moment it says so.
function isSetting(item: Element): boolean {
  return (
    item.hasAttribute("aria-pressed") || item.hasAttribute("aria-expanded")
  );
}

// OverflowMenu folds the verbs a record offers but a reader rarely wants —
// merge, archive, share — behind one control, so the header carries identity
// and the frequent actions rather than a row of buttons of equal weight where
// the destructive ones sit next to the routine ones.
//
// The children are the caller's own action components (each opening its own
// confirm flow), so the menu owns only the disclosure: it closes on Escape, on
// a click outside, and on an item being chosen — with the two exceptions
// `isSetting` and the `.overlay` test below name, an item that SETS rather than
// does, and one that put a dialog up which now owns the screen and the focus.
//
// The children are not rendered until the menu is first opened. They are
// components with their own reads — the company's edit form alone fetches the
// user roster and the custom-field catalogue — and every reader of every
// record page was paying for them without ever opening the menu. Once opened
// they STAY mounted, so a dialog survives the panel being hidden again.
//
// The PANEL is portalled to the body and positioned against the trigger. A
// menu that opened inside its own container was clipped by whatever that
// container clips — a Panel hides its overflow so full-bleed rows respect its
// radius — and a row near the bottom edge of a card lost the actions the menu
// exists to offer. Positioning against the trigger keeps it under the button
// it belongs to wherever that button has moved to.
export function OverflowMenu({
  label,
  keepMounted = false,
  children,
}: Readonly<{
  label: string;
  /**
   * Mount the children immediately rather than on the first open.
   *
   * The default defers them, and that is right for a menu drawn PER ROW: a
   * roster of two hundred rows would otherwise mount two hundred sets of verbs
   * and every dialog behind them before the reader has pressed anything.
   *
   * It is wrong for a control whose job starts at MOUNT. `CreateAction` reads
   * `startOpen` once, in `useState`, because that is what "this address means
   * open the form" has to hang on — so folded into a deferred menu, the create
   * dialog `#/deals/new` asks for never opened, and pressing the menu opened it
   * as a surprise instead. A list header carries one menu per page and a
   * handful of buttons in it, so mounting them costs nothing there.
   */
  keepMounted?: boolean;
  children: ReactNode;
}>) {
  const [open, setOpen] = useState(false);
  const [everOpened, setEverOpened] = useState(false);
  // How many times an item has been chosen. A counter rather than a flag
  // because it is a fact that RECURS: the second press of the second item has
  // to reach the effect below as its own event, and a boolean already true
  // would be no change at all.
  const [chosen, setChosen] = useState(0);
  const wrap = useRef<HTMLDivElement | null>(null);
  const panel = useRef<HTMLDivElement | null>(null);
  const trigger = useRef<HTMLButtonElement | null>(null);
  const panelId = useId();
  const at = useAnchoredToTrigger(open, trigger, panel);

  // Choosing an item closes the menu, one commit after the press.
  //
  // The delay is the whole design. Some items DO something and are finished —
  // and a menu still standing over the page after that reads as a control that
  // did not take the press. Others only open a dialog, which restores focus on
  // close to the control that opened it, so hiding that control first would
  // strand the reader on <body>. Which of the two happened is not knowable
  // while the item's own handler is running: the dialog is not in the document
  // until React has committed the state that handler set. So the press records
  // that it happened, and this effect — after that commit — reads the same
  // `.overlay` the Escape handler reads and answers accordingly.
  useEffect(() => {
    if (chosen === 0 || document.querySelector(".overlay")) {
      return;
    }
    setOpen(false);
    trigger.current?.focus();
  }, [chosen]);

  useEffect(() => {
    if (!open) {
      return;
    }
    const onKey = (event: KeyboardEvent) => {
      if (event.key !== "Escape") {
        return;
      }
      // A dialog opened from this menu owns Escape while it is up. Closing
      // both layers on one keypress would take the reader back past the menu
      // they were choosing from, and they would have to reopen it to pick
      // something else.
      if (document.querySelector(".overlay")) {
        return;
      }
      setOpen(false);
      trigger.current?.focus();
    };
    const onPointer = (event: MouseEvent) => {
      if (!(event.target instanceof Node)) {
        return;
      }
      // A dialog this menu opened is portalled to the body, so every click
      // inside it looks like a click outside the menu. Closing on those would
      // hide the item the dialog has to give focus back to when it closes.
      if (event.target instanceof Element && event.target.closest(".overlay")) {
        return;
      }
      // The panel lives at the body, not inside the wrapper, so "outside" is
      // outside BOTH — without the second test every click on an item would
      // read as a click away from the menu.
      if (
        !wrap.current?.contains(event.target) &&
        !panel.current?.contains(event.target)
      ) {
        setOpen(false);
      }
    };
    globalThis.addEventListener("keydown", onKey);
    globalThis.addEventListener("mousedown", onPointer);
    return () => {
      globalThis.removeEventListener("keydown", onKey);
      globalThis.removeEventListener("mousedown", onPointer);
    };
  }, [open]);

  return (
    <div className="overflow-menu" ref={wrap}>
      {/* A disclosure, not an ARIA menu. `role="menu"` promises arrow-key
          navigation and a roving tabstop; the items here are the caller's own
          buttons — each running its own verb, opening its own dialog or setting
          its own switch — and Tab through them is the behaviour a reader
          actually gets. The rows below are DRAWN as a menu, which changes
          nothing about that: the look is what tells a reader these are choices
          in a list, and the announcement still has to describe what the
          keyboard will really do. Announcing a menu we do not implement is
          worse than announcing the expandable region we do. */}
      {/* `iconOnly`, and not a class of this component's own: the ellipsis is
          the whole label, so the control is the --control-h-sm square the
          catalog defines and drops the width floor a WORD needs. Geometry
          belongs to the button — a padding rule beside it is a second author
          of one box, and which of the two wins is a fact about sheet order. */}
      <Button
        small
        iconOnly
        ref={trigger}
        aria-expanded={open}
        aria-controls={panelId}
        aria-label={label}
        title={label}
        onClick={() => {
          setEverOpened(true);
          setOpen((was) => !was);
        }}
      >
        <MoreHorizontal aria-hidden="true" />
      </Button>
      {/* Hidden, never unmounted once mounted. The items own their own dialogs,
          so unmounting them on close would throw away the dialog the click just
          opened. `hidden` also takes them out of the tab order, so a closed
          menu is closed for a keyboard reader too. WHEN they first mount is
          `keepMounted`'s question, and it is a real one — see the prop. */}
      {createPortal(
        // biome-ignore lint/a11y/noStaticElementInteractions: not a control — it observes that one of the caller's controls inside it was pressed
        // biome-ignore lint/a11y/useKeyWithClickEvents: the keyboard path IS this handler; Enter and Space on a button dispatch a click that bubbles here
        <div // NOSONAR: listener observes activation of the caller's own buttons; the panel itself is not pressable
          id={panelId}
          ref={panel}
          className="overflow-menu-items"
          hidden={!open}
          style={{
            top: `${at.top}px`,
            left: `${at.left}px`,
            maxHeight: `${at.maxHeight}px`,
          }}
          onClick={(event) => {
            if (!(event.target instanceof Element)) {
              return;
            }
            // A control was pressed, not merely the panel. The caller also puts
            // PROSE in here — the one sentence saying why an archived record
            // refuses these verbs — and a click landing on a paragraph has
            // chosen nothing. A refused item never arrives at all: `reason` and
            // `reasonId` disable the button natively.
            const item = event.target.closest("button, a");
            if (item && !isSetting(item)) {
              setChosen((count) => count + 1);
            }
          }}
        >
          {(keepMounted || everOpened) && children}
        </div>,
        document.body,
      )}
    </div>
  );
}

export function Disclosure({
  summary,
  action,
  open,
  className,
  children,
}: Readonly<{
  summary: ReactNode;
  /**
   * One verb belonging to this section, drawn on the summary's line and OUTSIDE
   * the `<summary>` element.
   *
   * That is the whole point of the prop. A `<summary>` is itself the control
   * that opens the section, so a button placed inside it is a control inside a
   * control: axe fails it as `nested-interactive`, and a reader who presses the
   * button also toggles the section under it. Two rail sections had done exactly
   * that, and the verb they nested was the one that opens a form — so pressing
   * "Add employment" collapsed the employments it was about to add to.
   *
   * It stays visible while the section is closed, which is what a section-level
   * verb wants: "Add employment" is a thing to do whether or not the list is on
   * screen.
   */
  action?: ReactNode;
  open?: boolean;
  className?: string;
  children: ReactNode;
}>) {
  const details = (
    <details
      className={className ? `disclosure ${className}` : "disclosure"}
      open={open}
    >
      <summary className="disclosure-summary">
        <ChevronRight className="disclosure-chevron" aria-hidden="true" />
        <span className="t-label">{summary}</span>
      </summary>
      <div className="disclosure-body">{children}</div>
    </details>
  );
  if (!action) {
    return details;
  }
  return (
    <div className="disclosure-wrap">
      {details}
      <span className="disclosure-action">{action}</span>
    </div>
  );
}
