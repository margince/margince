// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Callout } from "../design-system/callout";
import { useT } from "../i18n";

// What the user-mapping card says about ITSELF where the shared `WriteRefused`
// does not: the standing cost of the read it performs. Its three reads and
// writes that did not land are that shared notice, each keeping the server's
// own words — which one failed is what an admin goes and fixes.

/**
 * The cost of the read this card performs, at the weight a standing advisory
 * earns.
 *
 * It was the quietest type on the card — 12px meta grey, under everything
 * else — which is the one thing a disclosure about somebody else's money must
 * not be. Outside the row list on purpose: it is not a decision, and a row with
 * no answer on its right reads as a setting nobody has set.
 */
export function ReadCostNotice() {
  const t = useT();
  return (
    <Callout kind="standing" title={t("overlay.userMap.costTitle")}>
      {t("overlay.userMap.cost")}
    </Callout>
  );
}
