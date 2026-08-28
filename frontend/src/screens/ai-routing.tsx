import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useCan, useCanWrite } from "../app/capability";
import { Button, EmptyState, Field, TextInput } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { ComboBox } from "../design-system/combobox";
import { Panel, PanelBody } from "../design-system/panel";
import { Select } from "../design-system/select";
import { stable } from "../format/collate";
import { useLocale, useT } from "../i18n";
import {
  type ModelCatalogue,
  type ModelLane,
  suggestionsFor,
  useAiModelCatalogue,
} from "./ai-models";
import { problemMessageOf, QueryGate, throwProblem } from "./common";

// Which vendor this installation's text is sent to (ai-operational-spec §1.4).
//
// Read by admin/ops only, unlike most cards here: `ai_routing` is narrow on both
// verbs because nothing needs the grant to SEE which models are bound — the AI
// profile answers that from the running config. This is the editable document,
// and it decides where an installation's correspondence goes.
//
// Editing re-points a lane. It does NOT add or remove one: the tier vocabulary
// comes from the task contract rather than from a person, so the form offers
// the tiers the installation already binds. An installation that binds nothing
// says so and points at where a binding is declared, rather than presenting an
// empty form that cannot be completed here.

type Routing = components["schemas"]["AiRouting"];
type TierBinding = components["schemas"]["AiTierBinding"];

// The adapters a tier may name. Written out because the wire carries a free
// string — the server refuses an unknown one, and a reader choosing from a list
// should not have to discover that by being refused.
const PROVIDERS = [
  "gemini",
  "anthropic",
  "openai",
  "openai_compatible",
  "ollama",
  "vllm",
  "fake",
] as const;

const PROFILES = ["eu_hosted", "sovereign", "cloud_frontier"] as const;

// The one adapter with no host of its own: every OpenAI-wire vendor is reached
// through it, so the endpoint is the binding rather than a tweak to it.
const OPENAI_WIRE = "openai_compatible";

function useRouting(enabled: boolean) {
  return useQuery({
    enabled,
    queryKey: ["ai-routing"],
    queryFn: async () => {
      const { data, error, response } = await api.GET("/ai/routing");
      if (error || !response.ok) {
        throwProblem(error);
      }
      return data;
    },
  });
}

function useReplaceRouting() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (next: Routing) => {
      const { data, error } = await api.PUT("/ai/routing", { body: next });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    onSuccess: (data) => {
      queryClient.setQueryData(["ai-routing"], data);
    },
  });
}

export function AiRoutingCard() {
  const t = useT();
  // The read grant gates the QUERY, not only the form. This tab opens on other
  // grants, so a seat can reach the page without `ai_routing:read` — and asking
  // anyway drew a 403 error box, which reads as a broken installation rather
  // than as a permission. Withheld is the answer every card on this page gives.
  const canSee = useCan("ai_routing", "read");
  const canManage = useCanWrite("ai_routing", "update");
  const query = useRouting(canSee);

  return (
    <Panel title={t("aiRouting.title")}>
      <PanelBody className="form-stack">
        <p className="t-caption">{t("aiRouting.sub")}</p>
        {canSee ? (
          <QueryGate query={query}>
            {(routing) => (
              <RoutingForm routing={routing} canManage={canManage} />
            )}
          </QueryGate>
        ) : (
          <EmptyState>
            <p className="t-small">{t("aiRouting.withheld")}</p>
          </EmptyState>
        )}
      </PanelBody>
    </Panel>
  );
}

// The ladder, cheapest to most capable. Tiers are shown in THIS order rather
// than alphabetically, because alphabetical puts cheap_cloud above frontier and
// local_small above premium — an order that tells a reader nothing about what
// they are choosing between.
//
// A tier this list does not know still renders, last and in a stable order: the
// vocabulary comes from the task contract, so a new one must appear rather than
// vanish from a form that is the only way to bind it.
const TIER_ORDER = [
  "local_small",
  "cheap_cloud",
  "premium",
  "frontier",
  "local_large",
];

function orderedTiers(tiers: Routing["tiers"] | undefined): string[] {
  const rank = (tier: string) => {
    const i = TIER_ORDER.indexOf(tier);
    return i === -1 ? TIER_ORDER.length : i;
  };
  return Object.keys(tiers ?? {}).sort(
    (a, b) => rank(a) - rank(b) || stable(a, b),
  );
}

