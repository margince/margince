import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import {
  OnboardingStage,
  useStageTitleFocus,
} from "../../design-system/onboarding-stage";
import { useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import {
  usePlatformDeclined,
  useRememberPlatformDeclined,
} from "../../lib/installation-setup.decline";
import { CORE_LABELS } from "../../lib/onboarding-core-label";
import { throwProblem } from "../../lib/problem";
import { Ignition, useIgnitionCore } from "../onboarding/onboarding-ignition";
// The stylesheet carries the `onboarding-` prefix rather than this file's name
// on purpose: both onboarding CSS gates derive their corpus from that prefix,
// so a first-run sheet named after its component would style the coldest screen
// in the product from outside the censuses that keep the rest of it honest.
import "../onboarding/onboarding-first-run.css";
/**
 * What a fresh installation must be told before it can be used: the model
 * binding, which is the one step the server calls blocking. Everything else the
 * report names — the Google app today — is configured from settings, because an
 * installation with no Google app is fully usable and demanding one here locked
 * its operator out of every route.
 *
 * WHY THIS IS NOT THE SETTINGS CARDS. `AiRoutingCard` re-points lanes an
 * installation ALREADY binds — it says so and refuses an empty one, which is
 * precisely the cold start. This asks a different question: one vendor, one
 * key, two models, from nothing. The two surfaces write the same endpoints, and
 * the credential goes through `useSetProviderKey` rather than a second mutation,
 * because how long a key lives in memory is not a rule worth having two of.
 *
 * WHY IT READS THE SERVER RATHER THAN COUNTING STEPS ITSELF. The reader signs
 * in, changes their password and signs in AGAIN before they arrive here, and
 * they may close the tab in the middle. `GET /installation/setup` is the only
 * thing that knows what is still outstanding, so the screen resumes rather than
 * restarting — and it walks `steps` in the order it arrives, because that order
 * is the server's to decide.
 */
import { AiStep } from "./installation-setup.ai-step";
import { PlatformStep } from "./installation-setup.platform";

type Setup = components["schemas"]["InstallationSetup"];

type Step = components["schemas"]["InstallationSetupStep"];

export function useInstallationSetup() {
  return useQuery({
    queryKey: ["installation-setup"],
    queryFn: async (): Promise<Setup> => {
      const { data, error, response } = await api.GET("/installation/setup");
      if (error || !response.ok || !data) {
        throwProblem(error);
      }
      return data;
    },
  });
}

/**
 * The steps this screen has a panel for, in the order the server reports
 * them: the model binding, which blocks, and the company's OAuth app,
 * which does not — held by TestOnlyTheModelBindingBlocksFirstRun.
 *
 * It is read by `outstandingStep` rather than only by the render, and that is
 * the point. A frontend older than its server would otherwise meet a blocking
 * step it has no panel for, and the caller — which gates on this same
 * function — would keep drawing a component that renders nothing: a reader
 * held behind an empty screen, with nothing on it to say what it wants.
 */
const ASKABLE_STEPS: readonly Step["step"][] = ["ai_models", "oauth_app"];

/**
 * The first step that is not done yet AND that this screen can ask for, in the
 * server's order. A blocking step is always outstanding; the app step, which
 * does not block, is outstanding until it is configured or declined.
 *
 * `steps` is optional-chained as well as `setup`: an answer that arrives
 * without it is not an answer, and the alternative is this throwing inside a
 * render — which takes down the screen it was meant to stand in front of.
 * Undefined here reads as "nothing outstanding", which lets the reader past
 * rather than trapping them behind a gate that cannot say what it wants.
 */
export function outstandingStep(
  setup: Setup | undefined,
  platformDeclined: boolean,
): Step | undefined {
  return setup?.steps?.find(
    (s) =>
      !s.configured &&
      ASKABLE_STEPS.includes(s.step) &&
      (s.blocking || !platformDeclined),
  );
}

// What the room says while a step is answered, and for the four seconds after
// the binding lands, which carry no step's status line: it would be stale.
function head(
  step: Step["step"],
  ignited: boolean,
): Readonly<{ eyebrow?: MessageKey; title: MessageKey; sub: MessageKey }> {
  if (ignited) {
    return { title: "firstRun.ignite.title", sub: "firstRun.ignite.sub" };
  }
  return step === "oauth_app"
    ? {
        eyebrow: "firstRun.google.eyebrow",
        title: "firstRun.platform.title",
        sub: "firstRun.platform.sub",
      }
    : {
        eyebrow: "firstRun.ai.eyebrow",
        title: "firstRun.ai.title",
        sub: "firstRun.ai.sub",
      };
}

// Which board is up, for the render, the room and the focus. The ignition
// holds only while the report asks for the model, so moving past it ends it.
function boardOf(
  step: Step | undefined,
  ignited: string | null,
): "model" | "ignition" | "platform" | undefined {
  if (step === undefined) {
    return undefined;
  }
  if (step.step === "oauth_app") {
    return "platform";
  }
  return ignited === null ? "model" : "ignition";
}

function modelBound(setup: Setup | undefined): boolean {
  return (
    setup?.steps?.some((s) => s.step === "ai_models" && s.configured) ?? false
  );
}

/**
 * The setup gate. Renders nothing once the server says the installation is
 * complete, so the caller can put it in front of whatever comes next without
 * asking a second time whether it should.
 *
 * THE STAGE IS HERE RATHER THAN INSIDE EACH STEP, and that is the whole reason
 * the light works. A stage per step means React unmounts one and mounts the
 * next when the binding lands, and a CSS transition does not run on a mount:
 * the room would snap lit instead of coming up, and the Core would lose its GL
 * context and blink between the two questions. One stage, held across the flip,
 * and only its children change.
 */
export function InstallationSetup() {
  const t = useT();
  const queryClient = useQueryClient();
  const setup = useInstallationSetup();
  const declined = usePlatformDeclined();
  const decline = useRememberPlatformDeclined();
  const step = outstandingStep(setup.data, declined);
  // Owned here because the Core belongs to the stage and the write belongs to
  // the step. The step says when it is writing; nothing reads this but the orb.
  const [busy, setBusy] = useState(false);
  // The binding landed and the sequence is playing: the ROOM's state, so held
  // here. The vendor travels with it, since "sealed in the vault" without whose
  // key is a sentence about a mechanism, not about what the reader just did.
  const [ignited, setIgnited] = useState<string | null>(null);
  const board = boardOf(step, ignited);
  const ignition = board === "ignition";
  useStageTitleFocus(board);
  const igniting = useIgnitionCore(ignition);

  // While the answer has not arrived, nothing: a step drawn from a guess would
  // be replaced a moment later by the real one, and the reader would have
  // started typing into it.
  //
  // Nothing again once every blocking step is done — so a caller can put this
  // in front of whatever comes next without asking the same question twice, and
  // there is ONE reader of what is outstanding rather than a component and its
  // parent each holding an opinion.
  if (setup.isPending || !step) {
    return null;
  }
  const core = ignition ? igniting.state : busy ? "working" : ("idle" as const);
  const heading = head(step.step, ignition);
  return (
    <OnboardingStage
      flow={t("ob.stage.flow")}
      // The room lights the moment the binding lands, a beat before the server
      // is asked again. That is not a second meaning for the indigo — it is the
      // same claim, made by the client that just watched the write succeed
      // rather than by the read that confirms it.
      lit={ignition || modelBound(setup.data)}
      coreState={core}
      coreProgress={igniting.progress}
      coreFlash={ignition}
      // The Core is aria-hidden, so the band says in words what it is showing.
      // From `ob.core.*`, the vocabulary every onboarding surface reads: the
      // orb showing the same state on two screens must not read as two
      // different things.
      coreStateLabel={t(CORE_LABELS[core])}
      // The band names the step rather than counting it: two questions are
      // not a journey, and a counter over them would be ceremony.
      step={t(
        step.step === "oauth_app"
          ? "firstRun.step.platform"
          : "firstRun.step.model",
      )}
      eyebrow={heading.eyebrow && t(heading.eyebrow)}
      // The one qualification the step carries, on the card's bottom edge. It
      // is true of the whole screen rather than of any field on it, which is
      // what makes it chrome: as a callout in the board it read as an
      // instruction and pushed the actual form below the fold.
      hint={t(
        step.step === "oauth_app"
          ? "firstRun.platform.foot"
          : "firstRun.ai.foot",
      )}
      title={t(heading.title)}
      sub={t(heading.sub)}
    >
      {board === "platform" ? (
        <PlatformStep onBusy={setBusy} onDecline={decline} />
      ) : ignited === null ? (
        <AiStep
          onBusy={setBusy}
          onIgnite={(vendor) => {
            setBusy(false);
            setIgnited(vendor);
          }}
        />
      ) : (
        <Ignition
          vendor={ignited}
          onDone={async () => {
            // NOW the server is asked again, and only its answer lets go of
            // the ignition: the stale report would draw the model form again.
            await queryClient.invalidateQueries(
              { queryKey: ["installation-setup"] },
              { throwOnError: true },
            );
            setIgnited(null);
          }}
        />
      )}
    </OnboardingStage>
  );
}
