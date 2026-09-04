// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/**
 * The line of facts under a record's name: what it is, where it is, who owns
 * it, how to reach it.
 *
 * It is the second thing a reader looks at on a record page, after the name,
 * and every record has one — so it was written three times. The company's
 * lived in `company360.css` as `.co-meta-*`, the person's in `person360.css`
 * as `.pe-meta-*`, and the deal was about to grow a third. Two spellings of
 * one row is how a reader gets a 13px meta line on one record and a 13px
 * content-ink line with different gaps on the next, one click apart.
 *
 * The separator is the whole difference between the two shapes, so it is the
 * one prop: facts that run together as a sentence about the record take the
 * dot, facts that are separate ways to reach it (an address, a number, a
 * profile) stand apart on their own whitespace. The gap follows from it — a
 * dotted line needs a small one either side of the mark, an undotted one needs
 * a gap wide enough to read as a break.
 */

import { Fragment, type ReactNode } from "react";
import "./identityline.css";

/**
 * The stack a record's meta lines sit in. One line needs no stack; a record
 * that says two different things under its name — what it is, then when its
 * row was written — gets them as lines rather than as one long sentence a
 * reader has to re-parse.
 */
export function IdentityMeta({ children }: Readonly<{ children: ReactNode }>) {
  return <div className="identity-meta">{children}</div>;
}

/**
 * One line of facts.
 *
 * `separator` says how two facts are told apart: `"dot"` for facts that read
 * as one sentence about the record, `"space"` for facts that are each a
 * separate handle on it. Children are the facts; a `null` or `false` child is
 * a fact this record does not have and draws nothing — no stranded dot.
 */
export function IdentityLine({
  separator = "dot",
  children,
}: Readonly<{
  separator?: "dot" | "space";
  children: ReactNode;
}>) {
  const dotted = separator === "dot";
  const facts = flatten(children);
  return (
    <div
      className={
        dotted
          ? "identity-line identity-line-dotted"
          : "identity-line identity-line-spaced"
      }
    >
      {facts.map((fact, i) => (
        // The index is the identity: these are positional facts about one
        // record, never reordered, and two of them can carry the same text
        // (a company that sold to itself through a partner).
        // biome-ignore lint/suspicious/noArrayIndexKey: positional facts, never reordered
        <Fragment key={i}>
          {/* The dot belongs to the fact that FOLLOWS it, so a line that
              wraps never leaves a separator stranded at the end of a row. */}
          {dotted && i > 0 && (
            <span className="identity-sep" aria-hidden="true">
              ·
            </span>
          )}
          {fact}
        </Fragment>
      ))}
    </div>
  );
}

/**
 * One fact on the line: an optional glyph and the fact itself, kept on one
 * line together.
 *
 * `quiet` marks a fact that QUALIFIES the record rather than being a way to
 * act on it — the buying role, who owns it — which reads under the facts
 * beside it without leaving the line.
 */
export function IdentityFact({
  icon,
  quiet,
  className,
  children,
}: Readonly<{
  icon?: ReactNode;
  quiet?: boolean;
  className?: string;
  children: ReactNode;
}>) {
  return (
    <span
      className={[
        "identity-fact",
        quiet ? "identity-fact-quiet" : "",
        className,
      ]
        .filter(Boolean)
        .join(" ")}
    >
      {icon}
      {children}
    </span>
  );
}

// The facts actually present, with the absent ones dropped.
//
// A caller assembles this line by asking the record what it knows, and a record
// that does not know something renders nothing for it — `{city && <Fact/>}`.
// Left in the list those nothings still count as children, and the dot is drawn
// against the index: one absent fact in the middle of the line would then put
// two dots side by side, and an absent first fact would open the line with one.
// Dropping them here is what lets a caller write the natural conditional
// instead of assembling an array and separating it by hand, which is what both
// records did before this component existed.
function flatten(children: ReactNode): ReactNode[] {
  const out: ReactNode[] = [];
  const walk = (node: ReactNode) => {
    if (node == null || typeof node === "boolean" || node === "") {
      return;
    }
    if (Array.isArray(node)) {
      for (const child of node) {
        walk(child);
      }
      return;
    }
    out.push(node);
  };
  walk(children);
  return out;
}
