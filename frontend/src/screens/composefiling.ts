// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { useCallback, useEffect, useRef, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import {
  liveProjects,
  type PickableProject,
  useSoleProjectDefault,
} from "../design-system/projectpicker";
import { throwProblem } from "./common";
import { useCompany360 } from "./company360";
import { useThreadProject } from "./composeanchor";
import { recipientSuggestions } from "./composehead";
import { deadRecipientsAmong } from "./composereachability";
import type { RelinkKind } from "./composerelink";
import { useContact360 } from "./contact360";
import {
  stripSubjectTag,
  subjectTag,
  useProjectRecord,
  withSubjectTag,
} from "./projectrecord";

// A project record as the one row its own picker offers. `liveProjects` still
// filters it, so a closed project gets no picker. Nothing is offered while the
// read is out.
function projectItself(
  project: components["schemas"]["Project"] | null,
): PickableProject[] {
  if (!project) {
    return [];
  }
  return [
    {
      project_id: project.id,
      name: project.name,
      key: project.key,
      phase: project.phase,
    },
  ];
}

// The project and company the anchor record names. A deal carries both as
// columns; a project names itself and a company is its own account.
function useAnchorProject(
  entityType: RelinkKind,
  entityId: string,
): { projectId?: string | null; companyId?: string; settled: boolean } {
  const query = useQuery({
    // The deal page's own key, so the composer costs no request there and
    // cannot disagree with the page behind it.
    queryKey: ["deal", entityId],
    queryFn: async () => {
      const { data, error } = await api.GET("/deals/{id}", {
        params: { path: { id: entityId } },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    enabled: entityType === "deal",
    staleTime: 60_000,
  });
  return {
    projectId: entityType === "project" ? entityId : query.data?.project_id,
    companyId:
      entityType === "deal"
        ? (query.data?.company_id ?? undefined)
        : entityType === "company"
          ? entityId
          : undefined,
    // A failed read says nothing about the deal's project. Unsettled keeps
    // the composer quiet rather than dropping the filing.
    settled: entityType !== "deal" || (!query.isPending && !query.isError),
  };
}

/**
 * Which project this message files under, and the subject tag that follows it.
 *
 * The rep chooses and the composer suggests: the thread's project when the
 * conversation is filed, otherwise the anchor's. A suggestion is adopted once,
 * so a rep who picks None is not overruled later.
 *
 * The tag stays in the subject while a project is chosen, and returns whenever
 * a draft or a retype replaces the subject. The picker removes it.
 */
function useProjectFiling(input: {
  activityId?: string;
  anchorProjectId?: string | null;
  anchorSettled: boolean;
  projects: readonly PickableProject[];
  subject: string;
  setSubject: (next: string) => void;
  /** Whether the composer is showing; a shut one reads nothing. */
  open: boolean;
}): { projectId: string; setProjectId: (next: string) => void } {
  const thread = useThreadProject(input.activityId, input.open);
  // An empty string is a real answer ("None"), so unanswered is undefined.
  const filingKey = input.activityId ?? "";
  const [picks, setPicks] = useState<Record<string, string>>({});
  const picked = picks[filingKey];
  const setPicked = useCallback(
    (next: string) => {
      setPicks((current) => ({ ...current, [filingKey]: next }));
    },
    [filingKey],
  );
  const settled = thread.settled && input.anchorSettled;
  const suggested = settled
    ? (thread.projectId ?? input.anchorProjectId ?? "")
    : "";
  // Each message owns its filing choice, including an explicit No project.
  useEffect(() => {
    if (suggested && picked === undefined) setPicked(suggested);
  }, [suggested, picked, setPicked]);
  const chosen = picked ?? suggested;
  // The account path has nothing to suggest from, so a company with one live
  // project defaults to it. The two rules never fire together.
  useSoleProjectDefault(input.projects, chosen, setPicked, filingKey);
  // A value the option list lacks shows as no choice. Writing that back would
  // race the adoption above, so it is only read.
  const offered =
    chosen === "" ||
    input.projects.some((project) => project.project_id === chosen);
  const projectId = offered ? chosen : "";
  const { project } = useProjectRecord(projectId || undefined, input.open);
  const tag = subjectTag(project);
  const setSubject = input.setSubject;
  const subject = input.subject;
  const open = input.open;
  const previousTag = useRef("");
  useEffect(() => {
    // A shut composer reads no project, so this rule would strip the tag off
    // a draft nobody is looking at.
    if (!open) return;
    const priorTag = previousTag.current;
    previousTag.current = tag;
    const withoutOld = priorTag ? stripSubjectTag(subject, priorTag) : subject;
    const wanted = tag ? withSubjectTag(withoutOld, tag) : withoutOld;
    if (wanted !== subject) {
      setSubject(wanted);
    }
  }, [open, tag, subject, setSubject]);
  return { projectId, setProjectId: setPicked };
}

// Which projects a message may file under, per record kind: a contact's own,
// a project itself, and otherwise the account's.
function reachableFrom(
  entityType: RelinkKind,
  reads: Readonly<{
    contact: ReturnType<typeof useContact360>;
    ownProject: ReturnType<typeof useProjectRecord>;
    anchorCompany: ReturnType<typeof useCompany360>;
  }>,
): PickableProject[] {
  if (entityType === "contact") {
    return liveProjects(reads.contact.data?.projects);
  }
  if (entityType === "project") {
    return liveProjects(projectItself(reads.ownProject.project));
  }
  return liveProjects(reads.anchorCompany.data?.projects);
}

// The records around the message: its contact, its account, the projects it
// may file under, and the addresses the fields offer.
export function useComposeRecords({
  open,
  isChannelReply,
  entityType,
  entityId,
  recipientId,
  answering,
  subject,
  setSubject,
  addressed,
}: Readonly<{
  open: boolean;
  isChannelReply: boolean;
  entityType: RelinkKind;
  entityId: string;
  /** The contact an account draft picked; it outranks the page's record. */
  recipientId: string;
  answering: string | undefined;
  subject: string;
  setSubject: (next: string) => void;
  /** Everybody on the message: To, Cc and Bcc. */
  addressed: readonly string[];
}>) {
  // A channel reply's recipient is resolved on the server, so it asks nothing.
  const recipientContact = isChannelReply
    ? undefined
    : ((recipientId || undefined) ??
      (entityType === "contact" ? entityId : undefined));
  const contact = useContact360(
    recipientContact ?? "",
    open && recipientContact != null,
  );
  const anchorProject = useAnchorProject(entityType, entityId);
  // A project page's message is about that project, read as a record so the
  // picker names it the way the page does.
  const ownProject = useProjectRecord(
    entityType === "project" ? entityId : undefined,
    open,
  );
  // The account around the message, under the account page's own key. Its 360
  // includes the projects the company works as a partner or subcontractor.
  const anchorCompany = useCompany360(
    (entityType === "project"
      ? ownProject.project?.company_id
      : anchorProject.companyId) ?? "",
  );
  const reachableProjects = reachableFrom(entityType, {
    contact,
    ownProject,
    anchorCompany,
  });
  const projectFiling = useProjectFiling({
    activityId: answering,
    anchorProjectId: anchorProject.projectId,
    anchorSettled: anchorProject.settled,
    projects: reachableProjects,
    subject,
    setSubject,
    open,
  });
  return {
    reachableProjects,
    projectFiling,
    // Addresses known not to arrive, warned under the fields.
    deadRecipients: deadRecipientsAmong(contact.data, [...addressed]),
    // The contact first, then the account's roster. Both reads already run, so
    // the offer costs no request and agrees with the page behind it.
    recipients: recipientSuggestions(contact.data, anchorCompany.data),
  };
}

export type ComposeRecords = ReturnType<typeof useComposeRecords>;
