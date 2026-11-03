// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import type { components } from "../api/schema";
import { voiceRefetchDelay } from "./voice-setup";

type VoiceProfile = components["schemas"]["VoiceProfile"];
type VoiceBuild = components["schemas"]["VoiceBuild"];

const NOW = Date.parse("2026-07-01T12:00:00Z");

function profileWith(build: Partial<VoiceBuild> | null): VoiceProfile {
  return {
    id: "vp-1",
    owner_id: "u1",
    status: "collecting",
    maturity: "provisional",
    quality_band: "good",
    voice_profile_md: "",
    profile_version: 0,
    personality_md: "",
    auto_learning_enabled: false,
    active_source_hash: null,
    candidate_version: null,
    latest_build: build && {
      id: "vb-1",
      profile_id: "vp-1",
      reason: "manual",
      status: "queued",
      stage: null,
      source_hash: "h",
      source_count: 3,
      result_version: null,
      candidate_action: "none",
      status_code: null,
      status_detail: null,
      next_attempt_at: null,
      version: 1,
      created_at: "2026-07-01T11:00:00Z",
      started_at: null,
      completed_at: null,
      updated_at: "2026-07-01T11:00:00Z",
      archived_at: null,
      ...build,
    },
    last_built_at: null,
    source: "manual",
    captured_by: "human:u1",
    version: 1,
    created_at: "2026-07-01T00:00:00Z",
    updated_at: "2026-07-01T00:00:00Z",
    archived_at: null,
  };
}

describe("voiceRefetchDelay", () => {
  it("polls a queued or running build", () => {
    expect(voiceRefetchDelay(profileWith({ status: "queued" }), NOW)).toBe(
      10_000,
    );
    expect(voiceRefetchDelay(profileWith({ status: "running" }), NOW)).toBe(
      10_000,
    );
  });

  it("reads a deferred build once, just after it is due, instead of polling", () => {
    const deferred = profileWith({
      status: "deferred",
      next_attempt_at: "2026-07-01T13:00:00Z",
    });
    expect(voiceRefetchDelay(deferred, NOW)).toBe(60 * 60 * 1000 + 5_000);
  });

  it("stops once a deferral is past due, has no due time, or is out of timer reach", () => {
    for (const next of ["2026-07-01T11:59:00Z", null, "2026-12-31T00:00:00Z"]) {
      expect(
        voiceRefetchDelay(
          profileWith({ status: "deferred", next_attempt_at: next }),
          NOW,
        ),
      ).toBe(false);
    }
  });

  it("stops for a settled build and for no build at all", () => {
    for (const status of ["succeeded", "failed"] as const) {
      expect(voiceRefetchDelay(profileWith({ status }), NOW)).toBe(false);
    }
    expect(voiceRefetchDelay(profileWith(null), NOW)).toBe(false);
    expect(voiceRefetchDelay(null, NOW)).toBe(false);
  });
});
