// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useState } from "react";
import { Button, Field, TextInput } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { DateInput, type ISODate } from "../../design-system/dateinput";
import { refuseUnreadableField } from "../../design-system/formvalidity";
import { PanelBody } from "../../design-system/panel";
import {
  RecordPicker,
  type RecordPickerCandidate,
} from "../../design-system/recordpicker";
import { useFieldFaults } from "../../design-system/savepage";
import { useLocale, useT } from "../../i18n";
import { blankRequired, problemMessageOf } from "../../lib/problem";
import { useRecordTargets } from "../../lib/recordtargets";
import { useAddCommitment } from "../worklist/weeklyplan.queries";

// One more thing the week is FOR, typed into the plan it belongs to.
//
// Its own file rather than a form inside the panel: everything else in
// `brief.plan.tsx` reads or settles a commitment that already exists, and this
// is the one place a new one comes into being.

export function AddCommitment({ onDone }: Readonly<{ onDone: () => void }>) {
  const t = useT();
  const { locale } = useLocale();
  const [faults, faultsRoot] = useFieldFaults<HTMLDivElement>();
  const add = useAddCommitment();
  const { search, kindOf } = useRecordTargets();
  const [record, setRecord] = useState<RecordPickerCandidate | null>(null);
  const type = record ? kindOf(record.id) : null;
  const [label, setLabel] = useState("");
  const [dueOn, setDueOn] = useState<ISODate | "">("");
  const labelLabel = t("plan.new.label");

  return (
    <PanelBody>
      <div data-date-scope ref={faultsRoot}>
        <RecordPicker
          label={t("brief.plan.select")}
          searchTargets={search}
          selected={record}
          onPick={(next) => {
            setRecord(next);
            if (label === "" && next) {
              setLabel(next.name);
              faults.edited("label");
            }
          }}
        />
        <Field label={labelLabel} required error={faults.of("label")}>
          {(control) => (
            <TextInput
              {...control}
              value={label}
              maxLength={500}
              onChange={(event) => {
                setLabel(event.target.value);
                faults.edited("label");
              }}
            />
          )}
        </Field>
        <Field label={t("plan.new.due")}>
          {(control) => (
            <DateInput
              {...control}
              value={dueOn}
              onChange={(event) => setDueOn(event.target.value)}
            />
          )}
        </Field>
        {add.isError && (
          <Callout
            tone="danger"
            kind="outcome"
            title={problemMessageOf(add.error, t)}
          />
        )}
        <div className="form-actions">
          <Button variant="ghost" onClick={onDone}>
            {t("plan.new.cancel")}
          </Button>
          <Button
            onClick={(event) => {
              const blank = blankRequired(locale, {
                label: [labelLabel, label],
              });
              if (faults.show(blank)) return;
              if (refuseUnreadableField(event.currentTarget)) {
                return;
              }
              add.mutate(
                // An empty date is no date, not an empty string: the contract
                // types due_on as nullable, and "" is neither a date nor an
                // absence.
                {
                  label,
                  due_on: dueOn === "" ? null : dueOn,
                  ...(record && type
                    ? { linked_record: { type, id: record.id } }
                    : {}),
                },
                {
                  onSuccess: () => {
                    setLabel("");
                    setDueOn("");
                    onDone();
                  },
                },
              );
            }}
            pending={add.isPending}
          >
            {t("plan.new.save")}
          </Button>
        </div>
      </div>
    </PanelBody>
  );
}
