import type { LucideIcon } from "lucide-react";
import type { ReactNode } from "react";
import "./readings.css";
import { webUrl } from "../format/weburl";

// Three ways to draw a reading that is not a number in a box: a proportion as
// a bar, a series as a line, and a labelled attribute as a pill. Copy always
// arrives through props — these never hard-code a word.

// A proportion, drawn as a bar. `value` and `max` are the two halves of the
// same fact (7 of 9 inputs present), so the caller passes both rather than a
// pre-computed percentage: the bar and the label a caller writes beside it
// then cannot disagree, and a zero max reads as an empty bar instead of NaN.
export function Meter({
  value,
  max,
  label,
  tone,
  flat,
  dense,
  restTone,
}: Readonly<{
  value: number;
  max: number;
  label: string;
  // What colour the FILL takes. The accent gradient by default; "warn" and
  // "danger" for a reading the caller has decided is bad news at this value,
  // whichever end that is — a coverage bar that has run low, an overdue bar
  // that has run high.
  tone?: "warn" | "danger";
  // The gradient's second colour (`--away`) reads as a warning creeping in at
  // the high end, which is wrong for a reading with no low-is-bad meaning.
  // `flat` keeps the accent solid instead of fading toward it.
  flat?: boolean;
  // The bar as a LABEL'S OWN bar rather than a block of its own: thinner, and
  // with none of the vertical interval the default pays for standing alone. A
  // row per dimension — the company rail's health readings, the growth-fit
  // sub-scores — is two lines tall with this and three without.
  //
  // A size on the primitive rather than a height each caller sets, because two
  // screen sheets independently reached into `.meterbar` for the same 6px and
  // the same `margin: 0`, and a geometry with two authors drifts the first time
  // either moves.
  dense?: boolean;
  // What the TRACK is. Unset, it is empty space — the part of the whole this
  // reading has not reached, drawn recessed because it means nothing on its
  // own. Set, the remainder is itself a value the caller names beside the bar
  // (overdue against open: what is left is money that is simply not late yet),
  // so it takes a colour and the bar reads as two facts rather than one fact
  // and a gutter.
  restTone?: "accent";
}>) {
  const filled = max > 0 ? Math.min(100, Math.max(0, (value / max) * 100)) : 0;
  // Not a native <meter>: its children are fallback content that a supporting
  // engine never renders, so the token-drawn fill below would simply vanish,
  // and the bar every engine draws in its place takes none of our colours —
  // the same reason `<select>` is banned outright. The role and the value
  // triple carry the reading to assistive tech instead.
  return (
    // biome-ignore lint/a11y/useSemanticElements: <meter> discards the token-drawn fill and draws its own untokenised bar
    <div
      className={meterClass({ tone, flat, dense, restTone })}
      role="meter"
      aria-label={label}
      aria-valuenow={value}
      aria-valuemin={0}
      aria-valuemax={max}
    >
      <span style={{ width: `${filled}%` }} />
    </div>
  );
}

// The bar's four independent choices — fill colour, gradient or solid, its
// geometry, and whether the track carries a meaning — as one class string.
// Spelled out here rather than nested in the element, where a third condition
// turned a ternary into something nobody could read at a glance. Named rather
// than positional: four optional arguments in a row is a call site where a
// reader cannot tell which flag is which.
function meterClass({
  tone,
  flat,
  dense,
  restTone,
}: Readonly<{
  tone?: "warn" | "danger";
  flat?: boolean;
  dense?: boolean;
  restTone?: "accent";
}>): string {
  const classes = ["meterbar"];
  if (tone) {
    classes.push(`meterbar-${tone}`);
  } else if (flat) {
    classes.push("meterbar-flat");
  }
  if (dense) {
    classes.push("meterbar-dense");
  }
  if (restTone) {
    classes.push(`meterbar-rest-${restTone}`);
  }
  return classes.join(" ");
}

// A short series as a bare polyline: the shape of a trend, with no axes, no
// grid and no tooltip. It is a glyph, not a chart — a reader who needs the
// figures reads the table beside it, which is why the whole thing is
// aria-hidden behind the caller's own text label.
//
// Fewer than two points draws nothing: a single point is a dot the reader
// would read as a flat trend, which is a claim the data does not make.
export function Sparkline({
  points,
  label,
}: Readonly<{ points: readonly number[]; label: string }>) {
  if (points.length < 2) {
    return null;
  }
  const low = Math.min(...points);
  const high = Math.max(...points);
  // A flat series has no range to scale into; drawing it down the middle says
  // "unchanged", which is exactly what it is.
  const span = high - low || 1;
  const step = SPARK_WIDTH / (points.length - 1);
  const path = points
    .map((point, index) => {
      const x = (index * step).toFixed(1);
      const y = (
        SPARK_HEIGHT -
        ((point - low) / span) * (SPARK_HEIGHT - SPARK_INSET * 2) -
        SPARK_INSET
      ).toFixed(1);
      return `${x},${y}`;
    })
    .join(" ");
  return (
    <svg
      className="sparkline"
      viewBox={`0 0 ${SPARK_WIDTH} ${SPARK_HEIGHT}`}
      preserveAspectRatio="none"
      role="img"
      aria-label={label}
    >
      <polyline points={path} />
    </svg>
  );
}

