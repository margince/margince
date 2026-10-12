// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { PassportChip } from "../design-system/trust";
import { usePassportName } from "./passports.queries";

// The passport chip, named from the row itself where the row says, and from
// the reader's own passports otherwise. A component of its own so the list is
// read only where a row names a passport.
//
// agentClient is the server's answer for this row, resolved from the passport
// the row recorded.
//
// The list holds only the newest passport per connection.
//
// So a change made under a rotated token read as "An agent", even where the
// reader still held the connection.
export function ResolvedPassportChip({
  passportId,
  agentClient,
}: Readonly<{ passportId: string; agentClient?: string | null }>) {
  const fromList = usePassportName(passportId);
  return <PassportChip name={agentClient ?? fromList} />;
}
