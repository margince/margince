// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";

type VoiceProfile = components["schemas"]["VoiceProfile"];
type VoiceBuild = components["schemas"]["VoiceBuild"];

/**
 * Where the owner's writing voice stands, read off the one profile response.
 *
 * The setup checklist and the Writing voice page both answer from this, so the
 * top-bar reminder, Settings Overview and the page cannot disagree. Nothing is
 * stored for it: the active version, the pending candidate, the newest build
 * and the server's maturity verdict already say it all.
 */
export type VoiceSetupState =
  /** A voice is active. Later candidates and failed rebuilds do not reopen setup. */
  | "active"
  /** No active voice; a trained candidate waits for the owner's review. */
  | "review"
  /** No active voice; a build is queued or running. */
  | "building"
  /** No active voice; the newest build was deferred and retries later. */
  | "deferred"
  /** No active voice; the newest build failed. */
  | "failed"
  /** No active voice or pending build, and the server says a build can learn. */
  | "ready"
  /** Too few eligible words for a first build. */
  | "needsSamples";

/** The build facts a setup caption may quote. */
export type VoiceBuildFacts = Pick<
  VoiceBuild,
  "status" | "status_detail" | "deferral_reason"
>;

/**
 * The state for a profile read that answered. `null` is "no profile yet",
 * which is a first sample still to add, not an unknown.
 */
export function voiceSetupState(profile: VoiceProfile | null): VoiceSetupState {
  if (profile === null) {
    return "needsSamples";
  }
  // Zero is the schema's "no active artifact yet".
  if ((profile.profile_version ?? 0) > 0) {
    return "active";
  }
  if (profile.candidate_version !== null) {
    return "review";
  }
  switch (profile.latest_build?.status) {
    case "queued":
    case "running":
      return "building";
    case "deferred":
      return "deferred";
    case "failed":
      return "failed";
    default:
      // maturity is the server's readiness verdict; no word threshold here.
      return profile.maturity === "collecting" ? "needsSamples" : "ready";
  }
}

/** Whether a build of this profile is still on its way to a result. */
export function voiceBuildPending(profile: VoiceProfile | null): boolean {
  const status = profile?.latest_build?.status;
  return status === "queued" || status === "running" || status === "deferred";
}

/**
 * The address dial that asks the Writing voice page to scroll to and focus one
 * of its parts. Setup's voice step sets it; the page clears it once it moved
 * focus, so pressing the same step again while on the page moves focus again.
 */
export const VOICE_FOCUS_PARAM = "focus";

/** The parts of the Writing voice page a setup step can lead to. */
export type VoiceFocus = "review" | "voice" | "samples";

const VOICE_FOCI: readonly VoiceFocus[] = ["review", "voice", "samples"];

/** The focus a dial value names, or null for a value this page never mints. */
export function parseVoiceFocus(value: string | undefined): VoiceFocus | null {
  return VOICE_FOCI.find((focus) => focus === value) ?? null;
}

/** Where each setup state's one action takes the owner on the page. */
export function voiceFocusFor(state: VoiceSetupState | null): VoiceFocus {
  switch (state) {
    case "review":
      return "review";
    case "needsSamples":
      return "samples";
    default:
      return "voice";
  }
}
