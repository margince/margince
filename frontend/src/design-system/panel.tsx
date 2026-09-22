// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type ReactNode, useId } from "react";
import { Eyebrow } from "./eyebrow";
import { Heading, type HeadingElement } from "./heading";
import {
  type SectionDetail,
  type SectionState,
  SurfaceState,
} from "./surfacestate";
import "./panel.css";

/**
 * The lead vocabulary, as a VALUE the type is read from — so the story that
 * draws every tone and the gate that holds a tone to tinting rather than
 * reshaping walk the same list the compiler enforces.
 *
 * The five states come first and say what the FINDING is; `accent` is the
 * ordinary brand ask with no finding behind it; `ai` is a claim about who wrote
 * the panel rather than about the account.
 */
export const PANEL_TONES = [
  "accent",
  "info",
  "success",
  "warning",
  "danger",
  "discovery",
  "ai",
] as const;

export type PanelTone = (typeof PANEL_TONES)[number];

// Which element the title takes at each level. A table rather than an element
// name built from the number, which would render a raw magnitude — and the
// SIZE is not in it, because the head's type does not step with the outline: a
// panel head is a panel head at either depth.
const TITLE_ELEMENT: Readonly<Record<2 | 3, HeadingElement>> = {
  2: "h2",
  3: "h3",
};

