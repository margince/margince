// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useCallback, useId, useState } from "react";
import type { components } from "../api/schema";
import { useUnsavedGuard } from "../app/unsaved";
import { Button } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { Heading } from "../design-system/heading";
import { Modal } from "../design-system/modal";
import { useT } from "../i18n";
import { isConflict, readLatest, writeSlice } from "./ai-binding-write";
import { TierRecentCalls } from "./ai-call-figures";
import {
  type AvailableModels,
  type ModelCatalogue,
  useAvailableModels,
} from "./ai-models";
import {
  AdapterFields,
  DECISION_PROVIDERS,
  EmbeddingWidthField,
  openRouterPreset,
  PROVIDERS,
} from "./ai-routing-fields";
import {
  boundProviders,
  ROUTING_KEY,
  type RoutingRead,
} from "./ai-routing-query";
import { type SliceValue, sameSlice, sliceOf } from "./ai-routing-slice";
import { ServingSection, servingBlocked } from "./ai-serving-editor";
import { problemMessageOf } from "./common";
import { savedVertexLocation, VERTEX_PROVIDER } from "./vertex-location";

// One binding's editor: provider, model, and whatever else only that lane has.
//
// It owns ONE slice of the routing document and nothing else. The slice it
// opened on is remembered as its base; Save re-reads the document, and writes
// only if that slice is still what the reader started from. Checking at open
// rather than at save is the point: the query cache refetches on focus, so a
// base read at save time could already hold a colleague's edit, and writing
// over it would be an overwrite nobody was told about.

type KeyStatus = components["schemas"]["AiProviderKeyStatus"];
type DecisionsBinding = components["schemas"]["AiDecisionsBinding"];

export function BindingEditor({
  opened,
  initial,
  label,
  keys,
  catalogue,
  canManage,
  onClose,
}: Readonly<{
  // The document as it read when the editor opened.
  opened: RoutingRead;
  // Where the fields start: the stored slice, or a first decision binding.
  initial: SliceValue;
  label: string;
  keys: readonly KeyStatus[] | undefined;
  catalogue: ModelCatalogue;
  canManage: boolean;
  onClose: () => void;
}>) {
  const t = useT();
  const headingId = useId();
  const queryClient = useQueryClient();
  const [base, setBase] = useState(() => sliceOf(opened.routing, initial));
  const [draft, setDraft] = useState(initial);
  const [conflict, setConflict] = useState(false);
  // A serving block the server has not yet cleared holds the save, so a value
  // it would refuse never reaches the write.
  const [servingValid, setServingValid] = useState(true);
  const onServingValid = useCallback(
    (valid: boolean) => setServingValid(valid),
    [],
  );
  // A move away mid-edit asks first, as the page's other editors do.
  useUnsavedGuard(!sameSlice(draft, initial));
  const save = useMutation({
    mutationFn: (vars: { base: SliceValue; edit: SliceValue }) =>
      writeSlice(() => readLatest(queryClient), vars.base, vars.edit),
    onSuccess: async (saved) => {
      queryClient.setQueryData(ROUTING_KEY, saved);
      await queryClient.invalidateQueries({ queryKey: ["ai-status"] });
    },
  });

  const submit = (edit: SliceValue) => {
    setConflict(false);
    save.mutate(
      { base, edit },
      {
        onSuccess: onClose,
        onError: async (error) => {
          if (!isConflict(error)) return;
          // The reader's edit stays on screen; what moves is the base, so the
          // next Save is judged against the binding they have now been shown
          // was changed.
          setConflict(true);
          save.reset();
          // A failed re-read leaves the old base, so the next Save is caught
          // again rather than writing over a change nobody has seen.
          try {
            const fresh = await readLatest(queryClient);
            setBase(sliceOf(fresh.routing, edit));
          } catch {
            return;
          }
        },
      },
    );
  };

  const binding = draft.binding;
  const keyMissing = binding ? missingKey(binding.provider, keys) : false;
  const busy = !canManage || save.isPending;
  return (
    <Modal open onClose={onClose} labelledBy={headingId} size="wide">
      <Heading size="large" id={headingId} className="t-h2 modal-title">
        {draft.kind === "decisions" && base.binding === undefined
          ? t("aiRouting.decisions.add")
          : t("aiRouting.editTitle", { lane: label })}
      </Heading>
      {binding && (
        <div className="form-stack">
          <SliceFields
            draft={draft}
            current={base.binding?.provider}
            routing={opened.routing}
            keys={keys}
            catalogue={catalogue}
            disabled={busy}
            onChange={setDraft}
          />
          <NotListedHint
            provider={binding.provider}
            model={binding.model}
            lane={laneName(draft)}
          />
          <TierRecentCalls tier={callTier(draft)} />
          <ServingSection
            key={`${binding.provider}|${binding.model}`}
            value={draft}
            routing={opened.routing}
            disabled={busy}
            onValid={onServingValid}
            onChange={(next) => {
              if (draft.kind === "tier")
                setDraft({
                  ...draft,
                  binding: { ...draft.binding, routing: next },
                });
              if (draft.kind === "embeddings")
                setDraft({
                  ...draft,
                  binding: { ...draft.binding, routing: next },
                });
            }}
          />
        </div>
      )}
      {keyMissing && (
        <Callout
          tone="warning"
          kind="standing"
          title={t("aiRouting.keyMissing")}
        >
          {t("aiRouting.keyMissingHelp", { provider: binding?.provider ?? "" })}
        </Callout>
      )}
      {conflict && (
        <Callout
          tone="warning"
          kind="standing"
          title={t("aiAdmin.routingStale")}
        >
          {t("aiRouting.conflictHelp")}
        </Callout>
      )}
      {save.isError && !isConflict(save.error) && (
        <Callout tone="danger" kind="outcome" title={t("aiRouting.saveFailed")}>
          {problemMessageOf(save.error, t)}
        </Callout>
      )}
      <div className="actions">
        {draft.kind === "decisions" && base.binding !== undefined && (
          <span className="actions-lead">
            <Button
              variant="link"
              disabled={busy}
              onClick={() => submit({ kind: "decisions", binding: undefined })}
            >
              {t("aiRouting.decisions.remove")}
            </Button>
          </span>
        )}
        <span className="actions-pair">
          <Button onClick={onClose} disabled={save.isPending}>
            {t("aiAdmin.cancel")}
          </Button>
          <Button
            variant="primary"
            pending={save.isPending}
            busyLabel={t("aiRouting.saving")}
            disabled={
              !canManage ||
              !binding ||
              binding.model.trim() === "" ||
              keyMissing ||
              (servingBlocked(draft, opened.routing) === null && !servingValid)
            }
            reason={canManage ? undefined : t("aiRouting.adminOnly")}
            onClick={() => submit(draft)}
          >
            {t("aiRouting.saveBinding")}
          </Button>
        </span>
      </div>
    </Modal>
  );
}

