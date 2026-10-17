import { Search } from "lucide-react";
import {
  type ComponentPropsWithRef,
  type InputHTMLAttributes,
  type ReactNode,
  useId,
} from "react";
import { describedByIds } from "./describedby";

// `ComponentPropsWithRef`, not the bare attribute set. A caller that opens
// this field itself needs the node, such as an edit-in-place value that puts
// the caret where the reader clicked. React 19 passes `ref` as an ordinary
// prop to a function component.
export function TextInput(props: ComponentPropsWithRef<"input">) {
  return (
    <input {...props} className={`input ${props.className ?? ""}`.trim()} />
  );
}

/**
 * The one search field: a text input that announces itself as a search box and
 * carries the magnifier.
 *
 * `flush` is for a field whose container already draws the chrome: the ⌘K
 * palette's own bar, with its own ground, inset and bottom rule. Nested, the
 * ordinary field's border and radius read as a box inside a box. The field's
 * own padding also pushes the caret off the text column of the results below.
 * It is a variant rather than a caller overriding `.input` from outside. A call
 * site that reaches in to cancel a primitive's chrome is how the next surface
 * grows a second search field nobody can find. It takes a `ref` for the reason
 * `TextInput` does: a caller moving focus here needs the node. A field labelled
 * by `Field` carries no id the caller minted to find it by. */
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
 * Textarea carries no label of its own, like TextInput. The label is composed
 * outside it, by the `.field` wrapper a form uses or by a screen's own richer
 * shell. What it owns is the one spelling of the control's surface, so a note
 * field in a create form and one in settings cannot drift.
 *
 * The dropdown is not here: `Select` in select.tsx is a button and a portalled
 * listbox. A native `<select>` draws its own option list in the platform's
 * idiom, and no CSS reaches inside it. `Select` still reads `.input` for its
 * closed face, because a dropdown and a text input are the same field on screen.
 *
 * `ComponentPropsWithRef`, for the same reason `TextInput` takes it. A caller
 * that has to move focus here needs the node. A dialog whose fields are mostly
 * prose has no single-line input to land on instead.
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
 * Checkbox and Radio do carry their label, unlike the fields above. For a tick
 * the label is not a caption sitting nearby; it is the other half of the click
 * target. Wrapping the input makes the words clickable. It also gives the
 * control its accessible name without an `id` to thread. That is why seventeen
 * of the twenty hand-rolled sites already wrote this shape. Each had its own
 * wrapper class and its own idea of the gap.
 *
 * `label` is a ReactNode, not a string: a consent line carries emphasis and a
 * settings toggle carries a help line under the name.
 *
 * `className` lands on the label, not the input, because every existing call
 * site puts its layout there. A row that needs `align-items:flex-start` for a
 * two-line label says so there.
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
 * It owns the id. Before this, each call site minted its own (`${formId}-role`,
 * `${headingId}-expiry`, a hardcoded "overlay-region"). Each had to repeat it
 * in two places. A typo in either half silently unlabels the control, and
 * nothing fails. `useId` removes the chance to get it wrong.
 *
 * The label is a real `<label>` with `htmlFor`, the other reason this exists.
 * Eleven call sites drew the same row with a `<span>` and pointed at it with
 * `aria-labelledby`. That announces correctly but is not a label. Clicking the
 * words does not focus the control, and the browser's own form semantics never
 * engage.
 *
 * The hint sits outside the label. Inside, it would be swallowed into the
 * control's accessible name, so a reader would hear the entire help text every
 * time focus lands.
 *
 * `required` marks the label and the control from one prop. The asterisk is
 * `aria-hidden` because the control's own `required` already announces the
 * state. Spelling it twice is how a field ends up read as "Role star required".
 */
export function Field({
  label,
  labelHidden,
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
  // A node, not a string: a label is usually words. A field whose value was read
  // from somewhere carries its provenance in the label row: a confidence meter
  // and a source chip beside the name.
  label: ReactNode;
  /** The label for assistive tech alone, where the layout already names the
   *  field: an inline editor opened in place of the value it edits. */
  labelHidden?: boolean;
  /**
   * What sits at the far end of the label's own line: the "Forgot?" link
   * beside a password, a unit beside an amount. It belongs to the label row
   * rather than to the label, so it is not swallowed into the control's
   * accessible name.
   */
  labelEnd?: ReactNode;
  hint?: ReactNode;
  /**
   * Announce the hint when it changes, not only when focus reaches the field.
   *
   * Off by default, because a rule that is always true is a description. A
   * description read on every focus is noise. On for a hint that appears in
   * response to something the reader did. The caps-lock warning under a
   * password is the case this exists for, and the reason is its timing. Caps
   * lock gets pressed while typing. A warning heard only after leaving the
   * field and coming back arrives after the password it was about.
   */
  hintLive?: boolean;
  /**
   * Why the value in this field was refused.
   *
   * A separate slot from `hint`, because the two say different things and were
   * spelled the same way. The password forms put "too short" and "the
   * passwords do not match" through `hint`. So a refusal rendered in the same
   * meta-grey as neutral helper text. On one screen it was the same grey as the
   * success line four elements above it. This one announces, marks the control
   * `aria-invalid`, and reads in the danger tone. The hint stays visible beside
   * it, because what the field wants is still true while it is wrong.
   */
  error?: string;
  /**
   * A leading affordance inside the control's outline, such as a mail glyph on
   * an address field. Decorative: the label names the field.
   */
  icon?: ReactNode;
  /**
   * A control inside the outline at the trailing end: the password reveal.
   *
   * This is what `auth.tsx` forked its own `Field` for. `.input-icon` could
   * carry a leading glyph and nothing else. So a button that has to sit inside
   * the focus ring had no way to get there. A second field component grew on
   * the sign-in screens, with its own label size and its own gap. Its hint was
   * never wired to `aria-describedby`.
   */
  trailing?: ReactNode;
  required?: boolean;
  // Layout the surrounding form owns: a width, a grid span, a screen's own
  // field modifier. It lands on the wrapper, which is the only element a
  // caller has any business positioning.
  className?: string;
  children: (control: FieldControl) => ReactNode;
}>) {
  const id = useId();
  const hintId = hint ? `${id}-hint` : undefined;
  const errorId = error ? `${id}-error` : undefined;
  // Both, when both are on screen. A field that is refused and still carries
  // its rule has two things to say. Naming only one in `aria-describedby`
  // picks which one sighted and non-sighted readers get.
  const describedBy = describedByIds(errorId, hintId);
  const control = children({
    id,
    required,
    "aria-describedby": describedBy,
    "aria-invalid": error ? true : undefined,
  });
  const labelClass = labelHidden ? "sr-only" : "t-label";
  return (
    <div className={["field", className ?? ""].filter(Boolean).join(" ")}>
      {labelEnd ? (
        <span className="field-label-row">
          <label className={labelClass} htmlFor={id}>
            {label}
            {required && <span aria-hidden> *</span>}
          </label>
          {labelEnd}
        </span>
      ) : (
        <label className={labelClass} htmlFor={id}>
          {label}
          {required && <span aria-hidden> *</span>}
        </label>
      )}
      {/* The shell exists only when something has to sit inside the outline. A
          field with neither affordance emits the markup it always did. That
          keeps the two hundred existing call sites unchanged: the border and
          the focus ring stay on the input there. They move to the shell only
          where an icon or a control would otherwise sit beside the outline
          rather than within it. */}
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
