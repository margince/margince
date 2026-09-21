import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowUpRight, CheckSquare, FileText } from "lucide-react";
import { type ReactNode, useEffect, useId, useRef, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { ifMatch, requireVersion } from "../api/version";
import { useCanWrite, useRecordWriteRefusal } from "../app/capability";
import { PageAsideToggle, usePageAside } from "../app/pageaside";
import { useRecordZone } from "../app/recordzone";
import { scrollPageToTop } from "../app/reveal";
import { navigate, useRoute } from "../app/router";
import { useUrlParams } from "../app/urlstate";
import {
  Badge,
  Button,
  Disclosure,
  Field,
  OverflowMenu,
  Textarea,
  TextInput,
} from "../design-system/atoms";
import { ConfirmModal } from "../design-system/confirmmodal";
import { OpenEmailDrawer } from "../design-system/openemaildrawer";
import { Panel, PanelBody } from "../design-system/panel";
import { RecordTabs } from "../design-system/recordtabs";
import {
  type RecordTimeline,
  useRecordTimeline,
} from "../design-system/recordtimeline";
import { RecordView } from "../design-system/recordview";
import { useToast } from "../design-system/toast";
import {
  formatDateAbbrev,
  formatDecimal,
  formatNumber,
} from "../format/format";
import { leadIdentityName } from "../format/leadname";
import { viewerZone } from "../format/timezone";
import { useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { useClaimRecord } from "./claimrecord";
import { problemMessageOf, QueryGate, throwProblem, useMe } from "./common";
import type { CreateField } from "./create";
import { EntityRef, useEntityName } from "./entityref";
import { useRecordHistory } from "./history";
import { leadBand } from "./leadband";
import { LeadBrief } from "./leadbrief";
import { LeadFacts, LeadPulse, LeadSubtitle } from "./leadheader";
import { LeadHistoryTab } from "./leadhistory";
import { MergedLeadPanel } from "./leadmerged";
import {
  leadStatusLabel,
  promoteEligible,
  scoreFactorLabel,
  scoreTone,
} from "./leadpresentation";
import { LeadDealsProjectsTab, LeadRail } from "./leadraillinks";
import { LeadReadings } from "./leadreadings";
import { ACTION_PARAM, CALL_ACTION } from "./leads.address";
import { DisqualifyDialog } from "./leads.disqualify";
import { QualifyDialog } from "./leads.qualify";
import { LeadStepper } from "./leads.stepper";
import { LeadManualSignals } from "./leadsignals";
import { leadStanding } from "./leadstanding";
import { leadTodoRows } from "./leadtoday";
import { LogActivityAction } from "./logactivity";
import { useOpenEmail } from "./openemail";
import { RecordReading, TimelineThread, TodayPanel } from "./record360";
import { RecordCustomFields } from "./recordcustomfields";
import { saveRecordEdit } from "./recordedit";
import { RecordEmailVerb } from "./recordemail";
import { RecordFields } from "./recordfields";
import { searchProjectReferences, useRecordOwners } from "./recordreferences";
import { ShareAction } from "./share";
import "./leads.css";

// Leads (B-EP09.10a/b): visually SEGREGATED from the contact graph — the
// lead surface is accent-tinted, lead detail is its own screen (never
// contact.html — gap §3.5), and promote is eligibility-gated. Lead score is
// lead-local; the ≥60 / 40–59 / <40 colour thresholds are pinned by test.
// Search/filter/sort/pagination (P-14), the rich create modal (P-15), the
// If-Match edit form (P-1), and the dedupe view-existing link (P-16) are
// wired in here the same way as contacts (contacts.tsx) — the Promote button
// and score/status/company badges on the lead 360 stay exactly as they
// were. Status-change and score-override are Phase 4, not surfaced here.

type Lead = components["schemas"]["Lead"];
type UpdateLeadRequest = components["schemas"]["UpdateLeadRequest"];

import { sourcePickOptions, useLeadSources } from "./leadsources";

export { promoteEligible, scoreTone } from "./leadpresentation";
export { terminalBadge } from "./leadstanding";

import { leadKey, leadScoreKey, leadWriteKeys } from "./leadkeys";

export { LeadsScreen } from "./leads.list";

// The recorded trigger, as the outcome card names it — the wire token never
// reaches the reader.
function promotionTriggerLabel(
  trigger: string | null | undefined,
): MessageKey | null {
  switch (trigger) {
    case "inbound_reply":
      return "lead.trigger.inboundReply";
    case "meeting_booked":
      return "lead.trigger.meetingBooked";
    case "meeting_held":
      return "lead.trigger.meetingHeld";
    case "human_qualify":
      return "lead.trigger.humanQualify";
    default:
      return null;
  }
}

function stringField(value: unknown): string {
  return typeof value === "string" ? value : "";
}

// Status and score use their domain actions; Details sends only submitted identity fields.
export function mapLeadUpdate(
  values: Record<string, unknown>,
): UpdateLeadRequest {
  const field = (key: string) =>
    Object.hasOwn(values, key)
      ? stringField(values[key]).trim() || null
      : undefined;
  return {
    full_name: field("full_name"),
    email: field("email"),
    title: field("title"),
    company_name: field("company_name"),
    source: stringField(values.source).trim() || undefined,
    candidate_company_key: field("candidate_company_key"),
    project_id: field("project_id"),
    owner_id: stringField(values.owner_id).trim() || undefined,
  };
}

const leadEditFields: CreateField[] = [
  { key: "full_name", label: "create.fullName", required: true },
  { key: "email", label: "create.email", type: "email" },
  { key: "title", label: "create.contactTitle" },
  { key: "company_name", label: "create.companyName" },
];

// The decision-maker title pattern the score uses (formulas §3.1). Mirrored
// here ONLY to say why a title earned nothing; the score itself is computed
// server-side and this never adds to it.
const DECISION_MAKER_TITLE =
  /(chief|vp|head|director|founder|owner|c[a-z]o)\b/i;
const HIGH_INTENT_SOURCES = new Set(["inbound", "webform", "referral"]);
// The server PENALISES these five points rather than merely granting nothing
// (leadscore.go). Calling that "no buying intent on its own" would soften a
// subtraction into a neutral, which is a different and kinder claim than the
// model made.
const LOW_INTENT_SOURCES = new Set(["import", "crawl"]);

// What a lead is missing, in the model's own terms — shown when no retained
// decomposition exists yet.
//
// A zero score and an unscored lead look identical as a number and mean
// opposite things: "we assessed this and it earns nothing" versus "nothing
// has been assessed". A rep reads both as a bad prospect, and only one of
// them is (ADR-0108 §4). These reasons are always derivable, so the page
// states them rather than explaining our own storage history.
function ScoreShortfall({ lead }: Readonly<{ lead: Lead }>) {
  const t = useT();
  const missing: string[] = [];
  if (!lead.title) {
    missing.push(t("lead.shortfall.noTitle"));
  } else if (!DECISION_MAKER_TITLE.test(lead.title)) {
    missing.push(t("lead.shortfall.titleNotSenior", { title: lead.title }));
  }
  if (!lead.source) {
    // Split exactly as the title pair above is: interpolating an absent value
    // would print "Came in as undefined" at a rep.
    missing.push(t("lead.shortfall.noSource"));
  } else if (LOW_INTENT_SOURCES.has(lead.source)) {
    missing.push(t("lead.shortfall.sourcePenalised", { source: lead.source }));
  } else if (!HIGH_INTENT_SOURCES.has(lead.source)) {
    missing.push(t("lead.shortfall.sourceNoIntent", { source: lead.source }));
  }
  // Deliberately NOT a claim that no reply or meeting exists. Engagement lives
  // in linked activities the client never reads, and a decayed reply can round
  // to nothing while still being a reply — "no reply yet" would be a statement
  // about the prospect this page cannot support. What it CAN say is what
  // would move the score, which is the actionable half anyway.
  missing.push(t("lead.shortfall.engagementMoves"));

  return (
    <div className="lead-stack-tight">
      <span>{t("lead.shortfall.lead")}</span>
      <ul className="lead-plainlist">
        {missing.map((reason) => (
          <li key={reason}>{reason}</li>
        ))}
      </ul>
    </div>
  );
}

function ScoreBreakdown({ id, lead }: Readonly<{ id: string; lead: Lead }>) {
  const t = useT();
  const { locale } = useLocale();
  const explain = useQuery({
    queryKey: leadScoreKey(id),
    queryFn: async () => {
      const { data, error } = await api.GET("/leads/{id}/score", {
        params: { path: { id } },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });

  if (explain.isPending) {
    return <span>{t("lead.scoreLoading")}</span>;
  }
  if (explain.isError) {
    return <span>{problemMessageOf(explain.error, t)}</span>;
  }
  const current = explain.data?.current;
  if (!explain.data?.explained || !current) {
    // No retained decomposition. For a score of ZERO the reasons are still
    // derivable from the lead in hand, and they are what the reader came for
    // — "this score predates the breakdown" answers a question nobody asked
    // and leaves a 0 looking like a bad prospect rather than an unassessed
    // one (ADR-0108 §4).
    //
    // A NON-zero score is a different case: something did count, this client
    // cannot say what, and listing what is missing would state the opposite
    // of the truth. It says only that the breakdown is not stored yet.
    return lead.score === 0 ? (
      <ScoreShortfall lead={lead} />
    ) : (
      <span>{t("lead.scoreNotStoredYet")}</span>
    );
  }
  const factors = current.factors ?? [];
  // Under a Commercial Judgement override the displayed score is the
  // human's and these factors sum to the machine's, so the reader is told
  // which number they are looking at rather than left to assume.
  const overridden = current.override_reason != null;

  return (
    <div className="lead-stack-tight">
      {overridden && (
        <span>
          {t("lead.scoreFactorsExplainMachine", {
            score: formatNumber(current.score_computed, locale),
          })}
        </span>
      )}
      {factors.length === 0 ? (
        <span>{t("lead.scoreNoFactors")}</span>
      ) : (
        <ul className="lead-plainlist">
          {factors.map((factor) => (
            <li key={factor.factor} className="lead-factor">
              <span>{scoreFactorLabel(factor.factor, t)}</span>
              <span className="t-num">
                {formatDecimal(factor.points, locale, 1)}
              </span>
              {factor.base_points != null && (
                // The decay as arithmetic a reader can check: 25 halving
                // every 14 days is why this row reads 12.5 today.
                <span className="t-caption t-num">
                  {t("lead.scoreDecayed", {
                    base: formatNumber(factor.base_points, locale),
                  })}
                </span>
              )}
              {factor.source_activity_ids != null &&
                factor.source_activity_ids.length > 0 && (
                  // How many records fed the factor. The ids themselves are
                  // already filtered to what this reader may open, so the
                  // count never claims more than they can see.
                  <span className="t-caption">
                    {t("lead.scoreSources", {
                      count: formatNumber(
                        factor.source_activity_ids.length,
                        locale,
                      ),
                    })}
                  </span>
                )}
            </li>
          ))}
        </ul>
      )}
      <span className="t-caption t-num">
        {t("lead.scoreReconciles", {
          raw: formatDecimal(current.raw_sum, locale, 2),
          rounded: formatNumber(current.rounded_sum, locale),
          score: formatNumber(current.score_computed, locale),
        })}
      </span>
    </div>
  );
}

// Phase 4 lifecycle controls (P-10/11/12): status (new↔working only —
// promoted/disqualified are terminal and stay badge-only), the score
// explain/override panel (the read carries no per-factor breakdown, so
// "explain" here is honestly just the override-vs-machine story), and
// ownership — the owner's name plus reassignment to any workspace user.
// All three share one PATCH /leads/{id} + If-Match(lead.version) mutation.
// The score block: its explanation, and the Commercial Judgement override.
// Extracted from LeadLifecycle because guarding every terminal write pushed
// that render past the complexity budget — and because the score's own
// controls are a thing in themselves.
function LeadScorePanel({
  lead,
  id,
  terminalReasonId,
  overriding,
  setOverriding,
  scoreValue,
  setScoreValue,
  reasonValue,
  setReasonValue,
  writer,
}: Readonly<{
  lead: Lead;
  id: string;
  terminalReasonId: string;
  overriding: boolean;
  setOverriding: (next: boolean) => void;
  scoreValue: string;
  setScoreValue: (next: string) => void;
  reasonValue: string;
  setReasonValue: (next: string) => void;
  writer: LeadWriter;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const { readOnly } = writer;
  const reasonBlank = reasonValue.trim() === "";
  const scoreBlank = scoreValue.trim() === "";
  const parsedScore = Number(scoreValue);
  const scoreInvalid =
    scoreBlank ||
    !Number.isInteger(parsedScore) ||
    parsedScore < 0 ||
    parsedScore > 100;

  return (
    <div className="lead-stack">
      <span>{t("lead.explainScore")}</span>
      <ScoreBreakdown id={id} lead={lead} />
      {lead.score_override_reason ? (
        <div className="lead-stack">
          <p>
            {t("lead.scoreOverridden", {
              reason: lead.score_override_reason,
            })}
          </p>
          {lead.score_computed != null && (
            <p className="t-caption">
              {t("lead.machineScore", {
                score: formatNumber(lead.score_computed, locale),
              })}
            </p>
          )}
          <Button
            disabled={writer.patch.isPending || readOnly}
            reasonId={readOnly ? terminalReasonId : undefined}
            onClick={() => writer.save({ score: null })}
          >
            {t("lead.clearOverride")}
          </Button>
        </div>
      ) : overriding ? (
        <div className="lead-stack lead-override">
          <Field label={t("lead.overrideScoreValue")}>
            {(control) => (
              <TextInput
                {...control}
                type="number"
                min={0}
                max={100}
                value={scoreValue}
                onChange={(event) => setScoreValue(event.target.value)}
              />
            )}
          </Field>
          <Field label={t("lead.overrideReason")}>
            {(control) => (
              <TextInput
                {...control}
                value={reasonValue}
                onChange={(event) => setReasonValue(event.target.value)}
              />
            )}
          </Field>
          {/* ds:ignore a lead line is a labelled row; this one happens to hold only verbs */}
          <div className="lead-line">
            <Button
              variant="primary"
              disabled={reasonBlank || scoreInvalid || writer.patch.isPending}
              reasonId={readOnly ? terminalReasonId : undefined}
              onClick={() =>
                writer.save({
                  score: parsedScore,
                  score_override_reason: reasonValue.trim(),
                })
              }
            >
              {t("lead.saveOverride")}
            </Button>
            <Button onClick={() => setOverriding(false)}>
              {t("create.cancel")}
            </Button>
          </div>
        </div>
      ) : (
        // "Machine-computed score" was a label with no value beside it,
        // naming what the badge above already says. The override is a rare
        // action and stands alone.
        <Button
          reasonId={readOnly ? terminalReasonId : undefined}
          onClick={() => setOverriding(true)}
        >
          {t("lead.overrideScore")}
        </Button>
      )}
    </div>
  );
}

/**
 * The lead's own words, editable where they stand.
 *
 * Everything here was previously reachable only through the Edit modal, which
 * is four clicks and a context switch to fix a misspelled company name. The
 * modal stays — it is how a lead is edited wholesale, and how the fields this
 * grid does NOT carry are reached — but the four a rep corrects while reading
 * are corrected while reading.
 *
 * Every row saves through the SAME patch the lifecycle card uses, so one
 * inline edit and another cannot invalidate different caches or send a
 * different If-Match.
 */
function LeadIdentityFields({
  lead,
  writer,
}: Readonly<{ lead: Lead; writer: LeadWriter }>) {
  const t = useT();
  const project = useEntityName("project", lead.project_id);
  const sources = useLeadSources();
  const owners = useRecordOwners(lead.owner_id);
  const connector = (lead.source ?? "").startsWith("connector:");
  return (
    <>
      <RecordFields
        title={t("lead.details")}
        kind="lead"
        links={
          lead.linkedin_url
            ? {
                linkedin_url: {
                  href: lead.linkedin_url,
                  label: t("record.openProfile"),
                },
              }
            : undefined
        }
        record={lead}
        fields={[
          ...leadEditFields,
          {
            key: "owner_id",
            label: "list.owner",
            type: "select",
            options: owners,
            required: true,
          },
          {
            key: "status",
            label: "history.field.status",
            toInput: () => {
              const label = leadStatusLabel(lead.status);
              return label ? t(label) : lead.status;
            },
          },
          { key: "score", label: "lead.score", type: "number" },
          { key: "linkedin_url", label: "create.linkedinUrl" },
          { key: "candidate_company_key", label: "record.companyRoutingKey" },
          {
            key: "project_id",
            label: "lead.project",
            searchTargets: searchProjectReferences,
            options: lead.project_id
              ? [
                  {
                    value: lead.project_id,
                    label: project.name ?? lead.project_id,
                  },
                ]
              : [],
          },
          {
            key: "source",
            label: "lead.source",
            type: "select",
            required: true,
            options: sourcePickOptions(sources.data?.data, lead.source, t),
          },
        ]}
        groups={[{ label: t("create.email"), keys: ["email"] }]}
        canEdit={!writer.readOnly}
        readOnlyFields={{
          linkedin_url: t("record.leadProfileReadOnly"),
          status: t("record.leadStatusAction"),
          score: t("record.leadScoreAction"),
          ...(connector ? { source: t("lead.sourceFromConnector") } : {}),
        }}
        resolveExisting={(_code, id) => ({ screen: "leads", id })}
        save={async (values, _rows, opened) =>
          saveRecordEdit("lead", opened, mapLeadUpdate(values))
        }
      />
      <RecordCustomFields kind="lead" record={lead} />
    </>
  );
}

/**
 * The shared write, as the panels that call it see it: the mutation's STATE
 * (pending, refused, what it carried) and the two ways to start one. Handed
 * over as one object rather than as a mutation plus a loose function, so no
 * caller can reach past `save` to `mutate` and skip the version it stamps.
 */
export type LeadWriter = ReturnType<typeof useLeadPatch>;

/**
 * One write's variables: what to change, and the record it is changing.
 *
 * The version and the closed flag travel WITH the body rather than being read
 * off `lead` inside the mutation. A handler belongs to the render that drew
 * the control the reader pressed, so a version it hands over cannot be older
 * than that control — while a `mutationFn` reaching for `lead` reads whatever
 * render it happens to close over, which under React Query's passive re-arm
 * is not always the one on screen.
 */
type LeadWrite = {
  body: UpdateLeadRequest;
  version: number;
  /** Whether the record was already closed when the reader pressed. */
  archived: boolean;
};

function useLeadPatch(lead: Lead, id: string, onChanged: () => void) {
  const t = useT();
  // A lead that takes no writes — closed, or not this caller's to change —
  // refuses every control here by ONE fact, derived once rather than
  // re-tested per control, because the control that gets missed is the one
  // that had to remember on its own. The sentence travels with the answer so
  // each refused control can say which fact it was.
  const readOnlyReason = useRecordWriteRefusal("lead", lead, {
    archived: t("lead.terminalReadOnly"),
    notYours: t("lead.notYoursToChange"),
  });
  const readOnly = Boolean(readOnlyReason);
  const patch = useMutation({
    mutationKey: ["lead-edit", id],
    mutationFn: async ({ body, version, archived }: LeadWrite) => {
      // The last word on a terminal lead, and deliberately not a per-control
      // check: the server refuses every one of these writes, and a control
      // added later would otherwise have to remember on its own. It reads the
      // flag the PRESS carried, so a lead that went terminal while this page
      // was open is refused by the render that saw it go.
      if (archived) {
        // Catalog copy in a problem body, on the same terms as every other
        // refusal this screen shows: "a terminal lead takes no writes" is a
        // sentence for whoever reads this file, not for whoever is refused.
        throwProblem({ detail: t("lead.terminalReadOnly") });
      }
      const { data, error } = await api.PATCH("/leads/{id}", {
        params: { path: { id }, ...ifMatch(requireVersion(version)) },
        body,
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    onSuccess: onChanged,
  });

  // The record as it stands in THIS render, stamped onto the write the caller
  // is starting. Every control goes through one of these two, so no call site
  // has to remember to carry the version.
  const write = (body: UpdateLeadRequest): LeadWrite => ({
    body,
    version: requireVersion(lead.version),
    archived: Boolean(lead.archived_at),
  });
  const save = (body: UpdateLeadRequest) => patch.mutate(write(body));

  // Taking an unowned lead is a CLAIM, not a patch: the write arm refuses an
  // ownerless row on purpose, so the request that works here is the claim
  // door. It lives beside `patch` rather than in the control that calls it
  // because the page states what a write refused in ONE banner, and a refusal
  // thrown inside a rail that is not currently rendered reaches nobody.
  const claim = useMutation({
    mutationKey: ["lead-claim", lead.id],
    mutationFn: useClaimRecord("lead", lead.id, lead.version),
    onSuccess: onChanged,
  });

  return { patch, claim, readOnly, readOnlyReason, save };
}

/**
 * The page's LEAD: where this lead stands, and the one step to take next.
 *
 * It is the tinted panel because it is the only surface here asking for a
 * MOVE — everything else on the page reports. The tone follows the finding
 * rather than the layout: a first response already breached is bad news, and
 * the warning family is what says so, in the same pairing `Callout` draws.
 */
/**
 * The score as a card of the reading: it folds to one line with its top
 * factor, and opens for the breakdown and the override. Beside it, in the
 * pair, the inputs a rep enters by hand.
 */
function LeadScoreCard({
  lead,
  id,
  writer,
  terminalReasonId,
}: Readonly<{
  lead: Lead;
  id: string;
  writer: LeadWriter;
  terminalReasonId: string;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const [overriding, setOverriding] = useState(false);
  const [scoreValue, setScoreValue] = useState("");
  const [reasonValue, setReasonValue] = useState("");
  // THIS form's save, not any save. The mutation is shared with the owner
  // picker and the ladder, so `isSuccess` alone cleared a half-typed override
  // the moment somebody assigned an owner — and a refused save must leave what
  // the reader typed where they typed it either way. `score` is what both of
  // this form's writes carry (a value to set, or null to clear) and no other
  // control on the page sends.
  const saved =
    writer.patch.isSuccess && "score" in (writer.patch.variables?.body ?? {});
  useEffect(() => {
    if (saved) {
      setOverriding(false);
      setScoreValue("");
      setReasonValue("");
    }
  }, [saved]);
  return (
    <Panel title={t("lead.score")}>
      <PanelBody>
        <Disclosure
          summary={
            <span className="lead-score-summary">
              <Badge tone={scoreTone(lead.score)}>
                {t("lead.score")}: {formatNumber(lead.score, locale)}
              </Badge>{" "}
              <span className="t-caption">
                {lead.score_reason
                  ? scoreFactorLabel(lead.score_reason, t)
                  : t("lead.scoreNoSignals")}
              </span>
            </span>
          }
        >
          <LeadScorePanel
            lead={lead}
            id={id}
            terminalReasonId={terminalReasonId}
            overriding={overriding}
            setOverriding={setOverriding}
            scoreValue={scoreValue}
            setScoreValue={setScoreValue}
            reasonValue={reasonValue}
            setReasonValue={setReasonValue}
            writer={writer}
          />
        </Disclosure>
      </PanelBody>
    </Panel>
  );
}

/**
 * THE CALL on a lead: where it stands, as a fact the server already decided
 * said in a word, with the lead's own thread under it — what was said, the
 * silence since, what is dated ahead.
 */
function LeadCall({
  lead,
  thread,
  onOpenEmail,
}: Readonly<{
  lead: Lead;
  thread: RecordTimeline;
  // The page's one drawer, handed down to the thread. A conversation the
  // thread names and cannot open is the one place in the product that does
  // that.
  onOpenEmail: (activityId: string) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const standing = leadStanding(lead, t, locale, viewerZone());
  return (
    <LeadBrief
      standing={{ label: standing.label, tone: standing.tone }}
      because={standing.because}
      restsOn={standing.restsOn}
    >
      <TimelineThread thread={thread} onOpenEmail={onOpenEmail} />
    </LeadBrief>
  );
}

/**
 * What the promotion did, read from the promote audit row it wrote.
 *
 * `outcome` is a closed union with an explicit unknown, not a bare string: the
 * page states "merged into a contact we already knew" or "became a new
 * contact", and treating every non-"merged" value as "created" would make
 * schema drift, a bad row, or a future third outcome read as a confident
 * claim about a merge that never happened.
 */
type PromotionOutcome = "merged" | "created" | "unknown";

type PromotionRecord = {
  outcome: PromotionOutcome;
  trigger?: string;
  evidenceNote?: string;
  // The read's own state. Loading and failing are not "created" — a panel that
  // reported an outcome while its source was still in flight would show the
  // wrong one for as long as the request took, and forever on a 403.
  pending: boolean;
  failed: boolean;
};

/**
 * usePromotionRecord reads the promotion off the lead's audit trail.
 *
 * The outcome, trigger and evidence are not columns on `lead` — the write
 * shape puts them in the `promote` audit row, which is the honest source:
 * re-deriving "did this merge?" from today's data would answer about the
 * records as they are now, not about what actually happened.
 *
 * Only a promoted lead has a promotion to describe, so the read is disabled on
 * every other one rather than fetching a history nothing renders.
 */
function usePromotionRecord(id: string, promoted: boolean): PromotionRecord {
  // ONE row, asked for by verb. The history endpoint takes an `action` filter
  // now (#1611), so the promotion is the answer to the read rather than
  // something found by walking towards it.
  //
  // What that replaced is worth remembering, because it was a real wrong
  // answer and not merely a slow one: the trail is 20 rows to a page, so a lead
  // worked long enough to collect other audit rows carried its promotion on a
  // later page, and a reader that took the first page reported the outcome as
  // unknowable on exactly the leads somebody had worked hardest. Paging on
  // until it turned up fixed the answer and cost a round trip per page.
  //
  // A filtered read has at most one promote row — a lead is promoted once —
  // so there is no page after the first and nothing to walk.
  const history = useRecordHistory("lead", id, promoted, "promote");
  // `page?.data` for the same reason getNextPageParam needs it: a 200 with no
  // body is a shape the contract permits, and this read runs on every promoted
  // lead page.
  const entries = history.data?.pages.flatMap((page) => page?.data ?? []) ?? [];
  const row = entries[0];

  const after = (row?.after ?? {}) as Record<string, unknown>;
  const str = (key: string) =>
    typeof after[key] === "string" ? (after[key] as string) : undefined;
  const recorded = str("dedupe_outcome");
  return {
    outcome:
      recorded === "merged" || recorded === "created" ? recorded : "unknown",
    trigger: str("trigger"),
    evidenceNote: str("evidence_note"),
    // A read in flight is pending: reporting "we cannot tell" before the answer
    // arrives is the same false certainty as reporting "created". A FAILED read
    // is never pending — the panel checks pending first, so leaving both true
    // renders a waiting line over an error nobody ever sees.
    pending: promoted && !history.isError && history.isPending,
    failed: promoted && history.isError,
  };
}

/**
 * PromotePreviewLine says what promoting will DO before the rep commits
 * (ADR-0119/A170): merge into a contact we already hold, or create one. It
 * reads GET /leads/{id}/promote-preview, which runs the promotion's own dedupe
 * ladder without writing.
 *
 * An absent contact on a `merge` never means "no match" — it means the matched
 * contact is outside the reader's row scope, and the line says so rather than
 * promising a new contact the server will not create.
 */
/**
 * DemoteAction is the reversal ADR-0008 §4 promises, from the one page that
 * can honestly host it. A reason is required and recorded: an undo nobody
 * explained is later indistinguishable from a mistake.
 */
function DemoteAction({ id }: Readonly<{ id: string }>) {
  const t = useT();
  const queryClient = useQueryClient();
  const [open, setOpen] = useState(false);
  const [reason, setReason] = useState("");
  const demote = useMutation({
    mutationFn: async () => {
      const { data, error } = await api.POST("/leads/{id}/demote", {
        params: { path: { id } },
        body: { reason: reason.trim() },
      });
      if (error) {
        throwProblem(error, t);
      }
      return data;
    },
    onSuccess: () => {
      for (const key of leadWriteKeys(id)) {
        queryClient.invalidateQueries({ queryKey: key });
      }
      setOpen(false);
      setReason("");
    },
  });
  const close = () => {
    setOpen(false);
    demote.reset();
  };
  return (
    <>
      <Button onClick={() => setOpen(true)}>{t("lead.demote")}</Button>
      <ConfirmModal
        open={open}
        onClose={close}
        title={t("lead.demoteDialog")}
        confirmLabel={t("lead.demoteConfirm")}
        confirmReason={
          reason.trim() === "" ? t("lead.demoteReasonRequired") : undefined
        }
        onConfirm={() => demote.mutate()}
        pending={demote.isPending}
        error={demote.isError ? problemMessageOf(demote.error, t) : undefined}
      >
        <div className="lead-stack">
          <p className="t-body">{t("lead.demoteExplain")}</p>
          <Field label={t("lead.demoteReason")} required>
            {(control) => (
              <Textarea
                {...control}
                value={reason}
                onChange={(event) => setReason(event.target.value)}
              />
            )}
          </Field>
        </div>
      </ConfirmModal>
    </>
  );
}

/**
 * PromotedLeadPanel is what a promoted lead's page is FOR (ADR-0119/A170).
 *
 * The page used to redirect to the contact, which told the reader the lead had
 * ceased to exist — untrue of a record this product keeps, audits and can
 * reverse (ADR-0008 §4). It also left the reversal that ADR promises with no
 * surface to be started from, and hid whether promotion merged into a contact
 * we already knew or created a new one. That distinction is the difference
 * between "my prospect is now a contact" and "my prospect was already someone
 * we knew".
 */
function PromotedLeadPanel({
  lead,
  promotion,
}: Readonly<{ lead: Lead; promotion: PromotionRecord }>) {
  const t = useT();
  const { locale } = useLocale();
  const triggerLabel = promotionTriggerLabel(promotion.trigger);
  // Four states, not two. The contact link below is a fact the LEAD row carries,
  // so it renders either way; only the outcome waits on the audit read.
  const outcomeLine = () => {
    if (promotion.pending) {
      return t("lead.promotedOutcomePending");
    }
    if (promotion.failed) {
      return t("lead.promotedOutcomeUnavailable");
    }
    switch (promotion.outcome) {
      case "merged":
        return t("lead.promotedMerged");
      case "created":
        return t("lead.promotedCreated");
      // The audit row is missing, unreadable, or names an outcome this build
      // does not know. Saying so is the honest answer; picking one would be a
      // claim about a merge nobody recorded.
      case "unknown":
        return t("lead.promotedOutcomeUnavailable");
    }
  };
  return (
    <Panel title={t("lead.promotedTitle")}>
      <PanelBody>
        <div className="lead-stack">
          <p className="t-body">{outcomeLine()}</p>
          <p className="t-body">
            <EntityRef kind="contact" id={lead.promoted_contact_id} />
          </p>
          {lead.promoted_at && (
            <p className="t-caption">
              {t("lead.promotedAt")}{" "}
              {formatDateAbbrev(
                lead.promoted_at,
                locale,
                // The reader's own zone, the same one the shell stamps this
                // page's timeline rows in — a lead carries no location of its
                // own to prefer over where the reader is.
                viewerZone(),
              )}
            </p>
          )}
          {triggerLabel && (
            <p className="t-caption">
              {t("lead.promotedTrigger")} {t(triggerLabel)}
            </p>
          )}
          {promotion.evidenceNote && (
            <p className="t-caption">
              {t("lead.promotedEvidence")} {promotion.evidenceNote}
            </p>
          )}
          {/* The reversal lives here and nowhere else: this is the record the
              promotion is a fact about. */}
          <DemoteAction id={lead.id} />
        </div>
      </PanelBody>
    </Panel>
  );
}

const LEAD_TABS = ["overview", "deals", "history"] as const;
type LeadTab = (typeof LEAD_TABS)[number];

/** isLeadTab narrows a URL segment, which is any string a reader can type. */
function isLeadTab(value: string | undefined): value is LeadTab {
  return LEAD_TABS.some((tab) => tab === value);
}

// The lead's tab, addressed rather than held beside the address: a tab that
// survives a reload and can be linked to, and Back that steps between the tabs
// a reader opened instead of leaving the lead altogether. Same shape as the
// account's (screens/companies.tsx) and the contact's.
function useLeadTab(recordId: string): [LeadTab, (next: LeadTab) => void] {
  const route = useRoute();
  const addressed =
    route.screen === "leads" && route.id === recordId ? route.id2 : undefined;
  return [
    isLeadTab(addressed) ? addressed : "overview",
    (next: LeadTab) => navigate({ screen: "leads", id: recordId, id2: next }),
  ];
}

// The lead-360's "overview" pane, split out of LeadScreen so the tab switch
// doesn't push the render-prop closure over the cognitive-complexity budget.
// Every prop here is a value already resolved (or owned as local state) by
// LeadScreen — no new fetches, no behavior change from the pre-tab layout;
// the promote modal's open/trigger/note state stays lifted in the parent so
// it survives a tab switch away and back.
// The status ladder climbs a few seconds AFTER a logged touch: the workflow
// runs off the outbox, in the worker, not inside the POST. Without a delayed
// re-read the stepper keeps the old status until a manual reload — which
// reads as "logging did nothing". Three staggered re-reads cover the
// relay+workflow latency without polling forever; unmounting cancels them.
function useLadderRefresh(id: string): () => void {
  const queryClient = useQueryClient();
  const timers = useRef<number[]>([]);
  useEffect(
    () => () => {
      for (const timer of timers.current) {
        window.clearTimeout(timer);
      }
    },
    [],
  );
  return () => {
    for (const delay of [1500, 4000, 8000]) {
      timers.current.push(
        window.setTimeout(() => {
          for (const key of leadWriteKeys(id)) {
            queryClient.invalidateQueries({ queryKey: key });
          }
        }, delay),
      );
    }
  };
}

function LeadOverviewPane({
  lead,
  id,
  writer,
  promotion,
  terminalReasonId,
  thread,
  onReply,
  onOpenEmail,
}: Readonly<{
  lead: Lead;
  id: string;
  writer: LeadWriter;
  promotion: PromotionRecord;
  terminalReasonId: string;
  // The lead's unfiltered timeline read, which the thread under the call is
  // drawn from — the whole read, so its failure reaches the call too.
  thread: RecordTimeline;
  // The "Answer" row's own verb: opens the SAME composer the header's Email
  // verb opens, owned by LeadRecord so both controls answer to one open
  // state rather than each mounting its own copy of it.
  onReply: () => void;
  // The page's one email drawer, for the thread under the call.
  onOpenEmail: (activityId: string) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  // The lead carries no task id of its own, so both the panel head's way out
  // and the "Next task" row's own verb open the same queue.
  const onOpenTasks = () => navigate({ screen: "worklist" });
  return (
    <div className="record-stack">
      {/* The readings open the overview, as they do on every record page. */}
      <LeadReadings lead={lead} />
      {/* A merged-away lead's page leads with where it went, for the same
          reason: the reader arrived asking what happened to this prospect. */}
      {lead.merged_into_id && (
        <MergedLeadPanel mergedIntoId={lead.merged_into_id} />
      )}
      {/* A promoted lead's page leads with what the promotion did — the
          reader arrived asking whether this became a contact, and which one. */}
      {lead.promoted_contact_id && (
        <PromotedLeadPanel lead={lead} promotion={promotion} />
      )}
      {/* ONE READING, IN PARTS — the shape every record page reads in: the
          call with the lead's own thread under it, what needs a contact, and
          under them the two sections a reader consults rather than reads —
          why it scores what it scores, and what the rep knows about it. */}
      <RecordReading>
        <LeadCall lead={lead} thread={thread} onOpenEmail={onOpenEmail} />
        <TodayPanel onOpenTasks={onOpenTasks} tasksLabel={t("today.workQueue")}>
          {leadTodoRows(
            lead,
            t,
            locale,
            recordZone,
            onReply,
            onOpenTasks,
            writer.readOnly ? terminalReasonId : undefined,
          )}
        </TodayPanel>
        {/* Full width, in sequence, rather than side by side (RecordReadingPair):
            the score card is one line and the signals form is tall, and a
            short card beside a long one reads as a layout that ran out of
            content rather than as two readings a rep consults in order. */}
        <LeadScoreCard
          lead={lead}
          id={id}
          writer={writer}
          terminalReasonId={terminalReasonId}
        />
        <Panel title={t("lead.signalsTitle")}>
          <PanelBody>
            <LeadManualSignals
              // Keyed by lead: a half-typed input for one lead must not be
              // submitted against the next one the reader navigates to.
              key={id}
              id={id}
              readOnlyReason={writer.readOnlyReason}
            />
          </PanelBody>
        </Panel>
      </RecordReading>
    </div>
  );
}

// The lead's identity row and its verbs. Extracted from LeadScreen because
// the terminal-state branch pushed that render past the complexity budget,
// and because a header is a thing in its own right: the name, why the verbs
// are gone when they are, and the verbs themselves.
function LeadActions({
  lead,
  id,
  onQualify,
  onDisqualify,
  onLogged,
  terminalReasonId,
  refusedReasonId,
  emailOpen,
  onEmailOpenChange,
}: Readonly<{
  lead: Lead;
  id: string;
  onQualify: () => void;
  onDisqualify: () => void;
  // Fires once the Log activity/Add task drawer actually logs something, so
  // the page can re-poll the ladder fields an activity can move (status,
  // score, the next task), the same rule the drawer's own `onLogged` states.
  onLogged: () => void;
  // The id of the ONE sentence this page prints about why the lead takes no
  // changes. Every refused control points at it rather than repeating it,
  // which is what stops a terminal lead printing the same line five times.
  terminalReasonId: string;
  // That same id while the sentence is on the page — the lead is closed, or
  // not this caller's to change — and undefined while the lead takes writes.
  // Email is the one verb here that writes no lead row, so it keeps reading
  // the closure alone.
  refusedReasonId?: string;
  // The header's composer, lifted to LeadRecord: the "Answer" row's own Reply
  // verb opens this SAME state rather than a copy of it.
  emailOpen: boolean;
  onEmailOpenChange: (open: boolean) => void;
}>) {
  const t = useT();
  // useCanWrite, not useCan: the drawer below issues a POST, and a read seat
  // is refused before RBAC is consulted, the same rule contactactions.tsx
  // and companyheaderactions.tsx state for the identical pair of verbs.
  // Independent of the terminal check: a live lead a seat may not log
  // against is refused for THIS reason, not that one.
  const me = useMe();
  const canLog = useCanWrite("activity", "create");
  const logRefusedId = useId();
  // A guard that has not answered yet refuses nothing: claiming a refusal
  // `/me` has not decided is worse than a control that is briefly quiet.
  const logGrantKnown = me.data?.authorization !== undefined;
  const archived = lead.archived_at ? terminalReasonId : undefined;
  const logRefused =
    archived ?? (logGrantKnown && !canLog ? logRefusedId : undefined);
  const logPending = !archived && !logGrantKnown;
  // The verb the reader arrived to perform, named by the address rather than
  // guessed: a caller that sends somebody here to log a call says so, and the
  // drawer opens already showing it instead of the note they would have to
  // change it to.
  //
  // Left in the address rather than consumed, like every other dial this
  // product carries: the link is one somebody can paste, and Back returns to
  // the same screen it described.
  const [params] = useUrlParams();
  const askedToLogCall = params.get(ACTION_PARAM) === CALL_ACTION;
  const [drawer, setDrawer] = useState<"log" | "task" | null>(
    askedToLogCall ? "log" : null,
  );
  // A link pressed on the record the reader is ALREADY on changes the address
  // and nothing else, no remount, so the initializer above never runs again.
  useEffect(() => {
    if (askedToLogCall) {
      setDrawer("log");
    }
  }, [askedToLogCall]);
  return (
    <>
      {/* Promote is the page's ONE primary action and it leads, in the header
          where a reader looks for the verb (ADR-0108 §6). Ineligibility is
          stated on the control itself rather than as a sentence beside it —
          a disabled button whose reason is elsewhere is a dead button. */}
      {!lead.archived_at && (
        <Button
          variant="primary"
          data-testid="lead-qualify"
          reasonId={refusedReasonId}
          reason={
            promoteEligible(lead) ? undefined : t("lead.promoteIneligible")
          }
          onClick={onQualify}
        >
          {/* The glyph is the promotion itself: a lead leaving this page
              upward, for the contact and deal it becomes. No `size` — the
              button owns its icon's geometry, and a call site that names one
              is a second author of it. */}
          <ArrowUpRight aria-hidden="true" /> {t("lead.promote")}
        </Button>
      )}
      {/* The shared Email verb every record header carries. Its open state is
          lifted so the overview's "Answer" row can open this SAME composer. */}
      <RecordEmailVerb
        entityType="lead"
        entityId={id}
        recordAddress={lead.email ?? undefined}
        disabledReasonId={archived}
        open={emailOpen}
        onOpenChange={onEmailOpenChange}
      />
      {/* A hairline between reaching the record and recording what happened
          to it: two groups of verbs, not one toolbar. */}
      <span className="record-actions-sep" aria-hidden="true" />
      {!archived && logGrantKnown && !canLog && (
        <p id={logRefusedId}>{t("record.logActivityRefused")}</p>
      )}
      {/* A CRM a rep cannot write a meeting into is a CRM that only reads.
          This is the standing way in, the same pair contactactions.tsx and
          companyheaderactions.tsx carry: what happened, and what happens
          next. The drawer stays local to this header, unlike company's own
          daily-brief card, because nothing else on the lead page opens it. */}
      <Button
        disabled={logPending}
        reasonId={logRefused}
        onClick={() => setDrawer("log")}
      >
        <FileText aria-hidden="true" /> {t("log.title")}
      </Button>
      <Button
        disabled={logPending}
        reasonId={logRefused}
        onClick={() => setDrawer("task")}
      >
        <CheckSquare aria-hidden="true" /> {t("log.addTask")}
      </Button>
      {drawer && (
        <LogActivityAction
          entityType="lead"
          entityId={id}
          askedKind={
            drawer === "task" ? "task" : askedToLogCall ? "call" : undefined
          }
          triggerLabel={drawer === "task" ? "log.addTask" : undefined}
          openOnMount
          onLogged={onLogged}
          onClose={() => setDrawer(null)}
        />
      )}
      {/* Everything else this lead offers, behind one trigger. Qualify and
          Email are what a rep reaches for between calls; the rest are rare
          enough that a reader hunting one of them should not have to read
          past them to find a common verb. Worded and glyphless in here: a
          list of named actions with one unnamed square in it makes that
          square the only row a reader has to hover to identify.

          A terminal lead keeps these controls, DISABLED with the reason
          (STATE-4a): the reason is the information, and hiding the control
          hides a fact the reader needs. Both closures reach this page — a
          disqualified lead and, since ADR-0119/A170, a promoted one — and the
          band above names which, so these controls point at that one
          sentence rather than guessing at it. The band is also WHY the
          sentence is passed in rather than minted here: a reason living in
          the panel would not exist until the menu was first opened. */}
      <OverflowMenu label={t("record.moreActions")}>
        <ShareAction
          recordType="lead"
          recordId={lead.id}
          disabledReasonId={refusedReasonId}
        />
        {/* Last: it is the one verb here a reader cannot walk back from
                the header, so it does not sit where a pointer sliding down
                the list reaches it on the way to something routine. It asks
                why, in its own dialog, and stays a secondary verb rather than
                a red one — closing a lead is ordinary work, and the panel's
                seam (atoms.css) belongs to the destructive verbs. A terminal
                lead keeps the control, disabled with the page's one reason. */}
        <Button
          data-testid="lead-disqualify"
          reasonId={refusedReasonId}
          onClick={onDisqualify}
        >
          {t("record.disqualify")}
        </Button>
      </OverflowMenu>
    </>
  );
}

// The two governed-transition dialogs, mounted only while open and keyed by
// the lead so a half-filled deal block for one lead never carries to the
// next. The qualify outcome comes back as the sentence the page shows.
function LeadDialogs({
  lead,
  dialog,
  onClose,
  onQualified,
}: Readonly<{
  lead: Lead;
  dialog: "qualify" | "disqualify" | null;
  onClose: () => void;
  onQualified: (done: ReactNode) => void;
}>) {
  const t = useT();
  if (dialog === "qualify") {
    return (
      <QualifyDialog
        key={`qualify-${lead.id}`}
        lead={lead}
        open
        onClose={onClose}
        onQualified={(result) =>
          onQualified(
            <span>
              {t("lead.qualify.done", {
                name: leadIdentityName(lead),
              })}{" "}
              <EntityRef kind="contact" id={result.contact.id} />
              {result.deal_id && (
                <>
                  {" · "}
                  <EntityRef kind="deal" id={result.deal_id} />
                </>
              )}
            </span>,
          )
        }
      />
    );
  }
  if (dialog === "disqualify") {
    return (
      <DisqualifyDialog
        key={`disqualify-${lead.id}`}
        lead={lead}
        open
        onClose={onClose}
        onDisqualified={onClose}
      />
    );
  }
  return null;
}

/**
 * One lead's page, with the record in hand.
 *
 * Split from `LeadScreen` because the page's ONE write is a hook and the lead
 * it writes only exists inside the query gate: a hook cannot be called from a
 * render prop, and moving the mutation above the gate would have it guarding a
 * record it had not read yet.
 */
function LeadRecord({ lead, id }: Readonly<{ lead: Lead; id: string }>) {
  const t = useT();
  const queryClient = useQueryClient();
  const refreshAfterTouch = useLadderRefresh(id);
  const details = usePageAside();
  // ONE sentence about this lead being closed, minted here and pointed at by
  // every control the closure refuses (ADR-0108 §6).
  const terminalReasonId = useId();
  const [tab, setTab] = useLeadTab(id);
  // The thread under the call reads the WHOLE history, not whatever the
  // History tab's own filter has narrowed. A filter is a view of that tab; a
  // call that said "no reply since" because the reader had hidden emails
  // would be false.
  const threadQuery = useRecordTimeline("lead", id);
  const [openEmail, setOpenEmail] = useOpenEmail();
  const [dialog, setDialog] = useState<"qualify" | "disqualify" | null>(null);
  // What the last qualify did, said once: the contact and, when one was
  // opened, the deal. The page stays (ADR-0119) and the outcome panel below
  // carries the links on, so this is the confirmation rather than the record —
  // which is what `useToast` is, withdrawing itself instead of sitting in the
  // column as a second, permanent copy of the same news.
  const toast = useToast();
  // A promoted lead keeps its page (ADR-0119/A170). It no longer redirects to
  // the contact: the redirect said the lead had ceased to exist, which is
  // untrue of a record this product keeps, audits and can reverse — and it
  // left the reversal with nowhere to start from. The page reads the
  // promotion off its own audit row and says what happened.
  const promotion = usePromotionRecord(id, Boolean(lead.promoted_contact_id));
  const writer = useLeadPatch(lead, id, () => {
    for (const key of leadWriteKeys(id)) {
      queryClient.invalidateQueries({ queryKey: key });
    }
  });
  // The header's composer, lifted here so the overview's "Answer" row opens
  // the SAME one rather than a copy of it (RecordEmailVerb's own controlled
  // mode).
  const [composing, setComposing] = useState(false);

  return (
    <div className="record-sheet">
      <RecordView
        // One rung under the record scale: the name is still the largest thing
        // on the page, but beside a work column that opens on the agent's ask
        // it no longer needs to be the size of a masthead, the same rung the
        // contact page reads at (contactpage.tsx).
        scale="compact"
        // What a rep CONSULTS — who owns this and why it scores what it scores
        // — in the details pane beside the work, so the work column stays the
        // work and the context does not move when the tab does. The same pane,
        // fold and memory of it as every other record page.
        // At every width, and told whether it is showing, so the column
        // folds rather than vanishing when the toggle shuts it.
        aside={
          <LeadRail
            lead={lead}
            writer={writer}
            onQualify={() => setDialog("qualify")}
            reasonId={writer.readOnly ? terminalReasonId : undefined}
            details={<LeadIdentityFields lead={lead} writer={writer} />}
          />
        }
        asideOpen={details.open}
        name={leadIdentityName(lead) || t("lead.unnamed")}
        avatarSrc={null}
        // The role and the company, on the name's own line: the contact
        // page's register for the same two facts (ContactSubtitle). A lead
        // carries no company FK, so unlike the contact's this is never a link.
        nameBadge={<LeadSubtitle lead={lead} />}
        // The pills under the name: that this is a lead, and where it stands
        // on the ladder. A reader has to know this is a prospect and not a
        // contact BEFORE they read anything else about them (ADR-0108 §1).
        pulse={<LeadPulse lead={lead} />}
        // The facts strip: how to reach this lead, where it came from, and who
        // holds it. Owner keeps its Assign control live in the cell rather than
        // reading only: it used to live in the pulse row for the same reason
        // it lives here now: ownership was reachable only from the details
        // pane, which REMEMBERS being folded, so the one control that takes a
        // lead out of the unassigned queue was, for anyone who had ever
        // collapsed the pane, behind a toggle they had to remember. Rendered
        // here and nowhere else: a second copy in the pane made every owner
        // query on the page ambiguous, which is two controls disagreeing
        // waiting to happen.
        badges={
          <LeadFacts
            lead={lead}
            id={id}
            writer={writer}
            terminalReasonId={terminalReasonId}
          />
        }
        actions={
          <LeadActions
            lead={lead}
            id={id}
            terminalReasonId={terminalReasonId}
            refusedReasonId={writer.readOnly ? terminalReasonId : undefined}
            onQualify={() => setDialog("qualify")}
            onDisqualify={() => setDialog("disqualify")}
            onLogged={refreshAfterTouch}
            emailOpen={composing}
            onEmailOpenChange={setComposing}
          />
        }
        actionsInline
        // The shell stamps timeline rows in this zone. The viewer's own is the
        // honest default for a prospect: a lead carries no workspace location of
        // its own to prefer over where the reader is.
        zone={viewerZone()}
        // Where the lead stands, at the foot of its head — the same place a
        // deal's own ladder stands, for the question both records are asked
        // first.
        standing={
          <LeadStepper
            lead={lead}
            pending={writer.patch.isPending}
            readOnlyReason={writer.readOnlyReason}
            onStep={(status) => {
              // Same one-write-at-a-time rule as the inline rows: a status
              // sent while another save is in flight races it for If-Match.
              if (!writer.patch.isPending && !writer.readOnly) {
                writer.save({ status });
              }
            }}
            onQualify={() => setDialog("qualify")}
            onDisqualify={() => setDialog("disqualify")}
          />
        }
        band={leadBand({ lead, writer, reasonId: terminalReasonId, id, t })}
        // The same strip every record in the product carries: a place a reader
        // navigates, drawn as a rule with the open body underlined, rather than
        // a pill that offers a setting.
        tabs={
          <RecordTabs
            options={LEAD_TABS}
            value={tab}
            onChange={(next) => {
              setTab(next);
              scrollPageToTop();
            }}
            labels={{
              overview: t("tab.overview"),
              // The same key the account's own Deals & projects tab carries:
              // both hold one deal and one project, read the same way.
              deals: t("tab.dealsProjects"),
              history: t("tab.history"),
            }}
            // The switch for the details pane, at the end of the tab row: it
            // chooses what the page shows beside the work, so it stands with
            // the controls that choose what the work column shows. Drawn as a
            // link in the row rather than a boxed control, the same quiet
            // reading the contact page's tab strip gives it.
            trailing={
              <PageAsideToggle
                quiet
                labels={{
                  show: t("record.panel.showDetails"),
                  hide: t("record.panel.hideDetails"),
                }}
              />
            }
          />
        }
      >
        {tab === "overview" && (
          <LeadOverviewPane
            lead={lead}
            id={id}
            writer={writer}
            promotion={promotion}
            terminalReasonId={terminalReasonId}
            thread={threadQuery}
            onOpenEmail={setOpenEmail}
            onReply={() => setComposing(true)}
          />
        )}
        {tab === "deals" && (
          <LeadDealsProjectsTab
            lead={lead}
            writer={writer}
            onQualify={() => setDialog("qualify")}
            reasonId={writer.readOnly ? terminalReasonId : undefined}
          />
        )}
        <LeadDialogs
          lead={lead}
          dialog={dialog}
          onClose={() => setDialog(null)}
          onQualified={(done) => {
            setDialog(null);
            toast.show(done);
          }}
        />
        {tab === "history" && (
          <LeadHistoryTab lead={lead} onOpenEmail={setOpenEmail} />
        )}
        {/* One drawer over the lead, whichever tab is open: the Overview
            call's own thread and the History tab's rows both open into it. */}
        <OpenEmailDrawer
          activityId={openEmail}
          zone={viewerZone()}
          onClose={() => setOpenEmail(null)}
        />
      </RecordView>
    </div>
  );
}

export function LeadScreen({ id }: Readonly<{ id: string }>) {
  const t = useT();
  const leadQuery = useQuery({
    queryKey: leadKey(id),
    queryFn: async () => {
      const { data, error } = await api.GET("/leads/{id}", {
        params: { path: { id } },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });

  return (
    <div className="wrap lead-surface">
      <QueryGate query={leadQuery} pendingLabel={t("nav.leads")}>
        {(lead) => (
          // Keyed by lead: every piece of page state below — the open dialog,
          // the tab, a half-typed score override — is about THIS lead, and
          // this screen stays mounted from one to the next.
          <LeadRecord key={lead.id} lead={lead} id={id} />
        )}
      </QueryGate>
    </div>
  );
}
