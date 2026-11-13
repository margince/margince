// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Globe, type LucideIcon, PenLine } from "lucide-react";
import { api } from "../api/client";
import { AppLink } from "../design-system/applink";
import { Button } from "../design-system/atoms";
import { ErrorLine } from "../design-system/errorline";
import { formatNumber } from "../format/format";
import { useLocale, usePlural, useT } from "../i18n";
import { pendingApprovalCountKey } from "../lib/approvals.queries";
import { problemMessageOf, unwrap } from "../lib/problem";
import type { Route } from "../lib/router";
import { worklistKey } from "../lib/workkeys";

/** How many slipping deals one press drafts for: a batch a reader can read. */
const FOLLOW_UP_BATCH = 5;

/** What a finished job says, and where its work can be seen. */
type Outcome = Readonly<{ said: string; href?: string; open?: string }>;

export type AgentJob = Readonly<{
  id: "followUps" | "enrichCompany";
  /** The verb on the button. */
  label: string;
  /** What the orb says while it runs. */
  running: string;
  pending: boolean;
  outcome?: Outcome;
  /** Why it could not be done, in the reader's words. */
  refused?: string;
  run: () => void;
}>;

/**
 * The work a reader can hand the agent from where they stand. Each is a job
 * the server already runs for an agent tool, so the orb and a connected
 * assistant do the same thing. Nothing here sends or saves on its own: drafts
 * wait on the deal, and what a website said waits for approval.
 */
export function useAgentJobs(route: Route): readonly AgentJob[] {
  const company =
    route.screen === "companies" && route.id !== undefined
      ? route.id
      : undefined;
  const enrich = useEnrichJob(company);
  const followUps = useFollowUpsJob();
  return enrich ? [enrich, followUps] : [followUps];
}

/** Drafts a follow-up on each of the worst slipping deals. */
function useFollowUpsJob(): AgentJob {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const client = useQueryClient();
  const write = useMutation({
    mutationFn: async (limit: number) =>
      unwrap(
        await api.POST("/deals/follow-up-drafts", {
          body: { segment: "slipping", limit },
        }),
      ),
    onSuccess: () => void client.invalidateQueries({ queryKey: worklistKey }),
  });
  const drafted = write.data?.drafts;
  const [only] = drafted ?? [];
  const outcome = (): Outcome | undefined => {
    if (drafted === undefined) return undefined;
    if (drafted.length === 0) return { said: t("agent.job.followUps.none") };
    return {
      said: plural("agent.job.followUps.done", drafted.length, {
        count: formatNumber(drafted.length, locale),
      }),
      href:
        drafted.length === 1 && only ? `#/deals/${only.deal_id}` : "#/deals",
      open: t("agent.job.followUps.open"),
    };
  };
  return {
    id: "followUps",
    label: t("agent.job.followUps"),
    running: t("agent.job.followUps.running"),
    pending: write.isPending,
    outcome: outcome(),
    refused: write.isError ? problemMessageOf(write.error, t) : undefined,
    run: () => write.mutate(FOLLOW_UP_BATCH),
  };
}

/** Reads the open company's website into a proposal; absent off a company. */
function useEnrichJob(company: string | undefined): AgentJob | undefined {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const client = useQueryClient();
  const read = useMutation({
    mutationFn: async (id: string) =>
      unwrap(
        await api.POST("/companies/{id}/enrich", { params: { path: { id } } }),
      ),
    onSuccess: () => {
      void client.invalidateQueries({ queryKey: pendingApprovalCountKey });
      void client.invalidateQueries({ queryKey: worklistKey });
    },
  });
  if (company === undefined) return undefined;
  // A result is about the company it was read for, not the one now open.
  const mine = read.variables === company;
  const found = mine ? read.data?.fields.length : undefined;
  return {
    id: "enrichCompany",
    label: t("agent.job.enrich"),
    running: t("agent.job.enrich.running"),
    pending: read.isPending,
    outcome:
      found === undefined
        ? undefined
        : {
            said: plural("agent.job.enrich.done", found, {
              count: formatNumber(found, locale),
            }),
            href: "#/worklist",
            open: t("agent.job.enrich.open"),
          },
    refused: mine && read.isError ? problemMessageOf(read.error, t) : undefined,
    run: () => read.mutate(company),
  };
}

const JOB_MARK: Readonly<Record<AgentJob["id"], LucideIcon>> = {
  followUps: PenLine,
  enrichCompany: Globe,
};

/** One job as a recommendation: what it does, then the press that starts it. */
export function AgentJobRow({ job }: Readonly<{ job: AgentJob }>) {
  const t = useT();
  const Mark = JOB_MARK[job.id];
  return (
    <li className="aradvice">
      <span className="aradvicemark aradviceai" aria-hidden="true">
        <Mark />
      </span>
      <div className="aradvicetext">
        <p className="aradvicename">{job.label}</p>
        {!job.pending && job.outcome && (
          <p className="t-caption arjobsaid" role="status">
            {job.outcome.said}{" "}
            {job.outcome.href && (
              <AppLink className="link-button" href={job.outcome.href}>
                {job.outcome.open}
              </AppLink>
            )}
          </p>
        )}
        {!job.pending && <ErrorLine>{job.refused}</ErrorLine>}
      </div>
      <Button
        variant="aiQuiet"
        pending={job.pending}
        busyLabel={job.running}
        onClick={job.run}
        aria-label={t("agent.job.startNamed", { job: job.label })}
      >
        {t("agent.job.start")}
      </Button>
    </li>
  );
}
