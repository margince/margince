// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type ReactNode, useEffect, useRef, useState } from "react";
import type { components } from "../api/schema";
import { Button, Field, TextInput } from "../design-system/atoms";
import { ComboBox } from "../design-system/combobox";
import { Select } from "../design-system/select";
import { useLocale, useT } from "../i18n";
import {
  type AvailableModels,
  type ModelCatalogue,
  type ModelLane,
  offeredModels,
  useAvailableModels,
} from "./ai-models";
import { isOpenRouter } from "./ai-provider-links";
import "./ai-settings.css";
import {
  DEFAULT_VERTEX_LOCATION,
  VERTEX_PROVIDER,
  VertexLocationField,
} from "./vertex-location";
import { useVertexModelProbe } from "./vertex-model-probe";

type Routing = components["schemas"]["AiRouting"];
type ProviderSettings = components["schemas"]["AiProviderSettings"];
// The adapters a tier may name. Written out because the wire carries a free
// string — the server refuses an unknown one, and a reader choosing from a list
// should not have to discover that by being refused. A declared mirror of the
// server's provider registry, held in both directions by
// backend/gates/frontendproviders_test.go, which reads this `[…] as const` form.
export const PROVIDERS = [
  "gemini",
  "gemini_vertex",
  "anthropic",
  "openai",
  "openai_compatible",
  "ollama",
  "vllm",
  "fake",
] as const;

// The adapters the decision lane may name. A decision model answers a typed
// question with calibrated probabilities rather than text, so no chat adapter
// can serve it and none is offered. The same `[…] as const` mirror as
// PROVIDERS, held against the server's decision registry by
// backend/gates/frontendproviders_test.go.
export const DECISION_PROVIDERS = ["jev", "jev_compatible"] as const;

// The one-click binding the decision row offers on jev_compatible: OpenRouter's
// decisions endpoint and the model certified there. Written once, here; the
// commented `decisions:` blocks in config/presets must name the same endpoint
// and model, which backend/gates/decisionpreset_test.go holds.
export const OPENROUTER_DECISION_PRESET = {
  provider: "jev_compatible",
  base_url: "https://openrouter.ai/api/alpha/decisions",
  model: "typesafe/jev-1.13",
} as const;

// The providers that cannot be dialled until their host is set: neither has a
// host of its own. A lane bound to one with no host set says where to set it.
// Mirrors the server's chatHostMissing and decisionHostMissing; the server's
// refusal stays the hard check, so a drift here costs only the early notice.
const NEEDS_HOST: ReadonlySet<string> = new Set([
  "openai_compatible",
  "jev_compatible",
]);

// The providers whose embeddings model may run on a server of its own: a
// self-hosted vLLM serves one model per process, so the embedder is often not
// where the chat models are.
const EMBEDDINGS_SERVER: ReadonlySet<string> = new Set([
  "openai_compatible",
  "ollama",
  "vllm",
]);

type TierBindingLike = {
  provider: string;
  model: string;
  base_url?: string;
  location?: string;
  routing?: unknown;
  thinking_level?: string;
};

// Broker preferences and a thinking level were written for one provider at one
// address and one model: the server refuses preferences on a binding that is not
// OpenRouter and a level on one that is not a Gemini 3, and an `only:` pin names
// hosts that serve ONE model. Changing any of the three therefore takes both
// off, the same rule the server applies when it carries a stored lane's values
// onto a write that omits them. Which hosts ARE OpenRouter is the server's rule;
// the editor keeps no second copy of it, so it drops on any move instead of
// guessing.
export function rebind<B extends TierBindingLike>(
  binding: B,
  patch: Partial<Pick<TierBindingLike, "provider" | "model" | "base_url">>,
): B {
  const next: B = { ...binding, ...patch };
  const moved =
    next.provider !== binding.provider ||
    next.model !== binding.model ||
    (next.base_url ?? "") !== (binding.base_url ?? "");
  if (moved) {
    delete next.routing;
    delete next.thinking_level;
  }
  return next;
}

/**
 * The binding re-pointed at another adapter. A host belongs to the provider it
 * was written for, so another provider never inherits it; `location` belongs to
 * Vertex alone, and a Vertex binding keeps its own or takes the default.
 */
export function withProvider<B extends TierBindingLike>(
  binding: B,
  provider: string,
  vertexLocation: string,
): B {
  // A model id names a model on one vendor; carried onto another it names one
  // that vendor does not serve, so a provider change starts the model empty.
  const moved = provider !== binding.provider;
  const next = rebind(binding, {
    provider,
    ...(moved ? { model: "", base_url: undefined } : {}),
  });
  if (provider === VERTEX_PROVIDER) {
    return {
      ...next,
      base_url: undefined,
      location: binding.location ?? vertexLocation,
    };
  }
  return { ...next, location: undefined };
}

