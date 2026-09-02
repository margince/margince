import { useQuery } from "@tanstack/react-query";
import { useId, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useCan } from "../app/capability";
import {
  Badge,
  Button,
  DataTable,
  EmptyState,
  Modal,
} from "../design-system/atoms";
import { Panel, PanelBody } from "../design-system/panel";
import { forReader } from "../format/collate";
import { formatDate, formatNumber } from "../format/format";
import { viewerZone } from "../format/timezone";
import { useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
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
                <p>{t("aiCert.explainHow")}</p>
                <p>{t("aiCert.explainMeaning")}</p>
                <p>{t("aiCert.explainStale")}</p>
                <h3>{t("aiCert.breakdownTitle")}</h3>
                <p className="t-meta">{t("aiCert.breakdownSub")}</p>
                <SiteBreakdown cert={cert} />
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

// Each job broken into the invocation sites it ships, which is where the fold
// on the card came from. A job reads as its worst site, and this is the only
// place a reader can see WHICH part that was and how the others did.
function SiteBreakdown({ cert }: Readonly<{ cert: Certification }>) {
  const t = useT();
  const { locale } = useLocale();
  const multi = cert.jobs.filter(
    (job) => job.sites.length > 1 && JOB_NAME[job.task] !== undefined,
  );
  if (multi.length === 0) {
    return null;
  }
  return (
    <>
      {multi.map((job) => (
        <div key={job.task} className="cell-stack">
          <p className="t-small">{jobName(t, job.task)}</p>
          <ul>
            {job.sites.map((site) => (
              <li key={site.site} className="t-meta">
                {siteName(t, job.task, site.site)} —{" "}
                {t(RESULT_KEY[site.result])}
                {site.runs !== undefined && site.passed !== undefined
                  ? ` (${t("aiCert.runCounts", {
                      passed: formatNumber(site.passed, locale),
                      runs: formatNumber(site.runs, locale),
                    })})`
                  : ""}
              </li>
            ))}
          </ul>
        </div>
      ))}
    </>
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

  // Ordered by the name the reader sees, not by the contract identifier
  // underneath it — which is effectively random to somebody who never learns
  // the identifiers, and the whole point of the card is that they do not.
  const named = cert.jobs
    .filter((job) => JOB_NAME[job.task] !== undefined)
    .sort((a, b) => forReader(jobName(t, a.task), jobName(t, b.task), locale));
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
            render: (job) => jobName(t, job.task),
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
          {t("aiCert.unnamedJobs", {
            count: formatNumber(unnamedCount, locale),
          })}
        </p>
      ) : null}
    </>
  );
}

// A job's name in the reader's language.
//
// The empty string rather than the identifier for a job this build has no
// wording for: JobTable has already dropped those rows, and falling back to
// `site_fact_extract` would defeat what the card is for if one ever got here.
function jobName(t: ReturnType<typeof useT>, task: string): string {
  const key = JOB_NAME[task];
  return key ? t(key) : "";
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
      <Caveats job={job} />
    </span>
  );
}

// Everything a verdict alone would overstate.
//
// Split from the badge and counts above because each line below exists to stop
// one specific misreading, and they are easier to audit as a list than folded
// into the cell that draws the badge.
function Caveats({ job }: Readonly<{ job: Job }>) {
  const t = useT();
  const { locale } = useLocale();
  const lines: string[] = [];

  // A failing verdict beside a high count reads as a contradiction, so the row
  // says WHICH of the two reasons produced it rather than leaving a reader to
  // guess — or, worse, being told something untrue. A verdict is not decided by
  // the pass rate alone: certification also requires the grader's median score
  // to clear a bar, so a job can pass every run and still fail. Saying "one
  // kind of example fails every time" about that job would be false, and it is
  // a real case — draft_reply passes 12 of 12 and is not_supported.
  if (job.result === "not_reliable" && failedSomeRuns(job)) {
    lines.push(t("aiCert.oneKindFails"));
  }
  if (job.result === "not_reliable" && passedEveryRun(job)) {
    lines.push(t("aiCert.gradedBelowBar"));
  }
  if (job.result === "partly_checked" && job.pending_examples) {
    lines.push(
      t("aiCert.pendingExamples", {
        measured: formatNumber(job.measured_examples ?? 0, locale),
        total: formatNumber(
          (job.measured_examples ?? 0) + job.pending_examples,
          locale,
        ),
      }),
    );
  }
  if (job.result === "out_of_date" && job.measured_at) {
    lines.push(
      t("aiCert.measuredOn", {
        date: formatDate(job.measured_at, locale, viewerZone()),
      }),
    );
  }
  // What the measurement FOUND. "Out of date" and "partly checked" describe a
  // measurement's standing, not its finding, so without this a stale failure
  // and a stale success render identically — keeping the reassuring counts and
  // dropping the unflattering verdict. Two committed rows are stale
  // not_supported at 12 of 12 runs passed.
  if (job.measured_result) {
    lines.push(
      t("aiCert.whenMeasuredItRead", {
        finding: t(RESULT_KEY[job.measured_result]),
      }),
    );
  }
  // A measurement exists under a different hosting posture. It does not carry
  // over, and saying nothing would throw away evidence a reader could chase.
  if (job.measured_under_other_profile) {
    lines.push(t("aiCert.otherProfile"));
  }
  // A run that graded one reply of a longer exchange certifies less than the
  // exchange, so the claim is qualified rather than stated whole.
  if (isNarrow(job.scope)) {
    lines.push(t("aiCert.narrowScope"));
  }
  if (job.unmeasured_fallbacks?.length) {
    lines.push(
      t("aiCert.unmeasuredFallback", {
        model: job.unmeasured_fallbacks.join(", "),
      }),
    );
  }
  if (job.worst_site) {
    lines.push(
      t("aiCert.worstSite", { site: siteName(t, job.task, job.worst_site) }),
    );
  }

  return (
    <>
      {lines.map((line) => (
        <span key={line} className="t-meta">
          {line}
        </span>
      ))}
    </>
  );
}

// Which of the two ways a job can be unreliable this one is. Both need the
// counts, and neither may be assumed from the verdict: a job with no counts at
// all gets no explanation rather than the wrong one.
function failedSomeRuns(job: Job): boolean {
  return (
    job.passed !== undefined && job.runs !== undefined && job.passed < job.runs
  );
}

function passedEveryRun(job: Job): boolean {
  return (
    job.passed !== undefined &&
    job.runs !== undefined &&
    job.runs > 0 &&
    job.passed === job.runs
  );
}

// A run that graded less than the site's whole path.
//
// Inverted deliberately: the ONE word meaning full coverage is allowlisted and
// everything else is narrow. Allowlisting the narrow words instead — as this
// first did — fails OPEN, so a fourth scope word added to the task contract
// would render with no caveat and read as full coverage, which is exactly what
// the allowlist was supposed to prevent.
const FULL_SCOPE = "full_invocation";

function isNarrow(scope: string | undefined): boolean {
  return scope !== undefined && scope !== FULL_SCOPE;
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
  "capture_confidentiality_verdict.thread":
    "aiCert.site.capture_confidentiality_verdict.thread",
  "capture_counterparty_verdict.verdict":
    "aiCert.site.capture_counterparty_verdict.verdict",
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
