import { type LucideIcon, Sparkles } from "lucide-react";
import type {
  CSSProperties,
  ElementType,
  FormEventHandler,
  ReactNode,
} from "react";
import { formatNumber } from "../format/format";
import { useLocale } from "../i18n";
import { Heading, type HeadingElement, type HeadingSize } from "./heading";
import { useWaited } from "./waited";
import "./badge.css";
import "./atoms.css";
import "./evidencemark.css";

// The Margince atom library (B-EP09.2, re-scoped to our own system, no gw-ui
// port; atoms are added as screens need them). Copy always arrives through
// props — callers translate with t(); atoms never hard-code user-facing words.

// These parts live in their own modules and are re-exported here, so a
// caller keeps one import path for every atom.
export { Avatar } from "./avatar";
export { BusyMark, Button, type ButtonVariant } from "./button";
export {
  Checkbox,
  Field,
  type FieldControl,
  Radio,
  SearchField,
  Textarea,
  TextInput,
} from "./field";
export { OverflowMenu } from "./overflowmenu";
export { TableScroll } from "./tablescroll";

/**
 * The closed tone vocabulary, as a value the type is read from. The story and
 * the gate that walk every tone then share the list the compiler enforces.
 *
 * `discovery` says a thing is new. It reports no verdict: a status says how
 * something is going, this says how long it has been here.
 *
 * `info` is the quiet blue and never a status, which is one of the four kinds
 * of `StatusBadge`. It is not `accent`: accent is the brand asking for a press.
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

type PlainTone = Exclude<BadgeTone, "ai">;

// One leading mark, a glyph or the `live` dot; an `ai` badge's is Sparkles.
type BadgeMark =
  | { tone?: PlainTone; icon?: LucideIcon; live?: never; dot?: never }
  | { tone?: PlainTone; icon?: never; live?: boolean; dot?: boolean }
  | { tone: "ai"; icon?: never; live?: never; dot?: never };

export function Badge({
  variant = "soft",
  tone = "default",
  icon,
  live,
  dot,
  wrap,
  children,
}: Readonly<
  {
    // `soft` tints a status in prose or a column; `primary` fills a must-see.
    variant?: "soft" | "primary";
    wrap?: boolean;
    children: ReactNode;
  } & BadgeMark
>) {
  // `live` is true AS THE PAGE IS READ: the one place motion is a fact.
  const provenance = tone === "ai";
  const Icon = provenance ? Sparkles : icon;
  const classes = [
    "badge",
    variant === "primary" && "badge-primary",
    tone !== "default" && `badge-${tone}`,
    wrap && "badge-wrap",
  ].filter(Boolean);
  return (
    <span className={classes.join(" ")}>
      {live && !provenance && <span className="badge-live-dot" aria-hidden />}
      {dot && !live && !provenance && (
        <span className="badge-dot" aria-hidden />
      )}
      {Icon && <Icon size={12} aria-hidden="true" />}
      <span className="badge-label">{children}</span>
    </span>
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
  const waited = useWaited(delayMs);
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

// Re-exported so call sites that name only `Disclosure` keep one import path.
export { Disclosure } from "./disclosure";
export { Modal } from "./modal";
export { useScrollRegion } from "./scrollregion";