// The controls that name an adapter: which vendor and which model on it. Where
// the vendor is reached is the provider's, set on its sheet; the embeddings
// lane alone may name a server or a Vertex location of its own.
//
// One component rather than one per row. Both lanes ask the identical question
// and the answers are governed by the identical rule, so a second copy would
// only be a second place to forget when that rule moves. Two things genuinely
// differ, and both arrive as props: the label -- a tier row names the tier, the
// embedding row names itself -- and the LANE, which decides whether this field
// offers chat models or embedders. An embedder on a chat tier cannot serve a
// call, so offering one would be worse than offering nothing.
export function AdapterFields<B extends TierBindingLike>({
  label,
  lane,
  laneName,
  binding,
  catalogue,
  profile = "",
  vertexLocation = DEFAULT_VERTEX_LOCATION,
  providerSettings,
  disabled,
  onChange,
  providers = PROVIDERS,
  providerAside,
}: Readonly<{
  label: string;
  // A preset for THIS lane: the verb beside the provider it belongs to, and the
  // one line under the row that says what it fills and what it cannot.
  providerAside?: Readonly<{ action: ReactNode; note: ReactNode }>;
  lane: ModelLane;
  // The adapters this lane may name. Every chat tier and the embedder share
  // one list; the decision lane has its own, because no chat adapter answers a
  // decision question.
  providers?: readonly string[];
  // Which lane of the routing document this is, in the document's own words.
  // `lane` above says chat-or-embeddings, which is what a model is FOR; this
  // says which binding, which is what the host is read from.
  laneName: string;
  binding: B;
  catalogue: ModelCatalogue;
  // The draft's profile, which decides the Vertex locations on offer. The
  // decision lane binds no Vertex model, so it passes neither.
  profile?: string;
  // Where a lane newly pointed at Vertex starts: another saved Vertex lane's.
  vertexLocation?: string;
  // The bound provider's own settings, which say whether it can be dialled.
  providerSettings?: ProviderSettings;
  disabled: boolean;
  onChange: (next: B) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const vertex = binding.provider === VERTEX_PROVIDER;
  const location = binding.location ?? "";
  // Asked of the VENDOR, and only while these fields are open — this is a real
  // round-trip on the installation's own credential, not a table read. The lane
  // travels with it so an installation binding one vendor at two hosts is asked
  // at the one THIS lane points at; a Vertex lane is asked at its location.
  const available = useAvailableModels(
    binding.provider,
    laneName,
    true,
    vertex ? location : undefined,
  );
  const ownServer = laneName === "embeddings";
  // After a provider change the model is empty, and the next thing to do is
  // pick one: focus lands in the box, which opens what the new vendor serves.
  const modelBox = useRef<string | undefined>(undefined);
  const [pickModel, setPickModel] = useState(false);
  useEffect(() => {
    if (!pickModel) return;
    setPickModel(false);
    if (modelBox.current) document.getElementById(modelBox.current)?.focus();
  }, [pickModel]);
  // An embeddings server of its own is where that lane is reached, whatever its
  // provider holds.
  const unhosted =
    NEEDS_HOST.has(binding.provider) &&
    providerSettings !== undefined &&
    !providerSettings.base_url &&
    !(ownServer && binding.base_url);
  // A Vertex list is asked of the location model by model, which takes a
  // moment; until it answers, the price sheet is not offered in its place,
  // since most of what it names that location does not serve.
  const asking = vertex && location !== "" && available.isPending;
  const suggestions = asking
    ? []
    : offeredModels(available.data, catalogue, binding.provider, lane, locale);
  const probe = useVertexModelProbe({
    vertex,
    laneName,
    binding,
    location,
    available: available.data,
    onChange,
  });
  const hint = vertex ? probe.hint : undefined;
  return (
    <>
      <div className="binding-provider-row">
        <Field label={label}>
          {(control) => (
            <Select
              {...control}
              value={binding.provider}
              disabled={disabled}
              options={providers.map((p) => ({ value: p, label: p }))}
              onChange={(provider) => {
                probe.forget();
                onChange(withProvider(binding, provider, vertexLocation));
                if (provider !== binding.provider) setPickModel(true);
              }}
            />
          )}
        </Field>
        {providerAside?.action}
      </div>
      {providerAside?.note}
      {unhosted && (
        <p className="t-caption">
          {t("aiRouting.provider.noHost", { provider: binding.provider })}
        </p>
      )}
      {vertex && ownServer && (
        <VertexLocationField
          value={location}
          profile={profile}
          disabled={disabled}
          onChange={(next) => {
            probe.relocate(next);
            onChange({ ...binding, location: next });
          }}
        />
      )}
      {/* What the vendor serves, priced from the sheet where the sheet knows
          it. The list used to be the sheet ALONE, which answers what this
          installation can price rather than what exists — so a model released
          after somebody last edited that table was simply absent, and a reader
          looking for it concluded the product could not reach it.

          Still a text box. The server takes any id its vendor serves, a vendor
          ships a model on a Tuesday, and neither the vendor's list nor the
          sheet is a permitted set. */}
      <Field
        label={t("aiRouting.model.label")}
        hint={
          hint?.text ??
          (asking
            ? t("aiRouting.models.askingLocation", { location })
            : undefined) ??
          (available.data?.unavailable
            ? modelSourceNote(available.data.unavailable, t)
            : t("aiRouting.model.help"))
        }
        error={hint?.error}
      >
        {(control) => {
          modelBox.current = control.id;
          return (
            <ComboBox
              {...control}
              value={binding.model}
              suggestions={suggestions}
              disabled={disabled}
              onChange={(model) => {
                // A pick from the list is a choice worth checking; a keystroke
                // is not, and each probe is a call on the service account.
                probe.picked(
                  model,
                  suggestions.some((s) => s.value === model),
                );
                onChange(rebind(binding, { model }));
              }}
            />
          );
        }}
      </Field>
      {ownServer && EMBEDDINGS_SERVER.has(binding.provider) && (
        <Field
          label={t("aiRouting.embeddingsServer.label")}
          hint={t("aiRouting.embeddingsServer.help")}
        >
          {(control) => (
            <TextInput
              {...control}
              value={binding.base_url ?? ""}
              disabled={disabled}
              onChange={(e) =>
                onChange(rebind(binding, { base_url: e.target.value }))
              }
            />
          )}
        </Field>
      )}
    </>
  );
}

// The width this lane asks the provider for, which only it has. Blank means the
// compiled default rather than zero: the contract reads an omitted value and a 0
// the same way, so an empty box must send neither a 0 nor a NaN.
export function EmbeddingWidthField({
  binding,
  disabled,
  onChange,
}: Readonly<{
  binding: Routing["embeddings"];
  disabled: boolean;
  onChange: (next: Routing["embeddings"]) => void;
}>) {
  const t = useT();
  return (
    <Field
      label={t("aiRouting.dimensions.label")}
      hint={t("aiRouting.dimensions.help")}
    >
      {(control) => (
        <TextInput
          {...control}
          type="number"
          inputMode="numeric"
          value={binding.dimensions?.toString() ?? ""}
          disabled={disabled}
          onChange={(e) => {
            const raw = e.target.value.trim();
            const parsed = Number.parseInt(raw, 10);
            onChange({
              ...binding,
              dimensions:
                raw === "" || Number.isNaN(parsed) ? undefined : parsed,
            });
          }}
        />
      )}
    </Field>
  );
}

// Said in the field's own hint rather than as an error: the box still binds
// anything typed into it, and every one of these is a state of the installation
// somebody can act on — paste a key, fill in a host, start the local server —
// or one they cannot, which is worth knowing before they go looking for a model
// that will not appear.
function modelSourceNote(
  unavailable: NonNullable<AvailableModels["unavailable"]>,
  t: ReturnType<typeof useT>,
): string {
  switch (unavailable) {
    case "no_key":
      return t("aiRouting.models.noKey");
    case "no_endpoint":
      return t("aiRouting.models.noEndpoint");
    case "profile_forbids":
      return t("aiRouting.models.profileForbids");
    case "not_published":
      return t("aiRouting.models.notPublished");
    default:
      return t("aiRouting.models.unreachable");
  }
}

/**
 * The OpenRouter preset for the decision lane, as the verb beside its provider
 * and the note under the row. Only where the provider is the one OpenRouter
 * serves: the endpoint is a full URL nobody remembers, and the key is the one
 * thing the preset cannot fill. The endpoint rides on the lane only while the
 * provider has none, which the server then lifts onto the provider; a provider
 * that has one keeps it.
 */
export function openRouterPreset<
  B extends { provider: string; model: string; base_url?: string },
>(
  binding: B,
  providerHost: string | undefined,
  disabled: boolean,
  onChange: (next: B) => void,
  t: ReturnType<typeof useT>,
): { action: ReactNode; note: ReactNode } | undefined {
  // A provider already pointed at another decision server keeps it; setting
  // OpenRouter's model there would name a model that server does not serve.
  if (
    binding.provider !== OPENROUTER_DECISION_PRESET.provider ||
    (providerHost && !isOpenRouter(providerHost))
  ) {
    return undefined;
  }
  return {
    action: (
      <span className="binding-preset-action">
        <Button
          variant="link"
          disabled={disabled}
          onClick={() =>
            onChange({
              ...binding,
              base_url: providerHost
                ? undefined
                : OPENROUTER_DECISION_PRESET.base_url,
              model: OPENROUTER_DECISION_PRESET.model,
            })
          }
        >
          {t("aiRouting.decisions.preset.openrouter")}
        </Button>
      </span>
    ),
    note: (
      <div className="binding-preset-note">
        <p className="t-caption">
          {t("aiRouting.decisions.preset.openrouterKey")}
        </p>
      </div>
    ),
  };
}
