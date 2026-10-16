import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Sparkles } from "lucide-react";
import { useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useUnsavedGuard } from "../components/unsaved";
import { Badge, Button, Disclosure, Textarea } from "../design-system/atoms";
import { ErrorLine } from "../design-system/errorline";
import { Panel, PanelBody, PanelIntro } from "../design-system/panel";
import { QueryGate } from "../design-system/querystates";
import {
  type SettingControlProps,
  SettingList,
  SettingRow,
} from "../design-system/settingrow";
import { useToast } from "../design-system/toast";
import { formatNumber, identifierNumber } from "../format/format";
import { useLocale, useT } from "../i18n";
import { useCanWrite } from "../lib/capability";
import { problemMessageOf, throwProblem, unwrap } from "../lib/problem";
import { VoiceCorpusIntake } from "./voice-corpus-settings";
import { useVoiceProfile } from "./voice-profile";
import { bandLabel, useVoiceSources, VoiceSamples } from "./voice-samples";
import {
  ActiveVoiceInsights,
  VoiceChangeLog,
  VoiceHistory,
} from "./voice-versions";
import "./voice-dna.css";

type VoiceProfile = components["schemas"]["VoiceProfile"];

// What every mutation on this card does with a failure, spelled once: state it
// in words written for the reader. Keeping the raw failure readable is the
// client's mutation sink's job (app/queryclient.ts), so a save, a removal, an
// add or a build that broke is diagnosable without every mutation here saying
// so for itself.
//
// The parameter is `unknown` rather than react-query's default `Error`
// because what a rejected promise carries is not ours to assume: a thrown
// string reaches here just as a TypeError does, and problemMessageOf takes it
// as it comes.
function reportFailure(
  setError: (message: string) => void,
  t: ReturnType<typeof useT>,
): (error: unknown) => void {
  return (error: unknown) => {
    setError(problemMessageOf(error, t));
  };
}

// The two ADR-0066 maturity thresholds, mirrored so the build control can say
// how far a corpus still has to go. The SERVER decides what state a profile is
// in (VoiceProfile.maturity); these only phrase the distance to the next one.
const VOICE_FIRST_BUILD_WORDS = 800;
const VOICE_FULL_BUILD_WORDS = 4000;

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

/** How a build ended, and the server's own words about it when it has any. */
type BuildOutcome = Readonly<{
  status: "succeeded" | "failed" | "deferred" | "pending";
  /** `status_detail` from the build row: safe operator guidance, already
   * written for a reader. Null when the server had nothing to add. */
  detail?: string | null;
}>;

// What the status line says at each point in a build's life: running, finished,
// or never started. Named rather than nested in the JSX because the control
// around it is already at the complexity ceiling, and a reader asking "what
// does this line say while it builds?" should find one function that answers.
function buildStatusLine(
  t: ReturnType<typeof useT>,
  running: boolean,
  outcome: BuildOutcome | null,
): string {
  if (running) {
    return t("settings.voice.buildRunning");
  }
  if (outcome) {
    return buildOutcomeText(t, outcome);
  }
  return "";
}

// What the reader is told a finished build did.
//
// The server's own sentence wins whenever it sent one, because only the server
// knows WHY — a provider out of budget, a model answer it could not read, a
// configuration that is missing. The local strings stay as the fallback for an
// older server, and for the outcomes that carry no detail.
function buildOutcomeText(
  t: ReturnType<typeof useT>,
  outcome: BuildOutcome,
): string {
  const detail = outcome.detail?.trim();
  if (detail) {
    return detail;
  }
  return t(`settings.voice.buildStatus.${outcome.status}`);
}

