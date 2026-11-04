// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { api } from "../api/client";
import { useClipboardCopy } from "../design-system/clipboardcopy";

type CopyLabels = Parameters<typeof useClipboardCopy>[1];

/**
 * Copies the host's booking link and tells the server the page was shared.
 * Home warns a host about a paused page only once it was used, and a copied
 * link counts as use. The copy itself already worked, so a failed mark is
 * logged and never shown.
 */
export function useCopyBookingLink(
  url: string,
  labels: CopyLabels,
  onCopied?: () => void,
) {
  return useClipboardCopy(url, labels, () => {
    onCopied?.();
    api.POST("/scheduling/profile/shared").then(
      ({ error }) => {
        if (error) console.warn("booking link share was not recorded", error);
      },
      (reason: unknown) => {
        console.warn("booking link share was not recorded", reason);
      },
    );
  });
}
