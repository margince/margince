// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// THE MOMENT'S EVIDENCE, read the same way on every record.
//
// The contact page and the company page both draw a lead move from the
// server's moment, and each grew its own chip run for what it rests on: one
// inline run under the reason on the contact page, one behind a "rests on N
// sources" disclosure on the company page. A reader who reads both records
// met two answers to one question, so this is the one component both hand
// their moment's evidence to, and both now read it the same way.

import type { ReactNode } from "react";
import type { components } from "../../api/schema";
import { Button } from "../../design-system/atoms";
import { interactionIcon } from "../interactionchrome";
import "./record360.css";

type ContactMomentEvidence = components["schemas"]["ContactMomentEvidence"];

/**
 * MomentEvidence is the chip run under a moment's reason: a glyph naming the
 * kind of record, the verbatim words or the label, and a link where the
 * record can be opened.
 *
 * `kindOf` resolves the glyph's kind for one item. The contact page looks an
 * activity item up in its own activity list to draw the email glyph rather
 * than the generic "activity" one; the company page has no such list in
 * scope and leaves it absent, so the item's own `type` draws the glyph
 * instead.
 *
 * `onOpen` is absent where the page has nowhere to send the reader for a
 * kind of evidence: an item with an id then renders as a source rather than
 * a control that goes nowhere.
 */
export function MomentEvidence({
  evidence,
  kindOf,
  onOpen,
}: Readonly<{
  evidence: readonly ContactMomentEvidence[];
  kindOf?: (item: ContactMomentEvidence) => string | undefined;
  onOpen?: (item: ContactMomentEvidence) => void;
}>): ReactNode {
  const deduped = [
    ...new Map(
      evidence.map((item) => [`${item.type}:${item.id ?? item.label}`, item]),
    ).values(),
  ];
  return (
    <ul className="today-evidence">
      {deduped.map((item) => {
        const glyph = interactionIcon(kindOf ? kindOf(item) : item.type);
        return (
          <li key={`${item.type}-${item.id ?? item.label}`} className="t-sub">
            {item.id && onOpen ? (
              // The label is a record's own name — an email subject, a deal
              // title — so nothing about its length is ours to choose. `.btn`
              // pins `nowrap` for the label that IS ours, at a length we
              // picked; `btn-valuelabel` is the class for a label that carries
              // DATA, and a record's name is one.
              <Button
                variant="link"
                className="btn-valuelabel"
                onClick={() => onOpen(item)}
              >
                {glyph}
                {item.label}
              </Button>
            ) : (
              <span className="pe-source">
                {glyph}
                {item.label}
              </span>
            )}
            {item.snippet && <q>{item.snippet}</q>}
          </li>
        );
      })}
    </ul>
  );
}
