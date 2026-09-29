import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useCan, useCanWrite } from "../app/capability";
import { Button, EmptyState } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { Panel, PanelBody, PanelIntro } from "../design-system/panel";
import { stable } from "../format/collate";
import { useT } from "../i18n";
import { BindingEditor, reachableProviders } from "./ai-binding-editor";
import { TierFacts, UntrackedFacts, useLaneFactsSource } from "./ai-lane-facts";
import {
  type ModelCatalogue,
  unkeyedProviders,
  useAiModelCatalogue,
} from "./ai-models";
import { useProviderKeys } from "./ai-provider-keys";
import { DECISION_PROVIDERS } from "./ai-routing-fields";
import { DecisionLaneRow, LaneRow } from "./ai-routing-lane";
import { ROUTING_KEY, type RoutingRead, useRouting } from "./ai-routing-query";
import { type SliceValue, sliceOf } from "./ai-routing-slice";
import { problemMessageOf, QueryGate, throwProblem } from "./common";
import { RefreshModelPrices } from "./rate-catalogue-refresh";
import { SETUP_PROVIDERS } from "./setup-providers";
import "./ai-settings.css";

// Which provider and model each tier uses.
//
// Read by admin/ops only: `ai_routing` is narrow on both verbs because this is
// the editable document, and it decides where an installation's correspondence
// goes.
//
// Each row is a reading; Edit opens a dialog that owns that one binding and
// saves it alone. The tier vocabulary comes from the task contract rather than
// from a contact, so rows re-point a lane and never add or remove one — except
// the decision model, which a document may leave out.
//
// The installation profile is shown and never edited here. It decides which
// vendors a lane may name, and a Save it refuses says so; the line tells the
// reader which profile did the refusing.

type Routing = components["schemas"]["AiRouting"];

export function AiRoutingCard({
  onPriceSheet,
}: Readonly<{
  // Where the prices behind these bindings are read. A link rather than a
  // second copy of the sheet: it is one table.
  onPriceSheet?: () => void;
}>) {
  const t = useT();
  // The read grant gates the QUERY, not only the rows: asking without it draws
  // a 403 error box, which reads as a broken installation.
  const canSee = useCan("ai_routing", "read");
  const canWrite = useCanWrite("ai_routing", "update");
  const canReadBudget = useCan("ai_budget", "read");
  const canManage = canWrite && canReadBudget;
  const query = useRouting(canSee);

  if (!canSee) {
    return (
      <Panel title={t("aiRouting.title")}>
        <PanelBody>
          <EmptyState>{t("aiRouting.withheld")}</EmptyState>
        </PanelBody>
      </Panel>
    );
  }

  return (
    <QueryGate query={query} pendingLabel={t("aiRouting.title")}>
      {(read) => (
        <ModelTiers
          read={read}
          canManage={canManage}
          onPriceSheet={onPriceSheet}
        />
      )}
    </QueryGate>
  );
}

// The ladder, cheapest to most capable. A tier this list does not know still
// renders, last and in a stable order: the vocabulary comes from the task
// contract, so a new one must appear rather than vanish from the only screen
// that binds it.
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

// An open editor: the document it opened on, and where its fields start.
type Editing = { opened: RoutingRead; initial: SliceValue; label: string };

