// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { ProviderMark } from "../design-system/provider-mark";
import { Chip } from "../design-system/readings";
import { linkedinUrl } from "../format/weburl";
import { useT } from "../i18n";

/**
 * A record's LinkedIn page as a chip: LinkedIn's logo and the record's name,
 * opening the profile in a new tab. Dense, so the row keeps its line height.
 *
 * The logo claims the destination is LinkedIn, and a crawl, a connector or a
 * paste can write any host into the field. So an address `linkedinUrl` refuses
 * stays as its own text, and the reader keeps the fact without the claim.
 */
export function LinkedInChip({
  href,
  name,
}: Readonly<{ href: string; name?: string | null }>) {
  const t = useT();
  if (!linkedinUrl(href)) {
    return href;
  }
  const named = name?.trim();
  return (
    <Chip
      mark={<ProviderMark providerKey="linkedin" />}
      href={href}
      label={named ? t("record.linkedinProfile", { name: named }) : undefined}
      dense
    >
      {named || t("contact.page.linkedin")}
    </Chip>
  );
}
