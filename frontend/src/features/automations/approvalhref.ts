import { routeHash } from "../../lib/router";
import { hashWithParams } from "../../lib/urlstate";

/** The address parameter that opens one decision over Home. */
export const APPROVAL_PARAM = "approval";

/**
 * The address of one pending decision. Every surface that announces a decision
 * links here, so the reader arrives at that decision and not at the queue.
 */
export function approvalHref(id: string): string {
  return hashWithParams(
    routeHash({ screen: "home" }),
    new Map([[APPROVAL_PARAM, id]]),
  );
}
