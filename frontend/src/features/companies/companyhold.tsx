// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { CounterpartyHoldRow } from "../../components/counterparty-hold";
import { Disclosure } from "../../design-system/disclosure";
import { PanelBody } from "../../design-system/panel";
import { useT } from "../../i18n";
import { companyWebsite } from "./companyheader";
import { SectionSummary } from "./companyrailshared";

type Company = components["schemas"]["Company"];

// Keeping a whole account's correspondence private, from the account page.
//
// The account's own domain is what a hold names here. An advisory firm answers
// from whichever address picked up the file, so the domain covers the whole
// relationship. Drawn only when the account has a domain. A hold has to name
// something, and `website_url` is derived from the primary domain row, so its
// absence means there is nothing to name.
export function CompanyHoldSection({
  company,
}: Readonly<{ company: Company | undefined }>) {
  const t = useT();
  const website = company ? companyWebsite(company) : undefined;
  const host = hostOf(website);
  if (!host) {
    return null;
  }
  return (
    <Disclosure summary={<SectionSummary title={t("hold.sectionTitle")} />}>
      <PanelBody>
        <CounterpartyHoldRow scope={{ domain: host }} />
      </PanelBody>
    </Disclosure>
  );
}

// The registrable host inside a derived website URL, or nothing when the value
// is absent or not a URL this can read. Never throws at the caller: an account
// with an unparseable website simply offers no hold, rather than taking the
// rail down with it.
function hostOf(website: string | null | undefined): string | undefined {
  if (!website) {
    return undefined;
  }
  try {
    return new URL(website).hostname.replace(/^www\./, "").toLowerCase();
  } catch {
    return undefined;
  }
}
