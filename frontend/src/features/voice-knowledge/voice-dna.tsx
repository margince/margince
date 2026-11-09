import { type QueryClient, useQueryClient } from "@tanstack/react-query";
import { Lock } from "lucide-react";
import { useState } from "react";
import type { components } from "../../api/schema";
import { VoiceKinds } from "../../components/voicekinds";
import { Panel, PanelBody, PanelIntro } from "../../design-system/panel";
import { QueryGate } from "../../design-system/querystates";
import { SettingList } from "../../design-system/settingrow";
import { useT } from "../../i18n";
import { useCanWrite } from "../../lib/capability";
import { useVoiceProfile } from "../../lib/voice-profile";
import { useVoiceIntake, type VoiceIntake } from "./use-voice-intake";
import { VoiceCorpusIntake } from "./voice-corpus-settings";
import { VoiceStanding } from "./voice-dna.standing";
import { SharpenStrip, VoiceHero, VoiceStart } from "./voice-hero";
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
  LearnFromSentMail,
  type SentMailLearning,
  useSentMailLearning,
} from "./voice-sentmail";
import {
  CandidateBanner,
  CandidateReview,
  loadedVersions,
  useVoiceLearning,
  useVoiceVersions,
} from "./voice-versions";
import "./voice-dna.css";
import { VoiceBuilds, VoiceRules, VoiceSection } from "./voice-dna.parts";
import {
  parseVoiceTab,
  useFollowBuildOutcome,
  useSetupFocus,
  VOICE_SECTIONS,
  type VoiceTab,
} from "./voice-dna.setup";

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

