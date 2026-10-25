// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import { Badge } from "../design-system/atoms";
import { jurisdictionName } from "../format/jurisdiction";
import { useLocale, useT } from "../i18n";

type Mismatch = components["schemas"]["AddressCountryMismatch"];

// The stored country does not fit the postal code and city. The badge only
// flags it and the stored country stays. The reader is the one who can tell
// which of the two is wrong.
export function AddressCountryMismatchBadge({
  mismatch,
}: Readonly<{ mismatch?: Mismatch }>) {
  const t = useT();
  const { locale } = useLocale();
  if (!mismatch) {
    return null;
  }
  return (
    <Badge tone="warning" wrap>
      {t("co.address.countryMismatch", {
        country: jurisdictionName(mismatch.resolved_country, locale),
      })}
    </Badge>
  );
}
