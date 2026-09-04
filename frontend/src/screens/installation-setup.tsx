import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { useEffect, useState, useSyncExternalStore } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Button, Disclosure, Field, TextInput } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { ChoiceList } from "../design-system/choicelist";
import { ComboBox } from "../design-system/combobox";
import { OffsiteLink } from "../design-system/offsitelink";
import {
  OnboardingStage,
  StageActions,
} from "../design-system/onboarding-stage";
import { Panel, PanelBody } from "../design-system/panel";
import { ProviderMark } from "../design-system/provider-mark";
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
import { problemMessageOf, throwProblem, useMe } from "./common";
import { ImapMailboxForm } from "./imap-connect-form";
import {
  RedirectUris,
  useOAuthApp,
  useSetOAuthApp,
  type Vendor,
  vendorCopy,
} from "./oauth-app";
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
 * The steps this screen has a panel for, in the order the server reports
 * them: the model binding, which blocks, and the organisation's OAuth app,
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
 * Where "Not now" on the platform question is remembered.
 *
 * The app step is asked of the person running the cold start, once. The server
 * has no word for "asked and declined" — the step is simply unconfigured until
 * an app is stored, from here or from Settings — so the decline lives in this
 * browser.
 *
 * KEYED BY THE ACCOUNT THAT GAVE IT. A mark keyed on the browser alone outlives
 * the installation it was about: a machine that had run one cold start carried
 * that answer into the next, and the second installation's setup skipped the
 * platform question with nothing on screen to say why — the one step that asks
 * for the organization's OAuth app, silently gone, on the run that most needed
 * it. A re-claimed installation mints its own administrator, so its cold start
 * asks again; the same person on the same installation is still asked once.
 */
const PLATFORM_DECLINED_KEY = "margince.first-run.platform-declined";

function declinedKey(account: string): string {
  return `${PLATFORM_DECLINED_KEY}:${account}`;
}

/** Whether `account` declined the question. An unknown account — the session
 *  probe has not answered yet — has declined nothing, which is the reading
 *  that asks rather than the one that hides. */
function platformDeclined(account: string | null): boolean {
  if (account === null) {
    return false;
  }
  try {
    return window.localStorage.getItem(declinedKey(account)) === "1";
  } catch {
    // Storage blocked (a private window, a policy): the question is asked
    // again, which is the safe reading of not knowing.
    return false;
  }
}

// Who is watching the decline: the gate on this screen and the act that
// stands in front of it. Storage has no change event in the tab that wrote
// it, so the write tells them itself.
const declinedListeners = new Set<() => void>();

function subscribeDeclined(listener: () => void): () => void {
  declinedListeners.add(listener);
  return () => declinedListeners.delete(listener);
}

function rememberPlatformDeclined(account: string | null): void {
  try {
    if (account !== null) {
      window.localStorage.setItem(declinedKey(account), "1");
    }
  } catch {
    // Nothing to remember it in; the reader is let through regardless, and
    // asked again next time.
  }
  for (const listener of declinedListeners) {
    listener();
  }
}

/** The signed-in account the decline belongs to, or null while the session
 *  probe is still answering. */
function useAccount(): string | null {
  const me = useMe();
  return me.data?.user.id ?? null;
}

/**
 * Whether this account declined the platform question in this browser, live:
 * the answer every caller of `outstandingStep` passes it, so the gate and the
 * act in front of it re-read the same fact the moment it changes.
 */
export function usePlatformDeclined(): boolean {
  const account = useAccount();
  return useSyncExternalStore(
    subscribeDeclined,
    () => platformDeclined(account),
    () => false,
  );
}

/** Records the decline against the account that gave it. Its own hook so the
 *  gate does not have to hold the account itself to hand it back. */
