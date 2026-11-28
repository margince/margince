// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import { Callout } from "../design-system/callout";
import { formatNumber } from "../format/format";
import { useLocale, useT } from "../i18n";

type Copies = components["schemas"]["ChannelProviderCopies"];

export function ChannelProviderCopies({
  copies,
}: Readonly<{ copies: Copies }>) {
  const t = useT();
  const { locale } = useLocale();
  return (
    <>
      <p>
        {t("channelEnded.copies", {
          retained: formatNumber(copies.retained_messages, locale),
          redacted: formatNumber(copies.confirmed_redactions, locale),
        })}
      </p>
      {(copies.unverified_withdrawals > 0 ||
        copies.unconfirmed_deliveries > 0) && (
        <Callout tone="warning" title={t("channelLink.syncNeedsAttention")}>
          {t("channelEnded.action", {
            withdrawals: formatNumber(copies.unverified_withdrawals, locale),
            unknown: formatNumber(copies.unconfirmed_deliveries, locale),
          })}
        </Callout>
      )}
    </>
  );
}