// Panel is the titled-card shape Card does not offer: a header band, full-bleed
// rows under it, and an optional footer for a figure that belongs to the whole
// panel rather than to any one row.
//
// THE HEAD IS A TITLE AND, OPTIONALLY, THE VERBS THAT ACT ON IT. Never a
// description: the title and the body are what give the panel its meaning, and
// a sentence between them is a third voice saying what one of the two already
// said. So the band carries a title alone or a title beside a badge or a
// button, and both draw the same fixed `--panel-head-h`, which is what makes a
// page of panels read as a column of titles at one interval. A floor is what
// this used to be, and a floor is an invitation — a description raised the band
// here, a screen re-spaced it there, and the same card stood at three heights
// on one page. Whatever does not fit on one band is not header content: it goes
// in the body, as a toolbar row or a `PanelBody`. What overruns INSIDE the band
// ends in an ellipsis, and only the title gives way — a badge squeezed to buy
// the title room reads as a different control, or loses its label outright.
//
// The header and the body are two different rhythms living in one box — that
// band versus the body's padded content versus a row that wants to touch the
// panel's own edges — which is why the padded content is a separate
// `PanelBody` rather than a prop: a caller who needs both padded text and
// full-bleed rows in the same panel nests `PanelBody` and `PanelRow` as
// siblings instead of fighting one slot that tries to be both.
export function Panel({
  title,
  titleAction,
  tone,
  titleLevel,
  actions,
  footer,
  children,
  className,
}: Readonly<{
  title?: ReactNode;
  // Rendered right-aligned in the header, beside the title — a badge, a
  // button, a count. Absent leaves the title alone in its row.
  titleAction?: ReactNode;
  // The LEAD panel's tint: the one card on a page that ASKS FOR A MOVE rather
  // than reporting state, drawn with a tinted border, a tinted header band and
  // the title in the tone's own ink, so a reader finds it before the panels
  // around it. A tone only TINTS — the band's measure and the title's size are
  // the head's, whatever the tone. This is not a palette either: a second
  // tinted panel on the same page is two leads, which is none.
  //
  // The tones are kinds of lead, not colours to choose from. "accent" is the
  // ordinary ask, with no finding behind it. The five states say what the
  // FINDING is — "warning" for the bad news that can still be stopped, "danger"
  // for the one that cannot, "success" for the lead whose finding is that the
  // thing landed, "info" for the neutral report and the work still running,
  // "discovery" for a capability this reader has not met — where the tone is the
  // reading rather than decoration on it. And "ai" is a panel a MACHINE wrote or
  // read, which is a fact about its authorship rather than about the account.
  // That last one is why "ai" is not simply a second accent: an indigo band
  // means "Margince did this" everywhere in the product, so it must never be
  // reached for to make an ordinary panel look important.
  //
  // It is a prop rather than a class a screen sheet adds because the tint has
  // to reach `.panel-head` and `.panel-foot`, which are this component's own
  // internals: a screen reaching into them is a second author for a rhythm
  // this file owns, and the two drift the first time either moves.
  tone?: PanelTone;
  // Which heading level the title takes. A panel names a section of the page,
  // so h2 is right on a page — and wrong inside a dialog, where the dialog's
  // own title is already the h2 these sit under. A caller that knows its
  // surrounding outline says so rather than leaving a reader on a screen
  // reader two h2s that are not siblings.
  //
  // It moves the ELEMENT and nothing else. The band is one measure and its
  // title is one size, so a nested panel does not announce its depth by
  // shrinking — the outline says where it sits.
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
  // A titled panel is a LANDMARK, named by the title a reader already sees. A
  // page of panels is then a list of regions a screen-reader user jumps
  // between by name, which is the same way a sighted reader scans the column
  // of titles. A bare `<section>` has no role at all until it has an
  // accessible name, so without this the zones were invisible to that jump.
  //
  // The name comes from the title element rather than from a label prop: one
  // source, so the spoken name and the printed one cannot disagree. An
  // UNTITLED panel claims no landmark — it is a container the caller has
  // chosen not to name, and a nameless region in the list is worse than none.
  const titleId = useId();
  return (
    <section
      className={["panel", tone ? `panel-${tone}` : "", className ?? ""]
        .filter(Boolean)
        .join(" ")}
      aria-labelledby={title ? titleId : undefined}
    >
      {title && (
        <header className="panel-head">
          {/* The title is the row's far-end push: it absorbs the free space, so
              a titleAction lands at the end of the band whatever the title's
              length. */}
          <Heading
            size="medium"
            as={TITLE_ELEMENT[titleLevel ?? 2]}
            className="panel-title"
            id={titleId}
          >
            {title}
          </Heading>
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

// PanelIntro is the one descriptive line a panel may carry: the sentence that
// says what the zone is, standing first in a `PanelBody`, above the form or the
// rows it describes. A BODY child and never a prop, because the head is a title
// and the verbs that act on it — a description up there is a head at two
// heights, and `panelhead.test.tsx` holds that. Two may stack, a reading and
// the posture a reader holds over it, and the second is still one of these.
//
// The interval below it belongs to this component rather than to the screen
// writing the line: seventy-odd of these were spaced from one screen sheet, and
// three screens had each spelled a correction to that margin of their own.
export function PanelIntro({
  children,
  className,
}: Readonly<{ children: ReactNode; className?: string }>) {
  return (
    <p className={["panel-intro", className ?? ""].filter(Boolean).join(" ")}>
      {children}
    </p>
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
/**
 * The states a RailPanel can render.
 *
 * `stale` and `partial` are deliberately absent. SurfaceState renders each of
 * them as a caveat WITH the rows beside it — "as of Tuesday" over the figures,
 * "4 more" under them — and RailPanel hands its rows to Panel undecorated on
 * `ready` alone. A RailPanel asked for `stale` therefore showed the caveat over
 * an EMPTY card, which reads as "there is nothing here" rather than "this is
 * old": the opposite of what the state means, and the same "a refusal is not an
 * absence" mistake the seats rail was built to stop making.
 *
 * Narrowed rather than forwarded. Passing `children` through would light up the
 * rows and still drop the footer, which this component shows on `ready`/`empty`
 * only — half-supporting a state is how it stays broken quietly. A caller that
 * genuinely needs a stale rail gets a compile error and a decision instead.
 */
export type RailPanelState = Exclude<SectionState, "stale" | "partial">;

export function RailPanel({
  title,
  state,
  emptyLabel,
  detail,
  footer,
  children,
}: Readonly<{
  title: string;
  state: RailPanelState;
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
          {/* The panel's own title is what it is waiting for, so the caller is
              not asked to say it twice. Every other SurfaceState takes the
              label from whoever knows; here the wrapper knows. */}
          <SurfaceState
            state={state}
            emptyLabel={emptyLabel}
            loadingLabel={title}
            detail={detail}
          >
            {null}
          </SurfaceState>
        </PanelBody>
      )}
    </Panel>
  );
}