function useRememberPlatformDeclined(): () => void {
  const account = useAccount();
  return () => rememberPlatformDeclined(account);
}

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
  // What the binding cannot do without, by the label the field wears, so the
  // rail names the same thing the field marks once Continue is pressed early.
  const missing = [
    [apiKey.trim() === "", t("firstRun.ai.key")],
    [chatModel.trim() === "", t("firstRun.ai.chatModel")],
    [embedModel.trim() === "", t("firstRun.ai.embedModel")],
  ]
    .filter((need): need is [true, string] => need[0] === true)
    .map(([, label]) => label);
  const ready = missing.length === 0;
  const [attempted, setAttempted] = useState(false);
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
    // Pressable whatever is filled in: the press is what turns the missing
    // fields red and names them on the rail, rather than a grey button leaving
    // the reader to work out why.
    if (!ready) {
      setAttempted(true);
      return;
    }
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
      {/* The verb goes on the stage's rail rather than as the last thing in
          the stack of questions: a button that shares the fields' spacing reads
          as one more field, and one at the end of a long board is one the
          reader has scrolled away from. */}
      <StageActions>
        <StepNeeds attempted={attempted} missing={missing} />
        <Button variant="primary" pending={busy} onClick={submit}>
          {t("firstRun.continue")}
        </Button>
      </StageActions>
      <Panel>
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
            hint={t("firstRun.ai.keyHint")}
            error={
              attempted && apiKey.trim() === ""
                ? t("firstRun.needed")
                : undefined
            }
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

/**
 * What the organization runs on — ONE answer covering mail and sign-in.
 *
 * They are separate mechanisms in the server and the same fact about a
 * company: an organization on Workspace reads mail through a Google app and
 * signs its people in with Google accounts, through that same app and the same
 * console entry. Two questions would ask somebody to state one fact twice and
 * then keep the two answers agreeing.
 */
const PLATFORMS = ["google", "microsoft", "imap"] as const;
type Platform = (typeof PLATFORMS)[number];

// Typed by `Platform` in both directions: an answer with no copy fails, and
// copy for an answer that does not exist fails too.
const PLATFORM_COPY: Readonly<
  Record<Platform, { label: MessageKey; what: MessageKey }>
> = {
  google: {
    label: "firstRun.platform.google",
    what: "firstRun.platform.googleWhat",
  },
  microsoft: {
    label: "firstRun.platform.microsoft",
    what: "firstRun.platform.microsoftWhat",
  },
  imap: {
    label: "firstRun.platform.imap",
    what: "firstRun.platform.imapWhat",
  },
};

/** Google's own console, where the app is created and the two values are read. */
const GOOGLE_CREDENTIALS_CONSOLE =
  "https://console.cloud.google.com/apis/credentials";

/**
 * The redirect URIs the app must carry, in the open and each with its copy
 * button: the one thing on this step that is done in the vendor's console and
 * not here, and the one a first run most often skips. The Sign-in row is what
 * puts the button on the login page, so the hint says so before the list.
 */
function AppRedirectUris({
  vendor,
  uris,
}: Readonly<{
  vendor: Vendor;
  uris: readonly { purpose: string; url: string }[] | undefined;
}>) {
  const t = useT();
  return (
    <>
      <Callout tone="info" title={t("firstRun.platform.redirectTitle")}>
        <p>{t("firstRun.platform.redirectHint")}</p>
      </Callout>
      <RedirectUris uris={uris} sub={t(vendorCopy[vendor].redirectSub)} />
    </>
  );
}

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
        <li>{t("firstRun.google.helpStep3")}</li>
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

/**
 * The vendor's app: the two values every OAuth client has, and Microsoft's
 * optional directory pin. Its own component so the app query is asked only
 * for the vendor on screen, and so the fields reset when the answer changes.
 */