export function VoiceDnaCard({ tab }: Readonly<{ tab?: string }>) {
  const t = useT();
  const open = parseVoiceTab(tab);
  const canCreate = useCanWrite("voice_profile", "create");
  const qc = useQueryClient();
  const profile = useVoiceProfile();
  // Before the first sample there is no section to land on; the dial is still
  // answered, so pressing the step again works.
  useSetupFocus(profile.data === null);
  // Owned above the empty and profile branches. The first sample mints the
  // profile and swaps one branch for the other. The files still in flight then
  // belong to the card that remains.
  const intake = useVoiceIntake({
    profileId: profile.data?.id ?? null,
    onChanged: () => invalidateVoiceCorpus(qc),
  });
  const sentMail = useSentMailLearning(() => invalidateVoiceCorpus(qc));
  return (
    <QueryGate query={profile} pendingLabel={t("settings.voice.title")}>
      {(data) =>
        data ? (
          <VoiceDnaBody
            tab={open}
            profile={data}
            intake={intake}
            sentMail={sentMail}
          />
        ) : (
          // A profile is minted by the first add rather than by a step of
          // its own, so the add control renders here too. Without it an owner
          // who skipped the onboarding voice step could never start a Voice
          // DNA at all, and everything the profile unlocks stayed unreachable.
          // Every tab answers with the first add: there are no samples,
          // rules or builds yet, and the add is how they begin.
          <div className="vdna-page">
            {open === "overview" && <VoiceStart />}
            <VoiceSection id={VOICE_SECTIONS.sharpen}>
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
                    <LearnFromSentMail learning={sentMail} />
                    <VoiceCorpusIntake first intake={intake} />
                  </SettingList>
                )}
              </Panel>
            </VoiceSection>
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

// A profile that exists shows four tabs. They hold the voice, the writing it
// learns from, the owner's rules, and its builds.
function VoiceDnaBody({
  tab,
  profile,
  intake,
  sentMail,
}: Readonly<{
  tab: VoiceTab;
  profile: VoiceProfile;
  intake: VoiceIntake;
  sentMail: SentMailLearning;
}>) {
  const t = useT();
  // useCanWrite, not useCan: every affordance these children hold issues a
  // mutation, and the seat ceiling has to fold in — a read seat holding the
  // grant would otherwise be offered a write the server clamps.
  const canEdit = useCanWrite("voice_profile", "update");
  const qc = useQueryClient();
  // Intake and the build button sit on different tabs, so the fact that a
  // sample is still arriving is held by the parent they share.
  const [intakeBusy, setIntakeBusy] = useState(false);
  const invalidate = () => invalidateVoiceCorpus(qc);
  const versionsRead = useVoiceVersions(profile.id);
  const versions = loadedVersions(versionsRead.data?.pages);
  useSetupFocus(!versionsRead.isPending);
  useFollowBuildOutcome(profile, qc);
  const active = versions.find((version) => version.status === "active");
  const candidate = versions.find((version) => version.status === "candidate");
  // A first version awaiting review is the only voice there is. The page draws
  // it in full while the hero asks whether to use it.
  const shown = active ?? candidate;
  const insights = shown ? parseVoiceInsights(shown) : null;
  return (
    <div className="vdna-page">
      <div className="vdna-meta">
        <p className="t-caption vdna-privacy">
          <Lock aria-hidden /> {t("settings.voice.privacy")}
        </p>
        <VoiceStanding profile={profile} />
      </div>
      {/* Said once for the whole surface, not beside each control a denial
          disables: the README's withheld write, not a withheld surface. */}
      {!canEdit && (
        <p className="vdna-readonly">{t("settings.voice.readOnly")}</p>
      )}
      {tab === "overview" && (
        <VoiceOverview
          profile={profile}
          insights={insights}
          shownVersion={shown?.profile_version ?? 0}
          active={active}
          candidate={candidate}
          canEdit={canEdit}
          intakeBusy={intakeBusy}
          onChanged={invalidate}
        />
      )}
      {/* Until a build has learned something, adding writing and building
          are one flow, so the samples stand under the hero that builds. */}
      {tab === "overview" && insights === null && (
        <VoiceSamplesPanel
          profile={profile}
          intake={intake}
          sentMail={sentMail}
          canEdit={canEdit}
          onBusyChange={setIntakeBusy}
          onChanged={invalidate}
        />
      )}
      {tab === "samples" && (
        <VoiceSamplesPanel
          profile={profile}
          intake={intake}
          sentMail={sentMail}
          canEdit={canEdit}
          onBusyChange={setIntakeBusy}
          onChanged={invalidate}
        />
      )}
      {tab === "rules" && (
        <VoiceRules
          profile={profile}
          learned={insights !== null}
          canEdit={canEdit}
          onSaved={invalidate}
        />
      )}
      {tab === "builds" && (
        <VoiceBuilds
          profile={profile}
          canEdit={canEdit}
          intakeBusy={intakeBusy}
          onChanged={invalidate}
        />
      )}
    </div>
  );
}

// The writing the voice learns from, and the ways to add more.
function VoiceSamplesPanel({
  profile,
  intake,
  sentMail,
  canEdit,
  onBusyChange,
  onChanged,
}: Readonly<{
  profile: VoiceProfile;
  intake: VoiceIntake;
  sentMail: SentMailLearning;
  canEdit: boolean;
  onBusyChange: (busy: boolean) => void;
  onChanged: () => void;
}>) {
  const t = useT();
  return (
    <VoiceSection id={VOICE_SECTIONS.sharpen}>
      <Panel tone="ai" title={t("settings.voice.sharpenTitle")}>
        <PanelBody>
          <PanelIntro>{t("settings.voice.sharpenIntro")}</PanelIntro>
        </PanelBody>
        {/* The samples are the card's subject, so they stand in it as a table
            rather than behind a modal that would hide what a reader audits. */}
        <VoiceSamples
          profileId={profile.id}
          canEdit={canEdit}
          onChanged={onChanged}
        />
        {canEdit && (
          <SettingList bleed="settings">
            <LearnFromSentMail learning={sentMail} />
            <VoiceCorpusIntake intake={intake} onBusyChange={onBusyChange} />
          </SettingList>
        )}
      </Panel>
    </VoiceSection>
  );
}

type VoiceVersion = components["schemas"]["VoiceProfileVersion"];

// The voice itself: the hero, a review waiting on the owner, how far the
// writing goes, and what the build learned.
function VoiceOverview({
  profile,
  insights,
  shownVersion,
  active,
  candidate,
  canEdit,
  intakeBusy,
  onChanged,
}: Readonly<{
  profile: VoiceProfile;
  insights: VoiceInsightsData | null;
  shownVersion: number;
  active: VoiceVersion | undefined;
  candidate: VoiceVersion | undefined;
  canEdit: boolean;
  intakeBusy: boolean;
  onChanged: () => void;
}>) {
  const summary = useVoiceSources(profile.id).data?.summary;
  const learning = useVoiceLearning(profile.id).data;
  const next = useNextBestLine(insights);
  return (
    <>
      <VoiceHero
        profile={profile}
        insights={insights}
        profileVersion={shownVersion}
        summary={summary}
        hasDrafts={Boolean(insights && insights.sampleDrafts.length > 0)}
        canEdit={canEdit}
        intakeBusy={intakeBusy}
        onBuilt={onChanged}
        review={
          candidate &&
          !active && (
            <CandidateReview
              profileId={profile.id}
              candidate={candidate}
              canEdit={canEdit}
              onChanged={onChanged}
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
          onChanged={onChanged}
        />
      )}
      {/* Before the first build the hero itself guides the writer, so the
          strip would only say the same thing twice. */}
      {summary && insights && (
        <SharpenStrip summary={summary} next={next} canEdit={canEdit} />
      )}
      {insights && <VoiceFigures data={insights} learning={learning} />}
      {insights && <VoiceProof insights={insights} />}
    </>
  );
}
