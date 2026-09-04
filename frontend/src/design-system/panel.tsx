// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import { Eyebrow } from "./eyebrow";
import {
  type SectionDetail,
  type SectionState,
  SurfaceState,
} from "./surfacestate";
import "./panel.css";

// Panel is the titled-card shape Card does not offer: a fixed-height header
// row (a title alone, a title with a badge, or a title with a button all read
// the same height), full-bleed rows under it, and an optional footer for a
// figure that belongs to the whole panel rather than to any one row.
//
// That height is a FLOOR rather than a fixed measure, and a description under
// the title is the one thing that raises it: type on two lines wedged into the
// band would sit against the hairline above and the border below. Everything
// that shares the title's own line — a badge, a button, a count — reads at the
// one height, which is the promise the band exists to make.
//
// The header and the body are two different rhythms living in one box — the
// header's own 48px band versus the body's padded content versus a row that
// wants to touch the panel's own edges — which is why the padded content is a
// separate `PanelBody` rather than a prop: a caller who needs both padded text
// and full-bleed rows in the same panel nests `PanelBody` and `PanelRow` as
// siblings instead of fighting one slot that tries to be both.
export function Panel({
  title,
  sub,
  titleAction,
  tone,
  titleLevel,
  actions,
  footer,
  children,
  className,
}: Readonly<{
  title?: ReactNode;
  // One line of description under the title, inside the same header band. It
  // is here so that a card whose anatomy is otherwise exactly this one's —
  // header band, full-bleed rows, a footer carrying the total — does not have
  // to be a `Card` for the sake of one sentence. That trade is how the record
  // page and settings came to draw two different cards: the sentence was the
  // only thing Panel could not hold, so the caller changed surface instead of
  // asking for the slot.
  //
  // A caller-translated node, like every other slot in this file. No copy
  // lives in a primitive.
  sub?: ReactNode;
  // Rendered right-aligned in the header, beside the title — a badge, a
  // button, a count. Absent leaves the title alone in its row.
  titleAction?: ReactNode;
  // The LEAD panel's tint: the one card on a page that ASKS FOR A MOVE rather
  // than reporting state, drawn with a tinted border, a tinted header band and
  // the title at reading size so a reader finds it before the panels around
  // it. This is not a palette — a second tinted panel on the same page is two
  // leads, which is none.
  //
  // The three tones are three kinds of lead, not three colours to choose from:
  // "accent" is the ordinary ask; "warn" is a lead whose FINDING is the bad
  // news — a relationship that went quiet, a promise that is late — where the
  // tone is the reading rather than decoration on it; and "ai" is a panel a
  // MACHINE wrote or read, which is a fact about its authorship rather than
  // about the account. That last one is why "ai" is not simply a third accent:
  // an indigo band means "Margince did this" everywhere in the product, so it
  // must never be reached for to make an ordinary panel look important.
  //
  // It is a prop rather than a class a screen sheet adds because the tint has
  // to reach `.panel-head` and `.panel-foot`, which are this component's own
  // internals: a screen reaching into them is a second author for a rhythm
  // this file owns, and the two drift the first time either moves.
  tone?: "accent" | "warn" | "ai";
  // Which heading level the title takes. A panel names a section of the page,
  // so h2 is right on a page — and wrong inside a dialog, where the dialog's
  // own title is already the h2 these sit under. A caller that knows its
  // surrounding outline says so rather than leaving a reader on a screen
  // reader two h2s that are not siblings.
  titleLevel?: 2 | 3;
  // Verbs that CHANGE this panel, in their own band under the body — not one
  // more row, and not a footer, which reports rather than acts. A caller
  // renders them only when the panel's content is real: an "add a deal"
  // button under a section whose read failed offers a write nobody can say
  // makes sense.
  actions?: ReactNode;
  // A figure or a link that belongs to the SECTION rather than to any one row
  // — a lifetime total, a "see all" link — so it sits below the rows in its
  // own band rather than as one more row.
  footer?: ReactNode;
  children: ReactNode;
  className?: string;
}>) {
  return (
    <section
      className={["panel", tone ? `panel-${tone}` : "", className ?? ""]
        .filter(Boolean)
        .join(" ")}
    >
      {title && (
        <header className="panel-head">
          {/* The title and its description are ONE item in the header row, not
              two: the row's far-end push anchors on this block, so a
              titleAction lands at the end whether or not a description is
              there. Rendered even with no `sub`, because a wrapper that comes
              and goes is a second header shape, and the height the band
              guarantees is measured on this one. */}
          <div className="panel-head-text">
            {titleLevel === 3 ? (
              <h3 className="panel-title">{title}</h3>
            ) : (
              <h2 className="panel-title">{title}</h2>
            )}
            {sub && <span className="panel-head-sub">{sub}</span>}
          </div>
          {titleAction}
        </header>
      )}
      {children}
      {actions && <div className="panel-actions">{actions}</div>}
      {footer && <footer className="panel-foot">{footer}</footer>}
    </section>
  );
}

