// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useId, useState } from "react";
import { useCanWrite } from "../app/capability";
import { useRecordZone } from "../app/recordzone";
import { Button, Field, Modal, Textarea } from "../design-system/atoms";
import { Heading } from "../design-system/heading";
import { PanelRow } from "../design-system/panel";
import { SurfaceState } from "../design-system/surfacestate";
import { middayInstant } from "../format/calendarday";
import { formatDate } from "../format/format";
import { useLocale, useT } from "../i18n";
import { problemMessageOf } from "./common";
import { EntityRef } from "./entityref";
import {
  useAnswerCommitment,
  useTeammateWeeklyPlan,
  type WeeklyPlanCommitment,
} from "./weeklyplan.queries";

export function TeamPlanReview({
  owner,
  name,
}: Readonly<{ owner: string; name: string }>) {
  const t = useT();
  const [open, setOpen] = useState(false);
  return (
    <>
      <Button
        variant="ghost"
        onClick={(event) => {
          event.stopPropagation();
          setOpen(true);
        }}
      >
        {t("brief.team.plan")}
      </Button>
      {open && (
        <TeamPlanDialog
          owner={owner}
          name={name}
          onClose={() => setOpen(false)}
        />
      )}
    </>
  );
}

function TeamPlanDialog({
  owner,
  name,
  onClose,
}: Readonly<{ owner: string; name: string; onClose: () => void }>) {
  const t = useT();
  const plan = useTeammateWeeklyPlan(owner);
  const canAnswer = useCanWrite("weekly_plan", "update");
  const { locale } = useLocale();
  const zone = useRecordZone();
  const headingId = useId();
  return (
    <Modal open labelledBy={headingId} onClose={onClose}>
      <Heading size="large" id={headingId}>
        {t("brief.team.planFor", { name })}
      </Heading>
      {plan.data && (
        <p>
          {t("brief.plan.period", {
            date: formatDate(
              middayInstant(plan.data.local_week_start, zone),
              locale,
              zone,
            ),
          })}
        </p>
      )}
      <SurfaceState
        state={
          plan.isPending
            ? "loading"
            : plan.isError
              ? "failed"
              : !plan.data
                ? "empty"
                : "ready"
        }
        loadingLabel={t("plan.loading")}
        emptyLabel={t("brief.team.planUnavailable", { name })}
        detail={{ onRetry: () => void plan.refetch() }}
      >
        {plan.data?.commitments.length === 0 && (
          <p>{t("brief.team.noCommitments", { name })}</p>
        )}
        {plan.data?.commitments.map((commitment) => (
          <TeamCommitment
            key={commitment.id}
            owner={owner}
            commitment={commitment}
            editable={canAnswer && plan.data?.status === "open"}
          />
        ))}
      </SurfaceState>
    </Modal>
  );
}

function TeamCommitment({
  owner,
  commitment,
  editable,
}: Readonly<{
  owner: string;
  commitment: WeeklyPlanCommitment;
  editable: boolean;
}>) {
  const t = useT();
  const answer = useAnswerCommitment(owner);
  const { locale } = useLocale();
  const zone = useRecordZone();
  const [response, setResponse] = useState(commitment.manager_response ?? "");
  return (
    <PanelRow>
      <div>
        <p>{commitment.label}</p>
        <p>{t(`plan.state.${commitment.state}`)}</p>
        {commitment.due_on && (
          <p>
            {t("plan.due", {
              day: formatDate(
                middayInstant(commitment.due_on, zone),
                locale,
                zone,
              ),
            })}
          </p>
        )}
        {commitment.linked_record && (
          <EntityRef
            kind={commitment.linked_record.type}
            id={commitment.linked_record.id}
          />
        )}
        {commitment.help_requested && (
          <>
            <p>{commitment.help_requested}</p>
            {editable ? (
              <>
                <Field label={t("brief.team.response")}>
                  {(control) => (
                    <Textarea
                      {...control}
                      value={response}
                      maxLength={2000}
                      onChange={(event) => setResponse(event.target.value)}
                    />
                  )}
                </Field>
                <Button
                  pending={answer.isPending}
                  disabled={
                    response.trim() === "" ||
                    response === commitment.manager_response
                  }
                  onClick={() =>
                    answer.mutate({
                      id: commitment.id,
                      managerResponse: response,
                    })
                  }
                >
                  {t("brief.team.saveResponse")}
                </Button>
              </>
            ) : (
              <p>{commitment.manager_response}</p>
            )}
            {answer.isError && (
              <p role="alert">{problemMessageOf(answer.error, t)}</p>
            )}
          </>
        )}
      </div>
    </PanelRow>
  );
}
