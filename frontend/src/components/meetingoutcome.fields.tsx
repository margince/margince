// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import { Field, TextInput } from "../design-system/atoms";
import { DateInput, isoOrBlank } from "../design-system/dateinput";
import { Select } from "../design-system/select";
import type { ISODate } from "../format/datepattern";
import { useT } from "../i18n";
import type { TaskDraft } from "../lib/activitybody";
import { ActivityBodyField } from "./activitybodyfield";

// The two statuses the dialog can record. `canceled` is absent:
// the card's own verb writes it, and offering it here as well would give one
// answer two doors that could drift.
type HappenedStatus = "held" | "no_show";

/** What the dialog holds while it is open. */
export type OutcomeDraft = {
  status: HappenedStatus;
  subject: string;
  body: string;
  day: string;
  followUp?: TaskDraft;
};

/** The subject and notes a meeting without a live invitation takes with its outcome. */
function OutcomeWords({
  draft,
  transcript,
  onChange,
}: Readonly<{
  draft: OutcomeDraft;
  transcript: boolean;
  onChange: (draft: OutcomeDraft) => void;
}>) {
  const t = useT();
  return (
    <>
      <Field label={t("log.subject")} required>
        {(control) => (
          <TextInput
            {...control}
            value={draft.subject}
            onChange={(event) =>
              onChange({ ...draft, subject: event.target.value })
            }
          />
        )}
      </Field>
      {/* The editor Log activity writes with; a transcript keeps its
          lines, which a citation points at. */}
      <ActivityBodyField
        label={t("log.body")}
        hint={t("worklist.verb.meetingBodyHint")}
        transcript={transcript}
        rows={6}
        value={draft.body}
        onChange={(body) => onChange({ ...draft, body })}
      />
    </>
  );
}

export function OutcomeEditFields({
  draft,
  managed,
  transcript,
  days,
  onChange,
  onDateChange,
  children,
}: Readonly<{
  draft: OutcomeDraft;
  managed: boolean;
  transcript: boolean;
  days: Readonly<{ latest: ISODate | undefined; today: ISODate | undefined }>;
  onChange: (draft: OutcomeDraft) => void;
  onDateChange: () => void;
  children: ReactNode;
}>) {
  const t = useT();
  return (
    <>
      <div className="form-row">
        <Field label={t("worklist.verb.meetingWhatHappened")}>
          {(control) => (
            <Select
              {...control}
              options={[
                {
                  value: "held",
                  label: t("worklist.verb.meetingHeld"),
                },
                {
                  value: "no_show",
                  label: t("worklist.verb.meetingNoShow"),
                },
              ]}
              value={draft.status}
              // The Select can only hand back one of the two above; anything
              // else is the ordinary answer rather than a refusal.
              onChange={(value) =>
                onChange({
                  ...draft,
                  status: value === "no_show" ? "no_show" : "held",
                })
              }
            />
          )}
        </Field>
        {!managed && (
          <Field label={t("log.date")}>
            {(control) => (
              <DateInput
                {...control}
                value={isoOrBlank(draft.day)}
                max={days.latest}
                today={days.today}
                onChange={(event) => {
                  onDateChange();
                  onChange({ ...draft, day: event.target.value });
                }}
              />
            )}
          </Field>
        )}
      </div>
      {managed ? (
        <p className="t-caption">{t("worklist.verb.meetingCalendarManaged")}</p>
      ) : (
        <OutcomeWords
          draft={draft}
          transcript={transcript}
          onChange={onChange}
        />
      )}
      {children}
    </>
  );
}
