// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { routeHash } from "./router";
import { settingsHref } from "./settingsrouting";
import { hashWithParams } from "./urlstate";
import { worklistLaneTarget } from "./worklistrouting";

export const CAPTURE_REVIEW_PARAM = "review";
export type CaptureReview = "senders" | "held";

export function captureReview(value: string | undefined): "held" | null {
  return value === "held" ? value : null;
}

export function captureReviewTarget(review: CaptureReview) {
  if (review === "senders") return worklistLaneTarget("decisions");
  return {
    route: settingsHref("capture-activity"),
    params: new Map([[CAPTURE_REVIEW_PARAM, review]]),
  };
}

export function captureReviewLink(review: CaptureReview, base: string) {
  const target = captureReviewTarget(review);
  const url = new URL(base);
  url.search = "";
  url.hash = hashWithParams(routeHash(target.route), target.params);
  return url.href;
}
