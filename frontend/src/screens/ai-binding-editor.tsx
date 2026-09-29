// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useId, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useUnsavedGuard } from "../app/unsaved";
import { Button } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { Heading } from "../design-system/heading";
import { Modal } from "../design-system/modal";
import { useT } from "../i18n";
import { type ModelCatalogue, useAvailableModels } from "./ai-models";
import {
  AdapterFields,
  DECISION_PROVIDERS,
  EmbeddingWidthField,
  OPENROUTER_DECISION_PRESET,
  PROVIDERS,
} from "./ai-routing-fields";
import {
  fetchRouting,
  ROUTING_KEY,
  type RoutingRead,
} from "./ai-routing-query";
import {
  type SliceValue,
  sameSlice,
  sliceOf,
  withSlice,
} from "./ai-routing-slice";
import {
  problemCode,
  problemCodeOf,
  problemMessageOf,
  throwProblem,
} from "./common";

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

// The slice moved under the editor between open and Save.
class SliceMoved extends Error {}

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
    <Modal open onClose={onClose} labelledBy={headingId}>
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
              variant="danger"
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
              keyMissing
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
  keys,
  catalogue,
  disabled,
  onChange,
}: Readonly<{
  draft: SliceValue;
  current: string | undefined;
  keys: readonly KeyStatus[] | undefined;
  catalogue: ModelCatalogue;
  disabled: boolean;
  onChange: (next: SliceValue) => void;
}>) {
  const t = useT();
  const label = t("aiRouting.provider.label");
  switch (draft.kind) {
    case "tier":
      return (
        <AdapterFields
          label={label}
          lane="chat"
          laneName={draft.tier}
          binding={draft.binding}
          catalogue={catalogue}
          disabled={disabled}
          providers={reachableProviders(PROVIDERS, keys, current)}
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
            disabled={disabled}
            providers={reachableProviders(PROVIDERS, keys, current)}
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
  current,
  keys,
  catalogue,
  disabled,
  onChange,
}: Readonly<{
  binding: DecisionsBinding;
  current: string | undefined;
  keys: readonly KeyStatus[] | undefined;
  catalogue: ModelCatalogue;
  disabled: boolean;
  onChange: (next: DecisionsBinding) => void;
}>) {
  const t = useT();
  return (
    <>
      <AdapterFields
        label={t("aiRouting.provider.label")}
        lane="decisions"
        laneName="decisions"
        binding={binding}
        catalogue={catalogue}
        disabled={disabled}
        providers={reachableProviders(DECISION_PROVIDERS, keys, current)}
        onChange={(next) => onChange(reboundDecision(binding, next))}
      />
      {binding.provider === OPENROUTER_DECISION_PRESET.provider && (
        // The endpoint is a full URL nobody remembers, and OpenRouter's is the
        // one most installations want; the key is the one thing it cannot fill.
        <div>
          <Button
            disabled={disabled}
            onClick={() =>
              onChange({
                ...binding,
                base_url: OPENROUTER_DECISION_PRESET.base_url,
                model: OPENROUTER_DECISION_PRESET.model,
              })
            }
          >
            {t("aiRouting.decisions.preset.openrouter")}
          </Button>
          <p className="t-sub">
            {t("aiRouting.decisions.preset.openrouterKey")}
          </p>
        </div>
      )}
    </>
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

// The adapters an editor offers: those this installation can reach, and the
// one the binding names now even if it no longer can — dropping that would
// erase the lane's own state from its own editor. A vendor with no entry in
// the key list takes no key. While the list has not arrived nothing is hidden.
export function reachableProviders(
  all: readonly string[],
  keys: readonly KeyStatus[] | undefined,
  current: string | undefined,
): readonly string[] {
  if (!keys) return all;
  const status = new Map(keys.map((k) => [k.provider, k]));
  return all.filter((provider) => {
    const entry = status.get(provider);
    return provider === current || !entry || entry.configured || entry.optional;
  });
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

function laneName(value: SliceValue): string {
  return value.kind === "tier" ? value.tier : value.kind;
}

function isConflict(error: unknown): boolean {
  return error instanceof SliceMoved || problemCodeOf(error) === "version_skew";
}

function readLatest(
  queryClient: ReturnType<typeof useQueryClient>,
): Promise<RoutingRead> {
  return queryClient.fetchQuery({
    queryKey: ROUTING_KEY,
    queryFn: fetchRouting,
    staleTime: 0,
  });
}

// Puts one slice onto the latest document. A 409 means some write landed
// between the re-read and the PUT; it is retried once, because the re-read
// then tells a colleague's edit to ANOTHER lane — which is no conflict — from
// one to this lane, which throws SliceMoved as it would have at first.
export async function writeSlice(
  latestRead: () => Promise<RoutingRead>,
  base: SliceValue,
  edit: SliceValue,
): Promise<RoutingRead> {
  for (let attempt = 0; ; attempt++) {
    const latest = await latestRead();
    if (!sameSlice(sliceOf(latest.routing, base), base)) {
      throw new SliceMoved();
    }
    const { data, error, response } = await api.PUT("/ai/routing", {
      body: withSlice(latest.routing, edit),
      // Always sent: an absent If-Match is an unconditional overwrite.
      headers: { "If-Match": latest.version },
    });
    if (error) {
      if (attempt === 0 && problemCode(error) === "version_skew") continue;
      throwProblem(error);
    }
    if (!data) throw new Error("AI routing unavailable");
    return { routing: data, version: response.headers.get("ETag") ?? "" };
  }
}
