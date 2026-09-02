import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { useEffect, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Button, Field, TextInput } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { ComboBox } from "../design-system/combobox";
import { OnboardingStage } from "../design-system/onboarding-stage";
import { Panel, PanelBody } from "../design-system/panel";
import { Select } from "../design-system/select";
import { useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import {
  suggestionsFor,
  useAiModelCatalogue,
  useVendorCatalogue,
  type VendorCatalogue,
  vendorSuggestions,
} from "./ai-models";
import { useSetProviderKey } from "./ai-provider-keys";
import { ModelRatePlate } from "./ai-rates";
import { problemMessageOf, throwProblem } from "./common";
import { CORE_LABELS } from "./onboarding-core-label";
import { Ignition, useIgnitionCore } from "./onboarding-ignition";
// The stylesheet carries the `onboarding-` prefix rather than this file's name
// on purpose: both onboarding CSS gates derive their corpus from that prefix,
// so a first-run sheet named after its component would style the coldest screen
// in the product from outside the censuses that keep the rest of it honest.
import "./onboarding-first-run.css";
import {
  SETUP_PROVIDER_IDS,
  SETUP_PROVIDERS,
  type SetupProviderId,
} from "./setup-providers";

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
 * The steps this screen has a panel for. Today the model binding is the whole
 * list, and the server agrees — it reports every other step non-blocking, held
 * by TestOnlyTheModelBindingBlocksFirstRun.
 *
 * It is read by `outstandingStep` rather than only by the render, and that is
 * the point. A frontend older than its server would otherwise meet a blocking
 * step it has no panel for, and the caller — which gates on this same
 * function — would keep drawing a component that renders nothing: a reader held
 * behind an empty screen, with nothing on it to say what it wants.
 */
const ASKABLE_STEPS: readonly Step["step"][] = ["ai_models"];

/**
 * The first step that is not done yet AND that this screen can ask for, in the
 * server's order.
 *
 * `steps` is optional-chained as well as `setup`: an answer that arrives
 * without it is not an answer, and the alternative is this throwing inside a
 * render — which takes down the screen it was meant to stand in front of.
 * Undefined here reads as "nothing outstanding", which lets the reader past
 * rather than trapping them behind a gate that cannot say what it wants. A
 * blocking step with no panel takes the same exit, for the same reason: past a
 * gate is somewhere, and a settings screen can still be reached from there.
 */
export function outstandingStep(setup: Setup | undefined): Step | undefined {
  return setup?.steps?.find(
    (s) => s.blocking && !s.configured && ASKABLE_STEPS.includes(s.step),
  );
}

function useBindModels() {
  return useMutation({
    // Same reasoning as the provider-key mutation: nothing here is a secret,
    // but the two settle together and a stale binding on screen after a
    // success is the same confusion.
    gcTime: 0,
    mutationFn: async (vars: {
      provider: string;
      baseUrl?: string;
      chatModel: string;
      embedModel: string;
    }) => {
      // Every chat tier on the one model the reader chose. A tier left unbound
      // degrades honestly at runtime, but an onboarding that bound only some of
      // them would have the product answer for one task and refuse another with
      // no way for the reader to tell which they had configured.
      const binding = {
        provider: vars.provider,
        model: vars.chatModel,
        ...(vars.baseUrl ? { base_url: vars.baseUrl } : {}),
      };
      const { error } = await api.PUT("/ai/routing", {
        body: {
          // eu_hosted rather than a question: `sovereign` forbids the cloud
          // vendors this screen offers, and asking a first-time admin to choose
          // a location ladder before they have bound anything is asking them to
          // answer a question they cannot yet have.
          profile: "eu_hosted",
          tiers: {
            local_small: binding,
            cheap_cloud: binding,
            premium: binding,
            frontier: binding,
          },
          embeddings: {
            provider: vars.provider,
            model: vars.embedModel,
            ...(vars.baseUrl ? { base_url: vars.baseUrl } : {}),
          },
        },
      });
      if (error) {
        throwProblem(error);
      }
    },
    // NO invalidation here, and this is the one mutation in the file that holds
    // it back. Re-reading the setup report is what moves the screen on, and the
    // binding is the moment the ignition exists to mark — invalidating on
    // success would swap the step out from under a sequence the reader is
    // watching. The refetch happens when they press past it (`onDone`), which
    // means the screen is theirs to leave rather than the query's to take.
    //
    // The write has already landed either way: a reload mid-sequence finds the
    // server saying `ai_models` is configured and opens the next question, which
    // is correct and loses nothing but the ceremony.
  });
}

/**
 * A step's form, telling the stage above it when a write is in flight.
 *
 * The Core is the stage's, and whether anything is being written is the step's,
 * so one of them has to say so to the other. It travels up rather than the
 * mutation moving down: `useSetProviderKey` and `useSetGoogleApp` are where they
 * are for reasons of their own (how long a credential lives in memory), and
 * lifting a mutation to make an orb spin would be the tail wagging the dog.
 */
function StepForm({
  busy,
  onBusy,
  children,
}: Readonly<{
  busy: boolean;
  onBusy: (busy: boolean) => void;
  children: ReactNode;
}>) {
  useEffect(() => {
    onBusy(busy);
    // A step that unmounts mid-write leaves the stage claiming work nobody is
    // doing, and the next step would open with a spinning orb.
    return () => onBusy(false);
  }, [busy, onBusy]);
  return <div className="ob-fr-form">{children}</div>;
}

/** The AI step: one vendor, one key, the two models it will serve. */
/**
 * What the chat field's list is made of, added to the field's own hint.
 *
 * Silent for a vendor with no public catalogue, because there the list IS the
 * price sheet and the ordinary hint already says so. When there is a live list,
 * it names the MEASURE: an order presented as "top ten" with no measure behind
 * it is a claim the reader cannot check, and the measure is a third party's
 * published one rather than anything this product decided.
 */
function chatListNote(
  t: ReturnType<typeof useT>,
  vendor: VendorCatalogue | undefined,
): string {
  if (vendor === undefined) {
    return "";
  }
  if (vendor.unavailable) {
    return t("firstRun.ai.rankedUnavailable");
  }
  if (vendor.models.length === 0) {
    return "";
  }
  return t("firstRun.ai.rankedHint", { rankedBy: vendor.rankedBy });
}

function AiStep({
  onBusy,
  onIgnite,
}: Readonly<{
  onBusy: (busy: boolean) => void;
  /** Both writes landed, for the vendor named. The stage takes it from here. */
  onIgnite: (vendor: string) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const [choice, setChoice] = useState<SetupProviderId>("gemini");
  const preset = SETUP_PROVIDERS[choice];
  const [apiKey, setApiKey] = useState("");
  const [chatModel, setChatModel] = useState(preset.chatModel);
  const [embedModel, setEmbedModel] = useState(preset.embedModel);
  const saveKey = useSetProviderKey();
  const bind = useBindModels();
  // What this installation can already price. On a fresh install that is the
  // seeded sheet, so the two fields open with the vendor's own family in them
  // rather than with one id and no way to learn a second.
  const catalogue = useAiModelCatalogue();
  // What the vendor is serving TODAY, which the sheet cannot know. Asked only
  // of a vendor whose catalogue is public, which is OpenRouter alone, and only
  // once the reader has chosen it.
  const vendor = useVendorCatalogue(
    choice === "openrouter" ? "openrouter" : undefined,
  );

  // Switching vendor re-seeds the models, because the previous vendor's ids
  // mean nothing to this one — leaving them would offer a binding that cannot
  // serve a single call.
  const pick = (next: SetupProviderId) => {
    setChoice(next);
    setChatModel(SETUP_PROVIDERS[next].chatModel);
    setEmbedModel(SETUP_PROVIDERS[next].embedModel);
  };

  const busy = saveKey.isPending || bind.isPending;
  const ready =
    apiKey.trim() !== "" && chatModel.trim() !== "" && embedModel.trim() !== "";
  const failure = saveKey.error ?? bind.error;

  // The KEY first, then the binding. A binding whose vendor has no key is an
  // installation that reads as configured and fails on its first real call; a
  // key with no binding is simply a key, and the next attempt completes it.
  //
  // Chained through callbacks rather than awaited: mutateAsync REJECTS, and the
  // rejection has to be handled somewhere or it is an unhandled promise. More
  // importantly the field is cleared only after BOTH writes land. Clearing it
  // after the first one locked the reader out — a failed binding left `ready`
  // false with the Continue button disabled, the step still unconfigured, and a
  // reload restoring exactly that, so the only way on was to re-paste a key the
  // server already held.
  const submit = () => {
    saveKey.reset();
    bind.reset();
    saveKey.mutate(
      { provider: preset.provider, apiKey: apiKey.trim() },
      {
        onSuccess: () =>
          bind.mutate(
            {
              provider: preset.provider,
              baseUrl: preset.baseUrl,
              chatModel: chatModel.trim(),
              embedModel: embedModel.trim(),
            },
            {
              onSuccess: () => {
                // Both landed, so the field has done its job and this is the
                // only copy of the key the app was holding.
                setApiKey("");
                saveKey.reset();
                // And the screen becomes the ignition. Nothing is refetched
                // yet: see `useBindModels` for why the reader, not the query,
                // decides when this step is over.
                onIgnite(preset.label);
              },
            },
          ),
      },
    );
  };

  return (
    <StepForm busy={busy} onBusy={onBusy}>
      {/* The verb goes in the card's own action strip rather than as the last
          thing in the stack of questions: a button that shares the fields'
          spacing reads as one more field. */}
      <Panel
        footer={
          <Button
            variant="primary"
            pending={busy}
            disabled={!ready}
            onClick={submit}
          >
            {t("firstRun.continue")}
          </Button>
        }
      >
        <PanelBody>
          {failure && (
            <Callout tone="danger">{problemMessageOf(failure, t)}</Callout>
          )}
          <Field label={t("firstRun.ai.provider")}>
            {(control) => (
              <Select
                {...control}
                value={choice}
                disabled={busy}
                // Narrowed THROUGH the id list rather than asserted into it: a
                // Select answers with a string, and an answer that is not one of
                // these is not a provider this screen can bind.
                onChange={(v) => {
                  const next = SETUP_PROVIDER_IDS.find((id) => id === v);
                  if (next) {
                    pick(next);
                  }
                }}
                options={SETUP_PROVIDER_IDS.map((id) => ({
                  value: id,
                  label: SETUP_PROVIDERS[id].label,
                }))}
              />
            )}
          </Field>
          <Field label={t("firstRun.ai.key")} hint={t("firstRun.ai.keyHint")}>
            {(control) => (
              <TextInput
                {...control}
                // A password field so the browser does not offer to remember a
                // credential this app never stores client-side, and a screenshare
                // does not carry it.
                type="password"
                autoComplete="off"
                value={apiKey}
                disabled={busy}
                onChange={(e) => setApiKey(e.target.value)}
              />
            )}
          </Field>
          {/* Both fields offer what the sheet can price for the chosen vendor,
            in the lane that field binds — and take anything typed, because the
            server accepts any id its vendor serves and the whole point of the
            preset below is that it is a starting point. */}
          <Field
            label={t("firstRun.ai.chatModel")}
            hint={`${t("firstRun.ai.modelHint")} ${chatListNote(t, vendor.data)}`.trim()}
          >
            {(control) => (
              <ComboBox
                {...control}
                value={chatModel}
                // The sheet first, then what the vendor is serving today. In
                // that order because the sheet's rows are the ones this
                // installation can already price, and the live rows are the
                // answer to a different question: what else is there.
                suggestions={[
                  ...suggestionsFor(
                    catalogue.data,
                    preset.provider,
                    "chat",
                    locale,
                  ),
                  ...vendorSuggestions(
                    vendor.data,
                    catalogue.data,
                    preset.provider,
                    locale,
                  ),
                ]}
                disabled={busy}
                onChange={setChatModel}
              />
            )}
          </Field>
          <Field label={t("firstRun.ai.embedModel")}>
            {(control) => (
              <ComboBox
                {...control}
                value={embedModel}
                suggestions={suggestionsFor(
                  catalogue.data,
                  preset.provider,
                  "embeddings",
                  locale,
                )}
                disabled={busy}
                onChange={setEmbedModel}
              />
            )}
          </Field>
          {/* What the two ids above will cost, before the binding is written
            rather than in a usage report a month later. It follows the fields
            because it is the CONSEQUENCE of them: it re-reads on every
            keystroke, so a model typed by hand shows its price — or says it
            has none — while the reader is still deciding. */}
          <ModelRatePlate
            catalogue={catalogue.data}
            vendor={vendor.data}
            provider={preset.provider}
            chatModel={chatModel}
            embedModel={embedModel}
            locale={locale}
          />
        </PanelBody>
      </Panel>
    </StepForm>
  );
}

// What the room says while the one step is answered, and for the four seconds
// after it lands. Two states rather than one per step: first run asks a single
// question now, and the ignition is not a step but the moment the answer takes
// effect.
function head(
  ignited: boolean,
): Readonly<{ title: MessageKey; sub: MessageKey }> {
  return ignited
    ? { title: "firstRun.ignite.title", sub: "firstRun.ignite.sub" }
    : { title: "firstRun.ai.title", sub: "firstRun.ai.sub" };
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
  const step = outstandingStep(setup.data);
  // Owned here because the Core belongs to the stage and the write belongs to
  // the step. The step says when it is writing; nothing reads this but the orb.
  const [busy, setBusy] = useState(false);
  // The binding landed and the reader is watching the sequence. Held here
  // rather than in the step, because what it changes is the ROOM: the light, the
  // orb and what stands in the column all belong to the stage.
  //
  // The vendor travels with it because the sequence names whose key was sealed,
  // and "sealed in the vault" without saying whose is a sentence about a
  // mechanism rather than about what the reader just did.
  const [ignited, setIgnited] = useState<string | null>(null);
  const igniting = useIgnitionCore(ignited !== null);

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
  const core =
    ignited !== null ? igniting.state : busy ? "working" : ("idle" as const);
  return (
    <OnboardingStage
      flow={t("ob.stage.flow")}
      // The room lights the moment the binding lands, a beat before the server
      // is asked again. That is not a second meaning for the indigo — it is the
      // same claim, made by the client that just watched the write succeed
      // rather than by the read that confirms it.
      lit={ignited !== null || modelBound(setup.data)}
      coreState={core}
      coreProgress={igniting.progress}
      coreFlash={ignited !== null}
      // The Core is aria-hidden, so the band says in words what it is showing.
      // From `ob.core.*`, the vocabulary every onboarding surface reads: the
      // orb showing the same state on two screens must not read as two
      // different things.
      coreStateLabel={t(CORE_LABELS[core])}
      // ONE step, so the band names it rather than counting it. The model
      // binding is the whole of first run — the Google app is configured from
      // settings, where the card can also show the redirect URIs Google's
      // console asks for — and a progress counter over a flow of one would be
      // ceremony inventing a journey.
      step={t("firstRun.step.model")}
      eyebrow={t("firstRun.ai.eyebrow")}
      // The one qualification the step carries, on the card's bottom edge. It
      // is true of the whole screen rather than of any field on it, which is
      // what makes it chrome: as a callout in the board it read as an
      // instruction and pushed the actual form below the fold.
      hint={t("firstRun.ai.foot")}
      title={t(head(ignited !== null).title)}
      sub={t(head(ignited !== null).sub)}
    >
      {ignited === null ? (
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
          onDone={() => {
            setIgnited(null);
            // NOW the server is asked again, and the answer is what moves the
            // screen — the same rule every other step follows, just deferred
            // until the reader was done with this one.
            queryClient.invalidateQueries({ queryKey: ["installation-setup"] });
          }}
        />
      )}
    </OnboardingStage>
  );
}
