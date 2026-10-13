import type { MessageKey } from "../i18n/en";

export type KnownCapability = "view" | "comment";
export const CAPABILITIES: readonly KnownCapability[] = ["view", "comment"];

export const CAPABILITY_LABELS: Record<KnownCapability, MessageKey> = {
  view: "access.cap.view",
  comment: "access.cap.comment",
};

export const CAPABILITY_HINTS: Record<KnownCapability, MessageKey> = {
  view: "access.cap.viewHint",
  comment: "access.cap.commentHint",
};
