import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useRecordZone } from "../app/recordzone";
import { Button, Field, Textarea } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { ConfirmModal } from "../design-system/confirmmodal";
import { formatDateTime } from "../format/format";
import { useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { entityTimelineKeys } from "./activitykeys";
import { problemMessageOf, throwProblem } from "./common";

type ProjectFiling = components["schemas"]["ProjectFiling"];
type RefusalCode = components["schemas"]["ProjectFilingRefusal"]["code"];

// Taking an activity back out of a project. Filing under a project marks the
// correspondence as commercial, which erasure and the retention sweep must keep,
// so the undo is a named member's decision with a written reason, and the
// server refuses it whenever anything else still keeps the activity. The screen
// asks the server (`undoable`, `refusal`) rather than re-deriving that rule.

const REFUSAL_COPY: Record<RefusalCode, MessageKey> = {
  not_filed: "projectFiling.refusal.not_filed",
  other_basis_remains: "projectFiling.refusal.other_basis_remains",
  restricted: "projectFiling.refusal.restricted",
  qualifying_deal: "projectFiling.refusal.qualifying_deal",
};

const filingKey = (activityId: string) =>
  ["activity", activityId, "project-filing"] as const;

export function ProjectFilingAction({
  activityId,
  projectId,
}: Readonly<{ activityId: string; projectId: string }>) {
  const t = useT();
  const [open, setOpen] = useState(false);
  return (
    <>
      <Button onClick={() => setOpen(true)}>{t("projectFiling.action")}</Button>
      <ProjectFilingModal
        activityId={activityId}
        projectId={projectId}
        open={open}
        onClose={() => setOpen(false)}
      />
    </>
  );
}

export function ProjectFilingModal({
  activityId,
  projectId,
  open,
  onClose,
}: Readonly<{
  activityId: string;
  projectId: string;
  open: boolean;
  onClose: () => void;
}>) {
  const t = useT();
  const queryClient = useQueryClient();
  const [reason, setReason] = useState("");
  const [undone, setUndone] = useState<ProjectFiling | null>(null);

  const filing = useQuery({
    queryKey: filingKey(activityId),
    enabled: open,
    staleTime: 0,
    gcTime: 0,
    queryFn: async () => {
      const { data, error } = await api.GET("/activities/{id}/project-filing", {
        params: { path: { id: activityId } },
      });
      if (error) throwProblem(error, t);
      return data;
    },
  });

  // The reason arrives as the mutation's variable, never read off render state.
  const undo = useMutation({
    mutationFn: async (request: { activityId: string; reason: string }) => {
      const { data, error } = await api.POST(
        "/activities/{id}/project-filing/undo",
        {
          params: {
            header: { "Idempotency-Key": crypto.randomUUID() },
            path: { id: request.activityId },
          },
          body: { reason: request.reason.trim() },
        },
      );
      if (error) throwProblem(error, t);
      return data;
    },
    onSuccess: (state) => setUndone(state ?? null),
  });

  // The timeline is refreshed when the dialog closes, not when the undo lands:
  // the activity leaves the project's timeline, and with it the row this dialog
  // is mounted in, so refreshing first would take the audit entry off screen
  // before anybody read it.
  const refreshReads = () => {
    for (const key of entityTimelineKeys("project", projectId)) {
      queryClient.invalidateQueries({ queryKey: key });
    }
    queryClient.invalidateQueries({ queryKey: ["activity", activityId] });
  };

  const close = () => {
    if (undo.isPending) return;
    if (undone) refreshReads();
    undo.reset();
    setUndone(null);
    setReason("");
    onClose();
  };

  const state = undone ?? filing.data;
  const canUndo = !undone && Boolean(state?.filed && state.undoable);
  return (
    <ConfirmModal
      open={open}
      onClose={close}
      title={t(undone ? "projectFiling.doneTitle" : "projectFiling.title")}
      confirmLabel={t(undone ? "common.close" : "projectFiling.confirm")}
      confirmVariant={undone ? "primary" : "danger"}
      confirmDisabled={!undone && !canUndo}
      confirmReason={
        canUndo && !reason.trim()
          ? t("projectFiling.reasonRequired")
          : undefined
      }
      onConfirm={() => (undone ? close() : undo.mutate({ activityId, reason }))}
      pending={undo.isPending}
      error={undo.isError ? problemMessageOf(undo.error, t) : undefined}
    >
      {filing.isError && !state ? (
        <Callout
          tone="danger"
          kind="outcome"
          title={problemMessageOf(filing.error, t)}
        />
      ) : (
        <FilingBody
          state={state}
          done={Boolean(undone)}
          reason={reason}
          onReason={setReason}
        />
      )}
    </ConfirmModal>
  );
}

function FilingBody({
  state,
  done,
  reason,
  onReason,
}: Readonly<{
  state: ProjectFiling | undefined;
  done: boolean;
  reason: string;
  onReason: (reason: string) => void;
}>) {
  const t = useT();
  if (!state) {
    return <p className="t-caption">{t("projectFiling.loading")}</p>;
  }
  const projects = state.projects.map((project) => project.name).join(", ");
  return (
    <div className="compose-fields">
      {done && (
        <Callout
          tone="success"
          kind="outcome"
          title={t("projectFiling.done")}
        />
      )}
      {!done && !state.filed && (
        <Callout
          tone="info"
          kind="standing"
          title={t("projectFiling.refusal.not_filed")}
        />
      )}
      {!done && state.filed && state.refusal && (
        <Callout
          tone="warning"
          kind="standing"
          title={t(REFUSAL_COPY[state.refusal.code])}
        />
      )}
      {!done && state.undoable && (
        <>
          <p className="t-body">{t("projectFiling.explain", { projects })}</p>
          <Field
            label={t("projectFiling.reason")}
            hint={t("projectFiling.reasonHint")}
            required
          >
            {(control) => (
              <Textarea
                {...control}
                value={reason}
                onChange={(event) => onReason(event.target.value)}
              />
            )}
          </Field>
        </>
      )}
      <Decisions decisions={state.undone} />
    </div>
  );
}

// The audit entry of every undo on this activity: who decided, when, and the
// words they wrote. Read from the audit log itself, so what is shown is what
// was committed.
function Decisions({
  decisions,
}: Readonly<{ decisions: ProjectFiling["undone"] }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useRecordZone();
  if (decisions.length === 0) return null;
  return (
    <section aria-label={t("projectFiling.decisions")}>
      <p className="t-label">{t("projectFiling.decisions")}</p>
      <ul>
        {decisions.map((decision) => (
          <li key={`${decision.at}-${decision.by_name}`}>
            <span className="t-body">
              {t("projectFiling.decision", {
                name: decision.by_name,
                when: formatDateTime(decision.at, locale, zone),
              })}
            </span>
            <p className="t-caption">{decision.reason}</p>
          </li>
        ))}
      </ul>
    </section>
  );
}
