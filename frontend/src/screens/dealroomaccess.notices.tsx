// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Callout } from "../design-system/callout";
import { useT } from "../i18n";

/**
 * What became of the invitation the rep just issued.
 *
 * Two readings of one press, so one band with two headings rather than two
 * notices: the link went out to the buyer, or it did not and is the rep's to
 * send. `success` against `info` is the difference, because "not mailed" is not
 * a failure — the link in the field below is valid either way, and a `warn`
 * here would send a rep looking for a fault.
 *
 * Its own file, and it takes `queued` and `email` rather than the whole issued
 * record: the notice is about the delivery, and a component that held the
 * credential would be one more thing that could put it on screen.
 */
export function IssuedNotice({
  queued,
  email,
}: Readonly<{ queued: boolean; email: string }>) {
  const t = useT();
  const title = queued
    ? "access.issued.mailedTitle"
    : "access.issued.notMailedTitle";
  return (
    <Callout tone={queued ? "success" : "info"} kind="outcome" title={t(title)}>
      {queued
        ? t("access.issued.mailed", { email })
        : t("access.issued.notMailed")}
    </Callout>
  );
}
