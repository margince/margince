import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useCan, useCanWrite } from "../app/capability";
import { Button, EmptyState } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { Panel, PanelBody, PanelIntro } from "../design-system/panel";
import { stable } from "../format/collate";
import { formatDate, formatNumber } from "../format/format";
import { viewerZone } from "../format/timezone";
import { useLocale, useT } from "../i18n";
import { useAiStatus } from "./ai-admin";
import { BindingEditor } from "./ai-binding-editor";
import { TIER_ORDER, tierLabel, tierRank } from "./ai-decision-labels";
import { useAiHealth } from "./ai-health";
import {
  type ModelCatalogue,
  unkeyedProviders,
  useAiModelCatalogue,
  withBorrowedRows,
} from "./ai-models";
import { invalidateProviderHealth } from "./ai-provider-health";
import { useProviderKeys } from "./ai-provider-key-hooks";
import { providerName } from "./ai-provider-names";
import { reachableProviders } from "./ai-provider-reach";
import { DECISION_PROVIDERS } from "./ai-routing-fields";
import { type Lane, TiersTable } from "./ai-routing-lane";
import { ROUTING_KEY, type RoutingRead, useRouting } from "./ai-routing-query";
import { type SliceValue, sliceOf } from "./ai-routing-slice";
import { PanelTitle, TermLegend } from "./ai-terms";
import { problemMessageOf, QueryGate, throwProblem, useMe } from "./common";
import { SETUP_PROVIDERS } from "./setup-providers";
import "./ai-settings.css";

// Which provider and model each tier uses.
//
// The bindings are read on `ai_routing`, narrow on both verbs because this is
// the editable document and it decides where an installation's correspondence
// goes; the health column is read on `ai_diagnostics`, and a reader holding
// that alone still gets the lanes as rungs, unbound.
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

export function AiRoutingCard() {
  const t = useT();
  // The read grant gates the QUERY, not only the rows: asking without it draws
  // a 403 error box, which reads as a broken installation.
  const canSee = useCan("ai_routing", "read");
  const canWrite = useCanWrite("ai_routing", "update");
  const canReadBudget = useCan("ai_budget", "read");
  const canManage = canWrite && canReadBudget;
  const query = useRouting(canSee);

  if (!canSee) {
    return <HealthOnly />;
  }

  return (
    <QueryGate query={query} pendingLabel={t("aiRouting.title")}>
      {(read) => <ModelTiers read={read} canManage={canManage} />}
    </QueryGate>
  );
}

// A tier off the ladder still renders, last: it must appear rather than vanish
// from the only screen that binds it.
function orderedTiers(tiers: Routing["tiers"] | undefined): string[] {
  return Object.keys(tiers ?? {}).sort(
    (a, b) => tierRank(a) - tierRank(b) || stable(a, b),
  );
}

// A bound provider with no key fails every lane on it closed; said once here, not per row.
function UnkeyedNote({
  lanes,
  unkeyed,
}: Readonly<{ lanes: readonly Lane[]; unkeyed: ReadonlySet<string> | null }>) {
  const t = useT();
  const stranded = lanes.filter(
    (lane) => lane.binding && unkeyed?.has(lane.binding.provider),
  );
  if (stranded.length === 0) return null;
  const providers = [
    ...new Set(stranded.flatMap((lane) => lane.binding?.provider ?? [])),
  ];
  return (
    <Callout
      tone="warning"
      kind="standing"
      title={t("aiRouting.unkeyed.title")}
    >
      {t("aiRouting.unkeyed.body", {
        providers: providers.map((p) => providerName(p, t)).join(", "),
        lanes: stranded.map((lane) => tierLabel(lane.name, t)).join(", "),
      })}
    </Callout>
  );
}

// An open editor: the document it opened on, and where its fields start.
type Editing = { opened: RoutingRead; initial: SliceValue; label: string };

// A reader who may see how the lanes are doing and not how they are bound gets
// the health rows alone, so the page that opens for them still answers the
// question it opens for. With neither grant the card keeps its place and says
// so: an absent card would read as lanes that are fine.
function HealthOnly() {
  const t = useT();
  const { locale } = useLocale();
  const canDiagnose = useCan("ai_diagnostics", "read");
  const health = useAiHealth(canDiagnose);
  const me = useMe();
  return (
    <Panel title={<PanelTitle term="tier">{t("aiRouting.title")}</PanelTitle>}>
      {canDiagnose ? (
        <QueryGate query={health} pendingLabel={t("aiRouting.title")}>
          {(read) =>
            read.rungs.length === 0 ? (
              <PanelBody>
                <EmptyState>
                  {t("aiHealth.noCalls", {
                    hours: formatNumber(read.window_hours, locale),
                  })}
                </EmptyState>
              </PanelBody>
            ) : (
              <TiersTable
                lanes={[]}
                health={read}
                features={undefined}
                catalogue={undefined}
                canManage={false}
              />
            )
          }
        </QueryGate>
      ) : (
        <PanelBody>
          <QueryGate query={me} pendingLabel={t("aiRouting.title")}>
            {() => <EmptyState>{t("aiRouting.withheld")}</EmptyState>}
          </QueryGate>
        </PanelBody>
      )}
    </Panel>
  );
}

