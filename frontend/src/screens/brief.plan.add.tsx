// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useState } from "react";
import { Button, Field, TextInput } from "../design-system/atoms";
import { DateInput, type ISODate, isISODate } from "../design-system/dateinput";
import { PanelBody } from "../design-system/panel";
import { useT } from "../i18n";
import { useAddCommitment } from "./weeklyplan.queries";

// One more thing the week is FOR, typed into the plan it belongs to.
//
// Its own file rather than a form inside the panel: everything else in
// `brief.plan.tsx` reads or settles a commitment that already exists, and this
// is the one place a new one comes into being.

export function AddCommitment({ onDone }: Readonly<{ onDone: () => void }>) {
  const t = useT();
  const add = useAddCommitment();
  const [label, setLabel] = useState("");
  const [dueOn, setDueOn] = useState<ISODate | "">("");

  return (
    <PanelBody>
      <Field label={t("plan.new.label")} required>
        {(control) => (
          <TextInput
            {...control}
            value={label}
            maxLength={500}
            onChange={(event) => setLabel(event.target.value)}
          />
        )}
      </Field>
      <Field label={t("plan.new.due")}>
        {(control) => (
          <DateInput
            {...control}
            value={dueOn}
            // The element reports YYYY-MM-DD or "" and nothing else, so this
            // narrows what it said rather than validating it.
            onChange={(event) =>
              setDueOn(isISODate(event.target.value) ? event.target.value : "")
            }
          />
        )}
      </Field>
      {/* The submit row, not two buttons loose in the body: `.panel-body` is
          padding and nothing else, so bare siblings sat against each other on
          the line under the last field with no gap and no alignment of their
          own. */}
      <div className="form-actions">
        <Button variant="ghost" onClick={onDone}>
          {t("plan.new.cancel")}
        </Button>
        <Button
          onClick={() => {
            add.mutate(
              // An empty date is no date, not an empty string: the contract
              // types due_on as nullable, and "" is neither a date nor an
              // absence.
              { label, due_on: dueOn === "" ? null : dueOn },
              {
                onSuccess: () => {
                  setLabel("");
                  setDueOn("");
                  onDone();
                },
              },
            );
          }}
          // An empty label is a precondition the reader can meet; a write in
          // flight is a wait of seconds. Spelling the second as `disabled`
          // takes focus off the control they just pressed.
          disabled={label.trim() === ""}
          pending={add.isPending}
        >
          {t("plan.new.save")}
        </Button>
      </div>
    </PanelBody>
  );
}
