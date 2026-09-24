// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useEffect, useState } from "react";
import type { components } from "../api/schema";
import { Field, TextInput } from "../design-system/atoms";
import { ComboBox } from "../design-system/combobox";
import { Select } from "../design-system/select";
import { type Translator, useLocale, useT } from "../i18n";
import {
  type AvailableModels,
  type ModelCatalogue,
  type ModelLane,
  offeredModels,
  useAvailableModels,
  useModelProbe,
} from "./ai-models";
import { VERTEX_PROVIDER, VertexLocationField } from "./vertex-location";

type Routing = components["schemas"]["AiRouting"];
// The adapters a tier may name. Written out because the wire carries a free
// string — the server refuses an unknown one, and a reader choosing from a list
// should not have to discover that by being refused. A declared mirror of
// `ai.KnownProviders()`, held both ways by backend/gates/frontendproviders_test.go.
const PROVIDERS = [
  "gemini",
  "gemini_vertex",
  "anthropic",
  "openai",
  "openai_compatible",
  "ollama",
  "vllm",
  "fake",
] as const;

// The one adapter with no host of its own: every OpenAI-wire vendor is reached
// through it, so the endpoint is the binding rather than a tweak to it.
const OPENAI_WIRE = "openai_compatible";

type AdapterBinding = {
  provider: string;
  model: string;
  base_url?: string;
  location?: string;
};

/**
 * The binding re-pointed at another adapter. `location` belongs to Vertex alone
 * and `base_url` is refused there, so each is dropped where the server would
 * refuse it; a Vertex binding keeps its own location or takes the default.
 */
export function withProvider<B extends AdapterBinding>(
  binding: B,
  provider: string,
  vertexLocation: string,
): B {
  if (provider === VERTEX_PROVIDER) {
    return {
      ...binding,
      provider,
      base_url: undefined,
      location: binding.location ?? vertexLocation,
    };
  }
  return { ...binding, provider, location: undefined };
}