// PanelPlate is the recessed plate inside a panel, inset from its edges: what
// IS, set apart from what to DO. The device is the whole point of it — the
// rows below run full-bleed on the panel's own ground and read as pressable,
// the plate does not, and a reader can tell the two halves apart before
// reading a word of either. It holds context, never a control.
export function PanelPlate({
  children,
  className,
}: Readonly<{ children: ReactNode; className?: string }>) {
  return (
    <div className={["panel-plate", className ?? ""].filter(Boolean).join(" ")}>
      {children}
    </div>
  );
}

// PanelGroupHead names one GROUP inside a pane — the deals, then the projects,
// under the one head that names the pane. One level in from the pane's own
// title, as an eyebrow, so the groups read as parts of one reading rather than
// as two more panes; and its verb rides the same line, so a group keeps one
// place for it whatever state its rows are in — moved into an empty plate it
// changed position with the content, and a reader who has just read one group
// looks for the next verb where the last one was.
export function PanelGroupHead({
  title,
  level,
  action,
}: Readonly<{
  title: string;
  // Where the group sits in the outline: one under whatever heads the pane.
  level: "h3" | "h4";
  // The verb that opens one of these. Absent on a record nobody may write to.
  action?: ReactNode;
}>) {
  return (
    <PanelBody className="panel-grouphead">
      <Eyebrow as={level}>{title}</Eyebrow>
      {action}
    </PanelBody>
  );
}

// PanelBody is the padded content slot: text, a form, a FieldGrid — anything
// that is not a row and wants the panel's inner margin. Rows are passed as
// Panel's direct children instead, so they can run full-bleed against the
// panel's own edges.
export function PanelBody({
  children,
  className,
}: Readonly<{ children: ReactNode; className?: string }>) {
  return (
    <div className={["panel-body", className ?? ""].filter(Boolean).join(" ")}>
      {children}
    </div>
  );
}

// PanelRow is the hairline row every list inside a panel wants: content that
// runs edge to edge rather than sitting in the body's padding, with a rule
// against the row above it (none on the first). The rule itself is inset to the
// panel's padding, like every rule BETWEEN two pieces of a card's content — see
// the seam rule in panel.css. The header's and the footer's rules are the card's
// own chrome and stay edge to edge.
//
// A row is INERT unless the caller says otherwise, which is the reverse of what
// this component shipped with. The hover fill was unconditional, so a panel
// whose ruled blocks are read rather than clicked told the reader every one of
// them was pressable — and that was most of them: of the thirty-odd rows in
// this tree exactly one is a single press target filling its row. The rest
// carry a checkbox, a switch, a name that navigates, a verb at the far end —
// sub-targets with their own hover and focus states, under a row that is not
// itself a target.
export function PanelRow({
  interactive,
  children,
  className,
}: Readonly<{
  // The WHOLE row is one press target — a button or a link that fills it, so
  // pointing anywhere in the row aims at the same thing. That is what earns
  // the hover fill: the fill says "this, all of it, is what you would hit".
  //
  // A row that merely CONTAINS a control is not this. Its control draws its
  // own hover, and a fill behind it claims a hit area the row does not have.
  interactive?: boolean;
  children: ReactNode;
  className?: string;
}>) {
  return (
    <div
      className={[
        "panel-row",
        interactive ? "panel-row-interactive" : "",
        className ?? "",
      ]
        .filter(Boolean)
        .join(" ")}
    >
      {children}
    </div>
  );
}

/**
 * RailPanel is a Panel that knows the difference between "there is nothing
 * here" and "you may not read this" — the distinction that makes a record page
 * honest. A section the caller's role cannot read is ABSENT from the payload
 * and named in `sections_omitted`, so the card says "hidden from you" instead
 * of drawing an empty list that reads as "there is none".
 *
 * The message states (empty, withheld, unavailable, loading, failed) are
 * SurfaceState verbatim, padded in a PanelBody; `ready` is left to the caller,
 * so rows passed as children run edge to edge the way Panel is built to take
 * them.
 */
export function RailPanel({
  title,
  state,
  emptyLabel,
  detail,
  footer,
  children,
}: Readonly<{
  title: string;
  state: SectionState;
  emptyLabel: string;
  detail?: SectionDetail;
  // A figure belonging to the whole card rather than to one row. Shown only
  // on `ready`/`empty` — the states RailPanel's callers ever reach — because a
  // withheld or unavailable section has no figure to report either.
  footer?: ReactNode;
  children: ReactNode;
}>) {
  const present = state === "ready" || state === "empty";
  return (
    <Panel title={title} footer={present ? footer : undefined}>
      {state === "ready" ? (
        children
      ) : (
        <PanelBody>
          <SurfaceState state={state} emptyLabel={emptyLabel} detail={detail}>
            {null}
          </SurfaceState>
        </PanelBody>
      )}
    </Panel>
  );
}
