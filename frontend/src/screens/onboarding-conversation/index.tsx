import type { UseQueryResult } from "@tanstack/react-query";
import { useQuery } from "@tanstack/react-query";
import type { Dispatch } from "react";
import { useEffect, useReducer, useRef } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { navigate, navigateReplacing, useRoute } from "../../app/router";
import { Button } from "../../design-system/atoms";
import { useLocale, useT } from "../../i18n";
import { throwProblem } from "../common";
import {
  EMPTY_DRAFT,
  loadWizardState,
  pickBuiltVersion,
  useCompany,
} from "../onboarding";
import { BuildScene } from "../onboarding-build-scene";
import { BasisAct } from "./basis-act";
import { CompanyAct } from "./company-act";
import { ConnectAct } from "./connect-act";
import {
  type ConversationEvent,
  type ConversationPhase,
  type ConversationState,
  conversationReducer,
  initialConversationState,
} from "./conversation-machine";
import { InviteAct } from "./invite-act";
import { restorePlan, type VoiceRestoreProbe } from "./restore";
import { TeamAct } from "./team-act";
import type { WizardPersistInput } from "./use-wizard-state";
import { useWizardStatePersist } from "./use-wizard-state";
import { VoiceAct } from "./voice-act";
import { WorkbenchEntranceScope } from "./workbench";

// The conversational onboarding shell — THE onboarding experience: one pure
// machine owns where the conversation is, and each act renders inside the
// shared Margince workbench. On mount the shell reads the server truth
// (wizard state, company, voice) and restores through START + RESUME; the
// wizard state's `path` field is THE member signal, with company-exists only
// the fallback when no state row exists.

type OnboardingState = components["schemas"]["OnboardingState"];
type CompanySiteRead = components["schemas"]["CompanySiteRead"];

// The creator steps whose restore needs the voice server truth (built
// versions, corpus meter). "basis" and "invite" are in the set because the
// invite walks straight into the voice act without another restore: probing
// only from "voice" onwards handed VoiceAct a null summary, and the collect
// scene then reported zero words over a corpus the server already held. A
// member's journey BEGINS at voice, so their restore always needs it.
const voiceProbeSteps = new Set<OnboardingState["step"]>([
  "basis",
  "invite",
  "voice",
  "results",
  "connect",
]);

// Whether the restore needs the voice probe, from the same facts restorePlan
// routes on: the wizard row's path when one exists, else company existence.
function voiceProbeNeeded(
  wizard: OnboardingState | null,
  companyExists: boolean,
): boolean {
  if (wizard === null) {
    return companyExists;
  }
  return wizard.path === "member" || voiceProbeSteps.has(wizard.step);
}

// One probe per fact the restore needs: does a built version exist, and
// what does the server corpus meter say right now.
async function probeVoice(): Promise<VoiceRestoreProbe> {
  const list = await api.GET("/voice-profiles");
  if (list.error) {
    throwProblem(list.error);
  }
  const profileId = list.data.data[0]?.id;
  if (profileId === undefined) {
    return { built: false, summary: null };
  }
  const [versions, sources] = await Promise.all([
    api.GET("/voice-profiles/{id}/versions", {
      params: { path: { id: profileId } },
    }),
    api.GET("/voice-profiles/{id}/sources", {
      params: { path: { id: profileId } },
    }),
  ]);
  if (versions.error) {
    throwProblem(versions.error);
  }
  if (sources.error) {
    throwProblem(sources.error);
  }
  return {
    built: pickBuiltVersion(versions.data.data) !== null,
    summary: sources.data.summary,
  };
}

// Live act transitions the server must remember, keyed by the phase pair
// that only a user action (never a restore RESUME out of co.confirmed)
// produces. A pair absent here is a move the server need not remember.
//
// Declining the invite records BOTH personal steps as skipped on the way into
// the team act: the answer was about them, and a reload must not reopen a
// question already answered. Entering the connect screen shows both mail and
// LinkedIn at once, so there is nothing left behind a reload could strand —
// its checkpoint fires on arrival, unlike voice which fires on departure.
// Finishing is not here either: the connect act and the team act each write
// step "complete" themselves before they move, so the handoff never plays
// over a journey the server still holds open.
const actCheckpoints: ReadonlyMap<
  string,
  Omit<WizardPersistInput, "values">
> = new Map([
  ["co.review>bs.ask", { step: "basis" }],
  ["co.manual>bs.ask", { step: "basis" }],
  ["bs.ask>in.ask", { step: "invite" }],
  ["in.ask>vo.collecting", { step: "voice", voiceSkipped: false }],
  ["in.ask>tm.ask", { step: "team", voiceSkipped: true, connectSkipped: true }],
  ["vo.collecting>vo.skipped", { step: "voice", voiceSkipped: true }],
  ["vo.result>cn.consent", { step: "connect" }],
  ["vo.skipped>cn.consent", { step: "connect" }],
]);