const SPARK_WIDTH = 120;
const SPARK_HEIGHT = 32;
// Keeps the stroke off the top and bottom edges, where a 2px line drawn at
// the extreme would be clipped in half by the viewBox.
const SPARK_INSET = 3;

// An attribute of a record with the icon that names its kind: the website, the
// LinkedIn page, the location, the industry, the headcount. Distinct from
// `Badge`, which is a status in a closed tone vocabulary — a Chip is a fact,
// and it is a link when the fact has somewhere to go.
export function Chip({
  icon: Icon,
  children,
  href,
}: Readonly<{
  icon: LucideIcon;
  children: ReactNode;
  // An external destination. Present → the chip is an anchor and opens in a
  // new tab with `noreferrer`, since these point off our origin.
  href?: string;
}>) {
  const body = (
    <>
      <Icon size={14} aria-hidden="true" />
      <span>{children}</span>
    </>
  );
  const destination = href && webUrl(href) ? href : undefined;
  if (destination) {
    return (
      <a
        className="chip chip-link"
        href={destination}
        target="_blank"
        rel="noreferrer"
      >
        {body}
      </a>
    );
  }
  // A chip whose href was refused still shows the FACT — the reader loses the
  // link, not the value.
  return <span className="chip">{body}</span>;
}

// A ranked set of one-dimensional readings: label, bar, formatted amount, one
// row each. `Meter` draws one such bar; this is what a caller reaches for when
// there are several and the point is COMPARING them, which is why the rows
// share one denominator rather than each scaling to itself. Rows that each fill
// their own track are N readings; rows on one scale are a ranking.
//
// The amount arrives already formatted, because money and counts are the two
// things this list holds and both are the caller's locale to spell. Handing the
// component a number and a currency would make it the second author of a format
// the `format/` module owns.
export function BarList({
  rows,
  label,
  max,
}: Readonly<{
  rows: readonly BarListRow[];
  // What the whole list is a reading OF, for a reader who meets it without the
  // surrounding card — and the caption of the table equivalent below.
  label: string;
  // The denominator every bar is drawn against. Optional: the default is the
  // largest row, which makes the list a ranking. Pass it where the whole is a
  // known figure the rows do not reach — a stage's share of a pipeline total —
  // or the longest bar claims to be everything.
  max?: number;
}>) {
  // The bars would otherwise each scale to themselves, and `Meter` clamps at
  // its max, so an explicit max BELOW the largest row is a caller error that
  // silently pins several rows at full. Taking the larger of the two keeps
  // every row's share honest and leaves the caller's whole visible where it
  // is the bigger number.
  const largestRow = rows.reduce((high, row) => Math.max(high, row.value), 0);
  const denominator = Math.max(max ?? 0, largestRow);
  return (
    <div className="barlist">
      {/* The bars carry the shape and the table carries the figures. A reader
          on a screen reader gets the second, which is the one with the values
          in it — so the bars are hidden rather than announced twice. */}
      <ul className="barlist-rows" aria-hidden="true">
        {rows.map((row) => (
          <li key={row.key} className="barlist-row">
            <span className="barlist-label">{row.label}</span>
            <Meter
              value={row.value}
              max={denominator}
              label={row.label}
              tone={row.tone}
              dense
              flat
            />
            <span className="barlist-amount num">{row.amount}</span>
          </li>
        ))}
      </ul>
      <table className="sr-only">
        <caption>{label}</caption>
        <tbody>
          {rows.map((row) => (
            <tr key={row.key}>
              <th scope="row">{row.label}</th>
              <td>{row.amount}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

export type BarListRow = Readonly<{
  // Identity, separate from the label: two rows may legitimately read the same
  // (two stages named "Qualified" in different pipelines) and a list keyed by
  // its own display text loses one of them.
  key: string;
  label: string;
  // The figure the bar is drawn from, in whatever unit the list is counting.
  value: number;
  // The same figure, spelled for a human by the caller's own formatter.
  amount: string;
  tone?: "warn" | "danger";
}>;
