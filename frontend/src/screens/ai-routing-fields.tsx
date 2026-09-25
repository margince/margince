// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import { Field, TextInput } from "../design-system/atoms";
import { ComboBox } from "../design-system/combobox";
import { Select } from "../design-system/select";
import { useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import {
  type AvailableModels,
  type ModelCatalogue,
  type ModelLane,
  offeredModels,
  useAvailableModels,
} from "./ai-models";

type Routing = components["schemas"]["AiRouting"];
// The adapters a tier may name. Written out because the wire carries a free
// string — the server refuses an unknown one, and a reader choosing from a list
// should not have to discover that by being refused. A declared mirror of the
// server's provider registry, held in both directions by
// backend/gates/frontendproviders_test.go, which reads this `[…] as const` form.
const PROVIDERS = [
  "gemini",
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
export const DECISION_PROVIDERS = ["openrouter_decision", "laya"] as const;

// The adapters whose host this form asks for, and what each does with it.
//
// Two have no host of their own, so the endpoint is the binding rather than a
// tweak to it: every OpenAI-wire vendor is reached through openai_compatible,
// and openrouter_decision promises OpenRouter's decisions endpoint, which the
// server holds to an OpenRouter host it has to be told. laya has a default,
// but it is loopback, which is right only where the decision server runs
// beside the API — so its host is offered, blank meaning that default.
//
// The help differs because the path each appends differs: a sentence written
// for one told the others the wrong thing. The paths are the server's
// provider registry's (providerregistry.go), spelled here as copy.
type HostField = Readonly<{
  help: MessageKey;
  placeholder: MessageKey;
}>;
const HOST_FIELDS: ReadonlyMap<string, HostField> = new Map([
  [
    "openai_compatible",
    {
      help: "aiRouting.baseUrl.help",
      placeholder: "aiRouting.baseUrl.placeholder",
    },
  ],
  [
    "openrouter_decision",
    {
      help: "aiRouting.baseUrl.help.openrouterDecision",
      placeholder: "aiRouting.baseUrl.placeholder",
    },
  ],
  [
    "laya",
    {
      help: "aiRouting.baseUrl.help.laya",
      placeholder: "aiRouting.baseUrl.placeholder.laya",
    },
  ],
]);

type TierBindingLike = {
  provider: string;
  model: string;
  base_url?: string;
  routing?: unknown;
};

// Broker preferences were written for one provider at one address and one
// model: the server refuses them on a binding that is not OpenRouter, and an
// `only:` pin names hosts that serve ONE model. Changing any of the three
// therefore takes them off, the same rule the server applies when it carries a
// stored lane's preferences onto a write that omits them. Which hosts ARE
// OpenRouter is the server's rule; the editor keeps no second copy of it, so it
// drops on any move instead of guessing.
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
  }
  return next;
}

// The three controls that name an adapter: which vendor, which model on it,
// and -- only where the vendor has no address of its own -- where to reach it.
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
  disabled,
  onChange,
  providers = PROVIDERS,
}: Readonly<{
  label: string;
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
  disabled: boolean;
  onChange: (next: B) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  // Asked of the VENDOR, and only while these fields are open — this is a real
  // round-trip on the installation's own credential, not a table read. The lane
  // travels with it so an installation binding one vendor at two hosts is asked
  // at the one THIS lane points at.
  const available = useAvailableModels(binding.provider, laneName, true);
  const host = HOST_FIELDS.get(binding.provider);
  return (
    <>
      <Field label={label}>
        {(control) => (
          <Select
            {...control}
            value={binding.provider}
            disabled={disabled}
            options={providers.map((p) => ({ value: p, label: p }))}
            onChange={(provider) => onChange(rebind(binding, { provider }))}
          />
        )}
      </Field>
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
          available.data?.unavailable
            ? modelSourceNote(available.data.unavailable, t)
            : t("aiRouting.model.help")
        }
      >
        {(control) => (
          <ComboBox
            {...control}
            value={binding.model}
            suggestions={offeredModels(
              available.data,
              catalogue,
              binding.provider,
              lane,
              locale,
            )}
            disabled={disabled}
            onChange={(model) => onChange(rebind(binding, { model }))}
          />
        )}
      </Field>
      {/* Only where it is load-bearing. An adapter with no default host is
          refused a binding without one, so leaving this off the form made
          every broker unbindable from here: the write was accepted and the
          running role then declined to adopt it. A native vendor addresses
          its own API, and an empty box beside it invites somebody to fill it
          in with something that overrides a working default. */}
      {host && (
        <Field label={t("aiRouting.baseUrl.label")} hint={t(host.help)}>
          {(control) => (
            <TextInput
              {...control}
              value={binding.base_url ?? ""}
              disabled={disabled}
              placeholder={t(host.placeholder)}
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
