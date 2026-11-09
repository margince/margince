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
import type { components } from "../../../api/schema";
import { interactionIcon } from "../../../components/interactionchrome";
import { SourceLink } from "../../../design-system/sourcelink";
import "./record360.css";

type ContactMomentEvidence = components["schemas"]["ContactMomentEvidence"];

/**
 * MomentEvidence is the run of sources under a moment's reason, each a
 * `SourceLink` with an optional quote. `kindOf` picks the glyph's kind where
 * the page knows more than the item's own `type` (an activity's email kind).
 * Without `onOpen` each source is plain text rather than a dead control.
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
            <SourceLink
              icon={glyph}
              onOpen={item.id && onOpen ? () => onOpen(item) : undefined}
            >
              {item.label}
            </SourceLink>
            {item.snippet && <q>{item.snippet}</q>}
          </li>
        );
      })}
    </ul>
  );
}