function VendorAppFields({
  vendor,
  onBusy,
  onDecline,
}: Readonly<{
  vendor: Vendor;
  onBusy: (busy: boolean) => void;
  /** The way past for an admin without the console open: the step does not
   * block, and the same form waits in Settings. */
  onDecline: () => void;
}>) {
  const t = useT();
  const app = useOAuthApp(vendor);
  const save = useSetOAuthApp(vendor);
  const [clientId, setClientId] = useState("");
  const [clientSecret, setClientSecret] = useState("");
  const [tenant, setTenant] = useState("");
  // The directory is asked for HERE, unlike Settings, where it stays optional.
  // It is what decides whether Microsoft appears on the login page at all
  // (compose/microsoftsignin.go: an app on no directory names nobody and signs
  // nobody in), and an installation that registered Microsoft one step earlier
  // and then found no Microsoft button has no way to tell why. The server
  // refuses anything but a directory id, so "not empty" is the whole client
  // rule and the shape stays the server's.
  const missing = [
    [clientId.trim() === "", t("oauthApp.clientId")],
    [clientSecret.trim() === "", t("oauthApp.clientSecret")],
    [vendor === "microsoft" && tenant.trim() === "", t("oauthApp.tenant")],
  ]
    .filter((need): need is [true, string] => need[0] === true)
    .map(([, label]) => label);
  const ready = missing.length === 0;
  const [attempted, setAttempted] = useState(false);
  const needed = (absent: boolean) =>
    attempted && absent ? t("firstRun.needed") : undefined;
  useEffect(() => {
    onBusy(save.isPending);
    return () => onBusy(false);
  }, [save.isPending, onBusy]);
  return (
    <>
      <AppRedirectUris vendor={vendor} uris={app.data?.redirect_uris} />
      {vendor === "google" ? (
        <GoogleAppHelp />
      ) : (
        <>
          <p className="ob-fr-help-note">{t("firstRun.microsoft.note")}</p>
          {/* The pin below is also the directory sign-in runs on, which the
              Google form has no equivalent of: said here, because an admin
              who leaves it empty gets working mailboxes and no sign-in, and
              nothing else on this screen would say why. */}
          <p className="ob-fr-help-note">
            {t("firstRun.microsoft.helpSignIn")}
          </p>
        </>
      )}
      {save.error && (
        <Callout tone="danger">{problemMessageOf(save.error, t)}</Callout>
      )}
      <Field
        label={t("oauthApp.clientId")}
        error={needed(clientId.trim() === "")}
      >
        {(control) => (
          <TextInput
            {...control}
            value={clientId}
            disabled={save.isPending}
            autoComplete="off"
            placeholder={t(vendorCopy[vendor].clientIdPlaceholder)}
            onChange={(e) => setClientId(e.target.value)}
          />
        )}
      </Field>
      <Field
        label={t("oauthApp.clientSecret")}
        error={needed(clientSecret.trim() === "")}
      >
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
      {vendor === "microsoft" && (
        <Field
          label={t("oauthApp.tenant")}
          hint={t("firstRun.microsoft.tenantHint")}
          error={needed(tenant.trim() === "")}
        >
          {(control) => (
            <TextInput
              {...control}
              value={tenant}
              autoComplete="off"
              disabled={save.isPending}
              placeholder={t("oauthApp.tenantPlaceholder")}
              onChange={(e) => setTenant(e.target.value)}
            />
          )}
        </Field>
      )}
      <StageActions>
        <StepNeeds attempted={attempted} missing={missing} />
        <Button variant="ghost" onClick={onDecline} disabled={save.isPending}>
          {t("firstRun.platform.skip")}
        </Button>
        <Button
          variant="primary"
          pending={save.isPending}
          onClick={() => {
            if (!ready) {
              setAttempted(true);
              return;
            }
            save.reset();
            save.mutate(
              {
                clientId: clientId.trim(),
                clientSecret: clientSecret.trim(),
                tenant: tenant.trim(),
              },
              {
                onSuccess: () => {
                  // Cleared on the way out rather than left in state: the
                  // field is the only copy this app holds, and it has done
                  // its job. The invalidated setup report is what moves the
                  // screen on.
                  setClientSecret("");
                  save.reset();
                },
              },
            );
          }}
        >
          {t("firstRun.continue")}
        </Button>
      </StageActions>
    </>
  );
}

/**
 * What still stands between the reader and the next step, beside the button
 * they pressed. Nothing until Continue has been pressed with something missing:
 * a form that lists its own gaps before anyone has typed is a form scolding a
 * reader for arriving.
 */
