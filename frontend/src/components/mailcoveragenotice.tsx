// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import { Callout } from "../design-system/callout";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";

type Coverage = components["schemas"]["MailCaptureCoverage"];

const details: Record<Coverage["state"], MessageKey> = {
  unknown: "mailCoverage.unknown",
  partial: "mailCoverage.partial",
  unavailable: "mailCoverage.unavailable",
  withheld: "mailCoverage.withheld",
};

export function MailCoverageNotice({
  coverage,
}: Readonly<{ coverage?: Coverage }>) {
  const t = useT();
  const state = coverage?.state ?? "unknown";
  return (
    <Callout
      tone="info"
      kind="standing"
      title={t("mailCoverage.title")}
      actions={
        state === "withheld" ? undefined : (
          <a className="link-button" href="#/settings/connections">
            {t("mailCoverage.connections")}
          </a>
        )
      }
    >
      {t(details[state] ?? "mailCoverage.unknown")}
    </Callout>
  );
}
