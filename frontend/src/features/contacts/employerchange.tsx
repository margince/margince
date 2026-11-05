// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation } from "@tanstack/react-query";
import { type ReactNode, useId, useState } from "react";
import { api } from "../../api/client";
import { Button, Checkbox, Field, Modal } from "../../design-system/atoms";
import { ChoiceList } from "../../design-system/choicelist";
import { EditableValue } from "../../design-system/editablevalue";
import { ErrorLine } from "../../design-system/errorline";
import { Heading } from "../../design-system/heading";
import {
  RecordPicker,
  type RecordPickerCandidate,
} from "../../design-system/recordpicker";
import { useT } from "../../i18n";
import { stillHeld } from "../../lib/employmentcurrency";
import { unwrap } from "../../lib/problem";
import { useAddEmployment } from "./addemploymentmodal";
import {
  type Employment,
  type EmploymentActions,
  searchCompanyCandidates,
  useCanEditEmployment,
  useEmploymentActions,
} from "./contactemployers";
import type { Contact360 } from "./contactrail";
import "../../design-system/querystates.css";

/**
 * The About column's employer value, editing from where it is read. With no
 * employer it opens the Companies panel's add dialog; with one, the dialog
 * below that moves the contact to another company or to none.
 */
export function EmployerValue({
  view,
  employer,
  reading,
  notSet,
}: Readonly<{
  view: Contact360;
  employer: Employment | undefined;
  reading: ReactNode;
  notSet: string;
}>) {
  const t = useT();
  const contactId = view.contact.id;
  const canEdit = useCanEditEmployment(view.contact);
  const actions = useEmploymentActions(contactId);
  const employments = view.employments?.data ?? [];
  const adding = useAddEmployment(contactId, actions.create, employments);
  // `seq` keys the dialog, so every open starts from the employer as it is now.
  const [changing, setChanging] = useState({ open: false, seq: 0 });
  if (!canEdit) {
    return reading ?? notSet;
  }
  return (
    <>
      <EditableValue
        editLabel={t("inlineChoice.change", {
          field: t("contact.rail.employer"),
        })}
        placeholder={notSet}
        link={Boolean(employer)}
        block={Boolean(employer)}
        onEdit={() =>
          employer
            ? setChanging((prior) => ({ open: true, seq: prior.seq + 1 }))
            : adding.open()
        }
      >
        {reading}
      </EditableValue>
      {adding.modal}
      {employer && (
        <ChangeEmployerModal
          key={changing.seq}
          open={changing.open}
          onClose={() => setChanging((prior) => ({ ...prior, open: false }))}
          contactId={contactId}
          employer={employer}
          actions={actions}
        />
      )}
    </>
  );
}

type Fate = "left" | "wrong";
// What Save sends: the company to move to (none clears), and the old job's fate.
type Answer = Readonly<{ target: RecordPickerCandidate | null; fate: Fate }>;

/**
 * Moves the contact's current employer to another company, or to none. The
 * reader says what became of the old job. If they left it, it is kept as a
 * former employer; if it was never true, the link goes.
 */