// The fields for whichever lane this is. Every lane asks provider and model;
// the embedder adds its width, and the decision model its OpenRouter preset.
function SliceFields({
  draft,
  current,
  routing,
  keys,
  catalogue,
  disabled,
  onChange,
}: Readonly<{
  draft: SliceValue;
  current: string | undefined;
  routing: RoutingRead["routing"];
  keys: readonly KeyStatus[] | undefined;
  catalogue: ModelCatalogue;
  disabled: boolean;
  onChange: (next: SliceValue) => void;
}>) {
  const t = useT();
  const label = t("aiRouting.provider.label");
  const probes = useKeylessProbes(laneName(draft), draft.kind !== "decisions");
  // A lane newly pointed at Vertex starts at the provider's location, which
  // every tier on it is served from; else where another saved Vertex lane is.
  const vertexLocation =
    routing.providers?.[VERTEX_PROVIDER]?.location ??
    savedVertexLocation([...Object.values(routing.tiers), routing.embeddings]);
  switch (draft.kind) {
    case "tier":
      return (
        <AdapterFields
          label={label}
          lane="chat"
          laneName={draft.tier}
          binding={draft.binding}
          catalogue={catalogue}
          profile={routing.profile}
          vertexLocation={vertexLocation}
          providerSettings={routing.providers?.[draft.binding.provider] ?? {}}
          disabled={disabled}
          providers={reachableProviders(
            PROVIDERS,
            keys,
            current,
            probes,
            routing,
          )}
          onChange={(binding) => onChange({ ...draft, binding })}
        />
      );
    case "embeddings":
      return (
        <>
          <AdapterFields
            label={label}
            lane="embeddings"
            laneName="embeddings"
            binding={draft.binding}
            catalogue={catalogue}
            profile={routing.profile}
            vertexLocation={vertexLocation}
            providerSettings={routing.providers?.[draft.binding.provider] ?? {}}
            disabled={disabled}
            providers={reachableProviders(
              PROVIDERS,
              keys,
              current,
              probes,
              routing,
            )}
            onChange={(binding) => onChange({ ...draft, binding })}
          />
          <EmbeddingWidthField
            binding={draft.binding}
            disabled={disabled}
            onChange={(binding) => onChange({ ...draft, binding })}
          />
        </>
      );
    case "decisions":
      return draft.binding ? (
        <DecisionFields
          binding={draft.binding}
          providers={routing.providers}
          current={current}
          keys={keys}
          catalogue={catalogue}
          disabled={disabled}
          onChange={(binding) => onChange({ kind: "decisions", binding })}
        />
      ) : null;
  }
}

