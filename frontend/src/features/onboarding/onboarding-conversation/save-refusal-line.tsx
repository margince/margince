// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { NarrationBubble } from "./entries";

/**
 * One refusal line shared by every view that confirms, so a press that
 * moved nothing always says why.
 */
export function SaveRefusalLine({
  message,
}: Readonly<{ message: string | null }>) {
  if (message === null || message === "") {
    return null;
  }
  return (
    <div role="alert">
      <NarrationBubble
        entry={{
          kind: "narration",
          id: "review:confirm-failed",
          i18nKey: "ob.conv.review.confirmFailed",
          params: { detail: message },
        }}
      />
    </div>
  );
}
