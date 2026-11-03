// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Badge } from "../design-system/atoms";
import { dealStatusLabel, dealStatusTone } from "../format/dealstatus";
import { useT } from "../i18n";

/**
 * A deal's status in the reader's words, the one badge every screen draws it
 * with. A status this build has no word for reads as itself.
 */
export function DealStatusBadge({ status }: Readonly<{ status: string }>) {
  const t = useT();
  const key = dealStatusLabel(status);
  return <Badge tone={dealStatusTone(status)}>{key ? t(key) : status}</Badge>;
}
