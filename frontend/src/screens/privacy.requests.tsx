// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useCanWrite } from "../app/capability";
import { Badge, Button, Field, Textarea } from "../design-system/atoms";
import { DataTable, type DataTableColumn } from "../design-system/datatable";
import { DrawerBody, DrawerHead } from "../design-system/drawerbands";
import { ErrorLine } from "../design-system/errorline";
import { type Fact, FactList } from "../design-system/factlist";
import { Heading } from "../design-system/heading";
import { Modal } from "../design-system/modal";
import { Select, type SelectOption } from "../design-system/select";
import { formatDate } from "../format/format";
import { viewerZone } from "../format/timezone";
import { type Translator, useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { ProblemError, problemMessageOf, throwProblem } from "./common";
import {
  EntityRef,
  RosterPartialNote,
  rosterOwnerName,
  useRoster,
  useRosterPartial,
} from "./entityref";
import { useMemberName } from "./membernames";
import {
  DSR_KIND_LABEL,
  DSR_STATUS_LABEL,
  DSR_STATUS_TONE,
  type DsrStatus,
  isIllegalTransition,
  isOverdue,
  isTerminal,
  nextStatuses,
} from "./privacy.logic";
import "./privacy.css";

export type DataSubjectRequest = components["schemas"]["DataSubjectRequest"];
type UpdateDataSubjectRequest =
  components["schemas"]["UpdateDataSubjectRequest"];
type User = components["schemas"]["User"];

// A contact id, as opposed to an external identifier typed in by hand.
const SUBJECT_UUID_RE =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

// The label when the subject resolves; a contact id that does not resolve is
// withheld, and an external identifier reads as it was written.
export function subjectName(dsr: DataSubjectRequest, t: Translator): string {
  if (dsr.subject_label) {
    return dsr.subject_label;
  }
  return SUBJECT_UUID_RE.test(dsr.subject_ref)
    ? t("notice.contactHidden")
    : dsr.subject_ref;
}

export function DsrTable({
  rows,
  nowMs,
  onOpen,
}: Readonly<{
  rows: DataSubjectRequest[];
  nowMs: number;
  onOpen: (id: string) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  // A statutory deadline, read on the viewer's own calendar day.
  const tz = viewerZone();
  const columns: DataTableColumn<DataSubjectRequest>[] = [
    {
      key: "subject",
      header: t("privacy.subject"),
      render: (dsr) => (
        <button
          type="button"
          className="cell-link privacy-wrap"
          aria-haspopup="dialog"
          onClick={() => onOpen(dsr.id)}
        >
          {subjectName(dsr, t)}
        </button>
      ),
    },
    {
      key: "kind",
      header: t("privacy.kind"),
      render: (dsr) => t(DSR_KIND_LABEL[dsr.kind]),
    },
    {
      key: "status",
      header: t("privacy.status"),
      fold: "end",
      render: (dsr) => <DsrStatusBadge status={dsr.status} />,
    },
    {
      key: "due",
      header: t("privacy.dueAt"),
      align: "end",
      render: (dsr) => (
        <span className="dsr-due">
          <span>{formatDate(dsr.due_at, locale, tz)}</span>
          {isOverdue(dsr.due_at, dsr.status, nowMs) && (
            <Badge tone="danger">{t("privacy.overdue")}</Badge>
          )}
        </span>
      ),
    },
    {
      key: "assignee",
      header: t("privacy.assignee"),
      render: (dsr) =>
        dsr.assignee_id ? (
          <EntityRef kind="user" id={dsr.assignee_id} />
        ) : (
          <span className="t-caption">{t("notice.unassigned")}</span>
        ),
    },
  ];
  return (
    <DataTable
      label={t("settings.privacy")}
      bleed
      fold
      columns={columns}
      rows={rows}
      rowKey={(dsr) => dsr.id}
    />
  );
}

function DsrStatusBadge({ status }: Readonly<{ status: DsrStatus }>) {
  const t = useT();
  return (
    <Badge tone={DSR_STATUS_TONE[status]}>{t(DSR_STATUS_LABEL[status])}</Badge>
  );
}

// nextStatuses never routes back to open, so three verbs cover every move.
function transitionLabelKey(status: DsrStatus): MessageKey {
  if (status === "in_progress") return "privacy.inProgress";
  if (status === "fulfilled") return "privacy.fulfil";
  return "privacy.reject";
}

// Who a request can be handed to, led by the unassigned pool and, when the
// roster does not offer them, the request's own holder.
function assigneeOptions(
  users: readonly User[],
  current: SelectOption | null,
  t: Translator,
): SelectOption[] {
  return [
    { value: "", label: t("notice.unassigned") },
    ...(current ? [current] : []),
    ...users.map((user) => ({ value: user.id, label: user.display_name })),
  ];
}

// The holder as an option when the picker does not offer them. They may be
// deactivated, past the roster walk's bound, or an agent seat nobody may pick.
function useUnofferedAssignee(
  assigneeId: string | null | undefined,
  offered: readonly User[],
  t: Translator,
): SelectOption | null {
  const unoffered =
    Boolean(assigneeId) && !offered.some((member) => member.id === assigneeId);
  const name = useMemberName(unoffered ? assigneeId : null);
  if (!assigneeId || !unoffered) {
    return null;
  }
  return {
    value: assigneeId,
    label: rosterOwnerName(assigneeId, name, t, t("ref.notInRoster")),
    disabled: true,
  };
}

// One request, worked beside the queue. The erasure fulfil hands off to the
// typed-confirmation dialog at the card root, carrying the drafted answer.
export function DsrDetail({
  dsr,
  titleId,
  nowMs,
  onClose,
  onFulfilErasure,
}: Readonly<{
  dsr: DataSubjectRequest;
  titleId: string;
  nowMs: number;
  onClose: () => void;
  onFulfilErasure: (dsr: DataSubjectRequest, resolution: string) => void;
}>) {
  const t = useT();
  const queryClient = useQueryClient();
  const canWork = useCanWrite("privacy_request", "update");
  const [resolution, setResolution] = useState(dsr.resolution ?? "");

  const patch = useMutation({
    mutationFn: async (vars: {
      id: string;
      body: UpdateDataSubjectRequest;
    }) => {
      const { data, error } = await api.PATCH("/data-subject-requests/{id}", {
        params: { path: { id: vars.id } },
        body: vars.body,
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["dsrs"] }),
    // Only the moved-on race re-reads; a 403 or a 500 stays explained.
    onError: (error) => {
      if (error instanceof ProblemError && isIllegalTransition(error.problem)) {
        void queryClient.invalidateQueries({ queryKey: ["dsrs"] });
      }
    },
  });

  function transition(next: DsrStatus) {
    const answer = resolution.trim();
    if (dsr.kind === "erasure" && next === "fulfilled") {
      onFulfilErasure(dsr, answer);
      return;
    }
    // Omitted when blank: an empty string is a value the server would write.
    patch.mutate({
      id: dsr.id,
      body: answer ? { status: next, resolution: answer } : { status: next },
    });
  }

  const problem =
    patch.error instanceof ProblemError ? patch.error.problem : null;
  const movedOn = problem !== null && isIllegalTransition(problem);
  const open = canWork && !isTerminal(dsr.status);

  return (
    <Modal open onClose={onClose} labelledBy={titleId} intent="drawer">
      <DrawerHead>
        <Heading
          size="large"
          id={titleId}
          tabIndex={-1}
          className="t-h2 modal-title privacy-wrap"
        >
          {subjectName(dsr, t)}
        </Heading>
      </DrawerHead>
      <DrawerBody>
        <div className="form-stack">
          <DsrFacts dsr={dsr} nowMs={nowMs} canWork={canWork} />
          {canWork && (
            <DsrAssignee
              dsr={dsr}
              pending={patch.isPending}
              onAssign={(assignee) =>
                patch.mutate({ id: dsr.id, body: { assignee_id: assignee } })
              }
            />
          )}
          {/* Announced, never standing: the badges do not move on a refused
              write, so this line alone says the press changed nothing. */}
          {patch.isError && (
            <ErrorLine>
              {movedOn
                ? t("privacy.movedOn")
                : problemMessageOf(patch.error, t)}
            </ErrorLine>
          )}
          {isTerminal(dsr.status) && <p>{t("privacy.closed")}</p>}
          {open && (
            <Field
              label={t("privacy.resolution")}
              hint={t("privacy.resolutionRequired")}
            >
              {(control) => (
                <Textarea
                  {...control}
                  value={resolution}
                  onChange={(event) => {
                    setResolution(event.target.value);
                    if (patch.isError) patch.reset();
                  }}
                />
              )}
            </Field>
          )}
          {open && (
            <DsrTransitions
              status={dsr.status}
              answered={Boolean(resolution.trim() || dsr.resolution)}
              pending={patch.isPending}
              onTransition={transition}
            />
          )}
        </div>
      </DrawerBody>
    </Modal>
  );
}

function DsrFacts({
  dsr,
  nowMs,
  canWork,
}: Readonly<{ dsr: DataSubjectRequest; nowMs: number; canWork: boolean }>) {
  const t = useT();
  const { locale } = useLocale();
  const tz = viewerZone();
  const facts: Fact[] = [
    {
      key: "subject",
      term: t("privacy.subject"),
      value: SUBJECT_UUID_RE.test(dsr.subject_ref) ? (
        <EntityRef
          kind="contact"
          id={dsr.subject_ref}
          name={dsr.subject_label ?? t("notice.contactHidden")}
          newTab
        />
      ) : (
        <span className="privacy-wrap">{dsr.subject_ref}</span>
      ),
    },
    {
      key: "kind",
      term: t("privacy.kind"),
      value: t(DSR_KIND_LABEL[dsr.kind]),
    },
    {
      key: "status",
      term: t("privacy.status"),
      value: <DsrStatusBadge status={dsr.status} />,
    },
    {
      key: "due",
      term: t("privacy.dueAt"),
      value: (
        <span className="dsr-due">
          <span>{formatDate(dsr.due_at, locale, tz)}</span>
          {isOverdue(dsr.due_at, dsr.status, nowMs) && (
            <Badge tone="danger">{t("privacy.overdue")}</Badge>
          )}
        </span>
      ),
    },
  ];
  // The picker names the holder for a seat that may work the request.
  if (!canWork) {
    facts.push({
      key: "assignee",
      term: t("privacy.assignee"),
      value: dsr.assignee_id ? (
        <EntityRef kind="user" id={dsr.assignee_id} />
      ) : (
        t("notice.unassigned")
      ),
    });
  }
  if (dsr.resolution && isTerminal(dsr.status)) {
    facts.push({
      key: "resolution",
      term: t("privacy.resolution"),
      value: dsr.resolution,
    });
  }
  return <FactList facts={facts} />;
}

function DsrAssignee({
  dsr,
  pending,
  onAssign,
}: Readonly<{
  dsr: DataSubjectRequest;
  pending: boolean;
  onAssign: (assignee: string | null) => void;
}>) {
  const t = useT();
  const roster = useRoster("user", true);
  const partial = useRosterPartial("user", true);
  // An agent seat cannot hold the row scope a subject request needs.
  const assignable = (roster.data ?? []).flatMap((entry) =>
    "display_name" in entry && !entry.is_agent ? [entry] : [],
  );
  const current = useUnofferedAssignee(dsr.assignee_id, assignable, t);
  return (
    <div className="field">
      <Field label={t("privacy.assignee")}>
        {(control) => (
          <Select
            {...control}
            options={assigneeOptions(assignable, current, t)}
            value={dsr.assignee_id ?? ""}
            disabled={pending}
            onChange={(next) => {
              if (next !== (dsr.assignee_id ?? "")) onAssign(next || null);
            }}
          />
        )}
      </Field>
      <RosterPartialNote partial={partial} />
    </div>
  );
}

// A close needs its answer: the draft in the field or the one already stored.
function DsrTransitions({
  status,
  answered,
  pending,
  onTransition,
}: Readonly<{
  status: DsrStatus;
  answered: boolean;
  pending: boolean;
  onTransition: (next: DsrStatus) => void;
}>) {
  const t = useT();
  return (
    <div className="dsr-actions">
      {nextStatuses(status).map((next) => (
        <Button
          key={next}
          disabled={
            pending ||
            ((next === "fulfilled" || next === "rejected") && !answered)
          }
          onClick={() => onTransition(next)}
        >
          {t(transitionLabelKey(next))}
        </Button>
      ))}
    </div>
  );
}
