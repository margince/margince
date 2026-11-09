// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Ban } from "lucide-react";
import { useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Button } from "../design-system/atoms";
import { ConfirmModal } from "../design-system/confirmmodal";
import { Field, Textarea } from "../design-system/field";
import { formatDateAbbrev } from "../format/format";
import { type Translator, useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { problemMessageOf, unwrap } from "../lib/problem";
import { useViewerZone } from "../lib/viewerzone";

type ContactStop = components["schemas"]["ContactStop"];
type BlockedBy = NonNullable<ContactStop["lift_blocked_by"]>;

const KIND: Record<string, MessageKey> = {
  subject_request: "consent.stopKind.subject_request",
  marketing_objection: "consent.stopKind.marketing_objection",
  processing_restriction: "consent.stopKind.processing_restriction",
  hard_bounce: "consent.stopKind.hard_bounce",
};

// A stop the reader cannot take back says why. A stop held at a higher level
// says nothing: the reader can do nothing about it here.
const BLOCKED: Partial<Record<BlockedBy, MessageKey>> = {
  address: "consent.stopBlocked.address",
  restriction: "consent.stopBlocked.restriction",
  subject_decision: "consent.stopBlocked.subject_decision",
};

const REASON_MAX = 500;

/** The record whose stops are shown: a contact or a lead. */
type StopSubject = Readonly<{ kind: "contact" | "lead"; id: string }>;

/** The queries a lift changes, per subject kind. */
const LIFT_INVALIDATES: Record<StopSubject["kind"], string[]> = {
  contact: ["contactStops", "contactConsentGuard"],
  lead: ["leadStops"],
};

/** The live stops on a contact or a lead, as the send check reads them. */
function useStops(subject: StopSubject) {
  return useQuery({
    queryKey: [LIFT_INVALIDATES[subject.kind][0], subject.id],
    queryFn: async () => {
      const path = { params: { path: { id: subject.id } } };
      const answer =
        subject.kind === "contact"
          ? await api.GET("/contacts/{id}/consent/stops", path)
          : await api.GET("/leads/{id}/consent/stops", path);
      return unwrap(answer).stops;
    },
  });
}

/**
 * The stops on a contact, each with who recorded it and why. A stop the reader
 * may take back offers it, with a required reason. For an admin that includes
 * a marketing objection a colleague recorded by mistake.
 */
export function ContactStops({ contactId }: Readonly<{ contactId: string }>) {
  return <SubjectStops subject={{ kind: "contact", id: contactId }} />;
}

/** The stops on a lead, the same panel as a contact's. */
export function LeadStops({ leadId }: Readonly<{ leadId: string }>) {
  return <SubjectStops subject={{ kind: "lead", id: leadId }} />;
}

function SubjectStops({ subject }: Readonly<{ subject: StopSubject }>) {
  const t = useT();
  const stops = useStops(subject);
  const list = stops.data ?? [];
  if (list.length === 0) {
    return null;
  }
  return (
    <section aria-label={t("consent.stopsTitle")}>
      <p className="t-label">{t("consent.stopsTitle")}</p>
      {list.map((stop) => (
        <StopRow key={stop.id} subject={subject} stop={stop} />
      ))}
    </section>
  );
}

function StopRow({
  subject,
  stop,
}: Readonly<{ subject: StopSubject; stop: ContactStop }>) {
  const t = useT();
  const blocked = stop.lift_blocked_by
    ? BLOCKED[stop.lift_blocked_by]
    : undefined;
  return (
    <div>
      <div className="pe-rail-row">
        <span className="pe-rail-label">
          <Ban size={15} aria-hidden="true" />
          {t(KIND[stop.kind] ?? "consent.stopKind.unknown")}
        </span>
      </div>
      {stop.address && (
        <p className="t-caption">
          {t("consent.stopOnAddress", { address: stop.address })}
        </p>
      )}
      {stop.purpose_key && (
        <p className="t-caption">
          {t("consent.stopOnPurpose", { purpose: stop.purpose_key })}
        </p>
      )}
      <p className="pe-colleague-proof t-caption">
        <RecordedBy stop={stop} />
      </p>
      {stop.reason && <p className="t-caption">“{stop.reason}”</p>}
      {blocked && <p className="t-caption">{t(blocked)}</p>}
      {stop.may_lift && <LiftStop subject={subject} stop={stop} />}
    </div>
  );
}

function RecordedBy({ stop }: Readonly<{ stop: ContactStop }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useViewerZone();
  const date = formatDateAbbrev(stop.recorded_at, locale, zone);
  return <>{recordedByText(stop, date, t)}</>;
}

function recordedByText(stop: ContactStop, date: string, t: Translator) {
  if (stop.recorded_by_colleague) {
    return stop.recorded_by_name
      ? t("consent.stopRecordedBy", { name: stop.recorded_by_name, date })
      : t("consent.stopRecordedByColleague", { date });
  }
  return stop.authority === "subject"
    ? t("consent.stopRecordedBySubject", { date })
    : t("consent.stopRecordedAutomatically", { date });
}

function LiftStop({
  subject,
  stop,
}: Readonly<{ subject: StopSubject; stop: ContactStop }>) {
  const t = useT();
  const queryClient = useQueryClient();
  const [asking, setAsking] = useState(false);
  const [reason, setReason] = useState("");
  const lift = useMutation({
    mutationFn: async (words: string) => {
      const request = {
        params: { path: { id: subject.id, suppressionId: stop.id } },
        body: { reason: words },
      };
      unwrap(
        subject.kind === "contact"
          ? await api.POST(
              "/contacts/{id}/consent/suppress/{suppressionId}/lift",
              request,
            )
          : await api.POST(
              "/leads/{id}/consent/suppress/{suppressionId}/lift",
              request,
            ),
      );
    },
    onSuccess: () => {
      setAsking(false);
      setReason("");
      for (const key of LIFT_INVALIDATES[subject.kind]) {
        queryClient.invalidateQueries({ queryKey: [key, subject.id] });
      }
    },
  });
  const action = stop.correction ? "consent.stopCorrect" : "consent.stopLift";
  const words = reason.trim();
  return (
    <>
      <Button variant="link" onClick={() => setAsking(true)}>
        {t(action)}
      </Button>
      <ConfirmModal
        open={asking}
        onClose={() => {
          setAsking(false);
          lift.reset();
        }}
        title={t(
          stop.correction
            ? "consent.stopCorrectTitle"
            : "consent.stopLiftTitle",
        )}
        confirmLabel={t(action)}
        confirmVariant="danger"
        confirmDisabled={words === ""}
        onConfirm={() => lift.mutate(words)}
        pending={lift.isPending}
        error={lift.isError ? problemMessageOf(lift.error, t) : null}
      >
        <p className="t-body">
          {t(
            stop.correction
              ? "consent.stopCorrectBody"
              : "consent.stopLiftBody",
          )}
        </p>
        <Field label={t("consent.stopReason")} required>
          {(control) => (
            <Textarea
              {...control}
              value={reason}
              maxLength={REASON_MAX}
              onChange={(event) => setReason(event.target.value)}
            />
          )}
        </Field>
      </ConfirmModal>
    </>
  );
}
