// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Callout } from "../design-system/callout";
import { useT } from "../i18n";

// What the user-mapping card and its picker say about THEMSELVES: three reads
// or writes that did not land, and the standing cost of the read this card
// performs. None of them is content, and all four keep the server's own words
// where there are any — which read failed is what an admin goes and fixes.

/** The mapping read failed. Keeps the server's detail over the card's own
 *  fallback sentence: a refused token is a different repair from a timeout. */
export function MappingReadFailed({ message }: Readonly<{ message: string }>) {
  const t = useT();
  return (
    <Callout
      kind="outcome"
      tone="danger"
      title={t("overlay.userMap.loadFailedTitle")}
    >
      {message}
    </Callout>
  );
}

/** The incumbent's directory could not be read, so there is nobody to pick.
 *  It stands INSTEAD of the picker rather than above it. */
export function DirectoryFailed({ message }: Readonly<{ message: string }>) {
  const t = useT();
  return (
    <Callout
      kind="outcome"
      tone="danger"
      title={t("overlay.userMap.directoryFailedTitle")}
    >
      {message}
    </Callout>
  );
}

/** The mapping write the dialog just attempted, refused. */
export function MappingSaveRefused({ message }: Readonly<{ message: string }>) {
  const t = useT();
  return (
    <Callout
      kind="outcome"
      tone="danger"
      title={t("overlay.userMap.saveFailedTitle")}
    >
      {message}
    </Callout>
  );
}

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
