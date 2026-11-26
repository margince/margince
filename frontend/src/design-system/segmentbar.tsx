import "./readings.css";
import { Button } from "./atoms";
import { percentOf } from "./readings";

// Parts of one total laid end to end on one track, with a mark where a target
// sits. The parts are disjoint: their lengths add up to the whole the track
// shows, so a caller never passes sets that overlap. They run from most to
// least certain, and their colour steps down one hue.
//
// The track is aria-hidden: the legend carries every label and figure as text.
export function SegmentBar({
  parts,
  marker,
  ahead,
  label,
  onSelect,
  legend = "shown",
}: Readonly<{
  parts: SegmentBarParts;
  // "hidden" keeps the legend for a screen reader alone, where readings
  // beside the bar already name each part under a `SegmentSwatch`.
  legend?: "shown" | "hidden";
  onSelect?: (key: string) => void;
  // Not counted in the parts yet, such as open deals beside won: an outline after them.
  ahead?: SegmentBarPart;
  // A figure the parts are read against, not one of them. It is a line across
  // the track, drawn in ink so it never reads as a fourth part.
  marker?: SegmentBarPart;
  // What the legend is, for a reader who meets it as a list.
  label: string;
}>) {
  const total = parts.reduce((sum, part) => sum + Math.max(0, part.value), 0);
  const open = Math.max(0, ahead?.value ?? 0);
  // The scale is the larger of the parts with what is ahead, and the mark. A
  // mark beyond them leaves recessed track between, and that track is the gap.
  const scale = Math.max(total + open, marker?.value ?? 0);
  return (
    <div className="segbar">
      <div className="segbar-track" aria-hidden={onSelect ? undefined : true}>
        {parts.map((part, step) =>
          onSelect ? (
            <Button
              key={part.key}
              variant="link"
              className={segmentActionClass(step)}
              style={{ width: `${percentOf(part.value, scale)}%` }}
              aria-label={`${part.label}: ${part.amount}`}
              onClick={() => onSelect(part.key)}
            />
          ) : (
            <span
              key={part.key}
              className={`segbar-part segbar-step-${step}`}
              style={{ width: `${percentOf(part.value, scale)}%` }}
            />
          ),
        )}
        {ahead && open > 0 && (
          <span
            className="segbar-part segbar-ahead"
            style={{ width: `${percentOf(ahead.value, scale)}%` }}
          />
        )}
        {marker && (
          <span
            className="segbar-marker"
            style={{ left: `${percentOf(marker.value, scale)}%` }}
          />
        )}
      </div>
      <ul
        className={
          legend === "hidden" ? "segbar-legend sr-only" : "segbar-legend"
        }
        aria-label={label}
      >
        {parts.map((part, step) => (
          <li key={part.key} className="segbar-key">
            <span
              className={`segbar-swatch segbar-step-${step}`}
              aria-hidden="true"
            />
            {part.label}
            <span className="segbar-amount t-num">{part.amount}</span>
          </li>
        ))}
        {ahead && (
          <li className="segbar-key">
            <span className="segbar-swatch segbar-ahead" aria-hidden="true" />
            {ahead.label}
            <span className="segbar-amount t-num">{ahead.amount}</span>
          </li>
        )}
        {marker && (
          <li className="segbar-key">
            <span className="segbar-swatch-marker" aria-hidden="true" />
            {marker.label}
            <span className="segbar-amount t-num">{marker.amount}</span>
          </li>
        )}
      </ul>
    </div>
  );
}

/** The colour key of one `SegmentBar` part, beside a reading that names it. */
export function SegmentSwatch({
  step,
}: Readonly<{ step: 0 | 1 | 2 | "ahead" }>) {
  const tone = step === "ahead" ? "segbar-ahead" : `segbar-step-${step}`;
  return <span className={`segbar-swatch ${tone}`} aria-hidden="true" />;
}

export type SegmentBarPart = Readonly<{
  key: string;
  label: string;
  value: number;
  amount: string;
}>;

// One to three parts, as a type: a fourth step of one hue is not told apart.
export type SegmentBarParts =
  | readonly [SegmentBarPart]
  | readonly [SegmentBarPart, SegmentBarPart]
  | readonly [SegmentBarPart, SegmentBarPart, SegmentBarPart];

function segmentActionClass(step: number): string {
  return `segbar-part segbar-action segbar-step-${step}`;
}