function actCheckpoint(
  prev: ConversationPhase,
  next: ConversationPhase,
  buildSucceeded: boolean,
): Omit<WizardPersistInput, "values"> | null {
  // The one pair that is a checkpoint only on a particular outcome: a build
  // that failed or deferred leaves the voice act where it was.
  if (prev === "vo.building" && next === "vo.result") {
    return buildSucceeded ? { step: "voice", voiceSkipped: false } : null;
  }
  return actCheckpoints.get(`${prev}>${next}`) ?? null;
}

// The welcome gate: restore lookups still in flight, or a load failure with
// one retry that re-runs exactly the lookups that failed.
type RestoreLookup = Readonly<{ isError: boolean; refetch: () => unknown }>;

function RestoreGate({ lookups }: Readonly<{ lookups: RestoreLookup[] }>) {
  const t = useT();
  const failed = lookups.filter((lookup) => lookup.isError);
  return (
    <div className="ob-page ob-conv-page">
      {failed.length > 0 ? (
        <div className="readfail warn" role="alert">
          <p>{t("ob.conv.loadFailed")}</p>
          <Button
            small
            onClick={() => {
              for (const lookup of failed) {
                void lookup.refetch();
              }
            }}
          >
            {t("ob.conv.retry")}
          </Button>
        </div>
      ) : (
        <div className="ob-state-loading" role="status">
          <span className="ob-spinner" /> {t("ob.restoring")}
        </div>
      )}
    </div>
  );
}

// The restore lookups and the one START/RESUME dispatch, as a hook: the
// server truth (wizard state, company, voice, persisted read) is read once,
// and only a SETTLED set of lookups may route — a transient error must not
// send an existing member down the creator flow (nor a returning creator
// down the member flow).
function useRestore(
  state: ConversationState,
  dispatch: Dispatch<ConversationEvent>,
  routeConnect: boolean,
) {
  const { locale } = useLocale();
  const existing = useCompany(true);
  // The restore's own snapshot, not the live entry the shell gates on: a
  // checkpoint landing mid-journey must not re-run the restore's reads.
  const wizard = useQuery({
    queryKey: ["onboarding-conv-state"],
    queryFn: loadWizardState,
  });
  const voiceNeeded =
    wizard.isSuccess &&
    existing.isSuccess &&
    voiceProbeNeeded(wizard.data ?? null, existing.data !== null);
  const voice = useQuery({
    queryKey: ["onboarding-conv-voice"],
    queryFn: probeVoice,
    enabled: voiceNeeded,
  });
  // The persisted read is only worth fetching while the company act is
  // still open: a reload must reattach a running or finished read instead
  // of stranding the user's work behind a fresh intro.
  const persistedReadId =
    wizard.data != null &&
    (wizard.data.step === "read" || wizard.data.step === "confirm")
      ? (wizard.data.site_read_id ?? null)
      : null;
  const persistedRead = useQuery({
    queryKey: ["onboarding-conv-read", persistedReadId],
    enabled: persistedReadId !== null,
    queryFn: async (): Promise<CompanySiteRead | null> => {
      const { data, error, response } = await api.GET(
        "/company/site-reads/{readId}",
        { params: { path: { readId: persistedReadId ?? "" } } },
      );
      if (error) {
        // A read the server no longer serves is not a restore failure; the
        // company act simply reopens fresh.
        if (response.status === 404) {
          return null;
        }
        throwProblem(error);
      }
      return data;
    },
  });

  const restored = useRef(false);
  const settled =
    existing.isSuccess &&
    wizard.isSuccess &&
    (!voiceNeeded || voice.isSuccess) &&
    (persistedReadId === null || persistedRead.isSuccess);
  useEffect(() => {
    if (restored.current || state.act !== "welcome" || !settled) {
      return;
    }
    restored.current = true;
    const plan = restorePlan({
      state: wizard.data ?? null,
      profile: existing.data ?? null,
      voice: voice.data ?? null,
      read: persistedRead.data ?? null,
      routeConnect,
      locale,
    });
    if (plan.kind === "complete") {
      // Replacing, not pushing. This is the app's OTHER automatic navigator,
      // and it and the onboarding gate answer the same question from opposite
      // sides: a reader whose wizard is finished does not belong on
      // `#/onboarding`, and one whose installation is undescribed does not
      // belong anywhere else. Two pushes between those two addresses is a
      // history a reader cannot walk out of.
      navigateReplacing({ screen: "home" });
      return;
    }
    dispatch({
      type: "START",
      memberPath: plan.memberPath,
      companyConfirmed: plan.companyConfirmed,
      recap: plan.recap,
    });
    // Reattaching happens through the ordinary machine event, so legality
    // and correlation hold exactly as for a live read.
    if (plan.adoptRead !== null) {
      dispatch({ type: "READ_STARTED", readId: plan.adoptRead.id });
    }
    if (plan.companyConfirmed && plan.resumeTarget !== null) {
      dispatch({ type: "RESUME", target: plan.resumeTarget });
    }
  }, [
    state.act,
    settled,
    wizard.data,
    existing.data,
    voice.data,
    persistedRead.data,
    routeConnect,
    locale,
    dispatch,
  ]);

  return {
    existing,
    voice,
    persistedRead,
    lookups: [existing, wizard, voice, persistedRead],
  };
}

