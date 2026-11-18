import type { components } from "../api/schema";
import type { MessageKey } from "../i18n/en";
import { errorClassKey } from "./connector-status";
import { labelFor } from "./labelfor";

type Kind = components["schemas"]["CaptureConcernKind"];
export const CAPTURE_CONCERNS: Readonly<Record<Kind, MessageKey>> = {
  reauth_required: "captureConcern.reauth",
  connection_error: "captureConcern.error",
  sync_failing: "captureConcern.failing",
  notes_grant_missing: "captureConcern.notesGrant",
  notes_account_mismatch: "captureConcern.notesAccount",
  backfill_failed: "captureConcern.backfill",
  sync_stalled: "captureConcern.stalled",
  capture_silent: "captureConcern.silent",
};

/** The words for one concern. A role refusal or a provider's rate limit names
 *  its cause, because neither is a mailbox for the member to mend. */
export function concernKey(
  concern: Kind,
  errorClass: string | null | undefined,
): MessageKey {
  if (errorClass === "forbidden") return "captureConcern.roleCannotCapture";
  if (rateLimited(concern, errorClass)) return errorClassKey(errorClass);
  return labelFor(CAPTURE_CONCERNS, concern);
}

/** A failing sync the provider is pacing. It retries by itself, so the screen
 *  says when instead of asking the member to act. */
export function rateLimited(
  concern: Kind | undefined,
  errorClass: string | null | undefined,
): boolean {
  return concern === "sync_failing" && errorClass === "rate_limited";
}