function ModelTiers({
  read,
  canManage,
}: Readonly<{
  read: RoutingRead;
  canManage: boolean;
}>) {
  const t = useT();
  const { routing } = read;
  // One read for every row. The hook answers an empty list rather than
  // throwing when the sheet's own grant is withheld.
  const catalogue = useAiModelCatalogue();
  // Which vendors hold a credential, joined into the rows and the editor. Same
  // grant as this card, so no second denial to answer.
  const keys = useProviderKeys(true);
  const sheet = withBorrowedRows(catalogue.data, keys.data?.providers);
  const canDiagnose = useCan("ai_diagnostics", "read");
  const canBudget = useCan("ai_budget", "read");
  const health = useAiHealth(canDiagnose).data;
  const features = useAiStatus(canDiagnose && canBudget).data?.features;
  const [editing, setEditing] = useState<Editing | null>(null);
  const unkeyed = unkeyedProviders(keys.data?.providers);
  const open = (initial: SliceValue, lane: string) =>
    setEditing({ opened: read, initial, label: tierLabel(lane, t) });

  // Defensive on a field the contract marks required: a client that dies on an
  // unexpected shape takes the whole settings page with it.
  const tiers = orderedTiers(routing.tiers);
  if (tiers.length === 0) {
    return (
      <Panel
        title={<PanelTitle term="tier">{t("aiRouting.title")}</PanelTitle>}
      >
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

  // One row per bound lane. The embedder binds SEPARATELY on purpose: retrieval
  // has to survive a chat-budget exhaustion, and its model is a different one
  // even on the same vendor.
  const editDecisions = () =>
    open(
      {
        kind: "decisions",
        binding: routing.decisions ?? {
          provider: firstDecisionProvider(keys.data?.providers),
          model: "",
        },
      },
      "decisions",
    );
  const lanes: Lane[] = [
    ...tiers.map((tier) => ({
      name: tier,
      lane: "chat" as const,
      binding: routing.tiers[tier],
      onEdit: () => open(sliceOf(routing, { kind: "tier", tier }), tier),
    })),
    {
      name: "embeddings",
      lane: "embeddings" as const,
      binding: routing.embeddings,
      testId: "ai-routing-embeddings",
      onEdit: () =>
        open(sliceOf(routing, { kind: "embeddings" }), "embeddings"),
    },
    ...(routing.decisions
      ? [
          {
            name: "decisions",
            lane: "decisions" as const,
            binding: routing.decisions,
            testId: "ai-routing-decisions",
            onEdit: editDecisions,
          },
        ]
      : []),
  ];

  return (
    <Panel
      title={<PanelTitle term="tier">{t("aiRouting.title")}</PanelTitle>}
      footer={<SheetFooter catalogue={catalogue.data} />}
    >
      <PanelBody>
        <PanelIntro>{t("aiRouting.intro")}</PanelIntro>
        <TermLegend />
        <p className="t-sub" data-testid="ai-routing-profile">
          {t("aiRouting.profileLine", { profile: routing.profile })}
        </p>
        <UnkeyedNote lanes={lanes} unkeyed={unkeyed} />
      </PanelBody>
      <TiersTable
        lanes={lanes}
        health={health}
        features={features}
        catalogue={sheet}
        canManage={canManage}
        onAddDecisions={routing.decisions ? undefined : editDecisions}
      />
      {editing && (
        <BindingEditor
          opened={editing.opened}
          initial={editing.initial}
          label={editing.label}
          keys={keys.data?.providers}
          catalogue={sheet}
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
function SheetFooter({ catalogue }: Readonly<{ catalogue: ModelCatalogue }>) {
  const t = useT();
  const { locale } = useLocale();
  const asOf = sheetAsOf(catalogue);
  return (
    <div className="ai-sheet-age">
      <span className="t-caption">
        {asOf
          ? t("aiRouting.sheetAsOf", {
              date: formatDate(asOf, locale, viewerZone()),
            })
          : t("aiRouting.sheetUnknown")}
      </span>
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
      await invalidateProviderHealth(queryClient);
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
// then the preset's own profile is written instead.
export function firstBinding(
  id: keyof typeof SETUP_PROVIDERS,
  stored: string,
): Routing {
  const p = SETUP_PROVIDERS[id];
  const host = p.baseUrl ? { base_url: p.baseUrl } : {};
  const lane = {
    provider: p.provider,
    model: p.chatModel,
    ...host,
    ...(p.location ? { location: p.location } : {}),
  };
  return {
    profile: isProfile(stored) ? stored : p.profile,
    tiers: Object.fromEntries(TIER_ORDER.map((t) => [t, { ...lane }])),
    embeddings: {
      provider: p.provider,
      model: p.embedModel,
      ...host,
      ...(p.embedLocation ? { location: p.embedLocation } : {}),
    },
  };
}

// The day the price sheet was last written, which is the day its model list was
// last true. A sheet is re-priced row by row, so that day is its newest
// effective date.
function sheetAsOf(catalogue: ModelCatalogue): string | null {
  return (catalogue ?? []).reduce<string | null>(
    (latest, rate) =>
      latest === null || rate.effective_date > latest
        ? rate.effective_date
        : latest,
    null,
  );
}

// A declared mirror of the server's profiles, held both ways by
// backend/gates/frontendproviders_test.go, which reads this `[…] as const` form.
const PROFILES = ["eu_hosted", "sovereign", "cloud_frontier"] as const;

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
