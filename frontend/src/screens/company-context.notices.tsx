// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import { Callout } from "../design-system/callout";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";

// What the company-context card says about ITSELF: the refusals, the
// confirmation a save leaves behind, and the caveats a website read came back
// with.
//
// Their own file because the card drew six of these bands by hand and four of
// them were the same nine lines four times over — the cold-start form, the
// profile dialog, the site-read panel and the review's Apply. A band whose
// anatomy lives in one place cannot end up saying the same thing two ways.

/**
 * A refused write: the claim as the heading, and the cause the server sent
 * under it.
 *
 * The cause arrives already worded — `problemMessageOf` for a thrown failure,
 * the caller's own string where it holds one — because only the caller knows
 * which of its writes this is about. `outcome` + `danger` is what makes it
 * interrupt a screen reader, and that is derived rather than chosen here.
 */
export function Refusal({
  titleKey,
  cause,
}: Readonly<{ titleKey: MessageKey; cause: ReactNode }>) {
  const t = useT();
  return (
    <Callout tone="danger" kind="outcome" title={t(titleKey)}>
      {cause}
    </Callout>
  );
}

/**
 * The save landed, and the dialog it landed in is gone — so the confirmation
 * is left on the card that now shows the new values. A heading alone is the
 * whole notice; there is nothing to add to "Saved".
 */
export function SavedNotice() {
  const t = useT();
  return (
    <Callout tone="success" kind="outcome" title={t("settings.companySaved")} />
  );
}

/**
 * The caveats one website read came back with, a line each under ONE heading.
 *
 * A callout per warning made a five-page read look like five separate things
 * going wrong. Empty renders nothing, so the caller states no condition of its
 * own: whether there is anything to say is this notice's question.
 */
export function ReadWarnings({
  warnings,
}: Readonly<{ warnings: readonly string[] }>) {
  const t = useT();
  if (warnings.length === 0) {
    return null;
  }
  return (
    <Callout
      tone="warn"
      kind="standing"
      title={t("settings.companyRefreshWarnings")}
    >
      <ul>
        {warnings.map((warning) => (
          <li key={warning}>{warning}</li>
        ))}
      </ul>
    </Callout>
  );
}
