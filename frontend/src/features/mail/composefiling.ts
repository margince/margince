// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { useCallback, useEffect, useRef, useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { useRoster } from "../../components/entityref";
import {
  liveProjects,
  type PickableProject,
  useSoleProjectDefault,
} from "../../design-system/projectpicker";
import { unwrap } from "../../lib/problem";
import { useCompany360 } from "../companies/company360";
import { useContact360 } from "../contacts/contact360";
import {
  stripSubjectTag,
  subjectTag,
  useProjectRecord,
  withSubjectTag,
} from "../projects/projectrecord";
import { useThreadProject } from "./composeanchor";
import { recipientSuggestions } from "./composehead";
import { deadRecipientsAmong } from "./composereachability";
import type { RelinkKind } from "./composerelink";

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
      return unwrap(
        await api.GET("/deals/{id}", {
          params: { path: { id: entityId } },
        }),
      );
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
 * The rep chooses and the composer suggests. A thread filed on the deal the
 * rep writes from keeps its own project. A thread filed elsewhere can belong
 * to another deal of the same customer, so the deal's own project wins over
 * it. A suggestion is adopted once, so a rep who picks None is not overruled
 * later.
 *
 * The tag stays in the subject while a project is chosen and the switch is on.
 * It returns whenever a draft or a retype replaces the subject. The switch
 * turned off keeps it out the same way; picking None removes it.
 */
// The rep's switch, or a restored subject still to be read against the key.
type TagChoice = boolean | { saved: string };

function useProjectFiling(input: {
  activityId?: string;
  /** The deal the message is written from, when it is one. */
  anchorDealId?: string;
  anchorProjectId?: string | null;
  anchorSettled: boolean;
  projects: readonly PickableProject[];
  subject: string;
  setSubject: (next: string) => void;
  /** Whether the composer is showing; a shut one reads nothing. */
  open: boolean;
}) {
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
  const threadOnAnchor = (thread.activity?.links ?? []).some(
    (link) =>
      link.entity_type === "deal" && link.entity_id === input.anchorDealId,
  );
  const suggested = settled
    ? threadOnAnchor
      ? (thread.projectId ?? input.anchorProjectId ?? "")
      : (input.anchorProjectId ?? thread.projectId ?? "")
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
  const ownTag = subjectTag(project);
  // Off holds for one message and is never stored. The tag files the client's
  // answer, so the next message carries it again.
  const [choices, setChoices] = useState<Record<string, TagChoice>>({});
  const choice = choices[filingKey];
  const setTagged = useCallback(
    (next: boolean) => {
      setChoices((current) => ({ ...current, [filingKey]: next }));
    },
    [filingKey],
  );
  const adoptSavedSubject = useCallback(
    (saved: string) => {
      setChoices((current) => ({ ...current, [filingKey]: { saved } }));
    },
    [filingKey],
  );
  // A restored subject is read against this project's own key, so a foreign
  // key stays the rep's text. Until that key is known, nothing is decided.
  const awaiting = typeof choice === "object" && !ownTag;
  const tagged =
    typeof choice === "object"
      ? ownTag !== "" && choice.saved.startsWith(ownTag)
      : choice !== false;
  const tag = tagged ? ownTag : "";
  const subject = input.subject;
  const setSubject = input.setSubject;
  const open = input.open;
  const previousTag = useRef("");
  useEffect(() => {
    // A shut composer reads no project, so this rule would strip the tag off
    // a draft nobody is looking at.
    if (!open || awaiting) return;
    const priorTag = previousTag.current;
    previousTag.current = tag;
    const withoutOld = priorTag ? stripSubjectTag(subject, priorTag) : subject;
    const wanted = tag
      ? withSubjectTag(withoutOld, tag)
      : stripSubjectTag(withoutOld, ownTag);
    if (wanted !== subject) {
      setSubject(wanted);
    }
  }, [open, awaiting, tag, ownTag, subject, setSubject]);
  return {
    projectId,
    setProjectId: setPicked,
    /** Empty when the chosen project has no tag to offer. */
    ownTag,
    tagged,
    setTagged,
    adoptSavedSubject,
  };
}

/**
 * The contact an address belongs to, read off the two 360s the composer already
 * holds. Addresses compare case-blind, the way a mailbox does. An address
 * neither names is no contact the composer can record anything on.
 */
export function contactsByAddress(
  contact: ReturnType<typeof useContact360>["data"],
  company: ReturnType<typeof useCompany360>["data"],
): (address: string) => string | undefined {
  const owners = new Map<string, string>();
  const own = (address: string | null | undefined, contactId: string) => {
    const key = address?.trim().toLowerCase();
    if (key && !owners.has(key)) owners.set(key, contactId);
  };
  const subject = contact?.contact;
  for (const address of subject?.emails ?? []) {
    if (subject) own(address.email, subject.id);
  }
  for (const member of company?.contacts?.data ?? []) {
    own(member.primary_email, member.contact_id);
  }
  return (address) => owners.get(address.trim().toLowerCase());
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
  const colleagues = useRoster("user", open && !isChannelReply);
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
    anchorDealId: entityType === "deal" ? entityId : undefined,
    anchorProjectId: anchorProject.projectId,
    anchorSettled: anchorProject.settled,
    projects: reachableProjects,
    subject,
    setSubject,
    open,
  });
  return {
    // Whose conversations the rail lists, where the page's own read says.
    recordName:
      entityType === "contact"
        ? contact.data?.contact?.full_name
        : anchorCompany.data?.company?.display_name,
    contactOf: contactsByAddress(contact.data, anchorCompany.data),
    reachableProjects,
    projectFiling,
    // Addresses known not to arrive, warned under the fields.
    deadRecipients: deadRecipientsAmong(contact.data, [...addressed]),
    // The contact first, then the account's roster, then the workspace's
    // members. The reads already run (the members' cache entry is shared with
    // the anchor lookup), so the offer costs no request of its own.
    recipients: recipientSuggestions(
      contact.data,
      anchorCompany.data,
      colleagues.data,
    ),
  };
}

export type ComposeRecords = ReturnType<typeof useComposeRecords>;