function ModelTiers({
  read,
  canManage,
  onPriceSheet,
}: Readonly<{
  read: RoutingRead;
  canManage: boolean;
  onPriceSheet?: () => void;
}>) {
  const t = useT();
  const { routing } = read;
  // One read for every row. The hook answers an empty list rather than
  // throwing when the sheet's own grant is withheld.
  const catalogue = useAiModelCatalogue();
  // Which vendors hold a credential, joined into the rows and the editor. Same
  // grant as this card, so no second denial to answer.
  const keys = useProviderKeys(true);
  const facts = useLaneFactsSource();
  const [editing, setEditing] = useState<Editing | null>(null);
  const unkeyed = unkeyedProviders(keys.data?.providers);
  const open = (initial: SliceValue, label: string) =>
    setEditing({ opened: read, initial, label });

  // Defensive on a field the contract marks required: a client that dies on an
  // unexpected shape takes the whole settings page with it.
  const tiers = orderedTiers(routing.tiers);
  if (tiers.length === 0) {
    return (
      <Panel title={t("aiRouting.title")}>
        <PanelBody>
          <FirstBinding
            read={read}
            providers={keys.data?.providers}
            canManage={canManage}
          />
        </PanelBody>
      </Panel>
    );
  }

  return (
    <Panel
      title={t("aiRouting.title")}
      titleAction={
        onPriceSheet ? (
          <button type="button" className="link-button" onClick={onPriceSheet}>
            {t("aiRouting.priceSheet")}
          </button>
        ) : undefined
      }
      footer={<SheetFooter catalogue={catalogue.data} canManage={canManage} />}
    >
      <PanelBody>
        <PanelIntro>{t("aiRouting.intro")}</PanelIntro>
        <p className="t-sub" data-testid="ai-routing-profile">
          {t("aiRouting.profileLine", { profile: routing.profile })}
        </p>
      </PanelBody>
      {tiers.map((tier) => (
        <LaneRow
          key={tier}
          lane="chat"
          name={tier}
          binding={routing.tiers[tier]}
          catalogue={catalogue.data}
          unkeyed={unkeyed}
          onEdit={() => open(sliceOf(routing, { kind: "tier", tier }), tier)}
          facts={<TierFacts tier={tier} source={facts} />}
        />
      ))}
      {/* The embed lane binds SEPARATELY on purpose: retrieval has to survive a
          chat-budget exhaustion, and the model is a different one even on the
          same vendor. Its name is the document's own word, raw, like the tier
          names above it. */}
      <LaneRow
        lane="embeddings"
        name="embeddings"
        testId="ai-routing-embeddings"
        binding={routing.embeddings}
        catalogue={catalogue.data}
        unkeyed={unkeyed}
        onEdit={() =>
          open(sliceOf(routing, { kind: "embeddings" }), "embeddings")
        }
        facts={<UntrackedFacts />}
      />
      <DecisionLaneRow
        binding={routing.decisions}
        catalogue={catalogue.data}
        unkeyed={unkeyed}
        canManage={canManage}
        onEdit={() =>
          open(
            {
              kind: "decisions",
              binding: routing.decisions ?? {
                provider: firstDecisionProvider(keys.data?.providers),
                model: "",
              },
            },
            "decisions",
          )
        }
        facts={<UntrackedFacts />}
      />
      {editing && (
        <BindingEditor
          opened={editing.opened}
          initial={editing.initial}
          label={editing.label}
          keys={keys.data?.providers}
          catalogue={catalogue.data}
          canManage={canManage}
          onClose={() => setEditing(null)}
        />
      )}
    </Panel>
  );
}

// What the model lists in the editor are, and how to move them on. The sheet is
// a SNAPSHOT somebody took on a day; undated it reads as "these are the models",
// and the refresh is the way past it.
function SheetFooter({
  catalogue,
  canManage,
}: Readonly<{ catalogue: ModelCatalogue; canManage: boolean }>) {
  const t = useT();
  const asOf = sheetAsOf(catalogue);
  return (
    <div className="ai-sheet-age">
      <span className="t-caption">
        {asOf
          ? t("aiRouting.sheetAsOf", { date: asOf })
          : t("aiRouting.sheetUnknown")}
      </span>
      {canManage && <RefreshModelPrices />}
    </div>
  );
}

