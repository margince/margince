// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useId } from "react";
import { EntityRef } from "../../../components/entityref";
import { Eyebrow } from "../../../design-system/eyebrow";
import { useT } from "../../../i18n";
import type {
  CommitteeLineRow,
  CommitteeUnreached,
} from "./dealcommitteelines";
import "./dealcommittee.css";

/**
 * The pairs of colleague and buyer seat who have exchanged mail or met, then
 * the seats nobody has reached. Each pair is one sentence that opens the seat's
 * contact, so the list reads the same without sight.
 */
export function CommitteeLines({
  lines,
  unreached,
}: Readonly<{
  lines: readonly CommitteeLineRow[];
  unreached: readonly CommitteeUnreached[];
}>) {
  const t = useT();
  const titleId = useId();
  if (lines.length + unreached.length === 0) {
    return null;
  }
  return (
    <section className="dc-lines" aria-labelledby={titleId}>
      <Eyebrow as="h3" size="xxsmall" id={titleId}>
        {t("deal.committee.lines.title")}
      </Eyebrow>
      <ul className="dc-rows">
        {lines.map((line) => (
          <li key={line.key} className="dc-row">
            <span className="dc-row-name">
              <EntityRef kind="contact" id={line.contactId} name={line.text} />
            </span>
          </li>
        ))}
        {unreached.map((gap) => (
          <li key={gap.contactId} className="dc-row dc-row-gap">
            <span className="dc-dot dc-dot-gap" aria-hidden="true" />
            <span className="dc-row-name">{gap.text}</span>
          </li>
        ))}
      </ul>
    </section>
  );
}
