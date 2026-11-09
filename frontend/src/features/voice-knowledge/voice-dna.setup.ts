// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { QueryClient } from "@tanstack/react-query";
import { useEffect, useRef } from "react";
import { useJumpTo } from "../../design-system/jumpbar";
import { navigate, navigateReplacing, useRoute } from "../../lib/router";
import { SETTINGS_PAGES, type SettingsPage } from "../../lib/settingscatalog";
import { settingsHref } from "../../lib/settingsrouting";
import { currentParams, replaceDial, useUrlParams } from "../../lib/urlstate";
import {
  parseVoiceFocus,
  VOICE_FOCUS_PARAM,
  type VoiceFocus,
} from "../../lib/voice-setup";
import type { VoiceProfile } from "./voice-dna";

/** The ids the page's sections carry, so every link into the page lands on
 * the same places. */
export const VOICE_SECTIONS = {
  voice: "vdna-voice",
  review: "vdna-review",
  moves: "vdna-moves",
  drafts: "vdna-drafts",
  sharpen: "vdna-sharpen",
  rules: "vdna-rules",
  builds: "vdna-builds",
} as const;

type VoicePage = Extract<SettingsPage, { id: "voice" }>;
/** The tabs of the Writing voice page, read off the settings catalog. */
export type VoiceTab = VoicePage["tabs"][number]["id"];

/** The tab an address names; anything else is the page's first. */
export function parseVoiceTab(value: string | undefined): VoiceTab {
  const page = SETTINGS_PAGES.find(
    (each): each is VoicePage => each.id === "voice",
  );
  return page?.tabs.find((tab) => tab.id === value)?.id ?? "overview";
}

// Where each focus lands: the tab that draws it, then the section on it. A
// review that is no longer there falls back to the head of the page.
const FOCUS_TARGETS: Readonly<
  Record<VoiceFocus, Readonly<{ tab: VoiceTab; section: string }>>
> = {
  review: { tab: "overview", section: VOICE_SECTIONS.review },
  voice: { tab: "overview", section: VOICE_SECTIONS.voice },
  drafts: { tab: "overview", section: VOICE_SECTIONS.drafts },
  samples: { tab: "samples", section: VOICE_SECTIONS.sharpen },
  rules: { tab: "rules", section: VOICE_SECTIONS.rules },
  builds: { tab: "builds", section: VOICE_SECTIONS.builds },
};

const voiceTabHref = (tab: VoiceTab) =>
  settingsHref("voice", tab === "overview" ? undefined : tab);

function useOpenVoiceTab(): VoiceTab {
  return parseVoiceTab(useRoute().id2);
}

// Answers the focus dial once the page has drawn what it points at. A focus
// this tab does not draw opens the tab that does. The dial is then cleared, so the same step
// pressed again moves focus again.
export function useSetupFocus(ready: boolean) {
  const [params] = useUrlParams();
  const asked = params.get(VOICE_FOCUS_PARAM);
  const tab = useOpenVoiceTab();
  const jump = useJumpTo();
  useEffect(() => {
    if (!asked || !ready) {
      return;
    }
    const target = FOCUS_TARGETS[parseVoiceFocus(asked) ?? "voice"];
    const drawn = document.getElementById(target.section) !== null;
    if (!drawn && target.tab !== tab) {
      navigateReplacing(voiceTabHref(target.tab), currentParams());
      return;
    }
    jump(drawn ? target.section : VOICE_SECTIONS.voice);
    replaceDial(VOICE_FOCUS_PARAM, undefined);
  }, [asked, ready, tab, jump]);
}

/** Move to one part of the page: a jump where it is drawn, else its tab. */
export function useVoiceGo(): (focus: VoiceFocus) => void {
  const jump = useJumpTo();
  return (focus) => {
    const target = FOCUS_TARGETS[focus];
    if (document.getElementById(target.section) !== null) {
      jump(target.section);
      return;
    }
    navigate(voiceTabHref(target.tab), new Map([[VOICE_FOCUS_PARAM, focus]]));
  };
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