// An installation that binds nothing needs a FIRST binding, and it has to be
// reachable from HERE: `seeds.ai_routing` is consumed once, at company
// creation, so an installation that already exists can never take one.
//
// The one whole-document write on this card. A per-lane save cannot produce a
// valid document from nothing — the server requires every tier's sibling
// embeddings binding and a profile — so the preset writes them all at once, and
// every lane is then edited like any other.
function FirstBinding({
  read,
  providers,
  canManage,
}: Readonly<{
  read: RoutingRead;
  providers: readonly { provider: string; configured: boolean }[] | undefined;
  canManage: boolean;
}>) {
  const t = useT();
  const queryClient = useQueryClient();
  const bind = useMutation({
    mutationFn: async (vars: {
      id: keyof typeof SETUP_PROVIDERS;
      stored: string;
      version: string;
    }) => {
      const { data, error, response } = await api.PUT("/ai/routing", {
        body: firstBinding(vars.id, vars.stored),
        headers: { "If-Match": vars.version },
      });
      if (error) {
        throwProblem(error);
      }
      if (!data) throw new Error("AI routing unavailable");
      return { routing: data, version: response.headers.get("ETag") ?? "" };
    },
    onSuccess: async (saved) => {
      queryClient.setQueryData(ROUTING_KEY, saved);
      await queryClient.invalidateQueries({ queryKey: ["ai-status"] });
    },
  });
  const startable = startableProviders(providers);
  if (startable.length === 0) {
    // No key, so nothing to bind TO; adding one is the part a reader of THIS
    // screen can act on.
    return (
      <EmptyState title={t("aiRouting.unboundTitle")}>
        {t("aiRouting.unboundUnkeyed")}
      </EmptyState>
    );
  }
  return (
    <>
      <EmptyState
        title={t("aiRouting.unboundTitle")}
        action={startable.map(({ id, label }) => (
          <Button
            key={id}
            pending={bind.isPending && bind.variables?.id === id}
            disabled={!canManage || bind.isPending}
            reason={canManage ? undefined : t("aiRouting.adminOnly")}
            onClick={() =>
              bind.mutate({
                id,
                stored: read.routing.profile,
                version: read.version,
              })
            }
          >
            {t("aiRouting.unboundStart", { provider: label })}
          </Button>
        ))}
      >
        {t("aiRouting.unboundKeyed")}
      </EmptyState>
      {bind.isError && (
        <Callout tone="danger" kind="outcome" title={t("aiRouting.saveFailed")}>
          {problemMessageOf(bind.error, t)}
        </Callout>
      )}
    </>
  );
}

// The providers this installation could bind RIGHT NOW: keyed, and named by a
// preset so the binding opens on real model ids rather than blank fields.
//
// Deliberately the onboarding list rather than every keyed vendor: those two
// serve chat AND embeddings from one key, and a routing document REQUIRES an
// embeddings binding.
function startableProviders(
  providers: readonly { provider: string; configured: boolean }[] | undefined,
): readonly { id: keyof typeof SETUP_PROVIDERS; label: string }[] {
  const keyed = new Set(
    (providers ?? []).filter((p) => p.configured).map((p) => p.provider),
  );
  return (
    Object.keys(SETUP_PROVIDERS) as (keyof typeof SETUP_PROVIDERS)[]
  ).flatMap((id) => {
    const preset = SETUP_PROVIDERS[id];
    return keyed.has(preset.provider) ? [{ id, label: preset.label }] : [];
  });
}

// A complete, valid document on one provider's presets: every tier the contract
// declares, plus the embeddings binding without which the document is refused.
//
// The stored profile is kept when it is one: an operator who declared eu_hosted
// before binding anything must not be moved off it by a first click. A fresh
// installation stores an empty profile, which is no member of the enum, and
// then `cloud_frontier` — what both cloud presets need — is written instead.
export function firstBinding(
  id: keyof typeof SETUP_PROVIDERS,
  stored: string,
): Routing {
  const p = SETUP_PROVIDERS[id];
  const lane = {
    provider: p.provider,
    model: p.chatModel,
    ...(p.baseUrl ? { base_url: p.baseUrl } : {}),
  };
  return {
    profile: isProfile(stored) ? stored : "cloud_frontier",
    tiers: Object.fromEntries(TIER_ORDER.map((t) => [t, { ...lane }])),
    embeddings: {
      provider: p.provider,
      model: p.embedModel,
      ...(p.baseUrl ? { base_url: p.baseUrl } : {}),
    },
  };
}

// The day the price sheet was last written, which is the day its model list was
// last true: the NEWEST effective date across the sheet, since a sheet is
// re-priced row by row. Rendered as the wire's own ISO day — a calendar day
// rather than an instant, so a zone could shift it by one.
function sheetAsOf(catalogue: ModelCatalogue): string | null {
  return (catalogue ?? []).reduce<string | null>(
    (latest, rate) =>
      latest === null || rate.effective_date > latest
        ? rate.effective_date
        : latest,
    null,
  );
}

const PROFILES: readonly Routing["profile"][] = [
  "eu_hosted",
  "sovereign",
  "cloud_frontier",
];

function isProfile(value: string): value is Routing["profile"] {
  return PROFILES.some((p) => p === value);
}

// Where a new decision binding starts: the first decision adapter this
// installation can reach, so the editor does not open on one it would refuse.
function firstDecisionProvider(
  keys: Parameters<typeof reachableProviders>[1],
): string {
  return (
    reachableProviders(DECISION_PROVIDERS, keys, undefined)[0] ??
    DECISION_PROVIDERS[0]
  );
}