export function ChangeEmployerModal({
  open,
  onClose,
  contactId,
  employer,
  actions,
}: Readonly<{
  open: boolean;
  onClose: () => void;
  contactId: string;
  employer: Employment;
  actions: EmploymentActions;
}>) {
  const t = useT();
  const headingId = useId();
  const current = employer.company_name ?? t("field.unset");
  const [picked, setPicked] = useState<RecordPickerCandidate | null>(null);
  const [none, setNone] = useState(false);
  const [fate, setFate] = useState<Fate>("left");
  const moving = none || (picked !== null && picked.id !== employer.company_id);
  const change = useMutation({
    mutationFn: async (answer: Answer) => {
      // The reader's answer: a company named here is their primary employer.
      const named = answer.target !== null;
      if (answer.target !== null) {
        await promote(answer.target, named, contactId, employer, actions);
      }
      // A promotion demotes the old job in its own transaction, so the image
      // this end compares against is the demoted one.
      const old = {
        ...employer,
        is_current_primary: employer.is_current_primary && !named,
      };
      if (answer.fate === "wrong") {
        await actions.remove.mutateAsync(employer.relationship_id);
      } else {
        await actions.end.mutateAsync(old);
      }
    },
    onSuccess: onClose,
  });
  return (
    <Modal open={open} onClose={onClose} labelledBy={headingId} intent="form">
      <Heading size="large" id={headingId} className="t-h2 modal-title">
        {t("contact.employer.changeTitle")}
      </Heading>
      <div className="form-stack">
        <Field label={t("contact.rail.employer")}>
          {(control) => (
            <RecordPicker
              id={control.id}
              aria-describedby={control["aria-describedby"]}
              searchTargets={searchCompanyCandidates}
              selected={
                none
                  ? null
                  : (picked ?? { id: employer.company_id, name: current })
              }
              onPick={setPicked}
              disabled={none || change.isPending}
            />
          )}
        </Field>
        <Checkbox
          label={t("contact.employer.none")}
          checked={none}
          disabled={change.isPending}
          onChange={(event) => setNone(event.target.checked)}
        />
        {moving && (
          <ChoiceList<Fate>
            legend={t("contact.employer.fateQuestion", { company: current })}
            value={fate}
            onChange={setFate}
            disabled={change.isPending}
            choices={[
              {
                value: "left",
                label: t("contact.employer.fateLeft", { company: current }),
                description: t("contact.employer.fateLeftHint"),
              },
              {
                value: "wrong",
                label: t("contact.employer.fateWrong", { company: current }),
                description: t("contact.employer.fateWrongHint", {
                  company: current,
                }),
              },
            ]}
          />
        )}
        <ErrorLine error={change.error} />
      </div>
      <div className="actions">
        <Button onClick={onClose} disabled={change.isPending}>
          {t("create.cancel")}
        </Button>
        <Button
          variant="primary"
          disabled={!moving}
          pending={change.isPending}
          onClick={() => change.mutate({ target: none ? null : picked, fate })}
        >
          {t("record.save")}
        </Button>
      </div>
    </Modal>
  );
}

/**
 * Makes `company` the contact's current employer, primary as the reader said.
 * A job they still hold there takes the flag, since the server refuses a
 * second live edge to one company. Otherwise a new one is written. Either
 * write demotes the old primary in the same transaction.
 */
async function promote(
  company: RecordPickerCandidate,
  primary: boolean,
  contactId: string,
  employer: Employment,
  actions: EmploymentActions,
): Promise<void> {
  const held = (await employmentsAt(contactId, company.id)).find(
    (row) => row.relationship_id !== employer.relationship_id && stillHeld(row),
  );
  if (held) {
    await actions.update.mutateAsync({
      employment: held,
      body: { is_current_primary: primary },
    });
    return;
  }
  await actions.create.mutateAsync({
    kind: "employment",
    contact_id: contactId,
    company_id: company.id,
    is_current_primary: primary,
    source: "manual",
  });
}

// Every employment at one company, read from its owner rather than the page's
// summary, which holds only the first 25 roles.
async function employmentsAt(
  contactId: string,
  companyId: string,
): Promise<Employment[]> {
  const data = unwrap(
    await api.GET("/relationships", {
      params: {
        query: {
          contact_id: contactId,
          company_id: companyId,
          kind: "employment",
          limit: 200,
        },
      },
    }),
  );
  return data.data.map((row) => ({
    relationship_id: row.id,
    company_id: companyId,
    role: row.role,
    is_current_primary: row.is_current_primary,
    employment_status: row.employment_status,
    started_precision: row.started_precision,
    ended_precision: row.ended_precision,
    started_at: row.started_at,
    ended_at: row.ended_at,
    version: row.version,
  }));
}
