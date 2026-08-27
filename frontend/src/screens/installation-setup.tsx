import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Button, Field, TextInput } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { ComboBox } from "../design-system/combobox";
import { Panel, PanelBody } from "../design-system/panel";
import { Select } from "../design-system/select";
import { useT } from "../i18n";
import { suggestionsFor, useAiModelCatalogue } from "./ai-models";
import { useSetProviderKey } from "./ai-provider-keys";
import { problemMessageOf, throwProblem } from "./common";
import { useSetGoogleApp } from "./google-app";
import {
  SETUP_PROVIDER_IDS,
  SETUP_PROVIDERS,
  type SetupProviderId,
} from "./setup-providers";

/**
 * What a fresh installation must be told before it can be used, asked in the
 * order the server lists it: the model binding, then the Google app.
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
 * The first step that is not done yet, in the server's order.
 *
 * `steps` is optional-chained as well as `setup`: an answer that arrives
 * without it is not an answer, and the alternative is this throwing inside a
 * render — which takes down the screen it was meant to stand in front of.
 * Undefined here reads as "nothing outstanding", which lets the reader past
 * rather than trapping them behind a gate that cannot say what it wants.
 */
export function outstandingStep(setup: Setup | undefined): Step | undefined {
  return setup?.steps?.find((s) => s.blocking && !s.configured);
}

function useBindModels() {
  const queryClient = useQueryClient();
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
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: ["installation-setup"] }),
  });
}

/** The AI step: one vendor, one key, the two models it will serve. */
function AiStep() {
  const t = useT();
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
                // Nothing else: both mutations invalidate the setup query, so
                // what is still outstanding comes back from the server rather
                // than from this component's guess about what it just did.
              },
            },
          ),
      },
    );
  };

  return (
    <Panel title={t("firstRun.ai.title")}>
      <PanelBody>
        <p className="t-body">{t("firstRun.ai.sub")}</p>
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
        <Field
          label={t("firstRun.ai.key")}
          hint={t("firstRun.ai.keyHint", { envVar: preset.keyEnv })}
        >
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
          hint={t("firstRun.ai.modelHint")}
        >
          {(control) => (
            <ComboBox
              {...control}
              value={chatModel}
              suggestions={suggestionsFor(
                catalogue.data,
                preset.provider,
                "chat",
              )}
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
              )}
              disabled={busy}
              onChange={setEmbedModel}
            />
          )}
        </Field>
        <Button
          variant="primary"
          pending={busy}
          disabled={!ready}
          onClick={submit}
        >
          {t("firstRun.continue")}
        </Button>
      </PanelBody>
    </Panel>
  );
}

/** The Google step: the OAuth app a mailbox connection is made through. */
function GoogleStep() {
  const t = useT();
  const [clientId, setClientId] = useState("");
  const [clientSecret, setClientSecret] = useState("");
  const save = useSetGoogleApp();
  const ready = clientId.trim() !== "" && clientSecret.trim() !== "";

  return (
    <Panel title={t("firstRun.google.title")}>
      <PanelBody>
        <p className="t-body">{t("firstRun.google.sub")}</p>
        {save.error && (
          <Callout tone="danger">{problemMessageOf(save.error, t)}</Callout>
        )}
        <Field label={t("firstRun.google.clientId")}>
          {(control) => (
            <TextInput
              {...control}
              value={clientId}
              disabled={save.isPending}
              autoComplete="off"
              placeholder={t("firstRun.google.clientIdPlaceholder")}
              onChange={(e) => setClientId(e.target.value)}
            />
          )}
        </Field>
        <Field label={t("firstRun.google.clientSecret")}>
          {(control) => (
            <TextInput
              {...control}
              type="password"
              autoComplete="off"
              value={clientSecret}
              disabled={save.isPending}
              onChange={(e) => setClientSecret(e.target.value)}
            />
          )}
        </Field>
        <Button
          variant="primary"
          pending={save.isPending}
          disabled={!ready}
          onClick={() => {
            save.reset();
            save.mutate(
              { clientId: clientId.trim(), clientSecret: clientSecret.trim() },
              {
                onSuccess: () => {
                  // Cleared on the way out rather than left in state: the field
                  // is the only copy this app holds, and it has done its job.
                  setClientSecret("");
                  save.reset();
                },
              },
            );
          }}
        >
          {t("firstRun.continue")}
        </Button>
      </PanelBody>
    </Panel>
  );
}

/**
 * The setup gate. Renders nothing once the server says the installation is
 * complete, so the caller can put it in front of whatever comes next without
 * asking a second time whether it should.
 */
export function InstallationSetup() {
  const setup = useInstallationSetup();
  const step = outstandingStep(setup.data);

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
  return step.step === "ai_models" ? <AiStep /> : <GoogleStep />;
}