export function OnboardingConversationScreen() {
  const route = useRoute();
  const [state, dispatch] = useReducer(
    conversationReducer,
    initialConversationState,
  );
  const { persist } = useWizardStatePersist();
  const { existing, voice, persistedRead, lookups } = useRestore(
    state,
    dispatch,
    route.id === "connect",
  );

  // Act-transition checkpoints: the server remembers where the journey is,
  // so a mid-onboarding reload restores to the right act with recap. Only
  // pairs a live user action produces persist; the restore's RESUME lands
  // from co.confirmed and matches none of them. Best-effort by design: a
  // failed checkpoint never stalls the act (the finish write in the connect
  // act is the one gate that must land before navigation).
  const prevPhase = useRef<ConversationPhase | null>(null);
  useEffect(() => {
    const prev = prevPhase.current;
    prevPhase.current = state.phase;
    if (prev === null || prev === state.phase) {
      return;
    }
    const checkpoint = actCheckpoint(
      prev,
      state.phase,
      state.lastBuildStatus === "succeeded",
    );
    if (checkpoint !== null) {
      void persist({ ...checkpoint, values: EMPTY_DRAFT.values });
    }
  }, [state.phase, state.lastBuildStatus, persist]);

  if (state.act === "welcome") {
    return <RestoreGate lookups={lookups} />;
  }

  return (
    <div className="ob-page ob-conv-page">
      {/* Above the act switch on purpose: this is the one level that survives an
          act change, so it is the only place that can know whether the workbench
          frame has already introduced itself. */}
      <WorkbenchEntranceScope>
        <CurrentAct
          state={state}
          dispatch={dispatch}
          route={route}
          persist={persist}
          existing={existing}
          voice={voice}
          persistedRead={persistedRead}
        />
      </WorkbenchEntranceScope>
    </div>
  );
}

// Which act is on screen, and nothing else. Extracted from the screen because
// the screen's job is the machine, the restore and the checkpoints — every act
// added here would otherwise make that function harder to read for a reason
// that has nothing to do with it.
function CurrentAct({
  state,
  dispatch,
  route,
  persist,
  existing,
  voice,
  persistedRead,
}: Readonly<{
  state: ConversationState;
  dispatch: Dispatch<ConversationEvent>;
  route: ReturnType<typeof useRoute>;
  persist: (input: WizardPersistInput) => Promise<boolean>;
  existing: ReturnType<typeof useCompany>;
  voice: UseQueryResult<VoiceRestoreProbe>;
  persistedRead: UseQueryResult<CompanySiteRead | null>;
}>) {
  switch (state.act) {
    case "company":
      return (
        <CompanyAct
          state={state}
          dispatch={dispatch}
          profile={existing.data ?? null}
          persist={persist}
          adoptedRead={
            persistedRead.data != null &&
            persistedRead.data.id === state.activeReadId
              ? persistedRead.data
              : null
          }
        />
      );
    case "voice":
      return (
        <VoiceAct
          state={state}
          dispatch={dispatch}
          initialSummary={voice.data?.summary ?? null}
        />
      );
    case "basis":
      return <BasisAct state={state} dispatch={dispatch} />;
    case "invite":
      return <InviteAct state={state} dispatch={dispatch} />;
    case "team":
      return <TeamAct state={state} dispatch={dispatch} persist={persist} />;
    // Every journey ends on the same handoff, once the act that closed it has
    // recorded completion.
    case "done":
      return <BuildScene onDone={() => navigate({ screen: "home" })} />;
    case "connect":
      return (
        <ConnectAct
          state={state}
          dispatch={dispatch}
          persist={persist}
          outcome={route.id === "connect" ? route.id2 : undefined}
          returningProvider={route.id === "connect" ? route.id3 : undefined}
        />
      );
    // The welcome act never reaches here: the screen renders the restore gate
    // for it and returns before this switch.
    case "welcome":
      return null;
  }
}
