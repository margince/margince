import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { type ReactNode, useEffect, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useCan, useCanWrite } from "../app/capability";
import {
  Badge,
  Button,
  EmptyState,
  Field,
  TextInput,
} from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { ComboBox } from "../design-system/combobox";
import { Panel, PanelBody, PanelRow } from "../design-system/panel";
import { Select } from "../design-system/select";
import { SettingList, SettingRow } from "../design-system/settingrow";
import { stable } from "../format/collate";
import { formatUsdPerMTok } from "../format/format";
import { type Locale, useLocale, useT } from "../i18n";
import type { AvailableModels } from "./ai-models";
import {
  type ModelCatalogue,
  type ModelLane,
  offeredModels,
  unreadablePrice,
  useAiModelCatalogue,
  useAvailableModels,
} from "./ai-models";
import { useProviderKeys } from "./ai-provider-keys";
import { problemMessageOf, QueryGate, throwProblem } from "./common";
import { RefreshFromSources } from "./rate-refresh";
import "./ai-settings.css";

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

// The key the embedding lane is opened under. Not a tier name, and it cannot
// collide with one: the tier vocabulary is the task contract's and this is the
// one lane that is not in it.
const EMBEDDINGS_LANE = "\u0000embeddings";

export function useRouting(enabled: boolean) {
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

export function AiRoutingCard({
  onDirtyChange,
  onPriceSheet,
}: Readonly<{
  // The page above owns the strip a reader leaves this card by, and the draft
  // below is a document rather than a field that saves itself. The app's own
  // unsaved guard watches addresses and every tab on this page shares one, so
  // the card says whether it is holding work and the page decides what to do
  // about a move. Optional: the card is composed on its own in a story and in
  // the tests, where there is nothing to tell.
  onDirtyChange?: (dirty: boolean) => void;
  // Where the prices behind these bindings are read. A link rather than a
  // second copy of the sheet: it is one table, and the lane rows only need to
  // say which model each binds.
  onPriceSheet?: () => void;
}>) {
  const t = useT();
  // The read grant gates the QUERY, not only the form. This tab opens on other
  // grants, so a seat can reach the page without `ai_routing:read` — and asking
  // anyway drew a 403 error box, which reads as a broken installation rather
  // than as a permission. Withheld is the answer every card on this page gives.
  const canSee = useCan("ai_routing", "read");
  const canManage = useCanWrite("ai_routing", "update");
  const query = useRouting(canSee);

  if (!canSee) {
    return (
      <Panel title={t("aiRouting.title")} sub={t("aiRouting.sub")}>
        <PanelBody>
          <EmptyState>
            <p className="t-small">{t("aiRouting.withheld")}</p>
          </EmptyState>
        </PanelBody>
      </Panel>
    );
  }

  return (
    <QueryGate query={query}>
      {(routing) => (
        <RoutingForm
          routing={routing}
          canManage={canManage}
          onDirtyChange={onDirtyChange}
          onPriceSheet={onPriceSheet}
        />
      )}
    </QueryGate>
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
  onDirtyChange,
  onPriceSheet,
}: Readonly<{
  routing: Routing;
  canManage: boolean;
  onDirtyChange?: (dirty: boolean) => void;
  onPriceSheet?: () => void;
}>) {
  const t = useT();
  const replace = useReplaceRouting();
  // One read for every row on the card. No grant check in front of it: this
  // form only renders for a reader who already holds `ai_routing:read`, and the
  // hook answers an empty list rather than throwing when the sheet's own grant
  // is withheld — a field with nothing to suggest, which is what it was before.
  const catalogue = useAiModelCatalogue();
  // Which vendors hold a credential, joined into the rows below. The lanes and
  // the keys used to be two cards on one scroll, so a lane pointing at an
  // unkeyed vendor was visible by reading both; behind a tab strip it would not
  // be visible at all, and a lane that fails closed at call time has to say so
  // where it is bound. Same grant as this card, so no second denial to answer.
  const keys = useProviderKeys(true);
  // The stored document is the starting point, and the reader's edits live
  // here until they save. Keyed on the routing version so a binding another
  // role changed replaces an untouched form rather than being overwritten by
  // it — see the key at the call site.
  const [draft, setDraft] = useState<Routing>(routing);
  // The document this draft was seeded FROM, which is what "unsaved" is measured
  // against. Comparing to the live `routing` instead would call a form dirty the
  // moment somebody else saved, having changed nothing here.
  const [seeded, setSeeded] = useState(() => routingIdentity(routing));
  // Which lane is open for editing. One at a time: a lane row is a reading,
  // and every row expanded at once is the form this card used to be.
  const [editing, setEditing] = useState<string | null>(null);

  const dirty = JSON.stringify(draft) !== seeded;
  useEffect(() => onDirtyChange?.(dirty), [dirty, onDirtyChange]);

  // A binding another role changed re-seeds an UNTOUCHED form, and never
  // replaces one somebody is working in.
  //
  // Keying the whole form on the document did the first AND the second: it
  // remounted, so a reader mid-edit lost what they had typed with nothing on
  // screen saying why. Their work stands instead.
  const current = routingIdentity(routing);
  useEffect(() => {
    if (current !== seeded && !dirty) {
      setDraft(routing);
      setSeeded(current);
    }
  }, [current, seeded, dirty, routing]);

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
  const busy = !canManage || replace.isPending;
  const unkeyed = unkeyedProviders(keys.data?.providers);

  return (
    <>
      {/* The constraint FIRST, on its own card: it decides which vendors the
          lanes under it may name, so a reader meets the rule before the
          bindings it governs.

          No panel title. The card holds ONE decision and the row already names
          it, so a header band would print the same words twice, one above the
          other — which is what it did. */}
      <Panel>
        <PanelBody>
          <SettingList>
            <SettingRow
              label={t("aiRouting.profile.card")}
              description={t("aiRouting.profile.help")}
              control={(control) => (
                <Select
                  {...control}
                  className="settingrow-measure"
                  value={draft.profile}
                  disabled={busy}
                  options={PROFILES.map((p) => ({
                    value: p,
                    label: t(`aiRouting.profile.${p}`),
                  }))}
                  onChange={(value) =>
                    setDraft((d) => ({
                      ...d,
                      profile: value as Routing["profile"],
                    }))
                  }
                />
              )}
            />
          </SettingList>
        </PanelBody>
      </Panel>

      <Panel
        title={t("aiRouting.lanes.title")}
        sub={t("aiRouting.lanes.sub")}
        titleAction={
          onPriceSheet ? (
            <button
              type="button"
              className="link-button"
              onClick={onPriceSheet}
            >
              {t("aiRouting.priceSheet")}
            </button>
          ) : undefined
        }
        footer={
          // What the model lists below are, and how to move them on.
          //
          // The picker offers what the price sheet holds, and the sheet is a
          // SNAPSHOT somebody took on a day — it does not follow a vendor's
          // releases. Undated, it reads as "these are the models", and a reader
          // looking for something announced last month concludes the product
          // cannot reach it. The date says what it actually is, and the refresh
          // is the way past it, here rather than only on a tab the reader is not
          // on when the question occurs to them.
          <div className="ai-sheet-age">
            <span className="t-caption">
              {sheetAsOf(catalogue.data)
                ? t("aiRouting.sheetAsOf", {
                    date: sheetAsOf(catalogue.data) ?? "",
                  })
                : t("aiRouting.sheetUnknown")}
            </span>
            {canManage && (
              <RefreshFromSources path="/ai-model-rates/propose-refresh" />
            )}
          </div>
        }
      >
        {tiers.map((tier) => (
          <LaneRow
            key={tier}
            lane="chat"
            name={tier}
            binding={draft.tiers[tier]}
            catalogue={catalogue.data}
            unkeyed={unkeyed}
            disabled={busy}
            open={editing === tier}
            onOpen={() => setEditing(editing === tier ? null : tier)}
            onChange={(next) => setTier(tier, next)}
          />
        ))}
        {/* The embed lane, which the form used to leave out entirely — so a
            reader could re-point every chat tier and still be sending their
            retrieval to the vendor they had just moved away from, with nothing
            on screen saying so. It binds SEPARATELY on purpose: retrieval has to
            survive a chat-budget exhaustion, and the model is a different one
            even on the same vendor.

            Its name is the document's own word, raw, exactly as the tier names
            above it are: the name column is one vocabulary read down one edge,
            and a translated word among five identifiers reads as a different
            KIND of thing rather than as the same thing in the reader's
            language. */}
        <LaneRow
          lane="embeddings"
          name="embeddings"
          testId="ai-routing-embeddings"
          binding={draft.embeddings}
          catalogue={catalogue.data}
          unkeyed={unkeyed}
          disabled={busy}
          open={editing === EMBEDDINGS_LANE}
          onOpen={() =>
            setEditing(editing === EMBEDDINGS_LANE ? null : EMBEDDINGS_LANE)
          }
          onChange={(embeddings) => setDraft((d) => ({ ...d, embeddings }))}
          extra={
            <EmbeddingWidthField
              binding={draft.embeddings}
              disabled={busy}
              onChange={(embeddings) => setDraft((d) => ({ ...d, embeddings }))}
            />
          }
        />
      </Panel>

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
      <div className="card-actions">
        <Button
          onClick={() => replace.mutate(draft)}
          pending={replace.isPending}
          busyLabel={t("aiRouting.saving")}
          reason={canManage ? undefined : t("aiRouting.adminOnly")}
        >
          {t("aiRouting.save")}
        </Button>
        <p className="t-caption">{t("aiRouting.effect")}</p>
      </div>
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
  laneName,
  binding,
  catalogue,
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

// One lane, read as a row and edited in place.
//
// The row is the READING — which lane, which vendor, which model, and what is
// wrong with that pairing — and the fields open under it only when a reader asks
// to change one. The card was six lanes' worth of fields opened at once, which is
// three screens of controls to answer the question "where does premium go".
//
// The two pills are the whole reason a reader can be shown a binding without also
// being shown the key card and the price sheet. Both are joins, and both stay
// silent rather than guessing: a key list that has not arrived claims nothing, and
// an empty price sheet means the reader cannot read it rather than that nothing on
// this installation is priced.
function LaneRow<
  B extends { provider: string; model: string; base_url?: string },
>({
  name,
  lane,
  binding,
  catalogue,
  unkeyed,
  disabled,
  open,
  onOpen,
  onChange,
  extra,
  testId,
}: Readonly<{
  name: string;
  lane: ModelLane;
  binding: B;
  catalogue: ModelCatalogue;
  unkeyed: ReadonlySet<string> | null;
  disabled: boolean;
  open: boolean;
  onOpen: () => void;
  onChange: (next: B) => void;
  // A control this lane has and the others do not — the embedding width. It
  // rides in the opened body rather than in `AdapterFields`, which asks the one
  // question both lanes answer.
  extra?: ReactNode;
  testId?: string;
}>) {
  const t = useT();
  const { locale } = useLocale();
  return (
    <PanelRow>
      {/* The lane's addressable region: the summary line AND the fields it
          opens, so "the premium lane" names one thing whether it is folded or
          not. Inside the row rather than on it, because `PanelRow` owns the
          row's own geometry and takes no attributes of its own. */}
      <div data-testid={testId ?? `ai-routing-tier-${name}`}>
        <div className="ai-lane">
          {/* The lane's own id, and under it what the lane is FOR.
              The id stays because it is the routing document's vocabulary and
              what an operator greps for; the gloss is there because `premium`
              and `frontier` do not say which is dearer, and `local_small` says
              nothing at all to somebody meeting this page for the first time.
              A lane this build does not know gets no gloss rather than an
              invented one. */}
          <span className="ai-lane-name">
            <span className="ai-lane-id t-mono">{name}</span>
            {laneGloss(name, t) && (
              <span className="ai-lane-gloss">{laneGloss(name, t)}</span>
            )}
          </span>
          {/* The binding itself, as ONE flex item. Grouped rather than laid
              out beside the name as five siblings, because a row that wraps
              wraps at whatever item runs out of room — which put the Change
              button alone on a second line while the model beside it still had
              space. Wrapping now happens INSIDE this group, and the two things
              that anchor the row keep their edges. */}
          <span className="ai-lane-binding">
            {/* Filled, not `quiet`. Quiet draws a status DOT, and the vendor is
                not a status — the pills that follow it are, and a dot in front
                of the vendor would put three status marks on a row carrying one
                fact and two warnings. */}
            <Badge>{binding.provider}</Badge>
            <span className="ai-lane-model t-mono">{binding.model}</span>
            {/* WHERE the OpenAI-wire adapter is pointed. It is not a detail of
                the binding, it IS the vendor: `openai_compatible` names a
                protocol, and every broker on it — OpenRouter, Together, a
                self-hosted gateway — reads identically on this row without the
                host. Only this adapter has one, so nothing else grows it. */}
            {binding.base_url ? (
              <span className="ai-lane-host t-mono">
                {hostOf(binding.base_url)}
              </span>
            ) : null}
            {unkeyed?.has(binding.provider) && (
              <Badge tone="warn">{t("aiRouting.noKey")}</Badge>
            )}
            {isUnpriced(catalogue, binding.provider, binding.model, lane) ? (
              <Badge tone="warn">{t("aiRouting.unpriced")}</Badge>
            ) : (
              // What this lane costs to call, where the sheet can say. It is
              // the reason the ladder is ordered the way it is, and reading it
              // used to mean leaving for the price table and finding this model
              // in a list of two hundred.
              <span className="ai-lane-price">
                {priceLabel(
                  catalogue,
                  binding.provider,
                  binding.model,
                  lane,
                  locale,
                )}
              </span>
            )}
          </span>
          {/* Never refused, even to a reader who may not save. The row is a
            summary and the body under it is the rest of the binding — the host
            an OpenAI-wire vendor is reached at, the width the embedder asks for
            — so refusing to OPEN it would hide facts from somebody whose job is
            to report them. The fields inside carry the refusal, and so does
            Save. */}
          <span className="ai-lane-open">
            <Button onClick={onOpen} aria-expanded={open}>
              {open ? t("aiRouting.done") : t("aiRouting.change")}
            </Button>
          </span>
        </div>
        {open && (
          <div className="form-row">
            <AdapterFields
              label={t("aiRouting.provider.label")}
              lane={lane}
              laneName={name}
              binding={binding}
              catalogue={catalogue}
              disabled={disabled}
              onChange={onChange}
            />
            {extra}
          </div>
        )}
      </div>
    </PanelRow>
  );
}

// The width this lane asks the provider for, which only it has. Blank means the
// compiled default rather than zero: the contract reads an omitted value and a 0
// the same way, so an empty box must send neither a 0 nor a NaN.
function EmbeddingWidthField({
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

// The vendors this installation names but holds no credential for — or null
// while nobody knows.
//
// Null is not "none". A list that has not arrived, or one a reader may not have,
// must not draw a row as keyed: a lane that fails closed at call time and reads
// as fine here is the exact thing the pill exists to prevent.
function unkeyedProviders(
  providers: readonly { provider: string; configured: boolean }[] | undefined,
): ReadonlySet<string> | null {
  if (!providers) {
    return null;
  }
  return new Set(providers.filter((p) => !p.configured).map((p) => p.provider));
}

// Whether the price sheet can cost a call on this binding.
//
// An EMPTY sheet answers no. The reader who cannot read `ai_model_rate` gets an
// empty list from the catalogue hook by design, and marking every lane unpriced
// on the strength of that would report a fault in the installation where the
// truth is only that the sheet is not theirs.
//
// A row that EXISTS but carries a price nothing can parse counts as unpriced
// too. It is the same fact to a reader — this call cannot be costed — and
// treating it as priced left the row showing neither a figure nor the pill,
// which says nothing at all.
function isUnpriced(
  catalogue: ModelCatalogue,
  provider: string,
  model: string,
  lane: ModelLane,
): boolean {
  if (!catalogue || catalogue.length === 0) {
    return false;
  }
  const rate = catalogue.find(
    (r) => r.provider === provider && r.model_id === model && r.lane === lane,
  );
  if (!rate) {
    return true;
  }
  if (unreadablePrice(rate.input_per_mtok)) {
    return true;
  }
  // An embedding lane has no output, so a blank there is the sheet being right
  // rather than unreadable.
  return lane !== "embeddings" && unreadablePrice(rate.output_per_mtok);
}

// The host part of a base URL, for a row that has room for the address but not
// for the whole endpoint. Falls back to the string as given: a value an
// operator typed that does not parse is still what this lane is pointed at, and
// hiding it would leave the row claiming a vendor with no address at all.
function hostOf(baseUrl: string): string {
  try {
    return new URL(baseUrl).host;
  } catch {
    return baseUrl;
  }
}

// What each lane in the ladder is FOR, in words rather than in its id.
//
// An explicit switch rather than a key built from the tier name: the message
// catalog's type is a closed union, and a runtime-composed key would compile as
// any old string and ship a typo. It also means a tier the task contract grows
// later renders with no gloss — correct, because nobody has written one, and a
// missing sentence is better than a guessed one.
function laneGloss(name: string, t: ReturnType<typeof useT>): string | null {
  switch (name) {
    case "local_small":
      return t("aiRouting.lane.local_small");
    case "cheap_cloud":
      return t("aiRouting.lane.cheap_cloud");
    case "premium":
      return t("aiRouting.lane.premium");
    case "frontier":
      return t("aiRouting.lane.frontier");
    case "local_large":
      return t("aiRouting.lane.local_large");
    case "embeddings":
      return t("aiRouting.lane.embeddings");
    default:
      return null;
  }
}

// This binding's price, short enough to sit on the row: what goes in, what comes
// out, per million tokens. Empty where the sheet cannot say — the `unpriced`
// pill is what a reader sees instead, and printing a zero here would be the one
// thing this product is careful never to say by accident.
function priceLabel(
  catalogue: ModelCatalogue,
  provider: string,
  model: string,
  lane: ModelLane,
  locale: Locale,
): string {
  const rate = (catalogue ?? []).find(
    (r) => r.provider === provider && r.model_id === model && r.lane === lane,
  );
  if (!rate) {
    return "";
  }
  // A row the sheet cannot state a price for prints NOTHING rather than
  // reaching the formatter. `formatUsdPerMTok` hands the parsed number to
  // `Intl.NumberFormat`'s `minimumFractionDigits`, and NaN there throws a
  // RangeError — during render, on a card the whole settings page is composed
  // from. The picker's own hint guards the same way for the same reason.
  //
  // The output side is only asked about where it MEANS something: an embedding
  // lane has no output, so its price is a single figure and a blank second
  // column there is the sheet being right rather than unreadable.
  if (unreadablePrice(rate.input_per_mtok)) {
    return "";
  }
  const input = formatUsdPerMTok(rate.input_per_mtok, locale);
  if (lane === "embeddings") {
    return input;
  }
  if (unreadablePrice(rate.output_per_mtok)) {
    return "";
  }
  return `${input} → ${formatUsdPerMTok(rate.output_per_mtok, locale)}`;
}

// The day the price sheet was last written, which is the day its model list was
// last true.
//
// The NEWEST effective date across the sheet, not the oldest: a sheet is
// re-priced row by row, so the freshest row is the last time anybody looked. A
// sheet the reader cannot read answers null, and the caller says so rather than
// printing a date it does not have.
//
// Rendered as the wire's own ISO day, like every other effective date in this
// product (the price sheet's own column does the same). It is a CALENDAR day
// rather than an instant, so putting it through a zone could shift it by one —
// and this is an operator reading a date they will compare against the sheet
// beside it, not prose.
function sheetAsOf(catalogue: ModelCatalogue): string | null {
  return (catalogue ?? []).reduce<string | null>(
    (latest, rate) =>
      latest === null || rate.effective_date > latest
        ? rate.effective_date
        : latest,
    null,
  );
}

// Why the list a reader is looking at came only from the price sheet.
//
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

// The identity of the stored routing document, for the draft that starts from
// it. Every binding it holds, so any change by another role produces a
// different one and the form re-seeds from what is now stored.
//
// The document's own content rather than a version field: the wire carries no
// version on this shape, and a key that cannot see a change is a key that does
// not do its job.
function routingIdentity(routing: Routing): string {
  return JSON.stringify(routing);
}
