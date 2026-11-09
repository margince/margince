// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation } from "@tanstack/react-query";
import { type ReactNode, useState } from "react";
import { api } from "../../api/client";
import { useUnsavedGuard } from "../../components/unsaved";
import { Button, Disclosure, Textarea } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { Panel, PanelBody, PanelIntro } from "../../design-system/panel";
import {
  type SettingControlProps,
  SettingList,
  SettingRow,
} from "../../design-system/settingrow";
import { useToast } from "../../design-system/toast";
import { useT } from "../../i18n";
import { unwrap } from "../../lib/problem";
import type { VoiceProfile } from "./voice-dna";
import { BuildControls, reportFailure } from "./voice-dna.build";
import { useVoiceGo, VOICE_SECTIONS } from "./voice-dna.setup";
import { DerivedVoice } from "./voice-dna.standing";
import { VoiceChangeLog, VoiceHistory } from "./voice-versions";

// The Writing voice page's Rules and Builds tabs, and the section wrapper every
// tab lands links on.

export function VoiceRules({
  profile,
  learned,
  canEdit,
  onSaved,
}: Readonly<{
  profile: VoiceProfile;
  learned: boolean;
  canEdit: boolean;
  onSaved: () => void;
}>) {
  const t = useT();
  return (
    <VoiceSection id={VOICE_SECTIONS.rules}>
      <Panel title={t("settings.voice.rulesTitle")}>
        <PanelBody>
          <PanelIntro>{t("settings.voice.rulesIntro")}</PanelIntro>
        </PanelBody>
        <SettingList bleed="settings">
          {/* Stacked: the preferences are the longest text in settings. */}
          <SettingRow
            label={t("settings.voice.personalityLabel")}
            layout="stack"
            control={(control) => (
              <PersonalityEditor
                control={control}
                profile={profile}
                canEdit={canEdit}
                onSaved={onSaved}
              />
            )}
          />
          {/* Once a build is active the hero quotes this text, so showing it
              too would say it twice. */}
          {!learned && (
            <Disclosure summary={t("settings.voice.derivedLabel")}>
              <DerivedVoice profile={profile} />
            </Disclosure>
          )}
        </SettingList>
      </Panel>
    </VoiceSection>
  );
}

export function VoiceBuilds({
  profile,
  canEdit,
  intakeBusy,
  onChanged,
}: Readonly<{
  profile: VoiceProfile;
  canEdit: boolean;
  intakeBusy: boolean;
  onChanged: () => void;
}>) {
  const t = useT();
  const go = useVoiceGo();
  const [deltasAsked, setDeltasAsked] = useState(false);
  const built = (profile.profile_version ?? 0) > 0;
  return (
    <VoiceSection id={VOICE_SECTIONS.builds}>
      <Panel title={t("settings.voice.buildsTitle")}>
        {!built && canEdit && (
          <PanelBody>
            <PanelIntro>{t("settings.voice.firstBuildHint")}</PanelIntro>
            <div>
              <Button onClick={() => go("voice")}>
                {t("settings.voice.firstBuildGo")}
              </Button>
            </div>
          </PanelBody>
        )}
        <SettingList bleed="settings">
          {/* The first build starts in the hero, where the writer is guided to
              it; from then on rebuilds live with the history. intakeBusy holds
              a build back while a sample is arriving. */}
          {built && (
            <BuildControls
              profile={profile}
              canEdit={canEdit}
              onBuilt={onChanged}
              intakeBusy={intakeBusy}
            />
          )}
        </SettingList>
        <VoiceHistory
          profileId={profile.id}
          canEdit={canEdit}
          onChanged={onChanged}
        />
        {/* Diagnostic: the deltas answer "why did it change", which a reader
            asks occasionally and never on arrival, so they are read on the
            first opening rather than with the page. */}
        <Disclosure
          summary={t("voice.history.deltasLabel")}
          onToggle={(open) => open && setDeltasAsked(true)}
        >
          {deltasAsked && <VoiceChangeLog profileId={profile.id} />}
        </Disclosure>
      </Panel>
    </VoiceSection>
  );
}

// A section a link lands on: focusable by script only, so focus can follow
// the jump without adding a tab stop.
export function VoiceSection({
  id,
  children,
}: Readonly<{ id: string; children: ReactNode }>) {
  return (
    <div id={id} tabIndex={-1} className="vdna-section">
      {children}
    </div>
  );
}

// personality_md is the owner-authored preferences the model output never
// overwrites; the PATCH is version-guarded (If-Match on the profile version).
function PersonalityEditor({
  control,
  profile,
  canEdit,
  onSaved,
}: Readonly<{
  /** The naming its row already draws, so the box announces that same string
   * rather than a second aria-label nobody can see drifting from it. */
  control: SettingControlProps;
  profile: VoiceProfile;
  canEdit: boolean;
  onSaved: () => void;
}>) {
  const t = useT();
  const [text, setText] = useState(profile.personality_md);
  const [error, setError] = useState<string | null>(null);
  const toast = useToast();
  const save = useMutation({
    mutationFn: async () => {
      unwrap(
        await api.PATCH("/voice-profiles/{id}", {
          params: {
            path: { id: profile.id },
            header: { "If-Match": String(profile.version) },
          },
          body: { personality_md: text },
        }),
      );
    },
    onSuccess: () => {
      setError(null);
      onSaved();
      // A member most wants this save confirmed. Without the toast the only
      // sign was a button that stopped being pressable.
      toast.show(t("settings.saved"));
    },
    onError: reportFailure(setError, t),
  });
  const dirty = text !== profile.personality_md;
  // A voice profile is the longest thing anybody types in settings, which makes
  // it the draft a silent discard costs the most.
  useUnsavedGuard(dirty);
  // `settingrow-measure` gives the control the stacked row's full width.
  // Without it the textarea falls back to its 20-column intrinsic width.
  return (
    <div className="form-stack settingrow-measure">
      {/* readOnly rather than disabled: this seat may still read the rules.
          A disabled textarea leaves the tab order, so a keyboard could not reach them. */}
      <Textarea
        {...control}
        rows={4}
        value={text}
        readOnly={!canEdit}
        placeholder={t("settings.voice.personalityPlaceholder")}
        onChange={(e) => setText(e.target.value)}
      />
      {canEdit && (
        <div className="vdna-composer-actions">
          <Button
            disabled={!dirty || save.isPending}
            onClick={() => save.mutate()}
          >
            {t("settings.voice.savePreferences")}
          </Button>
          {error && <ErrorLine inline>{error}</ErrorLine>}
        </div>
      )}
    </div>
  );
}
