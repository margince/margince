import {
  type QueryClient,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query";
import { Lock } from "lucide-react";
import { type ReactNode, useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { useUnsavedGuard } from "../../components/unsaved";
import { VoiceKinds } from "../../components/voicekinds";
import { Badge, Button, Disclosure, Textarea } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { JumpBar, type JumpTarget } from "../../design-system/jumpbar";
import { Panel, PanelBody, PanelIntro } from "../../design-system/panel";
import { QueryGate } from "../../design-system/querystates";
import {
  type SettingControlProps,
  SettingList,
  SettingRow,
} from "../../design-system/settingrow";
import { useToast } from "../../design-system/toast";
import { identifierNumber } from "../../format/format";
import { type Translator, useT } from "../../i18n";
import { useCanWrite } from "../../lib/capability";
import { unwrap } from "../../lib/problem";
import { useVoiceProfile } from "../../lib/voice-profile";
import { useVoiceIntake, type VoiceIntake } from "./use-voice-intake";
import { VoiceCorpusIntake } from "./voice-corpus-settings";
import {
  SharpenStrip,
  VOICE_SECTIONS,
  VoiceHero,
  VoiceStart,
} from "./voice-hero";
import {
  DraftShowcase,
  parseVoiceInsights,
  useNextBestLine,
  VoiceFigures,
  type VoiceInsightsData,
  VoiceTraits,
} from "./voice-insights";
import { useVoiceSources, VoiceSamples } from "./voice-samples";
import {
  CandidateBanner,
  CandidateReview,
  loadedVersions,
  useVoiceLearning,
  useVoiceVersions,
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
// Every read a corpus change moves, by key prefix. An owner has one profile,
// so the prefix reaches the new profile's reads as well as the old one's.
function invalidateVoiceCorpus(qc: QueryClient) {
  for (const key of [
    "voice-profile",
    "voice-sources",
    "voice-versions",
    "voice-deltas",
    "voice-learning",
  ]) {
    qc.invalidateQueries({ queryKey: [key] });
  }
}

export function VoiceDnaCard() {
  const t = useT();
  const canCreate = useCanWrite("voice_profile", "create");
  const qc = useQueryClient();
  const profile = useVoiceProfile();
  // Owned above the empty and profile branches. The first sample mints the
  // profile and swaps one branch for the other. The files still in flight then
  // belong to the card that remains.
  const intake = useVoiceIntake({
    profileId: profile.data?.id ?? null,
    onChanged: () => invalidateVoiceCorpus(qc),
  });
  return (
    <QueryGate query={profile} pendingLabel={t("settings.voice.title")}>
      {(data) =>
        data ? (
          <VoiceDnaBody profile={data} intake={intake} />
        ) : (
          // A profile is minted by the first add rather than by a step of
          // its own, so the add control renders here too. Without it an owner
          // who skipped the onboarding voice step could never start a Voice
          // DNA at all, and everything the profile unlocks stayed unreachable.
          <div className="vdna-page">
            <VoiceStart />
            <Panel tone="ai" title={t("settings.voice.start.addTitle")}>
              <PanelBody>
                <PanelIntro>{t("settings.voice.emptyBody")}</PanelIntro>
                <VoiceKinds />
                {!canCreate && <p>{t("settings.voice.readOnly")}</p>}
              </PanelBody>
              {/* The first sample mints the profile, so adding it takes the
                  create grant rather than the update one. */}
              {canCreate && (
                <SettingList bleed="settings">
                  <VoiceCorpusIntake first intake={intake} />
                </SettingList>
              )}
            </Panel>
          </div>
        )
      }
    </QueryGate>
  );
}

function hasTraits(insights: VoiceInsightsData): boolean {
  return (
    insights.moves.length > 0 ||
    insights.obsessions.length > 0 ||
    insights.avoid.length > 0
  );
}

// The jump bar names only the sections the page draws: a voice with nothing
// learned yet has no moves or drafts to jump to.
function jumpTargets(
  t: Translator,
  insights: VoiceInsightsData | null,
): JumpTarget[] {
  const targets: JumpTarget[] = [
    { id: VOICE_SECTIONS.voice, label: t("settings.voice.jump.voice") },
  ];
  if (insights && hasTraits(insights)) {
    targets.push({
      id: VOICE_SECTIONS.moves,
      label: t("settings.voice.jump.moves"),
    });
  }
  if (insights && insights.sampleDrafts.length > 0) {
    targets.push({
      id: VOICE_SECTIONS.drafts,
      label: t("settings.voice.jump.drafts"),
    });
  }
  targets.push(
    { id: VOICE_SECTIONS.sharpen, label: t("settings.voice.jump.sharpen") },
    { id: VOICE_SECTIONS.rules, label: t("settings.voice.jump.rules") },
    { id: VOICE_SECTIONS.builds, label: t("settings.voice.jump.builds") },
  );
  return targets;
}

// What the build learned, proven: the moves in the writer's own words, then
// the drafts it writes with them.
function VoiceProof({ insights }: Readonly<{ insights: VoiceInsightsData }>) {
  const t = useT();
  return (
    <>
      {hasTraits(insights) && (
        <VoiceSection id={VOICE_SECTIONS.moves}>
          <Panel tone="ai" title={t("settings.voice.movesTitle")}>
            <PanelBody>
              <PanelIntro>{t("settings.voice.movesIntro")}</PanelIntro>
              <VoiceTraits data={insights} />
            </PanelBody>
          </Panel>
        </VoiceSection>
      )}
      {insights.sampleDrafts.length > 0 && (
        <VoiceSection id={VOICE_SECTIONS.drafts}>
          <Panel tone="ai" title={t("settings.voice.draftsTitle")}>
            <PanelBody>
              <PanelIntro>{t("settings.voice.draftsIntro")}</PanelIntro>
              <DraftShowcase drafts={insights.sampleDrafts} />
            </PanelBody>
          </Panel>
        </VoiceSection>
      )}
    </>
  );
}

// A profile that exists is one long page about the voice. It runs from who the
// writer is to the builds behind it, and the jump bar reaches each section.
function VoiceDnaBody({
  profile,
  intake,
}: Readonly<{ profile: VoiceProfile; intake: VoiceIntake }>) {
  const t = useT();
  // useCanWrite, not useCan: every affordance these children hold issues a
  // mutation, and the seat ceiling has to fold in — a read seat holding the
  // grant would otherwise be offered a write the server clamps.
  const canEdit = useCanWrite("voice_profile", "update");
  const qc = useQueryClient();
  // Intake lives in one card and the build button in another, so the fact that
  // a sample is still arriving has to be held by the parent they share.
  const [intakeBusy, setIntakeBusy] = useState(false);
  const [deltasAsked, setDeltasAsked] = useState(false);
  const invalidate = () => invalidateVoiceCorpus(qc);
  const versions = loadedVersions(useVoiceVersions(profile.id).data?.pages);
  const active = versions.find((version) => version.status === "active");
  const candidate = versions.find((version) => version.status === "candidate");
  // A first version awaiting review is the only voice there is. The page draws
  // it in full while the hero asks whether to use it.
  const shown = active ?? candidate;
  const insights = shown ? parseVoiceInsights(shown) : null;
  const summary = useVoiceSources(profile.id).data?.summary;
  const learning = useVoiceLearning(profile.id).data;
  const next = useNextBestLine(insights);
  const hasDrafts = Boolean(insights && insights.sampleDrafts.length > 0);
  const targets = jumpTargets(t, insights);
  return (
    <div className="vdna-page">
      <JumpBar
        label={t("settings.voice.jumpLabel")}
        targets={targets}
        action={<VoiceStanding profile={profile} />}
      />
      <p className="t-caption vdna-privacy">
        <Lock aria-hidden /> {t("settings.voice.privacy")}
      </p>
      {/* Said once for the whole surface, not beside each control a denial
          disables: the README's withheld write, not a withheld surface. */}
      {!canEdit && (
        <p className="vdna-readonly">{t("settings.voice.readOnly")}</p>
      )}
      <VoiceHero
        profile={profile}
        insights={insights}
        profileVersion={shown?.profile_version ?? 0}
        summary={summary}
        hasDrafts={hasDrafts}
        canEdit={canEdit}
        intakeBusy={intakeBusy}
        onBuilt={invalidate}
        review={
          candidate &&
          !active && (
            <CandidateReview
              profileId={profile.id}
              candidate={candidate}
              canEdit={canEdit}
              onChanged={invalidate}
              replacing={false}
            />
          )
        }
      />
      {/* Straight under the voice it would replace: a review waiting on a
          human is the most actionable thing below the hero. */}
      {candidate && active && (
        <CandidateBanner
          profileId={profile.id}
          candidate={candidate}
          canEdit={canEdit}
          onChanged={invalidate}
        />
      )}
      {/* Before the first build the hero itself guides the writer, so the
          strip would only say the same thing twice. */}
      {summary && insights && (
        <SharpenStrip summary={summary} next={next} canEdit={canEdit} />
      )}
      {insights && <VoiceFigures data={insights} learning={learning} />}

      {insights && <VoiceProof insights={insights} />}

      <VoiceSection id={VOICE_SECTIONS.sharpen}>
        <Panel tone="ai" title={t("settings.voice.sharpenTitle")}>
          <PanelBody>
            <PanelIntro>{t("settings.voice.sharpenIntro")}</PanelIntro>
          </PanelBody>
          {/* The samples are the card's subject, so they stand in it as a
              table rather than behind a modal that would hide what a reader
              audits. */}
          <VoiceSamples
            profileId={profile.id}
            canEdit={canEdit}
            onChanged={invalidate}
          />
          {canEdit && (
            <SettingList bleed="settings">
              <VoiceCorpusIntake intake={intake} onBusyChange={setIntakeBusy} />
            </SettingList>
          )}
        </Panel>
      </VoiceSection>

      <div className="vdna-split">
        <VoiceSection id={VOICE_SECTIONS.rules}>
          <Panel tone="ai" title={t("settings.voice.rulesTitle")}>
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
                    onSaved={invalidate}
                  />
                )}
              />
              {/* Once a build is active the hero quotes this text, so showing
                  it too would say it twice. */}
              {!insights && (
                <Disclosure summary={t("settings.voice.derivedLabel")}>
                  <DerivedVoice profile={profile} />
                </Disclosure>
              )}
            </SettingList>
          </Panel>
        </VoiceSection>

        <VoiceSection id={VOICE_SECTIONS.builds}>
          <Panel tone="ai" title={t("settings.voice.buildsTitle")}>
            <SettingList bleed="settings">
              {/* The first build starts in the hero, where the writer is
                  guided to it; from then on rebuilds live with the history.
                  intakeBusy holds a build back while a sample is arriving. */}
              {(profile.profile_version ?? 0) > 0 && (
                <BuildControls
                  profile={profile}
                  canEdit={canEdit}
                  onBuilt={invalidate}
                  intakeBusy={intakeBusy}
                />
              )}
            </SettingList>
            <VoiceHistory
              profileId={profile.id}
              canEdit={canEdit}
              onChanged={invalidate}
            />
            {/* Diagnostic: the deltas answer "why did it change", which a
                reader asks occasionally and never on arrival, so they are read
                on the first opening rather than with the page. */}
            <Disclosure
              summary={t("voice.history.deltasLabel")}
              onToggle={(open) => open && setDeltasAsked(true)}
            >
              {deltasAsked && <VoiceChangeLog profileId={profile.id} />}
            </Disclosure>
          </Panel>
        </VoiceSection>
      </div>
    </div>
  );
}

// A section the jump bar lands on: focusable by script only, so focus can
// follow the jump without adding a tab stop.
function VoiceSection({
  id,
  children,
}: Readonly<{ id: string; children: ReactNode }>) {
  return (
    <div id={id} tabIndex={-1} className="vdna-section">
      {children}
    </div>
  );
}

// The profile's standing at the end of the jump bar. A state shows only when
// it wants something, since ready is normal. The version follows.
function VoiceStanding({ profile }: Readonly<{ profile: VoiceProfile }>) {
  const t = useT();
  return (
    <span className="vdna-status">
      {profile.status !== "ready" && (
        <Badge>{t(`settings.voice.status.${profile.status}`)}</Badge>
      )}
      {/* Version 0 is a profile no build has written yet: nothing to name. */}
      {Boolean(profile.profile_version) && (
        <span className="t-caption">
          {t("settings.voice.version", {
            n: identifierNumber(profile.profile_version ?? 0),
          })}
        </span>
      )}
    </span>
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
