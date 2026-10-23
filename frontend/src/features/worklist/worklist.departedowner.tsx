// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useT } from "../../i18n";
import { useDepartedName } from "../../lib/membernames";
import type { WorklistItem } from "./worklist.queries";

/**
 * Says on a row that its owner left. A team view keeps counting a departed
 * colleague's open work, so the row has to say it needs a new owner.
 */
export function DepartedOwnerCaption({
  owner,
}: Readonly<{ owner: WorklistItem["owner"] }>) {
  const t = useT();
  const name = useDepartedName(owner?.kind === "user" ? owner.id : null);
  if (!name) {
    return null;
  }
  return <p className="t-caption">{t("worklist.owner.left", { name })}</p>;
}
