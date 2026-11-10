// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { routeHash } from "./router";
import { settingsHref } from "./settingsrouting";
import { hashWithParams } from "./urlstate";

export const SIGNATURE_REVIEW_PARAM = "signature_review";

export function signatureReviewHref(id: string): string {
  return hashWithParams(
    routeHash(settingsHref("privacy", "requests")),
    new Map([[SIGNATURE_REVIEW_PARAM, id]]),
  );
}