function StepNeeds({
  attempted,
  missing,
}: Readonly<{ attempted: boolean; missing: readonly string[] }>) {
  const t = useT();
  if (!attempted || missing.length === 0) {
    return null;
  }
  return (
    <p className="ob-stage-note" role="alert">
      {t("firstRun.stillNeeded", { fields: missing.join(", ") })}
    </p>
  );
}

/**
 * The platform step: which vendor's app mailboxes and calendars connect
 * through, asked once, of the person running the cold start. IMAP is an
 * answer too — each mailbox carries its own credentials — and so is
 * "not now": the step does not block, and Settings keeps the same form.
 */
function PlatformStep({
  onBusy,
  onDecline,
}: Readonly<{
  onBusy: (busy: boolean) => void;
  /** The reader passed on the question; the gate lets them through. */
  onDecline: () => void;
}>) {
  const t = useT();
  const [platform, setPlatform] = useState<Platform>("google");
  return (
    <div className="ob-fr-form">
      <Panel>
        <PanelBody>
          {/* Plates rather than a radio column: the answers are two vendors
              and a protocol, and an admin picks the platform their company
              already runs by recognising it. `imap` is a protocol, not a vendor,
              so it gets the component's own fallback mark rather than an
              invented logo. */}
          <ChoiceList<Platform>
            legend={t("firstRun.platform.legend")}
            hideLegend
            layout="cards"
            value={platform}
            onChange={setPlatform}
            choices={PLATFORMS.map((id) => ({
              value: id,
              label: t(PLATFORM_COPY[id].label),
              description: t(PLATFORM_COPY[id].what),
              mark: <ProviderMark providerKey={id} />,
            }))}
          />
          {platform === "imap" ? (
            <>
              <p className="ob-fr-help-note">
                {t("firstRun.platform.imapNote")}
              </p>
              {/* The same standing connect Settings makes, for the person on
                  screen. There is no installation-wide IMAP app to store, so a
                  confirmed mailbox and "not now" both answer the question. */}
              <ImapMailboxForm
                dismissLabel={t("firstRun.platform.skip")}
                onDismiss={onDecline}
                onConnected={onDecline}
                onPendingChange={onBusy}
                renderActions={(actions) => (
                  <StageActions>{actions}</StageActions>
                )}
              />
            </>
          ) : (
            <VendorAppFields
              key={platform}
              vendor={platform}
              onBusy={onBusy}
              onDecline={onDecline}
            />
          )}
        </PanelBody>
      </Panel>
    </div>
  );
}

// What the room says while a step is answered, and for the four seconds after
// the model binding lands: the ignition is not a step but the moment that
// answer takes effect.
function head(
  step: Step["step"],
  ignited: boolean,
): Readonly<{ title: MessageKey; sub: MessageKey }> {
  if (ignited) {
    return { title: "firstRun.ignite.title", sub: "firstRun.ignite.sub" };
  }
  return step === "oauth_app"
    ? { title: "firstRun.platform.title", sub: "firstRun.platform.sub" }
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
  const declined = usePlatformDeclined();
  const decline = useRememberPlatformDeclined();
  const step = outstandingStep(setup.data, declined);
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
      // The band names the step rather than counting it: two questions are
      // not a journey, and a counter over them would be ceremony.
      step={t(
        step.step === "oauth_app"
          ? "firstRun.step.platform"
          : "firstRun.step.model",
      )}
      eyebrow={t(
        step.step === "oauth_app"
          ? "firstRun.google.eyebrow"
          : "firstRun.ai.eyebrow",
      )}
      // The one qualification the step carries, on the card's bottom edge. It
      // is true of the whole screen rather than of any field on it, which is
      // what makes it chrome: as a callout in the board it read as an
      // instruction and pushed the actual form below the fold.
      hint={t(
        step.step === "oauth_app"
          ? "firstRun.platform.foot"
          : "firstRun.ai.foot",
      )}
      title={t(head(step.step, ignited !== null).title)}
      sub={t(head(step.step, ignited !== null).sub)}
    >
      {step.step === "oauth_app" ? (
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
