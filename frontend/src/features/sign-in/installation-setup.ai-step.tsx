import type { ReactNode } from "react";
import { useEffect, useState } from "react";
import { Button, Field } from "../../design-system/atoms";
import { ComboBox } from "../../design-system/combobox";
import { StageActions } from "../../design-system/onboarding-stage";
import { Panel, PanelBody } from "../../design-system/panel";
import { Select } from "../../design-system/select";
import { serviceAccountProblem } from "../../design-system/serviceaccountkeyfield";
import { useLocale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import {
  suggestionsFor,
  useAiModelCatalogue,
  useVendorCatalogue,
  type VendorCatalogue,
  vendorSuggestions,
} from "../ai/ai-models";
import { useSetProviderKey } from "../ai/ai-provider-key-hooks";
import { ModelRatePlate } from "../ai/ai-rates";
import { DEFAULT_VERTEX_LOCATION } from "../ai/vertex-location";
import {
  AiKeyFields,
  choiceLabel,
  useBindModels,
} from "./installation-setup.ai-key";
import { AiBindRefused } from "./installation-setup.notices";
import { StepNeeds } from "./installation-setup.platform";
import {
  SETUP_PROVIDER_IDS,
  SETUP_PROVIDERS,
  type SetupProviderId,
} from "./setup-providers";

/**
 * A step's form, telling the stage above it when a write is in flight.
 *
 * The Core is the stage's, and whether anything is being written is the step's,
 * so one of them has to tell the other. It travels up rather than the mutation
 * moving down. `useSetProviderKey` and `useSetGoogleApp` sit where they do for
 * how long a credential lives in memory. Lifting a mutation to make an orb spin
 * would be the tail wagging the dog.
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

/**
 * What the chat field's list is made of, added to the field's own hint.
 *
 * Silent for a vendor with no public catalogue, because there the list is the
 * price sheet and the ordinary hint already says so. With a live list, it names
 * the measure. A "top ten" with no measure behind it is a claim the reader cannot
 * check. The measure is a third party's published one, not this product's.
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

/** The AI step: one vendor, one key, the two models it will serve. */
export function AiStep({
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
  const [location, setLocation] = useState(
    preset.location ?? DEFAULT_VERTEX_LOCATION,
  );
  // Why the pasted key file cannot be sent, set by a press on Continue.
  const [keyRefusal, setKeyRefusal] = useState<MessageKey | undefined>();
  const keyFile = preset.credential === "service_account";
  const [chatModel, setChatModel] = useState(preset.chatModel);
  const [embedModel, setEmbedModel] = useState(preset.embedModel);
  const saveKey = useSetProviderKey();
  const bind = useBindModels();
  // What this installation can already price. On a fresh install that is the
  // seeded sheet. So the two fields open with the vendor's own family in them,
  // not with one id and no way to learn a second.
  const catalogue = useAiModelCatalogue();
  // What the vendor is serving today, which the sheet cannot know. Asked only
  // of a vendor whose catalogue is public, which is OpenRouter alone, and only
  // once the reader has chosen it.
  const vendor = useVendorCatalogue(
    choice === "openrouter" ? "openrouter" : undefined,
  );

  // Switching vendor re-seeds the models, because the previous vendor's ids
  // mean nothing to this one. Kept, they would offer a binding that cannot
  // serve a single call.
  const pick = (next: SetupProviderId) => {
    // A key is one vendor's: kept across a switch, it would be saved for the
    // next vendor and refused on its first call.
    if (next !== choice) {
      setApiKey("");
    }
    setKeyRefusal(undefined);
    setLocation(SETUP_PROVIDERS[next].location ?? DEFAULT_VERTEX_LOCATION);
    setChoice(next);
    setChatModel(SETUP_PROVIDERS[next].chatModel);
    setEmbedModel(SETUP_PROVIDERS[next].embedModel);
  };

  const busy = saveKey.isPending || bind.isPending;
  // What the binding cannot do without, by the label the field wears. So the
  // rail names the same thing the field marks once Continue is pressed early.
  const missing = [
    [
      apiKey.trim() === "",
      keyFile ? t("serviceAccountKey.label") : t("firstRun.ai.key"),
    ],
    [chatModel.trim() === "", t("firstRun.ai.chatModel")],
    [embedModel.trim() === "", t("firstRun.ai.embedModel")],
  ]
    .filter((need): need is [true, string] => need[0] === true)
    .map(([, label]) => label);
  const ready = missing.length === 0;
  const [attempted, setAttempted] = useState(false);

  // The key first, then the binding. A binding whose vendor has no key reads as
  // configured and fails on its first real call. A key with no binding is just
  // a key, and the next attempt completes it.
  //
  // Chained through callbacks rather than awaited: mutateAsync rejects, and a
  // rejection nobody handles is an unhandled promise. The field is cleared only after
  // both writes land. Clearing it after the first would lock the reader out on a
  // failed binding. `ready` stays false, Continue stays disabled, and a reload
  // restores that state. The only way on would be to re-paste a key the server
  // already holds.
  const submit = () => {
    // Pressable whatever is filled in. The press turns the missing fields red
    // and names them on the rail, so no grey button leaves the reader guessing.
    if (!ready) {
      setAttempted(true);
      return;
    }
    const problem = keyFile ? serviceAccountProblem(apiKey) : undefined;
    if (problem) {
      setKeyRefusal(problem);
      return;
    }
    saveKey.reset();
    bind.reset();
    saveKey.mutate(
      {
        provider: preset.provider,
        kind: preset.credential,
        secret: apiKey.trim(),
      },
      {
        // The key is stored, so its copy in the mutation goes now. A binding
        // that may fail would otherwise keep it there; the field keeps its own.
        onSuccess: () => {
          saveKey.reset();
          bind.mutate(
            {
              provider: preset.provider,
              baseUrl: preset.baseUrl,
              location: preset.location ? location : undefined,
              embedLocation: preset.embedLocation,
              profile: preset.profile,
              chatModel: chatModel.trim(),
              embedModel: embedModel.trim(),
            },
            {
              onSuccess: () => {
                // Both landed, so the field has done its job and this is the
                // only copy of the key the app was holding.
                setApiKey("");
                // And the screen becomes the ignition. Nothing is refetched
                // yet: see `useBindModels` for why the reader, not the query,
                // decides when this step is over.
                onIgnite(preset.label);
              },
            },
          );
        },
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
          <AiBindRefused keyError={saveKey.error} bindError={bind.error} />
          <Field label={t("firstRun.ai.provider")}>
            {(control) => (
              <Select
                {...control}
                value={choice}
                disabled={busy}
                // Narrowed through the id list rather than asserted into it. A
                // Select answers with a string, and an answer not in this list
                // is not a provider this screen can bind.
                onChange={(v) => {
                  const next = SETUP_PROVIDER_IDS.find((id) => id === v);
                  if (next) {
                    pick(next);
                  }
                }}
                options={SETUP_PROVIDER_IDS.map((id) => ({
                  value: id,
                  label: choiceLabel(SETUP_PROVIDERS[id], t),
                }))}
              />
            )}
          </Field>
          <AiKeyFields
            preset={preset}
            secret={apiKey}
            refusal={keyRefusal}
            attempted={attempted}
            location={location}
            disabled={busy}
            onSecret={(next) => {
              setKeyRefusal(undefined);
              setApiKey(next);
            }}
            onLocation={setLocation}
          />
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
                // The sheet first, then what the vendor is serving today. The
                // sheet's rows are the ones this installation can already price.
                // The live rows answer a different question: what else is there.
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
