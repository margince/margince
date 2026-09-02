// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Button } from "../../design-system/atoms";
import { PanelBody } from "../../design-system/panel";
import { useT } from "../../i18n";

// The thread when its read failed. A deal's and a lead's thread arrive on a
// request of their own, and a failure there handed the spine an empty page —
// which draws exactly like a record nobody has ever written to. The call is
// still stated; under it this says the thread is missing, not empty, and
// offers the one thing a reader can do about it.
export function ThreadFailed({ onRetry }: Readonly<{ onRetry: () => void }>) {
  const t = useT();
  return (
    <PanelBody className="co-thread-failed">
      <p className="t-small">{t("co.spine.failed")}</p>
      <Button small variant="ghost" onClick={onRetry}>
        {t("common.retry")}
      </Button>
    </PanelBody>
  );
}
