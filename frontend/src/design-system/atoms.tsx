import {
  ChevronRight,
  LoaderCircle,
  type LucideIcon,
  MoreHorizontal,
  Search,
  Sparkles,
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
import { liveDialogs, useDialogFocus } from "./dialogfocus";
import { Heading, type HeadingElement, type HeadingSize } from "./heading";
import "./atoms.css";
import "./evidencemark.css";

// The Margince atom library (B-EP09.2, re-scoped to our own system, no gw-ui
// port; atoms are added as screens need them). Copy always arrives through
// props — callers translate with t(); atoms never hard-code user-facing words.

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
 *
 * `link` is the quietest rung: the text affordance `.link-button` already draws
 * for an `<a>` or a hand-rolled `<button>`, reached through this component by a
 * verb that also needs what only this component gives — the refusal contract
 * and the `pending` one. It wears that same class rather than a look of its
 * own, so the two spellings of a link affordance cannot drift apart. `iconOnly`
 * says nothing here: the class has no fill, no width floor and no control
 * height for it to shrink.
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
 * way; sized by whatever control hosts it (`.btn svg` draws --controlIcon), so
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
      {/* `t-caption` is the supporting line's role: one size and one ink for
          every refusal, and the class atoms.css's dialog rule selects. */}
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

/**
 * The closed tone vocabulary, as a VALUE that the type is read from — so the
 * story that draws every tone and the gate that checks every tone is painted
 * walk the same list the compiler enforces. Spelled twice, they drift, and the
 * copy that goes stale is always the one nobody renders.
 *
 * `discovery` says a thing is NEW — newly arrived, newly offered, not yet how
 * things have always been. It is the one tone that reports no verdict: a status
 * says how something is GOING and this says how long it has been here, which is
 * why it is a word of its own rather than a borrowed `accent`.
 *
 * `info` is the QUIET verdict, and it is what a row reaches for while a job is
 * still writing it — queued, syncing, in progress — and for a status that only
 * reports. It is not `accent`: accent is the brand asking for a press, and a
 * status wearing it told a reader the row was the page's next move.
 */
export const BADGE_TONES = [
  "default",
  "accent",
  "info",
  "success",
  "warning",
  "danger",
  "ai",
  "discovery",
] as const;

type BadgeTone = (typeof BADGE_TONES)[number];
// The leading slot holds ONE mark, a glyph or the `live` dot. An `ai` badge's
// mark is always Sparkles, so that tone is given neither to choose.
type BadgeMark =
  | { tone?: Exclude<BadgeTone, "ai">; icon?: LucideIcon; live?: never }
  | { tone?: Exclude<BadgeTone, "ai">; icon?: never; live?: boolean }
  | { tone: "ai"; icon?: never; live?: never };

export function Badge({
  variant = "soft",
  tone = "default",
  icon,
  live,
  children,
}: Readonly<
  {
    // `soft` is the tint a status wears beside prose and down a column;
    // `primary` the solid fill for the one status a reader must not miss.
    variant?: "soft" | "primary";
    children: ReactNode;
  } & BadgeMark
>) {
  // `live` is true AS THE PAGE IS READ: the one place motion is a fact. The ai
  // mark is decided here as well, for a tone that arrives untyped.
  const provenance = tone === "ai";
  const Icon = provenance ? Sparkles : icon;
  const classes = [
    "badge",
    variant === "primary" && "badge-primary",
    tone !== "default" && `badge-${tone}`,
  ].filter(Boolean);
  return (
    <span className={classes.join(" ")}>
      {live && !provenance && <span className="badge-live-dot" aria-hidden />}
      {Icon && <Icon size={12} aria-hidden="true" />}
      <span className="badge-label">{children}</span>
    </span>
  );
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
  shape = "contact",
}: Readonly<{
  name: string;
  /**
   * What the tint is derived FROM, when that is not the displayed name — a
   * record id, an address, anything stable for the life of the record. The
   * name is the fallback and it is a poor key: renaming a contact or a company
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
   * The four sizes — `sm` every list row, `md` a record header, `lg` a wide
   * one, `xl` a record page's own mark — after four numbers in four
   * stylesheets for a prop that admitted two. `sm` is the FLOOR: a dense table
   * brings its chips down from its own sheet, density being its decision.
   */
  size?: "sm" | "md" | "lg" | "xl";
  /**
   * What KIND of thing this chip stands for, which decides its shape.
   *
   * A contact is round, the way a face is drawn everywhere; a company is
   * a rounded square, the way a logo is. The distinction is not decoration —
   * on a page carrying both, the shape is what tells a reader whether a chip
   * is a company or somebody at it before they have read a word of it.
   */
  shape?: "contact" | "company";
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
  if (shape === "company") classes.push("avatar-company");
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
 * field nobody can find. It takes a `ref` for the reason `TextInput` does: a
 * caller moving focus here needs the node, and a field labelled by `Field`
 * carries no id the caller minted to find it by. */
export function SearchField({
  flush,
  ...props
}: ComponentPropsWithRef<"input"> & Readonly<{ flush?: boolean }>) {
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

// StatCard lives in statcard.tsx — a reading, its meter and its evidence
// popover are one concept, and beside its own story they are findable. It is
// re-exported here because thirty callers import it from `atoms`, and moving a
// component is not a reason to move its address.
export { StatCard } from "./statcard";

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
 * `title`/`actions` render the card's SectionHeader, so the header sits at the
 * top of the card's own padding without the caller re-deriving that; a card
 * whose head is genuinely bespoke passes children only.
 */
export function Card({
  as = "section",
  inset,
  title,
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
  // The card's head, drawn as its SectionHeader: a title and, optionally, the
  // verbs that act on it. A description belongs in the body — see SectionHeader.
  title?: string;
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
        <SectionHeader title={title} actions={actions} level={level} />
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
 * types, where the usual answer arrives faster than a contact can perceive: a
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
      // A caller that drops the delay wants the pending state NOW, and the
      // effect has to say so: leaving `waited` where the previous delay left it
      // hides the body for good, since nothing re-runs to release it.
      setWaited(true);
      return;
    }
    // The clock is per MOUNT and per delay, not per read. A pending body that
    // stays mounted while one query replaces another keeps the time it has
    // already served — a reader typing through a slow search watches one bar
    // rather than a bar that blinks out on every keystroke.
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
        <p className="pending-note">{label}</p>
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
      <Heading size="large" className="t-h2 empty-title">
        {title}
      </Heading>
      <div className="empty-body">{children}</div>
      {action && <div className="empty-action">{action}</div>}
    </Card>
  );
}

// Level picks the ELEMENT; between 2 and 3 the type stays put, because a card
// head is a card head wherever it sits and the outline already says where the
// group belongs. `1` is not a step in that scale but a different job: that
// header IS the page's name, the only thing naming a surface the shell has
// yielded to, so it reads at the page-title size. A table rather than a tag
// built from the number, which would render a raw magnitude.
const LEVEL_HEADING: Readonly<
  Record<1 | 2 | 3, { size: HeadingSize; as: HeadingElement }>
> = {
  1: { size: "large", as: "h1" },
  2: { size: "medium", as: "h2" },
  3: { size: "medium", as: "h3" },
};

export function SectionHeader({
  title,
  actions,
  level = 2,
}: Readonly<{
  // A head is a title and, optionally, the verbs that act on it. Never a
  // description: the title and the body below it are what give the section its
  // meaning, and a sentence between them is a third voice saying what one of
  // the two already said. Copy that genuinely adds something is body content.
  title: string;
  // Controls that act on this section, placed beside the title rather than
  // under it. A caller that needs them anywhere else lays them out itself.
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
  // reader navigating by heading cannot tell where they are. Between 2 and 3
  // it moves the ELEMENT and nothing else — a card head reads at one size
  // however deeply it sits. `1` is the exception, because it is not a deeper
  // head but the page's own name, and it reads at the page-title size.
  level?: 1 | 2 | 3;
}>) {
  return (
    <div className="section-header">
      <Heading {...LEVEL_HEADING[level]} className="section-header-title">
        {title}
      </Heading>
      {actions && <div className="section-header-actions">{actions}</div>}
    </div>
  );
}

/**
 * The figure beside an option's name, in a strip that counts what is behind
 * each one.
 *
 * One component because the count is four decisions, not a number: tabular
 * figures so a column of them lines up, the reader's own number format, the
 * CHIP it is drawn as, and the SEPARATOR — which is the one that was missing.
 * Both strips rendered `{label}{count}` as adjacent nodes, so the accessible
 * name a screen reader speaks was "Contacts2", "Deals0", "Tasks0". The comma is
 * visually hidden because the gap between them is already drawn in CSS; what it
 * fixes is the spoken name, where there was nothing between the two at all.
 *
 * The chip is the component's own (`.optioncount`, atoms.css) and takes no
 * class from its caller: every host was styling the same figure itself, which
 * is how one count came to be drawn two ways on two strips of one record. It
 * carries no ink of its own either — it inherits the host's, so the figure goes
 * quiet beside a quiet label and dark beside a pressed one.
 *
 * It renders INSIDE the option's button, which is what puts the figure in that
 * option's accessible name rather than leaving it to a sighted reader alone.
 */
export function OptionCount({ count }: Readonly<{ count: number }>) {
  const { locale } = useLocale();
  return (
    <>
      <span className="sr-only">, </span>
      <span className="optioncount t-num">{formatNumber(count, locale)}</span>
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
  // screen reader announces "Contacts 6" rather than leaving the figure to a
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
            {count !== undefined && <OptionCount count={count} />}
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

// The one dialog lives in modal.tsx and is published from here, because `Modal`
// is the name three hundred call sites import from `./atoms` and moving a file
// is not a reason to make every one of them say where it went.
export { Modal } from "./modal";

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
// `isSetting` and `liveDialogs` below name, an item that SETS rather than
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
  // that it happened, and this effect — after that commit — asks the same
  // `liveDialogs` the Escape handler asks and answers accordingly.
  useEffect(() => {
    if (chosen === 0 || liveDialogs().length > 0) {
      return;
    }
    setOpen(false);
    trigger.current?.focus();
  }, [chosen]);

  // FOCUS GOES IN. The panel is portalled to the body, so it sits at the end of
  // the document rather than after its trigger: without this a reader who opens
  // a menu Tabs to the NEXT row's trigger, then through every control on the
  // page, and reaches the items they opened it for last. On a list drawing one
  // menu per row that is dozens of stops, which is a menu no keyboard reaches.
  //
  // The same hook the dialogs use, and for the same reason its own header
  // gives: a surface that keeps its chrome private still owes the keyboard one
  // answer to "where is focus now". Escape and the click-outside stay below,
  // because this menu closes on a third thing a dialog does not have — an item
  // being chosen — and that path has to leave the dialog it may have opened
  // holding focus.
  useDialogFocus({
    open,
    onClose: () => setOpen(false),
    container: panel,
    returnFocusTo: () => trigger.current,
  });

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
      if (liveDialogs().length > 0) {
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
          the whole label, so the control is the --controlHeight square the
          catalog defines and drops the width floor a WORD needs. Geometry
          belongs to the button — a padding rule beside it is a second author
          of one box, and which of the two wins is a fact about sheet order. */}
      <Button
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
  onToggle,
  className,
  children,
}: Readonly<{
  summary: ReactNode;
  /** A section-level action stays outside summary to avoid nested controls and
   * remains visible while the section is closed. */
  action?: ReactNode;
  open?: boolean;
  onToggle?: (open: boolean) => void;
  className?: string;
  children: ReactNode;
}>) {
  const details = (
    <details
      className={className ? `disclosure ${className}` : "disclosure"}
      open={open}
      onToggle={(event) => onToggle?.(event.currentTarget.open)}
    >
      <summary className="disclosure-summary">
        <ChevronRight className="disclosure-chevron" aria-hidden="true" />
        <span className="disclosure-label">{summary}</span>
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
