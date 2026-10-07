// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useCanWrite } from "../app/capability";
import { Button } from "../design-system/atoms";
import { ErrorLine } from "../design-system/errorline";
import { useT } from "../i18n";
import { useClaimSettle } from "./taskactions";

/**
 * Dismissing a commitment the reader judges was never made. Kept beside the
 * words it was read from, so the reader can see what the reading got wrong.
 */
export function DismissClaimButton({ id }: Readonly<{ id: string }>) {
  const t = useT();
  const settle = useClaimSettle([]);
  // Settling writes the contact's record, so a reader who may not update
  // contacts is not offered a verb the server would refuse.
  const canSettle = useCanWrite("contact", "update");
  if (!canSettle) {
    return null;
  }
  return (
    <>
      <Button
        variant="link"
        pending={settle.isPending}
        onClick={() => settle.mutate({ id, outcome: "dismissed" })}
      >
        {t("commitment.dismiss")}
      </Button>
      <ErrorLine error={settle.error} inline />
    </>
  );
}
