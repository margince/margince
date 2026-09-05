// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Modal, Skeleton } from "../design-system/atoms";
import { EmailReference } from "../design-system/emailreference";
import { PipelineLadder } from "../design-system/pipelineladder";
import { SurfaceState } from "../design-system/surfacestate";
import { formatDateTime } from "../format/format";
import { viewerZone } from "../format/timezone";
import { useLocale, useT } from "../i18n";
import { useProviderLabel } from "./channelproviders";
import { throwProblem } from "./common";

type TraceEntry = components["schemas"]["CaptureTraceEntry"];

// The drill-down: what the pipeline did with ONE message, step by step.
//
// A right-anchored drawer rather than a centred dialog, because the reader is
// checking this rung against the row they clicked, and the list behind it is
// the thing they are comparing against.
//
// The ladder itself holds no knowledge of the pipeline — it walks whatever the
// server sends. So a stage added later appears here with no change to this file
// and no frontend release, which is the whole reason the surface is shaped this
// way rather than as a fixed set of fields.

export function CaptureActivityDrawer({
  traceId,
  message,
  onOpenEmail,
  onClose,
}: Readonly<{
  traceId: string;
  /**
   * The row this drawer was opened from, which is what knows WHICH message the
   * ladder is about. The pipeline read below is keyed by the trace and answers
   * with rungs; the correspondence is already on screen behind the drawer, so
   * naming it here costs no request and no contract field.
   */
  message?: TraceEntry;
  /**
   * Opens the message itself. The page owns that drawer and CLOSES this one
   * first: two `Modal placement="right"` sheets at once are two focus traps
   * and two Escape handlers over one another, so the reader moves from the
   * trace to the message rather than stacking them.
   */
  onOpenEmail?: (activityId: string) => void;
  onClose: () => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const providerLabel = useProviderLabel();
  const trace = useQuery({
    queryKey: ["capture-trace-pipeline", traceId],
    queryFn: async () => {
      const { data, error } = await api.GET("/capture/traces/{id}", {
        params: { path: { id: traceId } },
      });
      if (error) throwProblem(error);
      return data;
    },
  });

  return (
    <Modal
      open
      onClose={onClose}
      labelledBy="capture-pipeline-title"
      placement="right"
    >
      <h2 id="capture-pipeline-title">{t("pipeline.title")}</h2>
      <p className="capture-activity__drawer-sub t-sub">{t("pipeline.sub")}</p>
      {/* WHICH message this ladder is about, named the way every other citation
          of a message in the product names one. The reader arrived here from a
          row that showed a sender and a subject in the trace's own layout; this
          is the same fact, drawn once, and it opens.

          `activity_id` is the server's gate: it is set only where a timeline
          row exists AND this caller may read it, so an entry whose message
          moved out of their scope names itself and offers nothing to press,
          rather than handing back a link that proves the row exists. */}
      {message && (
        <p className="capture-activity__drawer-message">
          <EmailReference
            subject={message.subject}
            occurredAt={formatDateTime(
              message.occurred_at,
              locale,
              viewerZone(),
            )}
            onOpen={openerFor(message, onOpenEmail)}
          />
        </p>
      )}
      {trace.isPending ? (
        <Skeleton width="100%" height={240} />
      ) : trace.data ? (
        <>
          {trace.data.connector && (
            <p className="capture-activity__drawer-transport t-sub">
              {t("pipeline.transport")}{" "}
              <strong>{providerLabel(trace.data.connector)}</strong>
            </p>
          )}
          <PipelineLadder
            stages={trace.data.stages}
            payloadsEnabled={trace.data.payload_capture_enabled}
          />
        </>
      ) : (
        // `unavailable` rather than `empty`: the ladder always has a rung per
        // registered stage, so nothing here means the read failed — and drawing
        // that as "there are none" would state a fact about the pipeline that
        // we do not have. emptyLabel is required by the component and unused by
        // this state; it names what there would be none OF, if the state were
        // ever `empty`.
        <SurfaceState
          loadingLabel={t("pipeline.title")}
          state="unavailable"
          emptyLabel={t("pipeline.unavailable")}
        >
          {null}
        </SurfaceState>
      )}
    </Modal>
  );
}

/**
 * The opener for a traced message, or nothing.
 *
 * A function rather than a conditional in the JSX because both facts have to
 * hold at the same moment and TypeScript has to see it: `activity_id` is null
 * for a message that never became a row and for one this caller may not read,
 * and a page that passed no opener has nowhere to send them either way.
 */
function openerFor(
  message: TraceEntry,
  onOpenEmail: ((activityId: string) => void) | undefined,
): (() => void) | undefined {
  const activityId = message.activity_id;
  if (!activityId || !onOpenEmail) {
    return undefined;
  }
  return () => onOpenEmail(activityId);
}
