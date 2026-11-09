// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation } from "@tanstack/react-query";
import { api } from "../api/client";
import { useClipboardCopy } from "../design-system/clipboardcopy";
import { problemStatus, unwrap } from "./problem";

type CopyLabels = Parameters<typeof useClipboardCopy>[1];

const SHARE_ATTEMPTS = 4;

// The mark is idempotent, so a dropped connection or a server fault is worth
// asking again. A refusal the server answered (404, 403) stays refused.
function retryShare(failures: number, error: Error): boolean {
  const status = problemStatus(error);
  return failures < SHARE_ATTEMPTS - 1 && (status === 0 || status >= 500);
}

/**
 * Copies the host's booking link and tells the server the page was shared.
 * Home warns a host about a paused page only once it was used, and a copied
 * link counts as use. The copy itself already worked, so a mark that still
 * fails after its retries is logged and never shown.
 */
export function useCopyBookingLink(
  url: string,
  labels: CopyLabels,
  onCopied?: () => void,
) {
  const share = useMutation({
    mutationFn: async () => {
      unwrap(await api.POST("/scheduling/profile/shared"));
    },
    retry: retryShare,
    onError: (reason) => {
      console.warn("booking link share was not recorded", reason);
    },
  });
  return useClipboardCopy(url, labels, () => {
    onCopied?.();
    share.mutate();
  });
}
