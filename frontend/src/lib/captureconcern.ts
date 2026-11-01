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
