// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "./waterfall.css";

// A total, what moved it, and the total it became.
//
// The bars between the two anchors are a CLAIM: that these named causes account
// for the whole difference. A waterfall whose steps do not reach the closing
// anchor is a picture that has to be explained away, so this component checks
// the arithmetic it is drawing and says so on the page when it does not hold.
//
// That check runs in production, not behind a development flag. A dev-only
// assertion is compiled out of the shipped build, which leaves a manager
// looking at bars that quietly do not add up — the exact reader who cannot
// tell.
export function Waterfall({
  opening,
  closing,
  steps,
  label,
  reconciliationWarning,
}: Readonly<{
  opening: WaterfallAnchor;
  closing: WaterfallAnchor;
  // The named causes, in the order they are drawn. Order is the caller's:
  // a waterfall is read left to right as a story, and re-sorting it here would
  // tell a different one.
  steps: readonly WaterfallStep[];
  // What the whole figure is a reading OF, for a reader who meets it without
  // the surrounding card — and the caption of the table equivalent.
  label: string;
  // Shown when the steps do not reach the closing anchor. Passed in rather
  // than spelled here, because this tier holds no copy and the sentence has to
  // arrive in the reader's own language.
  reconciliationWarning: string;
}>) {
  const stepped = steps.reduce((sum, step) => sum + step.value, 0);
  const reconciles = opening.value + stepped === closing.value;

  // Every bar is drawn against the largest absolute figure in the picture, so
  // a step's height is comparable to the anchors it sits between. Scaled to
  // itself, a small step would draw as tall as the total it moved.
  const scale = Math.max(
    Math.abs(opening.value),
    Math.abs(closing.value),
    ...steps.map((step) => Math.abs(step.value)),
    1,
  );

  return (
    <div className="waterfall">
      {/* The bars carry the shape; the table carries the figures. A reader on
          a screen reader gets the table, which is the one with the values. */}
      <ol className="waterfall-bars" aria-hidden="true">
        <li className="waterfall-bar waterfall-anchor">
          <span className="waterfall-label">{opening.label}</span>
          <span
            className="waterfall-fill"
            style={{ height: `${(Math.abs(opening.value) / scale) * 100}%` }}
          />
        </li>
        {steps.map((step) => (
          <li key={step.key} className={stepClass(step)}>
            <span className="waterfall-label">{step.label}</span>
            <span
              className="waterfall-fill"
              style={{ height: `${(Math.abs(step.value) / scale) * 100}%` }}
            />
          </li>
        ))}
        <li className="waterfall-bar waterfall-anchor">
          <span className="waterfall-label">{closing.label}</span>
          <span
            className="waterfall-fill"
            style={{ height: `${(Math.abs(closing.value) / scale) * 100}%` }}
          />
        </li>
      </ol>

      {/* Visible, and in the reader's language. The alternative to saying this
          on the page is a picture that is quietly wrong. */}
      {!reconciles && (
        <p className="waterfall-warning" role="status">
          {reconciliationWarning}
        </p>
      )}

      <table className="sr-only">
        <caption>{label}</caption>
        <tbody>
          <tr>
            <th scope="row">{opening.label}</th>
            <td>{opening.amount}</td>
          </tr>
          {steps.map((step) => (
            <tr key={step.key}>
              <th scope="row">{step.label}</th>
              <td>{step.amount}</td>
            </tr>
          ))}
          <tr>
            <th scope="row">{closing.label}</th>
            <td>{closing.amount}</td>
          </tr>
        </tbody>
      </table>
    </div>
  );
}

// stepClass gives a step its direction. Tone comes from the SIGN rather than
// from the caller, because a waterfall's up and down are what it means: a
// caller free to colour a negative step as positive could draw a loss as a
// gain.
function stepClass(step: WaterfallStep): string {
  const direction = step.value < 0 ? "down" : "up";
  return `waterfall-bar waterfall-${direction}`;
}

// WaterfallAnchor is a total the picture starts or ends at.
export type WaterfallAnchor = Readonly<{
  label: string;
  // The figure the bar is drawn from, in the unit the whole picture counts in.
  value: number;
  // The same figure, spelled for a human by the caller's own formatter.
  amount: string;
}>;

// WaterfallStep is one named cause and what it moved.
export type WaterfallStep = Readonly<{
  // Identity, separate from the label: two steps may legitimately read the
  // same, and a list keyed by its display text loses one of them.
  key: string;
  label: string;
  value: number;
  amount: string;
}>;