function DecisionFields({
  binding,
  providers,
  current,
  keys,
  catalogue,
  disabled,
  onChange,
}: Readonly<{
  binding: DecisionsBinding;
  providers: RoutingRead["routing"]["providers"];
  current: string | undefined;
  keys: readonly KeyStatus[] | undefined;
  catalogue: ModelCatalogue;
  disabled: boolean;
  onChange: (next: DecisionsBinding) => void;
}>) {
  const t = useT();
  const settings = providers?.[binding.provider] ?? {};
  return (
    <AdapterFields
      label={t("aiRouting.provider.label")}
      lane="decisions"
      laneName="decisions"
      binding={binding}
      catalogue={catalogue}
      disabled={disabled}
      providerSettings={settings}
      providers={reachableProviders(DECISION_PROVIDERS, keys, current)}
      onChange={(next) => onChange(reboundDecision(binding, next))}
      // The endpoint is a full URL nobody remembers, and OpenRouter's is the
      // one most installations want; the key is the one thing it cannot fill.
      providerAside={openRouterPreset(
        binding,
        settings.base_url,
        disabled,
        onChange,
        t,
      )}
    />
  );
}

// A model id the vendor's own list does not carry. A hint and never a gate:
// the list is not a permitted set — a vendor ships a model on a Tuesday — and
// the server's refusal stays the only hard check.
function NotListedHint({
  provider,
  model,
  lane,
}: Readonly<{ provider: string; model: string; lane: string }>) {
  const t = useT();
  const available = useAvailableModels(provider, lane, true);
  const listed = available.data;
  if (
    !listed ||
    listed.unavailable ||
    listed.models.length === 0 ||
    model.trim() === "" ||
    listed.models.some((m) => m.id === model.trim())
  ) {
    return null;
  }
  return (
    <p className="t-sub" role="status">
      {t("aiRouting.notListed", { provider })}
    </p>
  );
}

// The adapters an editor offers: those this installation can use, and the one
// the binding names now even if it no longer can — dropping that would erase
// the lane's own state from its own editor. A vendor with a key row is usable
// once a key is sealed; a keyless one (ollama, vllm) once the availability
// probe reached it; `fake` only where the routing document already binds it.
// While the key list has not arrived nothing is hidden; once it has, a keyless
// adapter stays out until its probe answers, since an option that appears late
// costs less than one offered and then refused.
export function reachableProviders(
  all: readonly string[],
  keys: readonly KeyStatus[] | undefined,
  current: string | undefined,
  probes: KeylessProbes = NO_PROBES,
  routing?: RoutingRead["routing"],
): readonly string[] {
  if (!keys) return all;
  const status = new Map(keys.map((k) => [k.provider, k]));
  return all.filter((provider) => {
    if (provider === current) return true;
    if (provider === "fake") {
      return (
        routing !== undefined && boundProviders(routing)?.has(provider) === true
      );
    }
    if (isKeyless(provider)) {
      const answer = probes.get(provider);
      return answer !== undefined && !answer.unavailable;
    }
    const entry = status.get(provider);
    return !entry || entry.configured || entry.optional;
  });
}

const KEYLESS_ADAPTERS = ["ollama", "vllm"] as const;

function isKeyless(provider: string): boolean {
  return KEYLESS_ADAPTERS.some((adapter) => adapter === provider);
}

type KeylessProbes = ReadonlyMap<string, AvailableModels | undefined>;
const NO_PROBES: KeylessProbes = new Map();

// What each keyless chat adapter answers when asked, for the lane being edited.
// Two queries that fire only while the editor is mounted, and not at all for
// the decision lane, whose adapters both take a key.
function useKeylessProbes(lane: string, enabled: boolean): KeylessProbes {
  const ollama = useAvailableModels("ollama", lane, enabled);
  const vllm = useAvailableModels("vllm", lane, enabled);
  return new Map([
    ["ollama", ollama.data],
    ["vllm", vllm.data],
  ]);
}

// A provider that takes a key and holds none. Keyless adapters (no entry in
// the key list) and optional-key ones are never missing a key.
export function missingKey(
  provider: string,
  keys: readonly KeyStatus[] | undefined,
): boolean {
  const entry = keys?.find((k) => k.provider === provider);
  return entry !== undefined && !entry.configured && !entry.optional;
}

// A decision model and its host name one adapter's endpoint, so a provider
// switch starts the binding over rather than pointing the new adapter at the
// old one's address.
function reboundDecision(
  previous: DecisionsBinding,
  next: DecisionsBinding,
): DecisionsBinding {
  return next.provider === previous.provider
    ? next
    : { provider: next.provider, model: "" };
}

/** The tier a lane's calls are recorded under in the call record. */
function callTier(value: SliceValue): string {
  switch (value.kind) {
    case "tier":
      return value.tier;
    case "embeddings":
      return "embed";
    case "decisions":
      return "decide";
  }
}

function laneName(value: SliceValue): string {
  return value.kind === "tier" ? value.tier : value.kind;
}
