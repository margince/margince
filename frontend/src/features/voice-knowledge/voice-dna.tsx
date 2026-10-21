import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { useUnsavedGuard } from "../../components/unsaved";
import { Badge, Button, Disclosure, Textarea } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { Panel, PanelBody, PanelIntro } from "../../design-system/panel";
import { QueryGate } from "../../design-system/querystates";
import {
  type SettingControlProps,
  SettingList,
  SettingRow,
} from "../../design-system/settingrow";
import { useToast } from "../../design-system/toast";
import { identifierNumber } from "../../format/format";
import { useT } from "../../i18n";
import { useCanWrite } from "../../lib/capability";
import { unwrap } from "../../lib/problem";
import { useVoiceProfile } from "../../lib/voice-profile";
import { VoiceCorpusIntake } from "./voice-corpus-settings";
import { bandLabel, VoiceSamples } from "./voice-samples";
import {
  ActiveVoiceInsights,
  VoiceChangeLog,
  VoiceHistory,
} from "./voice-versions";
import "./voice-dna.css";
import { BuildControls, reportFailure } from "./voice-dna.build";

export type VoiceProfile = components["schemas"]["VoiceProfile"];

// The "…later in Settings" surface the onboarding Voice step promises: the
// owner's own profile, its corpus, and its builds. A profile that does not
// exist yet is ONE card, because a heading over an empty body describes a
// surface the owner does not have.
export function VoiceDnaCard() {
  const t = useT();
  const canCreate = useCanWrite("voice_profile", "create");
  const qc = useQueryClient();
  const profile = useVoiceProfile();
  return (
    <QueryGate query={profile} pendingLabel={t("settings.voice.title")}>
      {(data) =>
        data ? (
          <VoiceDnaBody profile={data} />
        ) : (
          // A profile is minted by the first add rather than by a step of
          // its own, so the add control renders here too. Without it an owner
          // who skipped the onboarding voice step could never start a Voice
          // DNA at all, and everything the profile unlocks (corpus, builds,
          // sample drafts) stayed unreachable. There is no empty-state box
          // above it: the card's one job here is to take the first sample,
          // and a box saying there is nothing yet only pushes that job down.
          <Panel title={t("settings.voice.title")}>
            <PanelBody>
              <PanelIntro>{t("settings.voice.emptyBody")}</PanelIntro>
              {!canCreate && <p>{t("settings.voice.readOnly")}</p>}
            </PanelBody>
            {/* The first sample mints the profile, so adding it takes the
                create grant rather than the update one. */}
            {canCreate && (
              <SettingList bleed="settings">
                <VoiceCorpusIntake
                  first
                  profileId={null}
                  onChanged={() =>
                    qc.invalidateQueries({ queryKey: ["voice-profile"] })
                  }
                />
              </SettingList>
            )}
          </Panel>
        )
      }
    </QueryGate>
  );
}

