// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useState } from "react";
import { Button, Field, Textarea } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { Panel, PanelBody, PanelRow } from "../design-system/panel";
import { formatNumber } from "../format/format";
import { useLocale, useT } from "../i18n";
import { useSetPlanContract, type WeeklyPlan } from "./weeklyplan.queries";

// The rows' own sheet, beside the plan's: one surface, one stylesheet, and it
// has to be imported HERE as well because this panel is rendered on its own —
// by its stories, and by anything else that reaches for it without the section
// around it.
import "./brief.plan.css";

// What could go wrong with the week, and whether there is room for it.
//
// The commitments beside this say what the week is FOR. This says what it is up
// against, and it is the half a lead reads first: five commitments with no room
// to do them is a plan that has already failed and nobody has said so.
//
// TWO HALVES, TWO ROWS. Each half is a question with an answer, so it draws as
// a `PanelRow` — the name on the leading edge, the answer and the verb that
// changes it on the trailing one, the panel's own hairline between them. As a
// stack of headings and paragraphs the same two halves were six lines of text
// floating in a body with nothing saying which line answered which.

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
  const { locale } = useLocale();
  if (!plan) return null;
  const capacity = plan.capacity;
  const commitments = plan.commitments.length;
  const crowded = capacity !== undefined && isCrowded(capacity, commitments);
  const n = (value: number) => formatNumber(value, locale);
  return (
    <Panel
      title={t("plan.contract.title")}
      // What the week ALREADY holds qualifies both rows under it, which is what
      // a panel's description is for — as a paragraph in the body it was a
      // stray sentence above two headings, reading as a third half.
      //
      // ABSENT draws nothing at all: the server omits `capacity` when no
      // calendar reader is composed, and "0 meetings booked" would tell a rep
      // their week is clear on the strength of a missing integration. Absent
      // too when the week is crowded, where the Callout below says it louder.
      sub={
        capacity !== undefined && !crowded
          ? t("plan.contract.capacityLine", {
              meetings: n(capacity.meetings),
              tasks: n(capacity.tasks),
            })
          : undefined
      }
    >
      {/* A warn Callout only when the week is actually crowded. Drawn always,
          the tone would stop meaning anything and a reader would learn to skip
          it. */}
      {capacity !== undefined && crowded && (
        <PanelBody>
          <Callout
            tone="warn"
            kind="standing"
            title={t("plan.contract.crowded")}
          >
            {t("plan.contract.crowdedBody", {
              committed: n(capacity.meetings + capacity.tasks),
              commitments: n(commitments),
            })}
          </Callout>
        </PanelBody>
      )}
      <ContractRow
        label={t("plan.contract.risks")}
        hint={t("plan.contract.risksHint")}
        value={plan.risks}
        editable={editable}
        field="risks"
      />
      <ContractRow
        label={t("plan.contract.capacityNote")}
        hint={t("plan.contract.capacityNoteHint")}
        value={plan.capacity_note}
        editable={editable}
        field="capacity_note"
      />
    </Panel>
  );
}

/** Whether the week has more in it than the arithmetic thinks it can hold. */
function isCrowded(capacity: Capacity, commitments: number): boolean {
  return capacity.meetings + capacity.tasks + commitments > crowdedWeek;
}

// One half of the contract, read until somebody edits it.
//
// UNWRITTEN AND EMPTY ARE DIFFERENT and both are drawn: null is a rep who has
// said nothing, "" is a rep who looked and says there is nothing to name. A
// lead who cannot tell those apart is reading a week they have not been told
// about as if it were a safe one.
function ContractRow({
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

  if (editing) {
    // The editor takes the whole row: a textarea squeezed into the answer's
    // column would be a writing surface a third of a sentence wide.
    return (
      <PanelRow>
        <div className="plan-contract-edit">
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
          <div className="form-actions">
            <Button variant="ghost" onClick={() => setEditing(false)}>
              {t("plan.contract.cancel")}
            </Button>
            <Button
              pending={save.isPending}
              onClick={() => {
                // Only THIS half travels. The other is omitted, which the
                // server reads as "leave it alone" — sending both would let a
                // stale draft overwrite a note the rep changed in the other
                // field.
                save.mutate(
                  { [field]: draft },
                  { onSuccess: () => setEditing(false) },
                );
              }}
            >
              {t("plan.contract.save")}
            </Button>
          </div>
        </div>
      </PanelRow>
    );
  }
  return (
    <PanelRow>
      <div className="plan-contract-row">
        <span className="t-label plan-contract-name">{label}</span>
        <span className="plan-contract-answer">
          {/* Nothing written is not an empty answer but the absence of one,
              so it reads at the caption's tone rather than as a value the rep
              chose. */}
          <span className={value == null ? "t-caption" : undefined}>
            {value == null
              ? t("plan.contract.unwritten")
              : value === ""
                ? t("plan.contract.nothingToName")
                : value}
          </span>
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
        </span>
      </div>
    </PanelRow>
  );
}
