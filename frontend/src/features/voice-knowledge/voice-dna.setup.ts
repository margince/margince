// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { QueryClient } from "@tanstack/react-query";
import { useEffect, useRef } from "react";
import { useJumpTo } from "../../design-system/jumpbar";
import { replaceDial, useUrlParams } from "../../lib/urlstate";
import {
  parseVoiceFocus,
  VOICE_FOCUS_PARAM,
  type VoiceFocus,
} from "../../lib/voice-setup";
import type { VoiceProfile } from "./voice-dna";
import { VOICE_SECTIONS } from "./voice-hero";

// Where each focus dial value lands. A review that is no longer there (applied
// or discarded elsewhere) falls back to the head of the page.
const FOCUS_TARGETS: Readonly<Record<VoiceFocus, string>> = {
  review: VOICE_SECTIONS.review,
  voice: VOICE_SECTIONS.voice,
  samples: VOICE_SECTIONS.sharpen,
};

// Answers setup's focus dial once the page has drawn what it points at. The
// dial is then cleared, so the same step pressed again moves focus again.
export function useSetupFocus(ready: boolean) {
  const [params] = useUrlParams();
  const asked = params.get(VOICE_FOCUS_PARAM);
  const jump = useJumpTo();
  useEffect(() => {
    if (!asked || !ready) {
      return;
    }
    const focus = parseVoiceFocus(asked);
    const target = focus ? FOCUS_TARGETS[focus] : VOICE_SECTIONS.voice;
    jump(document.getElementById(target) ? target : VOICE_SECTIONS.voice);
    replaceDial(VOICE_FOCUS_PARAM, undefined);
  }, [asked, ready, jump]);
}

// The profile read is polled while a build runs. When it reports a new
// outcome, the reads drawn from the build's result are read again too.
export function useFollowBuildOutcome(profile: VoiceProfile, qc: QueryClient) {
  const outcome = [
    profile.profile_version,
    profile.candidate_version,
    profile.latest_build?.id,
    profile.latest_build?.status,
  ].join(":");
  const seen = useRef(outcome);
  useEffect(() => {
    if (seen.current === outcome) {
      return;
    }
    seen.current = outcome;
    for (const key of ["voice-sources", "voice-versions", "voice-learning"]) {
      qc.invalidateQueries({ queryKey: [key] });
    }
  }, [outcome, qc]);
}