// A profile that exists answers three questions, so it is three cards: what
// the voice IS (its state, what the last build learned, the preferences that
// steer it), what it is BUILT FROM, and what its builds have DONE. Five cards
// stood here before, one per component; the row language is what let the
// preferences editor and the derived text rejoin the subject they belong to
// instead of each needing a header band to be findable.
function VoiceDnaBody({ profile }: Readonly<{ profile: VoiceProfile }>) {
  const t = useT();
  // useCanWrite, not useCan: every affordance these children hold issues a
  // mutation, and the seat ceiling has to fold in — a read seat holding the
  // grant would otherwise be offered a write the server clamps.
  const canEdit = useCanWrite("voice_profile", "update");
  const qc = useQueryClient();
  // Intake lives in one card and the build button in another, so the fact that
  // a sample is still arriving has to be held by the parent they share.
  const [intakeBusy, setIntakeBusy] = useState(false);
  const invalidate = () => {
    qc.invalidateQueries({ queryKey: ["voice-profile"] });
    qc.invalidateQueries({ queryKey: ["voice-sources", profile.id] });
    qc.invalidateQueries({ queryKey: ["voice-versions", profile.id] });
    qc.invalidateQueries({ queryKey: ["voice-deltas", profile.id] });
    qc.invalidateQueries({ queryKey: ["voice-learning", profile.id] });
  };
  return (
    <>
      <Panel title={t("settings.voice.title")}>
        <PanelBody>
          <PanelIntro>{t("settings.voice.privacy")}</PanelIntro>
          {/* Said once for the whole surface, not beside each control a denial
              disables: the README's withheld write, not a withheld surface. */}
          {!canEdit && (
            <p className="vdna-readonly">{t("settings.voice.readOnly")}</p>
          )}
          <div className="vdna-status">
            {/* Ready is the normal reading, so only a profile that wants
                something carries a badge. */}
            {profile.status !== "ready" && (
              <Badge>{t(`settings.voice.status.${profile.status}`)}</Badge>
            )}
            {profile.quality_band && (
              <span className="t-caption">
                {bandLabel(t, profile.quality_band)}
              </span>
            )}
            <span className="t-caption vdna-version">
              {t("settings.voice.version", {
                n: identifierNumber(profile.profile_version ?? 0),
              })}
            </span>
          </div>

          {/* Above the rows, and deliberately not inside one: it also carries
              the candidate-review banner, which is the most actionable thing on
              this page, and a review waiting on a human does not belong under
              two settings rows. It renders for EVERY profile state — a
              review-required first build must be actionable while the profile
              is still collecting. */}
          <ActiveVoiceInsights
            profileId={profile.id}
            canEdit={canEdit}
            onChanged={invalidate}
          />
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
                onSaved={invalidate}
              />
            )}
          />
          {/* Once ready, the insights above quote this text, so showing it too
              would say it twice. */}
          {profile.status !== "ready" && (
            <Disclosure summary={t("settings.voice.derivedLabel")}>
              <DerivedVoice profile={profile} />
            </Disclosure>
          )}
        </SettingList>
      </Panel>

      <Panel title={t("settings.voice.corpusLabel")}>
        {/* The samples are the card's subject, so they stand in it as a table
            rather than behind a modal that would hide what a reader audits. */}
        <VoiceSamples
          profileId={profile.id}
          canEdit={canEdit}
          onChanged={invalidate}
        />
        {canEdit && (
          <SettingList bleed="settings">
            <VoiceCorpusIntake
              profileId={profile.id}
              onChanged={invalidate}
              onBusyChange={setIntakeBusy}
            />
          </SettingList>
        )}
      </Panel>

      <Panel title={t("settings.voice.buildsTitle")}>
        <SettingList bleed="settings">
          {/* A build started while a sample is still arriving would describe
              a corpus that no longer exists by the time it finishes. */}
          <BuildControls
            profile={profile}
            canEdit={canEdit}
            onBuilt={invalidate}
            intakeBusy={intakeBusy}
          />
        </SettingList>
        <VoiceHistory
          profileId={profile.id}
          canEdit={canEdit}
          onChanged={invalidate}
        />
        {/* Diagnostic: the deltas answer "why did it change", which a reader
            asks occasionally and never on arrival. */}
        <Disclosure summary={t("voice.history.deltasLabel")}>
          <VoiceChangeLog profileId={profile.id} />
        </Disclosure>
      </Panel>
    </>
  );
}

function DerivedVoice({ profile }: Readonly<{ profile: VoiceProfile }>) {
  const t = useT();
  return profile.voice_profile_md ? (
    <p className="vdna-derived">{profile.voice_profile_md}</p>
  ) : (
    <p>{t("settings.voice.derivedEmpty")}</p>
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
      // The voice a member writes for their own mail is the one save on this
      // page a reader most wants confirmed, and it said nothing: the button
      // simply stopped being pressable once the draft matched the server.
      toast.show(t("settings.saved"));
    },
    onError: reportFailure(setError, t),
  });
  const dirty = text !== profile.personality_md;
  // A voice profile is the longest thing anybody types in settings, which makes
  // it the draft a silent discard costs the most.
  useUnsavedGuard(dirty);
  // `settingrow-measure` is the row primitive's own hook for a control that
  // takes the stacked row's full width: without it the wrapper sizes to its
  // content and the box inside falls back to a textarea's 20-column intrinsic
  // width, which is the defect atoms.css already documents for callers who
  // forgot it.
  return (
    <div className="form-stack settingrow-measure">
      {/* readOnly rather than disabled: the preferences are a READ this seat
          still holds, and a disabled textarea drops out of the tab order, so a
          keyboard reader could not reach the words to read them. */}
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