function RoutingForm({
  routing,
  canManage,
}: Readonly<{ routing: Routing; canManage: boolean }>) {
  const t = useT();
  const replace = useReplaceRouting();
  // One read for every row on the card. No grant check in front of it: this
  // form only renders for a reader who already holds `ai_routing:read`, and the
  // hook answers an empty list rather than throwing when the sheet's own grant
  // is withheld — a field with nothing to suggest, which is what it was before.
  const catalogue = useAiModelCatalogue();
  // The stored document is the starting point, and the reader's edits live
  // here until they save. Keyed on the routing version so a binding another
  // role changed replaces an untouched form rather than being overwritten by
  // it — see the key at the call site.
  const [draft, setDraft] = useState<Routing>(routing);

  // Defensive on a field the contract marks required: a client that dies on an
  // unexpected shape takes the whole settings page with it, and the shape it
  // dies on is the one an error path or an older server produces.
  const tiers = orderedTiers(draft.tiers);
  if (tiers.length === 0) {
    // Not an error and not an empty form: this installation binds nothing, and
    // the way to give it a first binding is the deployment file's seed, which
    // is where a deployment declares one. Saying so beats a form whose every
    // field is blank and whose Save cannot produce a valid document.
    return <Callout tone="info">{t("aiRouting.unbound")}</Callout>;
  }

  const setTier = (tier: string, next: TierBinding) =>
    setDraft((d) => ({ ...d, tiers: { ...d.tiers, [tier]: next } }));

  return (
    <>
      <Field
        label={t("aiRouting.profile.label")}
        hint={t("aiRouting.profile.help")}
      >
        {(control) => (
          <Select
            {...control}
            value={draft.profile}
            disabled={!canManage || replace.isPending}
            options={PROFILES.map((p) => ({
              value: p,
              label: t(`aiRouting.profile.${p}`),
            }))}
            onChange={(value) =>
              setDraft((d) => ({ ...d, profile: value as Routing["profile"] }))
            }
          />
        )}
      </Field>

      {tiers.map((tier) => (
        <TierRow
          key={tier}
          tier={tier}
          binding={draft.tiers[tier]}
          catalogue={catalogue.data}
          disabled={!canManage || replace.isPending}
          onChange={(next) => setTier(tier, next)}
        />
      ))}

      {/* The embed lane, which the form used to leave out entirely — so a
          reader could re-point every chat tier and still be sending their
          retrieval to the vendor they had just moved away from, with nothing on
          screen saying so. It binds SEPARATELY on purpose: retrieval has to
          survive a chat-budget exhaustion, and the model is a different one even
          on the same vendor. */}
      <EmbeddingsRow
        binding={draft.embeddings}
        catalogue={catalogue.data}
        disabled={!canManage || replace.isPending}
        onChange={(embeddings) => setDraft((d) => ({ ...d, embeddings }))}
      />

      {replace.isError && (
        <Callout tone="danger" live="alert">
          {problemMessageOf(replace.error, t)}
        </Callout>
      )}
      {replace.isSuccess && (
        <Callout tone="success" live="status">
          {t("aiRouting.saved")}
        </Callout>
      )}
      <Button
        onClick={() => replace.mutate(draft)}
        pending={replace.isPending}
        busyLabel={t("aiRouting.saving")}
        reason={canManage ? undefined : t("aiRouting.adminOnly")}
      >
        {t("aiRouting.save")}
      </Button>
    </>
  );
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
function AdapterFields<
  B extends { provider: string; model: string; base_url?: string },
>({
  label,
  lane,
  binding,
  catalogue,
  disabled,
  onChange,
}: Readonly<{
  label: string;
  lane: ModelLane;
  binding: B;
  catalogue: ModelCatalogue;
  disabled: boolean;
  onChange: (next: B) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  return (
    <>
      <Field label={label}>
        {(control) => (
          <Select
            {...control}
            value={binding.provider}
            disabled={disabled}
            options={PROVIDERS.map((p) => ({ value: p, label: p }))}
            onChange={(provider) => onChange({ ...binding, provider })}
          />
        )}
      </Field>
      {/* The models this installation can PRICE, which is a different and more
          useful question than the models the vendor publishes: one outside the
          sheet serves calls and reports UNPRICED, and nobody notices until a
          usage report comes back with a week missing. Still a text box, because
          the server accepts any id its vendor offers and the sheet is a
          starting point rather than a permitted list. */}
      <Field
        label={t("aiRouting.model.label")}
        hint={t("aiRouting.model.help")}
      >
        {(control) => (
          <ComboBox
            {...control}
            value={binding.model}
            suggestions={suggestionsFor(
              catalogue,
              binding.provider,
              lane,
              locale,
            )}
            disabled={disabled}
            onChange={(model) => onChange({ ...binding, model })}
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

// The embeddings lane. Its own row rather than a TierRow, because the two
// shapes differ in both directions: this one takes `dimensions` and no `input`,
// and a widened row that accepted either field on either lane would offer a
// setting the server refuses.
function EmbeddingsRow({
  binding,
  catalogue,
  disabled,
  onChange,
}: Readonly<{
  binding: Routing["embeddings"];
  catalogue: ModelCatalogue;
  disabled: boolean;
  onChange: (next: Routing["embeddings"]) => void;
}>) {
  const t = useT();
  return (
    <div className="form-row" data-testid="ai-routing-embeddings">
      <AdapterFields
        label={t("aiRouting.embeddings.label")}
        lane="embeddings"
        binding={binding}
        catalogue={catalogue}
        disabled={disabled}
        onChange={onChange}
      />
      {/* The width this lane asks the provider for, which only it has. Blank
          means the compiled default rather than zero: the contract reads an
          omitted value and a 0 the same way, so an empty box must send neither
          a 0 nor a NaN. */}
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
    </div>
  );
}

// One tier's binding: which adapter serves it, and which of that adapter's
// models. Two controls rather than one, because the model id is the vendor's
// vocabulary and no list this app holds could stay current with it.
//
// The tier name is rendered RAW, like the provider name beside it. Both are the
// routing document's own vocabulary — an operator editing this reads `premium`
// and `gemini` in the same file — and translating an identifier would need a
// key built at runtime, which the narrow MessageKey type refuses for the good
// reason that a typo in one would otherwise ship.
function TierRow({
  tier,
  binding,
  catalogue,
  disabled,
  onChange,
}: Readonly<{
  tier: string;
  binding: TierBinding;
  catalogue: ModelCatalogue;
  disabled: boolean;
  onChange: (next: TierBinding) => void;
}>) {
  return (
    <div className="form-row" data-testid={`ai-routing-tier-${tier}`}>
      <AdapterFields
        label={tier}
        lane="chat"
        binding={binding}
        catalogue={catalogue}
        disabled={disabled}
        onChange={onChange}
      />
    </div>
  );
}
