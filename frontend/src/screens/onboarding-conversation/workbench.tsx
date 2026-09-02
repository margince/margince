import { useQuery } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { createContext, useContext, useEffect, useRef, useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import type { MarginceCoreState } from "../../design-system/margince-core";
import { AiRuntimeChip } from "../../design-system/margince-workbench";
import {
  OnboardingStage,
  type StageProgress,
} from "../../design-system/onboarding-stage";
import { useLocale, useT } from "../../i18n";
import { throwProblem } from "../common";
import { configuredModelLabel } from "../onboarding-read";
import type { ConversationState } from "./conversation-types";
import { loadWizardState } from "./index";
import { isDetour, railStops, stopState } from "./rail";

// The one room every conversation act is played in: the band, the Core, the
// board, the rail. Acts supply only what differs — the question, the scene that
// answers it, and the way onward.
//
// ONE ROOM, NOT TWO PANES. This shell used to be MarginceWorkbench: a chat rail
// on the left and a live artifact pane on the right. Onboarding is one question
// at a time on one stage, and the two organising ideas cannot both be true on
// the same screen — a reader crossing from the gate into the journey walked out
// of one room and into another halfway through a single setup. The workbench
// stays in the design system, for the product after setup, which is where its
// conversation and its artifact both belong.

type AiRunSummary = components["schemas"]["AiRunSummary"];
type AiProfile = components["schemas"]["AiProfile"];
type CompanySiteRead = components["schemas"]["CompanySiteRead"];

// The detailed AI profile, and the label every onboarding surface names the
// configured model with. One hook so the gate, the read theatre and the
// workbench cannot disagree about what is answering — and one ["ai-profile"]
// cache entry, so naming it in three places still costs one request.
export function useConfiguredModel(): string {
  const t = useT();
  const profile = useQuery({
    queryKey: ["ai-profile"],
    queryFn: async (): Promise<AiProfile> => {
      const { data, error } = await api.GET("/ai/profile");
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    staleTime: Number.POSITIVE_INFINITY,
  });
  return configuredModelLabel(profile.data, t("ob.ai.runtimeUnavailable"), t);
}

/**
 * What this cold start has spent, for every screen that is not the one holding
 * the number.
 *
 * THE READ IS WHERE A COLD START SPENDS. The crawl, the extract and each
 * clarify are charged to the site read; nothing before it or after it calls a
 * model. So the read's own summary is the setup's bill rather than a fragment
 * of it, and a screen later in the flow showing nothing was not being careful,
 * it was dropping the figure on the floor.
 *
 * A FALLBACK, never an override: an act holding the live summary passes it and
 * wins, because during a running read that copy is fresher than any poll of
 * this one. Both go through the same `["onboarding-conv-read", readId]` entry,
 * so naming it on five screens still costs one request.
 */
function useSetupRuntime(): AiRunSummary | undefined {
  const wizard = useQuery({
    queryKey: ["onboarding-conv-state"],
    queryFn: loadWizardState,
  });
  const readId = wizard.data?.site_read_id ?? null;
  const read = useQuery({
    queryKey: ["onboarding-conv-read", readId],
    enabled: readId !== null,
    queryFn: async (): Promise<CompanySiteRead | null> => {
      const { data, error, response } = await api.GET(
        "/company/site-reads/{readId}",
        { params: { path: { readId: readId ?? "" } } },
      );
      if (error) {
        // A read the server no longer serves costs the band its figure, not
        // the screen: the chip says it has nothing, nothing throws.
        if (response.status === 404) {
          return null;
        }
        throwProblem(error);
      }
      return data;
    },
  });
  return read.data?.ai_runtime;
}

/**
 * Whether the stage appearing right now is the FIRST one of this setup.
 *
 * Each act mounts its own shell, so the entrance animation replayed on every act
 * change: the rail, the brand line, the orb and the runtime chip — the parts that
 * are meant to be the stable frame — faded and rose again each time the reader
 * moved forward. The shell cannot remember this itself, because it is exactly
 * what unmounts; the scope sits above the act switch, which does not.
 * What is shared is a mutable box, not a boolean: the answer has to flip when a
 * SHELL appears, and the company act opens on the full-screen gate, so anything
 * keyed on the scope's own mount would already have flipped before the first
 * workbench ever rendered.
 */
const WorkbenchShown = createContext<{ current: boolean }>({ current: false });

export function WorkbenchEntranceScope({
  children,
}: Readonly<{ children: ReactNode }>) {
  const shown = useRef(false);
  return (
    <WorkbenchShown.Provider value={shown}>{children}</WorkbenchShown.Provider>
  );
}

// Frozen per mount in state, not recomputed per render: the shell re-renders on
// every poll, and an entrance that evaluated again would drop its own class
// mid-animation.
function useFirstWorkbench(): boolean {
  const shown = useContext(WorkbenchShown);
  const [first] = useState(() => !shown.current);
  useEffect(() => {
    shown.current = true;
  }, [shown]);
  return first;
}

export function ConversationWorkbench({
  core,
  progress,
  status,
  runtime,
  railState,
  eyebrow,
  title,
  sub,
  where,
  hint,
  actions,
  children,
}: Readonly<{
  core: MarginceCoreState;
  progress?: number;
  status: string;
  runtime?: AiRunSummary;
  railState: ConversationState;
  eyebrow?: string;
  /** The question this screen is asking, as the board's own headline. */
  title: string;
  sub?: string;
  /**
   * Which part of the current stop this screen is, beside the stop's name.
   *
   * A stop can take several screens — Confirm holds the fact deck AND the
   * entity question — and the dashes cannot say that: they count stops.
   */
  where?: string;
  hint?: string;
  actions?: ReactNode;
  /** The board: what the reader is here to answer. */
  children: ReactNode;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const configured = useConfiguredModel();
  const setupRuntime = useSetupRuntime();
  const first = useFirstWorkbench();
  // Built here rather than per act, so four acts cannot drift into four
  // different ideas of where the journey is.
  const stops = railStops(railState.memberPath);
  const steps = stops.map((stop) => ({
    label: t(stop.labelKey),
    state: stopState(stop.key, railState),
  }));
  // Which dash is lit, and which stop the band names. The read runs before any
  // stop is current, and the band has nothing to say then rather than an
  // invented first one.
  const current = steps.findIndex((step) => step.state === "now");
  const stageProgress: StageProgress | undefined =
    current < 0
      ? undefined
      : { steps: steps.map((step) => step.label), at: current };
  // A detour is not a stop: it is somewhere the flow went from one, so it takes
  // the sub-label beside the stop's name rather than a dash of its own. The
  // screen's own `where` wins, because it knows which detour this is.
  const bandWhere =
    where ?? (isDetour(railState) ? t("ob.conv.scene.detour") : undefined);
  return (
    // ob-workbench-panel stays although the workbench does not: entries.tsx
    // resolves the composer through it, so it is a behavioural contract rather
    // than a styling hook.
    //
    // ob-panel carries nothing but the entrance, so it is worn once: the room
    // assembles when it first appears, and thereafter the frame is simply there
    // while the acts change inside it.
    <section className={`ob-workbench-panel${first ? " ob-panel" : ""}`}>
      <OnboardingStage
        flow={t("ob.stage.flow")}
        // Nothing reaches an act until first run has bound a model, so the room
        // is lit for all of them. Deriving it per act would be a second trigger
        // for the one thing the indigo means.
        lit
        coreState={core}
        coreProgress={progress}
        coreScale="work"
        anchor="start"
        coreStateLabel={status}
        aside={
          // The band's right slot, and the reason this screen can be trusted
          // about what it costs: the same disclosure the workbench carries,
          // in the one room onboarding has.
          <AiRuntimeChip
            runtime={runtime ?? setupRuntime}
            configured={configured}
            locale={locale}
            labels={{
              configured: t("ob.ai.configured"),
              used: t("ob.ai.modelsUsed"),
              route: t("ob.ai.route"),
              calls: t("ob.ai.calls"),
              tokens: t("ob.ai.tokens"),
              latency: t("ob.ai.latency"),
              estimatedCost: t("ob.ai.estimatedCost"),
              partial: t("ob.ai.partialEstimate"),
              awaiting: t("ob.ai.awaitingModel"),
              unavailable: t("ob.ai.notAvailableYet"),
              chip: t("ob.ai.runtimeChip"),
              answering: t("ob.ai.answeringNow"),
              scope: t("ob.ai.runScope"),
              tokensShort: t("ob.rail.tokensUnit"),
            }}
          />
        }
        progress={stageProgress}
        where={bandWhere}
        eyebrow={eyebrow}
        title={title}
        sub={sub}
        hint={hint}
        actions={actions}
      >
        {children}
      </OnboardingStage>
    </section>
  );
}
