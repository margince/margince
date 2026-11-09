// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { routeHash } from "./router";
import { settingsHref } from "./settingsrouting";
import { hashWithParams } from "./urlstate";
import { worklistLaneTarget } from "./worklistrouting";

export const CAPTURE_REVIEW_PARAM = "review";
/** Narrows the held list to the threads with one contact. */
export const CAPTURE_CONTACT_PARAM = "contact_id";
export type CaptureReview = "senders" | "held";

export function captureReview(value: string | undefined): "held" | null {
  return value === "held" ? value : null;
}

export function captureReviewTarget(review: CaptureReview, contactId?: string) {
  if (review === "senders") return worklistLaneTarget("decisions");
  const params = new Map<string, string>([[CAPTURE_REVIEW_PARAM, review]]);
  if (contactId) params.set(CAPTURE_CONTACT_PARAM, contactId);
  return {
    route: settingsHref("connections", "activity"),
    params,
  };
}

export function captureReviewLink(review: CaptureReview, base: string) {
  const target = captureReviewTarget(review);
  const url = new URL(base);
  url.search = "";
  url.hash = hashWithParams(routeHash(target.route), target.params);
  return url.href;
}

/** The same address with no contact filter on the held list. */
export function withoutCaptureContact(params: ReadonlyMap<string, string>) {
  const next = new Map(params);
  next.delete(CAPTURE_CONTACT_PARAM);
  return next;
}
