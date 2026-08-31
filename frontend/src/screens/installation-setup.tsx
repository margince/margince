import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { useEffect, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Button, Disclosure, Field, TextInput } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { ChoiceList } from "../design-system/choicelist";
import { ComboBox } from "../design-system/combobox";
import { OffsiteLink } from "../design-system/offsitelink";
import { OnboardingStage } from "../design-system/onboarding-stage";
import { Panel, PanelBody } from "../design-system/panel";
import { Select } from "../design-system/select";
import { useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { suggestionsFor, useAiModelCatalogue } from "./ai-models";
import { useSetProviderKey } from "./ai-provider-keys";
import { ModelRatePlate } from "./ai-rates";
import { problemMessageOf, throwProblem } from "./common";
import { useSetGoogleApp } from "./google-app";
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
function AiStep({ onBusy }: Readonly<{ onBusy: (busy: boolean) => void }>) {
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
                  locale,
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

/**
 * The redirect URIs a Google app has to authorize, and why they are written as
 * a PATTERN rather than a finished address.
 *
 * The host is `MARGINCE_API_BASE_URL` (falling back to
 * `MARGINCE_PUBLIC_BASE_URL`) — the api's externally-reachable base, which is
 * server configuration the browser cannot see. On a split deployment the SPA's
 * own origin is a different host, so deriving the URI from `location.origin`
 * would print a confident, wrong address — and a redirect URI that does not
 * match is the one Google failure that says nothing useful:
 * `redirect_uri_mismatch` at the consent screen, after the operator has already
 * finished here.
 *
 * So the screen shows the paths, names the host, and lets the operator supply
 * the one value only their deployment knows.
 */
const REDIRECT_PATHS: ReadonlyArray<{ label: MessageKey; path: string }> = [
  {
    label: "firstRun.google.helpRedirectMail",
    path: "/v1/connectors/gmail/callback",
  },
  {
    label: "firstRun.google.helpRedirectCalendar",
    path: "/v1/connectors/gcal/callback",
  },
  {
    label: "firstRun.google.helpRedirectSignIn",
    path: "/v1/auth/oidc/google/callback",
  },
];

/**
 * What the organization runs on — ONE answer covering mail and sign-in.
 *
 * They are separate mechanisms in the server and the same fact about a company:
 * an organization on Workspace reads mail through a Google app and signs its
 * people in with Google accounts, through that same app and the same console
 * entry. Two questions would ask somebody to state one fact twice and then keep
 * the two answers agreeing.
 */
const PLATFORMS = ["google", "microsoft", "other"] as const;
type Platform = (typeof PLATFORMS)[number];

// Typed by `Platform` in both directions: an answer with no copy fails, and
// copy for an answer that does not exist fails too. `operator` is absent on the
// Google path, and its absence is the statement — that path has nothing for
// somebody else to do, because the form below it is the whole of the work.
const PLATFORM_COPY: Readonly<
  Record<
    Platform,
    { label: MessageKey; what: MessageKey; operator?: MessageKey }
  >
> = {
  google: {
    label: "firstRun.platform.google",
    what: "firstRun.platform.googleWhat",
  },
  microsoft: {
    label: "firstRun.platform.microsoft",
    what: "firstRun.platform.microsoftWhat",
    operator: "firstRun.platform.microsoftOperator",
  },
  other: {
    label: "firstRun.platform.other",
    what: "firstRun.platform.otherWhat",
    operator: "firstRun.platform.otherOperator",
  },
};

/** Google's own console, where the app is created and the two values are read. */
const GOOGLE_CREDENTIALS_CONSOLE =
  "https://console.cloud.google.com/apis/credentials";

/**
 * Where the client id and secret come from, folded away.
 *
 * A fold rather than four paragraphs above the fields: an operator who has done
 * this before wants the two boxes, and one who has not needs every step. Open
 * by default would push the actual form below the fold for everybody.
 */
function GoogleAppHelp() {
  const t = useT();
  return (
    <Disclosure summary={t("firstRun.google.helpToggle")}>
      <ol className="ob-fr-help">
        <li>{t("firstRun.google.helpStep1")}</li>
        <li>{t("firstRun.google.helpStep2")}</li>
        <li>
          {t("firstRun.google.helpStep3")}
          <dl className="ob-fr-uris">
            {REDIRECT_PATHS.map((uri) => (
              <div key={uri.path}>
                <dt>{t(uri.label)}</dt>
                <dd className="t-mono">{`{host}${uri.path}`}</dd>
              </div>
            ))}
          </dl>
          <p className="ob-fr-help-note">
            {t("firstRun.google.helpRedirectHost", { host: "{host}" })}
          </p>
        </li>
        <li>{t("firstRun.google.helpStep4")}</li>
      </ol>
      <p className="ob-fr-help-note">
        <OffsiteLink href={GOOGLE_CREDENTIALS_CONSOLE}>
          {t("firstRun.google.helpConsole")}
        </OffsiteLink>
      </p>
      <p className="ob-fr-help-note">{t("firstRun.google.helpDocs")}</p>
    </Disclosure>
  );
}

/** The Google step: the OAuth app a mailbox connection is made through. */
function GoogleStep({ onBusy }: Readonly<{ onBusy: (busy: boolean) => void }>) {
  const t = useT();
  const [platform, setPlatform] = useState<Platform>("google");
  const [clientId, setClientId] = useState("");
  const [clientSecret, setClientSecret] = useState("");
  const save = useSetGoogleApp();
  // The app is what this step WRITES, whatever the platform answer is — so a
  // reader on the Microsoft or IMAP path can still finish by pasting one, which
  // is the only way past the gate today. The fields are theirs to use rather
  // than hidden: hiding the one control that can complete the step would leave
  // them with a refusal and no way to answer it.
  const ready = clientId.trim() !== "" && clientSecret.trim() !== "";
  // What this answer leaves for whoever runs the server. Absent on the Google
  // path, which is also what says whether the gap notice below belongs here.
  const operatorWork = PLATFORM_COPY[platform].operator;

  return (
    <StepForm busy={save.isPending} onBusy={onBusy}>
      <Panel
        footer={
          <Button
            variant="primary"
            pending={save.isPending}
            disabled={!ready}
            onClick={() => {
              save.reset();
              save.mutate(
                {
                  clientId: clientId.trim(),
                  clientSecret: clientSecret.trim(),
                },
                {
                  onSuccess: () => {
                    // Cleared on the way out rather than left in state: the
                    // field is the only copy this app holds, and it has done
                    // its job.
                    setClientSecret("");
                    save.reset();
                  },
                },
              );
            }}
          >
            {t("firstRun.continue")}
          </Button>
        }
      >
        <PanelBody>
          <ChoiceList<Platform>
            legend={t("firstRun.platform.legend")}
            hideLegend
            value={platform}
            disabled={save.isPending}
            onChange={setPlatform}
            choices={PLATFORMS.map((id) => ({
              value: id,
              label: t(PLATFORM_COPY[id].label),
              description: t(PLATFORM_COPY[id].what),
            }))}
          />
          {/* What the two answers that need no app HERE still need, and where.
              It is not this screen's work to do, so the reader is told whose it
              is rather than left with an empty form. */}
          {/* Every answer carries what it still leaves undone, the Google one
              included — its option promises sign-in, and saving the app below
              does not turn the login door on. A screen that states two paths'
              gaps and hides the third's is worse than one that states none: it
              reads as a guarantee for the path it says nothing about. */}
          <Callout tone="info" live="status">
            {t(operatorWork ?? "firstRun.platform.googleSignInCaveat")}
          </Callout>
          {/* And what the two answers that need no app here run into anyway.
              Silent rather than a second live region: both change on the same
              press, and two regions announcing together read the news twice. */}
          {/* The gap, stated. `google_app` is blocking on the server whatever
              the answer above, so these two paths cannot finish first run — and
              saying so beside the fields that CAN finish it is the only honest
              shape while that is true. */}
          {operatorWork === undefined ? null : (
            <Callout tone="warn">
              {t("firstRun.platform.stillNeedsGoogle")}
            </Callout>
          )}
          <GoogleAppHelp />
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
        </PanelBody>
      </Panel>
    </StepForm>
  );
}

/**
 * Whether this installation has a model bound, from the server's own step list
 * rather than from which step is on screen.
 *
 * The distinction is what makes the light honest for a reader who arrives
 * mid-way: `ai_models` can be configured while `google_app` is not, and a
 * `lit` derived from "this is the second step" would say the same thing by
 * accident rather than by reading it.
 */
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
  const setup = useInstallationSetup();
  const step = outstandingStep(setup.data);
  // Owned here because the Core belongs to the stage and the write belongs to
  // the step. The step says when it is writing; nothing reads this but the orb.
  const [busy, setBusy] = useState(false);

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
  const ai = step.step === "ai_models";
  return (
    <OnboardingStage
      lit={modelBound(setup.data)}
      coreState={busy ? "working" : "idle"}
      // The Core is aria-hidden, so the band says in words what it is showing.
      // From `ob.core.*`, the vocabulary every onboarding surface reads: the
      // orb showing the same state on two screens must not read as two
      // different things.
      coreStateLabel={t(busy ? "ob.core.working" : "ob.core.idle")}
      // The server's step list, in the server's order — the same array the gate
      // walks, so the band cannot claim a different flow than the one being
      // walked.
      progress={{
        steps: (setup.data?.steps ?? []).map((s) =>
          t(
            s.step === "ai_models"
              ? "firstRun.step.model"
              : "firstRun.step.platform",
          ),
        ),
        at: (setup.data?.steps ?? []).findIndex((s) => s.step === step.step),
      }}
      eyebrow={t(ai ? "firstRun.ai.eyebrow" : "firstRun.google.eyebrow")}
      title={t(ai ? "firstRun.ai.title" : "firstRun.platform.title")}
      sub={t(ai ? "firstRun.ai.sub" : "firstRun.platform.sub")}
    >
      {ai ? <AiStep onBusy={setBusy} /> : <GoogleStep onBusy={setBusy} />}
    </OnboardingStage>
  );
}
