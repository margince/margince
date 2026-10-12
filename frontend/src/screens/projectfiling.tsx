import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { type RefObject, useEffect, useRef, useState } from "react";
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
import { problemMessageOf, unwrap } from "./common";

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
  archived: "projectFiling.refusal.archived",
  erasure_pending: "projectFiling.refusal.erasure_pending",
  legal_hold: "projectFiling.refusal.legal_hold",
  hidden_project: "projectFiling.refusal.hidden_project",
  qualifying_deal: "projectFiling.refusal.qualifying_deal",
};

const filingKey = (activityId: string) =>
  ["activity", activityId, "project-filing"] as const;

export function ProjectFilingAction({
  activityId,
  projectId,
  subject,
}: Readonly<{ activityId: string; projectId: string; subject?: string }>) {
  const t = useT();
  const [open, setOpen] = useState(false);
  return (
    <>
      {/* Named by the activity it acts on: a timeline repeats this button on
          every row, and a screen reader's list of buttons is otherwise a column
          of identical labels. */}
      <Button
        aria-label={
          subject ? t("projectFiling.actionFor", { subject }) : undefined
        }
        onClick={() => setOpen(true)}
      >
        {t("projectFiling.action")}
      </Button>
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
  const reasonBox = useRef<HTMLTextAreaElement>(null);

  const filing = useQuery({
    queryKey: filingKey(activityId),
    enabled: open,
    staleTime: 0,
    gcTime: 0,
    queryFn: async () => {
      return unwrap(
        await api.GET("/activities/{id}/project-filing", {
          params: { path: { id: activityId } },
        }),
        t,
      );
    },
  });

  // The reason arrives as the mutation's variable, never read off render state.
  const undo = useMutation({
    mutationFn: async (request: { activityId: string; reason: string }) => {
      return unwrap(
        await api.POST("/activities/{id}/project-filing/undo", {
          params: { path: { id: request.activityId } },
          body: { reason: request.reason.trim() },
        }),
        t,
      );
    },
    onSuccess: (state) => setUndone(state ?? null),
  });

  // The timeline is refreshed when the dialog closes, not when the undo lands:
  // the activity leaves the project's timeline, and with it the row this dialog
  // is mounted in, so refreshing first would take the audit entry off screen
  // before anybody read it.
  const refreshReads = () => {
    // The timeline keys are prefixes of the filtered and paged reads under them,
    // so they refetch all of those; the activity's own key is exact so the filing
    // query beside it is not refetched as the dialog closes.
    for (const key of entityTimelineKeys("project", projectId)) {
      queryClient.invalidateQueries({ queryKey: key });
    }
    queryClient.invalidateQueries({
      queryKey: ["activity", activityId],
      exact: true,
    });
  };

  const close = () => {
    if (undone) refreshReads();
    undo.reset();
    setUndone(null);
    setReason("");
    onClose();
  };

  // A cached verdict is not shown while a fresh one is being read: it may say
  // the filing is undoable after it has been undone, and a second undo would
  // be sent from it.
  const state = undone ?? (filing.isFetching ? undefined : filing.data);
  const canUndo = !undone && Boolean(state?.filed && state.undoable);
  // The reason field exists only once the verdict has arrived, after the dialog
  // has already taken focus, so focus follows it in.
  useEffect(() => {
    if (open && canUndo) reasonBox.current?.focus();
  }, [open, canUndo]);
  return (
    <ConfirmModal
      open={open}
      onClose={close}
      // Null until the form has rendered: the verdict arrives after the dialog
      // opens, and a refusal has no field to focus.
      initialFocusTo={() => reasonBox.current}
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
      {filing.isError && !state && !filing.isFetching ? (
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
          reasonBox={reasonBox}
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
  reasonBox,
}: Readonly<{
  state: ProjectFiling | undefined;
  done: boolean;
  reason: string;
  onReason: (reason: string) => void;
  reasonBox: RefObject<HTMLTextAreaElement | null>;
}>) {
  const t = useT();
  if (!state) {
    return (
      <p className="t-caption" role="status">
        {t("projectFiling.loading")}
      </p>
    );
  }
  const projects = state.projects
    .map((project) =>
      project.hidden ? t("projectFiling.hiddenProject") : project.name,
    )
    .join(", ");
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
                ref={reasonBox}
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
          <li key={decision.id}>
            <span className="t-body">
              {decision.redacted
                ? t("projectFiling.decisionRedacted", {
                    when: formatDateTime(decision.at, locale, zone),
                  })
                : t("projectFiling.decision", {
                    name: decision.by_name,
                    when: formatDateTime(decision.at, locale, zone),
                  })}
            </span>
            {!decision.redacted && (
              <p className="t-caption">{decision.reason}</p>
            )}
          </li>
        ))}
      </ul>
    </section>
  );
}