// Build creates a durable background build; poll to a terminal state. A slow or
// budget-deferred build is honestly reported, not spun on forever.
//
// One row: what the verb does on the left, the verb and what the last build
// said on the right. The distance still to go is the row's DESCRIPTION rather
// than a sentence beside the button, because it is two sentences of prose and
// the naming column is the one wide enough to read them — and when that
// distance is a refusal, `reasonId` points the button at the very same words
// instead of printing them twice.
function BuildControls({
  profile,
  canEdit,
  onBuilt,
  intakeBusy = false,
}: Readonly<{
  canEdit: boolean;
  profile: VoiceProfile;
  onBuilt: () => void;
  intakeBusy?: boolean;
}>) {
  const t = useT();
  const { locale } = useLocale();
  // The outcome AND what the server said about it. A terminal build carries
  // `status_detail` — safe operator guidance the server composed for exactly
  // this moment — and dropping it left one fixed sentence standing in for
  // every cause: a spending cap, an unreadable model answer and a broken
  // provider all read as "the build didn't finish", so "try again" was advice
  // that could not work.
  const [outcome, setOutcome] = useState<BuildOutcome | null>(null);
  const [error, setError] = useState<string | null>(null);

  const build = useMutation({
    mutationFn: async (): Promise<BuildOutcome> => {
      const created = await api.POST("/voice-profiles/{id}/builds", {
        params: { path: { id: profile.id } },
        body: { reason: "manual" },
      });
      if (created.error) {
        throwProblem(created.error);
      }
      const buildId = created.data.id;
      for (let attempt = 0; attempt < 40; attempt++) {
        const data = unwrap(
          await api.GET("/voice-profiles/{id}/builds/{buildId}", {
            params: { path: { id: profile.id, buildId } },
          }),
        );
        // Spelled as three comparisons rather than a set lookup: this is what
        // proves to the compiler that the status is one a BuildOutcome may
        // hold, so a new server state cannot be passed through untyped.
        if (
          data.status === "succeeded" ||
          data.status === "failed" ||
          data.status === "deferred"
        ) {
          return { status: data.status, detail: data.status_detail };
        }
        await new Promise((resolve) => {
          globalThis.setTimeout(resolve, 1500);
        });
      }
      // Still queued/running after the poll budget — honestly "pending", not
      // "deferred" (which specifically means the AI budget snoozed it). There
      // is no detail to carry: nothing terminal happened to describe.
      return { status: "pending", detail: null };
    },
    onSuccess: (finalOutcome) => {
      setOutcome(finalOutcome);
      setError(null);
      onBuilt();
    },
    onError: reportFailure(setError, t),
  });

  // The corpus summary rides the same query key CorpusManifest already read,
  // so asking for the word total here costs no extra request.
  const corpus = useVoiceSources(profile.id);
  // maturity is the SERVER's verdict on whether a build can say anything, so
  // it — not a locally recomputed threshold — decides whether the button is
  // offered. The word counts below only phrase the distance to the next state.
  const tooThin = profile.maturity === "collecting";
  // The distance is quoted only from a corpus total that actually loaded. A
  // failed fetch would otherwise read as zero words and announce a confident
  // "about 800 more words" whose real cause was the failure — the button's
  // state still follows maturity, which comes from a different request.
  const words = corpus.isSuccess ? corpus.data.summary.total_words : null;
  const blocked =
    tooThin && words !== null
      ? t("settings.voice.buildNeedsWords", {
          n: formatNumber(Math.max(0, VOICE_FIRST_BUILD_WORDS - words), locale),
        })
      : null;
  const reach =
    profile.maturity === "provisional" && words !== null
      ? t("settings.voice.buildProvisional", {
          n: formatNumber(Math.max(0, VOICE_FULL_BUILD_WORDS - words), locale),
        })
      : null;

  return (
    <SettingRow
      label={t("settings.voice.buildRowLabel")}
      description={blocked ?? reach ?? undefined}
      control={(control) =>
        canEdit ? (
          <div className="vdna-buildcell">
            <Button
              variant="primary"
              // A refusal the reader can act on, attached to the control rather
              // than left in a `title` no screen reader announces on a disabled
              // button. It points at the description this row already draws.
              reasonId={
                blocked === null ? undefined : control["aria-describedby"]
              }
              aria-describedby={control["aria-describedby"]}
              disabled={!build.isPending && (tooThin || intakeBusy)}
              pending={build.isPending}
              busyLabel={t("settings.voice.building")}
              onClick={() => build.mutate()}
            >
              <Sparkles aria-hidden />{" "}
              {/* "Rebuild" names a build that has happened. Before the first
                  one there is nothing to redo, and the verb the reader came
                  for is the first build itself. */}
              {t(
                (profile.profile_version ?? 0) > 0
                  ? "settings.voice.rebuild"
                  : "settings.voice.buildFirst",
              )}
            </Button>
            {/* Mounted whether or not there is an outcome yet. A build runs for
                about a minute behind a poll, so the reader who started it has
                looked away — and a live region inserted together with its text
                is not reliably announced, which would leave the one thing they
                are waiting for arriving in silence.
                
                While it runs this says so IN WORDS. The button carries the
                spinner and a visually-hidden busy label, which is the whole
                story for a screen reader and none of it for a reader looking
                at the page: a control that swallows a second press without
                saying why invites the press again. It also says the wait can
                be left, because a build outlives this page and a reader who
                does not know that sits and watches it. */}
            <p className="t-caption" role="status">
              {buildStatusLine(t, build.isPending, outcome)}
            </p>
            {error && <ErrorLine>{error}</ErrorLine>}
          </div>
        ) : null
      }
    />
  );
}
