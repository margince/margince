import type { components } from "../api/schema";
import type { MessageKey } from "../i18n/en";

type Kind = components["schemas"]["CaptureConcernKind"];
export const CAPTURE_CONCERNS: Readonly<Record<Kind, MessageKey>> = {
  reauth_required: "captureConcern.reauth",
  connection_error: "captureConcern.error",
  sync_failing: "captureConcern.failing",
  backfill_failed: "captureConcern.backfill",
  sync_stalled: "captureConcern.stalled",
  capture_silent: "captureConcern.silent",
};

/** The words for one concern. A sync refused for the member's role is a role
 *  to change, not a mailbox to mend, so it names that cause instead. */
export function concernKey(
  concern: Kind,
  errorClass: string | null | undefined,
): MessageKey {
  return errorClass === "forbidden"
    ? "captureConcern.roleCannotCapture"
    : CAPTURE_CONCERNS[concern];
}
