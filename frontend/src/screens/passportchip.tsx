// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { PassportChip } from "../design-system/trust";
import { usePassportName } from "./passports.queries";

// The passport chip with its name looked up in the reader's own passports. A
// component of its own so the list is read only where a row names a passport.
export function ResolvedPassportChip({
  passportId,
}: Readonly<{ passportId: string }>) {
  return <PassportChip name={usePassportName(passportId)} />;
}
