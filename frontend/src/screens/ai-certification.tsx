import { useQuery } from "@tanstack/react-query";
import { useId, useState } from "react";
import { useCan } from "../app/capability";
import { Badge, Button, DataTable, EmptyState, Modal } from "../design-system/atoms";
import { Panel, PanelBody } from "../design-system/panel";
import { formatDate, formatNumber } from "../format/format";
import { viewerZone } from "../format/timezone";
import { useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { QueryGate, throwProblem } from "./common";

// How well the models this workspace is bound to actually do each AI job.
//
// The binding card above says WHICH model serves a job. This says whether that
// model can be trusted with it — measured against a fixed set of realistic
// examples, graded by a separate model, and committed alongside the code.
//
// Written for somebody who does not know what a tier or a prompt version is.
// Every word on this card is a claim a non-technical reader can act on, and the
// rules that keep those claims honest are worth stating because a shorter
// vocabulary would have hidden all three:
//
//   - A job reads as its WORST invocation site, never an average. Averaging
//     would let three sound sites carry one that fails every time.
//   - Counts, never a bare percentage. The verdict folds to the worst example,
//     so a job can pass 23 of 24 runs and still be unreliable — "96%" beside
//     "not reliable enough" reads to a person as a contradiction.
//   - A real measurement stays visible when it goes out of date. Its age goes on
//     the same line; hiding an old 88% behind "not checked" is the dishonest
//     option, not the cautious one.

type Certification = components["schemas"]["AiCertification"];
type Job = components["schemas"]["AiCertificationJob"];
type Result = components["schemas"]["AiCertificationResult"];

export function AiCertificationCard() {
  const t = useT();
  const canSee = useCan("ai_routing", "read");
  const [explaining, setExplaining] = useState(false);
  const titleId = useId();

  const query = useQuery({
    queryKey: ["ai-certification"],
    queryFn: async () => {
      const { data, error } = await api.GET("/ai/certification");
      if (error) throwProblem(error);
      return data;
    },
    enabled: canSee,
  });

  if (!canSee) {
    return null;
  }
  return (
    <Panel title={t("aiCert.title")}>
      <PanelBody>
        <p className="settings-panel-sub">{t("aiCert.sub")}</p>
        <QueryGate query={query}>
          {(cert) => (
            <>
              <JobTable cert={cert} />
              <div className="panel-foot">
                <Button variant="ghost" onClick={() => setExplaining(true)}>
                  {t("aiCert.explainOpen")}
                </Button>
              </div>
              <Modal
                open={explaining}
                onClose={() => setExplaining(false)}
                labelledBy={titleId}
              >
                <h2 id={titleId}>{t("aiCert.explainTitle")}</h2>
                <p>{t("aiCert.explainWhat")}</p>
                <p>
                  {t("aiCert.explainHow", {
                    runs: String(cert.runs_per_example),
                  })}
                </p>
                <p>{t("aiCert.explainMeaning")}</p>
                <p>{t("aiCert.explainStale")}</p>
                <Button onClick={() => setExplaining(false)}>
                  {t("aiCert.explainClose")}
                </Button>
              </Modal>
            </>
          )}
        </QueryGate>
      </PanelBody>
    </Panel>
  );
}

function JobTable({ cert }: Readonly<{ cert: Certification }>) {
  const t = useT();
  const { locale } = useLocale();

  // Nothing is bound yet. That is a choice nobody has made, not a gap in the
  // evidence, so the card says which — a reader told "not checked" would go
  // looking for a measurement that would not help them.
  if (cert.binding_state === "unbound") {
    return (
      <EmptyState>
        <p className="t-small">{t("aiCert.unbound")}</p>
      </EmptyState>
    );
  }

  const named = cert.jobs.filter((job) => JOB_NAME[job.task] !== undefined);
  const unnamedCount = cert.jobs.length - named.length;

  return (
    <>
      <DataTable<Job>
        label={t("aiCert.title")}
        rows={named}
        rowKey={(job) => job.task}
        columns={[
          {
            key: "job",
            header: t("aiCert.colJob"),
            render: (job) => <JobName task={job.task} />,
          },
          {
            key: "model",
            header: t("aiCert.colModel"),
            render: (job) =>
              job.model ? (
                <span className="t-mono t-small">{job.model}</span>
              ) : (
                <span className="t-meta">—</span>
              ),
          },
          {
            key: "result",
            header: t("aiCert.colResult"),
            render: (job) => <ResultCell job={job} />,
          },
        ]}
      />
      {unnamedCount > 0 ? (
        // A job this build of the app has no wording for. Its identifier is not
        // printed: the whole point of this card is that a reader does not have
        // to know what `site_fact_extract` is, and printing it would be worse
        // than saying nothing. Counted, so the omission is visible.
        <p className="t-meta">
          {t("aiCert.unnamedJobs", { count: formatNumber(unnamedCount, locale) })}
        </p>
      ) : null}
    </>
  );
}

// A job's name in the reader's language.
//
// Its own component because the lookup cannot fail here: JobTable has already
// dropped the jobs this build has no wording for, so the key is present by
// construction and the row never falls back to an identifier.
function JobName({ task }: Readonly<{ task: string }>) {
  const t = useT();
  const key = JOB_NAME[task];
  return <>{key ? t(key) : null}</>;
}

// The name of the site that set a job's result, or its raw variant when this
// build has no wording for it. Unlike the job column, a missing site name only
// costs one parenthetical rather than the row, so it degrades instead of hiding.
function siteName(
  t: ReturnType<typeof useT>,
  task: string,
  variant: string,
): string {
  const key = SITE_NAME[`${task}.${variant}`];
  return key ? t(key) : variant;
}

// One job's verdict, with the evidence that makes it readable.
//
// The counts sit under the words because the words alone cannot be checked: a
// reader who sees only "not reliable enough" has to take it on faith, and a
// reader who sees only "23 of 24" cannot tell that the one failure is the same
// example every time.
function ResultCell({ job }: Readonly<{ job: Job }>) {
  const t = useT();
  const { locale } = useLocale();
  return (
    <span className="cell-stack">
      <Badge tone={toneOf(job.result)} quiet>
        {t(RESULT_KEY[job.result])}
      </Badge>
      {job.runs !== undefined && job.passed !== undefined ? (
        <span className="t-meta">
          {t("aiCert.runCounts", {
            passed: formatNumber(job.passed, locale),
            runs: formatNumber(job.runs, locale),
          })}
        </span>
      ) : null}
      {/* A high pass rate under a failing verdict is the case that reads as a
          contradiction, so it is explained on the row rather than left to the
          modal a reader may never open. */}
      {job.result === "not_reliable" &&
      job.passed !== undefined &&
      job.runs !== undefined &&
      job.passed > 0 ? (
        <span className="t-meta">{t("aiCert.oneKindFails")}</span>
      ) : null}
      {job.result === "partly_checked" && job.pending_examples ? (
        <span className="t-meta">
          {t("aiCert.pendingExamples", {
            measured: formatNumber(job.measured_examples ?? 0, locale),
            total: formatNumber(
              (job.measured_examples ?? 0) + job.pending_examples,
              locale,
            ),
          })}
        </span>
      ) : null}
      {job.result === "out_of_date" && job.measured_at ? (
        <span className="t-meta">
          {t("aiCert.measuredOn", {
            date: formatDate(job.measured_at, locale, viewerZone()),
          })}
        </span>
      ) : null}
      {/* A measurement exists under a different hosting posture. It does not
          carry over, and saying nothing would throw away evidence the reader
          could go and look at. */}
      {job.measured_under_other_profile ? (
        <span className="t-meta">{t("aiCert.otherProfile")}</span>
      ) : null}
      {/* Narrowed scope: a run that graded one reply of a longer exchange
          certifies less than the exchange, so the claim is qualified rather
          than stated whole. */}
      {isNarrow(job.scope) ? (
        <span className="t-meta">{t("aiCert.narrowScope")}</span>
      ) : null}
      {job.unmeasured_fallbacks?.length ? (
        <span className="t-meta">
          {t("aiCert.unmeasuredFallback", {
            model: job.unmeasured_fallbacks.join(", "),
          })}
        </span>
      ) : null}
      {job.worst_site ? (
        <span className="t-meta">
          {t("aiCert.worstSite", { site: siteName(t, job.task, job.worst_site) })}
        </span>
      ) : null}
    </span>
  );
}

// A run that graded less than the site's whole path. Named here rather than
// inferred from a substring so a new scope word cannot silently start reading
// as full coverage.
function isNarrow(scope: string | undefined): boolean {
  return scope === "single_turn" || scope === "single_call";
}

function toneOf(result: Result): "success" | "warn" | "danger" | undefined {
  switch (result) {
    case "reliable":
      return "success";
    case "mostly_reliable":
    case "partly_checked":
    case "out_of_date":
      return "warn";
    case "not_reliable":
      return "danger";
    default:
      // not_checked and no_model are absences, not findings. A tone would give
      // them an urgency the card does not mean.
      return undefined;
  }
}

// The seven verdicts as catalog keys.
//
// A literal map rather than a computed `aiCert.result.${result}`: a template
// key is a string the catalog cannot typecheck, and this card is the one place
// where an unresolved key would print machine vocabulary at a reader who came
// here precisely so they would not have to read any.
const RESULT_KEY: Record<Result, MessageKey> = {
  reliable: "aiCert.result.reliable",
  mostly_reliable: "aiCert.result.mostly_reliable",
  not_reliable: "aiCert.result.not_reliable",
  partly_checked: "aiCert.result.partly_checked",
  out_of_date: "aiCert.result.out_of_date",
  not_checked: "aiCert.result.not_checked",
  no_model: "aiCert.result.no_model",
};

// One human name per shipped job, and per site within it. The server sends
// identifiers, so these are looked up rather than derived, and a lookup that
// MISSES is the version-skew case: this build of the app has met a job it has
// no wording for. Such a job is counted, never printed — see JobTable.
//
// gates/aicertificationnames_test.go derives the required set from the
// shipped-site census, so a job added without a name fails a test rather than
// reaching a reader as `site_fact_extract`.
const JOB_NAME: Readonly<Record<string, MessageKey>> = {
  agent_loop: "aiCert.job.agent_loop",
  brief_ranking: "aiCert.job.brief_ranking",
  capture_classify: "aiCert.job.capture_classify",
  capture_confidentiality_verdict: "aiCert.job.capture_confidentiality_verdict",
  capture_counterparty_verdict: "aiCert.job.capture_counterparty_verdict",
  cold_start: "aiCert.job.cold_start",
  corpus_ask: "aiCert.job.corpus_ask",
  deal_health: "aiCert.job.deal_health",
  document_extract: "aiCert.job.document_extract",
  draft_reply: "aiCert.job.draft_reply",
  enrich: "aiCert.job.enrich",
  growth_fit: "aiCert.job.growth_fit",
  offer_draft: "aiCert.job.offer_draft",
  propose_roles: "aiCert.job.propose_roles",
  rate_extract: "aiCert.job.rate_extract",
  signal_extract: "aiCert.job.signal_extract",
  site_extract: "aiCert.job.site_extract",
  site_fact_extract: "aiCert.job.site_fact_extract",
  site_triage: "aiCert.job.site_triage",
  summarize: "aiCert.job.summarize",
  transcript_propose: "aiCert.job.transcript_propose",
  voice_build: "aiCert.job.voice_build",
  weekly_review: "aiCert.job.weekly_review",
};

const SITE_NAME: Readonly<Record<string, MessageKey>> = {
  "agent_loop.loop": "aiCert.site.agent_loop.loop",
  "brief_ranking.rank": "aiCert.site.brief_ranking.rank",
  "capture_classify.classify": "aiCert.site.capture_classify.classify",
  "capture_confidentiality_verdict.thread": "aiCert.site.capture_confidentiality_verdict.thread",
  "capture_counterparty_verdict.verdict": "aiCert.site.capture_counterparty_verdict.verdict",
  "cold_start.acts": "aiCert.site.cold_start.acts",
  "cold_start.company_message": "aiCert.site.cold_start.company_message",
  "cold_start.field_extract": "aiCert.site.cold_start.field_extract",
  "cold_start.sitereadmessage": "aiCert.site.cold_start.sitereadmessage",
  "corpus_ask.corpus_ask": "aiCert.site.corpus_ask.corpus_ask",
  "deal_health.deal_status": "aiCert.site.deal_health.deal_status",
  "document_extract.fields": "aiCert.site.document_extract.fields",
  "draft_reply.account": "aiCert.site.draft_reply.account",
  "draft_reply.first": "aiCert.site.draft_reply.first",
  "draft_reply.intro": "aiCert.site.draft_reply.intro",
  "draft_reply.person": "aiCert.site.draft_reply.person",
  "draft_reply.reply": "aiCert.site.draft_reply.reply",
  "enrich.signature": "aiCert.site.enrich.signature",
  "growth_fit.growth_fit": "aiCert.site.growth_fit.growth_fit",
  "offer_draft.draft": "aiCert.site.offer_draft.draft",
  "propose_roles.committee": "aiCert.site.propose_roles.committee",
  "rate_extract.fx": "aiCert.site.rate_extract.fx",
  "rate_extract.pricing": "aiCert.site.rate_extract.pricing",
  "signal_extract.thread_events": "aiCert.site.signal_extract.thread_events",
  "site_extract.profile": "aiCert.site.site_extract.profile",
  "site_fact_extract.page_facts": "aiCert.site.site_fact_extract.page_facts",
  "site_triage.triage": "aiCert.site.site_triage.triage",
  "summarize.meeting_plan": "aiCert.site.summarize.meeting_plan",
  "summarize.org_ask": "aiCert.site.summarize.org_ask",
  "summarize.org_brief": "aiCert.site.summarize.org_brief",
  "summarize.org_dossier": "aiCert.site.summarize.org_dossier",
  "transcript_propose.next_steps": "aiCert.site.transcript_propose.next_steps",
  "voice_build.derive": "aiCert.site.voice_build.derive",
  "voice_build.eval_draft": "aiCert.site.voice_build.eval_draft",
  "voice_build.eval_scores": "aiCert.site.voice_build.eval_scores",
  "weekly_review.narrative": "aiCert.site.weekly_review.narrative",
};
