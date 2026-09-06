// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useState } from "react";
import { Button, Field, Textarea } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { Panel, PanelBody } from "../design-system/panel";
import { formatNumber } from "../format/format";
import { useLocale, useT } from "../i18n";
import { useSetPlanContract, type WeeklyPlan } from "./weeklyplan.queries";

// What could go wrong with the week, and whether there is room for it.
//
// The commitments beside this say what the week is FOR. This says what it is up
// against, and it is the half a lead reads first: five commitments with no room
// to do them is a plan that has already failed and nobody has said so.

type Capacity = NonNullable<WeeklyPlan["capacity"]>;

// How many commitments a week can hold before the calendar is the problem.
//
// Not a server rule and deliberately not presented as one: the line WARNS, it
// never refuses. A rep who knows their week better than this arithmetic does is
// right, and a plan that argued with them would be a worse plan.
const crowdedWeek = 8;

export function PlanContract({
  plan,
  editable,
}: Readonly<{ plan: WeeklyPlan | null | undefined; editable: boolean }>) {
  const t = useT();
  if (!plan) return null;
  return (
    <Panel title={t("plan.contract.title")}>
      <PanelBody>
        <CapacityLine
          capacity={plan.capacity}
          commitments={plan.commitments.length}
        />
        <ContractField
          label={t("plan.contract.risks")}
          hint={t("plan.contract.risksHint")}
          value={plan.risks}
          editable={editable}
          field="risks"
        />
        <ContractField
          label={t("plan.contract.capacityNote")}
          hint={t("plan.contract.capacityNoteHint")}
          value={plan.capacity_note}
          editable={editable}
          field="capacity_note"
        />
      </PanelBody>
    </Panel>
  );
}

// CapacityLine says what next week already holds.
//
// ABSENT draws nothing at all. The server omits `capacity` when no calendar
// reader is composed, and a line reading "0 meetings booked" would tell a rep
// their week is clear on the strength of a missing integration — the one thing
// this panel must never do.
function CapacityLine({
  capacity,
  commitments,
}: Readonly<{ capacity: Capacity | undefined; commitments: number }>) {
  const t = useT();
  const { locale } = useLocale();
  if (!capacity) return null;
  const n = (value: number) => formatNumber(value, locale);
  const committed = capacity.meetings + capacity.tasks;
  const crowded = committed + commitments > crowdedWeek;
  const line = t("plan.contract.capacityLine", {
    meetings: n(capacity.meetings),
    tasks: n(capacity.tasks),
  });
  // A warn Callout only when the week is actually crowded. Drawn always, the
  // tone would stop meaning anything and a reader would learn to skip it.
  if (!crowded) return <p className="plan-capacity">{line}</p>;
  return (
    <Callout tone="warn" title={t("plan.contract.crowded")}>
      {t("plan.contract.crowdedBody", {
        committed: n(committed),
        commitments: n(commitments),
      })}
    </Callout>
  );
}

// One half of the contract, read until somebody edits it.
//
// UNWRITTEN AND EMPTY ARE DIFFERENT and both are drawn: null is a rep who has
// said nothing, "" is a rep who looked and says there is nothing to name. A
// lead who cannot tell those apart is reading a week they have not been told
// about as if it were a safe one.
function ContractField({
  label,
  hint,
  value,
  editable,
  field,
}: Readonly<{
  label: string;
  hint: string;
  value: string | null | undefined;
  editable: boolean;
  field: "risks" | "capacity_note";
}>) {
  const t = useT();
  const save = useSetPlanContract();
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState(value ?? "");

  if (!editing) {
    return (
      <div className="plan-contract-field">
        <h3>{label}</h3>
        <p className={value == null ? "plan-contract-unwritten" : undefined}>
          {value == null
            ? t("plan.contract.unwritten")
            : value === ""
              ? t("plan.contract.nothingToName")
              : value}
        </p>
        {editable && (
          <Button
            variant="ghost"
            small
            onClick={() => {
              setDraft(value ?? "");
              setEditing(true);
            }}
          >
            {t("plan.contract.edit")}
          </Button>
        )}
      </div>
    );
  }
  return (
    <div className="plan-contract-field">
      <Field label={label} hint={hint}>
        {(control) => (
          <Textarea
            {...control}
            value={draft}
            maxLength={2000}
            rows={3}
            onChange={(event) => setDraft(event.target.value)}
          />
        )}
      </Field>
      <Button
        pending={save.isPending}
        onClick={() => {
          // Only THIS half travels. The other is omitted, which the server
          // reads as "leave it alone" — sending both would let a stale draft
          // overwrite a note the rep changed in the other field.
          save.mutate(
            { [field]: draft },
            { onSuccess: () => setEditing(false) },
          );
        }}
      >
        {t("plan.contract.save")}
      </Button>
      <Button variant="ghost" onClick={() => setEditing(false)}>
        {t("plan.contract.cancel")}
      </Button>
    </div>
  );
}
