// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useId, useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { Button, Field, TextInput } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { refuseUnreadableField } from "../../design-system/formvalidity";
import {
  RecordPicker,
  type RecordPickerCandidate,
} from "../../design-system/recordpicker";
import {
  focusFirstRefused,
  useFieldFaults,
} from "../../design-system/savepage";
import { Select } from "../../design-system/select";
import { useLocale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { labelFor } from "../../lib/labelfor";
import { isOption } from "../../lib/options";
import { DSR_KIND_LABEL } from "../../lib/privacy.logic";
import { blankRequired, problemFieldErrorsOf, unwrap } from "../../lib/problem";
import { useViewerZone } from "../../lib/viewerzone";
import { dsrsKey } from "../../lib/workkeys";
import {
  type DeadlineDraft,
  DsrDeadlineFields,
  deadlineBody,
  EMPTY_DEADLINE,
  earlierDueFault,
  useRequestDeadline,
} from "./privacy.deadline";

// A new data subject request: its kind, its subject and its deadline.

type CreateDataSubjectRequest =
  components["schemas"]["CreateDataSubjectRequest"];
type DsrKind = CreateDataSubjectRequest["kind"];
type RequestDeadline = components["schemas"]["RequestDeadline"];

const DSR_KINDS: readonly DsrKind[] = [
  "access",
  "rectify",
  "erasure",
  "restriction",
];

// The kinds whose fulfilment acts on one contact record, so they are opened
// against a picked contact, each with the hint that says why.
const CONTACT_KIND_HINT: Partial<Record<DsrKind, MessageKey>> = {
  erasure: "privacy.erasureNeedsContact",
  restriction: "privacy.restrictionNeedsContact",
  access: "privacy.accessPicksContact",
};

async function searchContactCandidates(
  q: string,
): Promise<RecordPickerCandidate[]> {
  const data = unwrap(
    await api.GET("/contacts", {
      params: { query: { q, limit: 10 } },
    }),
  );
  return data.data.map((contact) => ({
    id: contact.id,
    name: contact.full_name,
  }));
}

type DsrDraft = Readonly<{
  kind: DsrKind;
  subjectRef: string;
  contact: RecordPickerCandidate | null;
  deadline: DeadlineDraft;
}>;

const EMPTY_DSR: DsrDraft = {
  kind: "access",
  subjectRef: "",
  contact: null,
  deadline: EMPTY_DEADLINE,
};

// Kind flips the subject field: a contact kind locks onto a picked contact.
export function NewDsrForm({ onDone }: Readonly<{ onDone: () => void }>) {
  const t = useT();
  const { locale } = useLocale();
  const [faults, faultsRoot] = useFieldFaults();
  const queryClient = useQueryClient();
  const [draft, setDraft] = useState<DsrDraft>(EMPTY_DSR);
  const formId = useId();
  // Minted in the operator's zone, the one the row renders it back in. A bare
  // `new Date(day)` reads the day as UTC midnight and rolls it back west of UTC.
  const tz = useViewerZone();

  const deadline = useRequestDeadline(draft.kind, draft.deadline, tz);

  const create = useMutation({
    mutationFn: async ({
      request,
      computed,
    }: {
      request: DsrDraft;
      computed: RequestDeadline;
    }) => {
      const body: CreateDataSubjectRequest = {
        kind: request.kind,
        subject_ref: request.subjectRef.trim(),
        ...deadlineBody(request.deadline, computed, tz),
      };
      return unwrap(
        await api.POST("/data-subject-requests", {
          body,
        }),
      );
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: dsrsKey });
      setDraft(EMPTY_DSR);
      onDone();
    },
  });

  function edit(next: Partial<DsrDraft>) {
    setDraft((was) => ({ ...was, ...next }));
    if ("subjectRef" in next) faults.edited("subjectRef");
    if (create.isError) {
      create.reset();
    }
  }

  const dueRefused = problemFieldErrorsOf(create.error).some(
    (problem) => problem.field === "due_at",
  );
  const computed = deadline.data;
  const contactHint = CONTACT_KIND_HINT[draft.kind];
  const subjectLabel = contactHint
    ? t("privacy.contact")
    : t("privacy.subjectRef");
  // The deadline fields already say why an earlier due day is refused.
  const dueFault = earlierDueFault(draft.deadline, computed, tz);
  return (
    <>
      <form
        id={formId}
        ref={faultsRoot}
        className="form-stack"
        noValidate
        onSubmit={(event) => {
          event.preventDefault();
          const blank = blankRequired(locale, {
            subjectRef: [subjectLabel, draft.subjectRef],
          });
          if (faults.show(blank) || create.isPending) return;
          if (dueFault !== undefined) {
            focusFirstRefused(event.currentTarget);
            return;
          }
          if (
            computed !== undefined &&
            !refuseUnreadableField(event.currentTarget)
          )
            create.mutate({ request: draft, computed });
        }}
      >
        <Field label={t("privacy.kind")}>
          {(control) => (
            <Select
              {...control}
              options={DSR_KINDS.map((value) => ({
                value,
                label: t(labelFor(DSR_KIND_LABEL, value)),
              }))}
              value={draft.kind}
              onChange={(value) => {
                // Neither subject shape carries across: a stale value of the
                // other shape would ride into the request unnoticed.
                if (isOption(value, DSR_KINDS)) {
                  edit({ kind: value, subjectRef: "", contact: null });
                }
              }}
            />
          )}
        </Field>

        {contactHint ? (
          <Field
            label={subjectLabel}
            hint={t(contactHint)}
            required
            error={faults.of("subjectRef")}
          >
            {(control) => (
              <RecordPicker
                id={control.id}
                aria-describedby={control["aria-describedby"]}
                aria-invalid={control["aria-invalid"]}
                searchTargets={searchContactCandidates}
                selected={draft.contact}
                onPick={(candidate) =>
                  edit({ contact: candidate, subjectRef: candidate.id })
                }
              />
            )}
          </Field>
        ) : (
          <Field label={subjectLabel} required error={faults.of("subjectRef")}>
            {(control) => (
              <TextInput
                {...control}
                value={draft.subjectRef}
                onChange={(event) => edit({ subjectRef: event.target.value })}
              />
            )}
          </Field>
        )}

        <DsrDeadlineFields
          draft={draft.deadline}
          query={deadline}
          refused={dueRefused}
          zone={tz}
          onChange={(next) =>
            edit({ deadline: { ...draft.deadline, ...next } })
          }
        />

        {!dueRefused && <ErrorLine error={create.error} />}
      </form>
      <div className="actions">
        <Button
          type="submit"
          form={formId}
          variant="primary"
          pending={create.isPending}
        >
          {t("privacy.openRequest")}
        </Button>
      </div>
    </>
  );
}
