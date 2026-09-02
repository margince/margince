// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useState } from "react";
import { useRecordZone } from "../app/recordzone";
import {
  Badge,
  Button,
  Checkbox,
  Field,
  TextInput,
} from "../design-system/atoms";
import { DateInput, type ISODate, isISODate } from "../design-system/dateinput";
import { Panel, PanelBody, PanelRow } from "../design-system/panel";
import { SurfaceState } from "../design-system/surfacestate";
import { formatDate, formatNumber } from "../format/format";
import { useLocale, usePlural, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { EntityRef } from "./entityref";
import {
  type SettableState,
  useAddCommitment,
  useAskForHelp,
  useSetCommitmentState,
  useStartWeeklyPlan,
  useWeeklyPlan,
  type WeeklyPlanCommitment,
} from "./weeklyplan.queries";

import "./brief.plan.css";

// The week ahead, on the Brief's weekly.
//
// The frozen review above it says what happened; this says what the rep means
// to do next, and it is the only part of that page anybody can still change.

/** How each state reads. `missed` is the close's verdict, so it is shown but
 *  never offered — the controls below settle to `done` or `dropped` only. */
const STATE_LABEL: Readonly<Record<WeeklyPlanCommitment["state"], MessageKey>> =
  {
    open: "plan.state.open",
    done: "plan.state.done",
    missed: "plan.state.missed",
    dropped: "plan.state.dropped",
  };

/**
 * Which states get a tone, and which stay untinted.
 *
 * `open` and `dropped` carry none: an open commitment is the ordinary case, and
 * dropping one is a decision rather than a failure — tinting either would tell
 * a reader something is wrong when nothing is.
 */
const STATE_TONE: Readonly<
  Partial<Record<WeeklyPlanCommitment["state"], "success" | "warn">>
> = {
  done: "success",
  missed: "warn",
};

/**
 * A commitment nobody has settled yet — the only kind a checkbox may write.
 *
 * `missed` and `dropped` are both settled answers, and a box that reopened them
 * would let one click undo the week's own close.
 */
function isSettleable(commitment: WeeklyPlanCommitment): boolean {
  return commitment.state === "open" || commitment.state === "done";
}

/** What the staged set says one commitment should become on save. */
function stagedState(
  commitment: WeeklyPlanCommitment,
  staged: ReadonlySet<string>,
): SettableState | null {
  if (!staged.has(commitment.id)) {
    return null;
  }
  return commitment.state === "done" ? "open" : "done";
}

export function PlanSection() {
  const t = useT();
  const { locale } = useLocale();
  const plan = useWeeklyPlan();
  const start = useStartWeeklyPlan();
  const setState = useSetCommitmentState();
  const plural = usePlural();
  // Ticking a box stages it; nothing reaches the wire until Save. The design
  // system's rule is that a control which IS the write is a Switch and one that
  // stages a choice is a Checkbox, so this set is what makes the checkbox
  // honest rather than a decoration on an immediate write.
  const [staged, setStaged] = useState<ReadonlySet<string>>(new Set());
  const [adding, setAdding] = useState(false);

  const state = plan.isPending
    ? "loading"
    : plan.isError
      ? "unavailable"
      : "ready";
  const commitments = plan.data?.commitments ?? [];
  // A closed week is history. The weekly job settles it and freezes its outcome
  // into the review, so accepting an edit afterwards would make the review's
  // counts disagree with the rows they were counted from.
  const editable = plan.data?.status === "open";

  function toggle(id: string) {
    setStaged((current) => {
      const next = new Set(current);
      if (next.has(id)) {
        next.delete(id);
      } else {
        next.add(id);
      }
      return next;
    });
  }

  async function save() {
    // One call per staged row: the endpoint settles a single commitment, and
    // failing them one at a time keeps a refusal attributable to its own row.
    const writes = commitments
      .map((commitment) => ({
        id: commitment.id,
        state: stagedState(commitment, staged),
      }))
      .filter(
        (write): write is { id: string; state: SettableState } =>
          write.state !== null,
      );
    for (const write of writes) {
      await setState.mutateAsync(write);
    }
    setStaged(new Set());
  }

  return (
    <section id="brief-plan" aria-label={t("plan.title")}>
      <Panel
        title={t("plan.title")}
        sub={t("plan.sub")}
        titleAction={
          editable && !adding ? (
            <Button onClick={() => setAdding(true)}>{t("plan.add")}</Button>
          ) : undefined
        }
        footer={
          // Save appears only once a box has changed. A save bar standing in the
          // resting layout would say there is something to save on a week nobody
          // has touched.
          staged.size > 0 ? (
            <Button onClick={() => void save()} disabled={setState.isPending}>
              {plural("plan.save", staged.size, {
                count: formatNumber(staged.size, locale),
              })}
            </Button>
          ) : undefined
        }
      >
        <SurfaceState
          state={state}
          emptyLabel={t("plan.empty")}
          loadingLabel={t("plan.loading")}
          detail={{ onRetry: () => void plan.refetch() }}
        >
          {plan.isSuccess && plan.data === null && (
            <PanelBody>
              <p>{t("plan.none")}</p>
              <Button onClick={() => start.mutate()} disabled={start.isPending}>
                {t("plan.start")}
              </Button>
            </PanelBody>
          )}
          {plan.data && (
            <>
              {commitments.length === 0 && (
                <PanelBody>
                  <p>{t("plan.empty")}</p>
                </PanelBody>
              )}
              {commitments.map((commitment) => (
                <CommitmentRow
                  key={commitment.id}
                  commitment={commitment}
                  editable={editable}
                  staged={staged.has(commitment.id)}
                  onToggle={() => toggle(commitment.id)}
                />
              ))}
              {adding && <AddCommitment onDone={() => setAdding(false)} />}
            </>
          )}
        </SurfaceState>
      </Panel>
    </section>
  );
}

function CommitmentRow({
  commitment,
  editable,
  staged,
  onToggle,
}: Readonly<{
  commitment: WeeklyPlanCommitment;
  editable: boolean;
  staged: boolean;
  onToggle: () => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  const settleable = editable && isSettleable(commitment);
  // The box shows what the row WILL be once saved, not what it is: a staged tick
  // that drew itself unchecked would make the stage invisible.
  const checked = staged
    ? commitment.state !== "done"
    : commitment.state === "done";

  return (
    <PanelRow>
      <div className="plan-row">
        {settleable ? (
          <Checkbox
            label=""
            aria-label={commitment.label}
            checked={checked}
            onChange={onToggle}
          />
        ) : null}
        <div className="plan-row-body">
          <span className="plan-row-label">{commitment.label}</span>
          <span className="plan-row-meta">
            <Badge tone={STATE_TONE[commitment.state]} quiet>
              {t(STATE_LABEL[commitment.state])}
            </Badge>
            {commitment.linked_record && (
              <EntityRef
                kind={commitment.linked_record.type}
                id={commitment.linked_record.id}
              />
            )}
            {commitment.due_on && (
              <span>
                {t("plan.due", {
                  day: formatDate(commitment.due_on, locale, recordZone),
                })}
              </span>
            )}
          </span>
          <HelpExchange commitment={commitment} editable={editable} />
        </div>
      </div>
    </PanelRow>
  );
}

/**
 * What the rep asked their lead, and what the lead said back.
 *
 * READ state at rest, with the textarea behind an Edit press. An always-open
 * editor would put an empty box on every row of a week where nobody needs
 * anything, which is most weeks.
 */
function HelpExchange({
  commitment,
  editable,
}: Readonly<{ commitment: WeeklyPlanCommitment; editable: boolean }>) {
  const t = useT();
  const ask = useAskForHelp();
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState(commitment.help_requested ?? "");

  if (editing) {
    return (
      <div className="plan-help">
        <Field label={t("plan.help.label")}>
          {(control) => (
            <TextInput
              {...control}
              value={draft}
              maxLength={2000}
              onChange={(event) => setDraft(event.target.value)}
            />
          )}
        </Field>
        <Button
          onClick={() => {
            ask.mutate(
              { id: commitment.id, helpRequested: draft },
              { onSuccess: () => setEditing(false) },
            );
          }}
          disabled={ask.isPending}
        >
          {t("plan.help.send")}
        </Button>
        <Button variant="ghost" onClick={() => setEditing(false)}>
          {t("plan.help.cancel")}
        </Button>
      </div>
    );
  }

  return (
    <div className="plan-help">
      {commitment.help_requested && (
        <p className="plan-help-asked">
          {t("plan.help.asked", { text: commitment.help_requested })}
        </p>
      )}
      {commitment.manager_response ? (
        <p className="plan-help-answer">
          <EntityRef kind="user" id={commitment.manager_user_id} />{" "}
          {commitment.manager_response}
        </p>
      ) : (
        commitment.help_requested && (
          <p className="plan-help-waiting">{t("plan.help.waiting")}</p>
        )
      )}
      {editable && (
        <Button variant="ghost" onClick={() => setEditing(true)}>
          {commitment.help_requested ? t("plan.help.edit") : t("plan.help.ask")}
        </Button>
      )}
    </div>
  );
}

function AddCommitment({ onDone }: Readonly<{ onDone: () => void }>) {
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
      <Button
        onClick={() => {
          add.mutate(
            // An empty date is no date, not an empty string: the contract types
            // due_on as nullable, and "" is neither a date nor an absence.
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
        disabled={label.trim() === "" || add.isPending}
      >
        {t("plan.new.save")}
      </Button>
      <Button variant="ghost" onClick={onDone}>
        {t("plan.new.cancel")}
      </Button>
    </PanelBody>
  );
}