/** One probe: whether `location` serves `model`, and what to do if it does not. */
type ProbeTarget = Readonly<{
  location: string;
  model: string;
  clearIfUnserved: boolean;
}>;

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
export function AdapterFields<B extends AdapterBinding>({
  label,
  lane,
  laneName,
  binding,
  catalogue,
  profile,
  vertexLocation,
  disabled,
  onChange,
}: Readonly<{
  label: string;
  lane: ModelLane;
  // Which lane of the routing document this is, in the document's own words.
  // `lane` above says chat-or-embeddings, which is what a model is FOR; this
  // says which binding, which is what the host is read from.
  laneName: string;
  binding: B;
  catalogue: ModelCatalogue;
  // The draft's profile, which decides the Vertex locations on offer.
  profile: string;
  // Where a lane newly pointed at Vertex starts: another saved Vertex lane's.
  vertexLocation: string;
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
  const suggestions = offeredModels(
    available.data,
    catalogue,
    binding.provider,
    lane,
    locale,
  );
  const [probeTarget, setProbeTarget] = useState<ProbeTarget | undefined>();
  const [cleared, setCleared] = useState<ProbeTarget | undefined>();
  const probe = useModelProbe(
    binding.provider,
    laneName,
    vertex ? probeTarget : undefined,
  );
  // The probe speaks for the field only while it asked about what the field
  // holds; a model typed since is a different question.
  const probed =
    vertex &&
    probeTarget !== undefined &&
    probeTarget.model === binding.model &&
    probeTarget.location === location;
  const unserved = probed && probe.data?.unavailable === "no_endpoint";
  // A location change that the model does not survive empties the field
  // rather than leaving a binding the save would refuse.
  useEffect(() => {
    if (unserved && probeTarget?.clearIfUnserved) {
      setCleared(probeTarget);
      // Once: the same model typed back in is flagged, not cleared again.
      setProbeTarget({ ...probeTarget, clearIfUnserved: false });
      onChange({ ...binding, model: "" });
    }
  }, [unserved, probeTarget, binding, onChange]);

  const hint = vertex
    ? vertexModelHint({
        available: available.data,
        probe: probed ? probe.data : undefined,
        probing: probed && probe.isFetching,
        cleared,
        location,
        t,
      })
    : undefined;
  return (
    <>
      <Field label={label}>
        {(control) => (
          <Select
            {...control}
            value={binding.provider}
            disabled={disabled}
            options={PROVIDERS.map((p) => ({ value: p, label: p }))}
            onChange={(provider) => {
              setProbeTarget(undefined);
              setCleared(undefined);
              onChange(withProvider(binding, provider, vertexLocation));
            }}
          />
        )}
      </Field>
      {vertex && (
        <VertexLocationField
          value={location}
          profile={profile}
          disabled={disabled}
          onChange={(next) => {
            setCleared(undefined);
            setProbeTarget(
              binding.model === ""
                ? undefined
                : {
                    location: next,
                    model: binding.model,
                    clearIfUnserved: true,
                  },
            );
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
          (available.data?.unavailable
            ? modelSourceNote(available.data.unavailable, t)
            : t("aiRouting.model.help"))
        }
        error={hint?.error}
      >
        {(control) => (
          <ComboBox
            {...control}
            value={binding.model}
            suggestions={suggestions}
            disabled={disabled}
            onChange={(model) => {
              setCleared(undefined);
              // A pick from the list is a choice worth checking; a keystroke
              // is not, and each probe is a call on the service account.
              if (vertex && suggestions.some((s) => s.value === model)) {
                setProbeTarget({ location, model, clearIfUnserved: false });
              }
              onChange({ ...binding, model });
            }}
          />
        )}
      </Field>
      {/* Only where it is load-bearing. openai_compatible has no default host
          and the server refuses a binding without one, so leaving this off the
          form made every broker unbindable from here: the write was accepted
          and the running role then declined to adopt it. A native vendor
          addresses its own API, and an empty box beside it invites somebody to
          fill it in with something that overrides a working default. */}
      {binding.provider === OPENAI_WIRE && (
        <Field
          label={t("aiRouting.baseUrl.label")}
          hint={t("aiRouting.baseUrl.help")}
        >
          {(control) => (
            <TextInput
              {...control}
              value={binding.base_url ?? ""}
              disabled={disabled}
              placeholder={t("aiRouting.baseUrl.placeholder")}
              onChange={(e) =>
                onChange({ ...binding, base_url: e.target.value })
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

/**
 * What a Vertex lane's model field says: the probe's verdict on the chosen
 * model when there is one, else what the location's list could tell. Undefined
 * text falls through to the note every vendor shares.
 */
function vertexModelHint({
  available,
  probe,
  probing,
  cleared,
  location,
  t,
}: Readonly<{
  available: AvailableModels | undefined;
  probe: AvailableModels | undefined;
  probing: boolean;
  cleared: ProbeTarget | undefined;
  location: string;
  t: Translator;
}>): { text?: string; error?: string } {
  if (cleared) {
    return {
      text: t("aiRouting.probe.cleared", {
        model: cleared.model,
        location: cleared.location,
      }),
    };
  }
  if (probing) {
    return { text: t("aiRouting.probe.checking", { location }) };
  }
  if (probe?.unavailable === "no_endpoint") {
    return { error: t("aiRouting.probe.notServed", { location }) };
  }
  if (probe?.unavailable) {
    return { text: t("aiRouting.probe.unverified", { location }) };
  }
  if (probe) {
    return { text: t("aiRouting.probe.served", { location }) };
  }
  if (available?.unavailable === "no_key") {
    return { text: t("aiRouting.location.noKey") };
  }
  if (available?.unavailable === "profile_forbids") {
    return { text: t("aiRouting.location.forbidden") };
  }
  if (available && !available.unavailable && available.models.length === 0) {
    return { text: t("aiRouting.location.noModels", { location }) };
  }
  return {};
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
