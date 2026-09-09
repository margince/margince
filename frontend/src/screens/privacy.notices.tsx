// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Callout } from "../design-system/callout";
import { useT } from "../i18n";

/**
 * Why an erasure will not go through, inside the confirm that asked for it.
 *
 * The one spelling of "what this surface says about itself" here. Both of these
 * were a hand-rolled bordered panel — `.dsr-legal-hold`, at its own padding and
 * its own radius — which is a `Callout` with a different name and a second set
 * of numbers to keep in step.
 *
 * The band owns the interval, because a notice has no layout hook and both of
 * these can stand at once. Neither interrupts: both are true as the confirm
 * opens, and a dialog that announced them on open would announce them again on
 * every re-open of a request nothing has changed about.
 *
 * A legal hold is a documented, lawful refusal (Art. 17(3)(b)) rather than a
 * mistake the operator made, and neither is a request another officer already
 * decided — so both are stated in the danger family and neither is dressed as a
 * fault of the reader's.
 */
export function ErasureRefusals({
  held,
  movedOn,
}: Readonly<{ held: boolean; movedOn: boolean }>) {
  const t = useT();
  if (!held && !movedOn) {
    return null;
  }
  return (
    <div className="dsr-refusal">
      {held && (
        <Callout
          kind="standing"
          tone="danger"
          title={t("privacy.legalHoldTitle")}
        >
          <p>{t("privacy.legalHold")}</p>
        </Callout>
      )}
      {/* Heading only, and the SAME key the row's own report uses: one sentence
          is a complete callout, and a second spelling of "your click changed
          nothing" would let the two drift. */}
      {movedOn && (
        <Callout kind="standing" tone="danger" title={t("privacy.movedOn")} />
      )}
    </div>
  );
}
