// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useEffect, useRef, useState } from "react";
import type { components } from "../api/schema";
import { useCanWriteRecord } from "../app/capability";
import { Badge, Field } from "../design-system/atoms";
import { Select } from "../design-system/select";
import { useT } from "../i18n";
import { problemMessageOf } from "./common";
import {
  useCompanyFieldPatch,
  useCompanyReadOnlyReason,
} from "./companyheader";
import { LIFECYCLE_LABELS, LIFECYCLE_OPTIONS } from "./companylookups";

type Company = components["schemas"]["Company"];
type Lifecycle = NonNullable<Company["lifecycle"]>;
type UpdateCompanyRequest = components["schemas"]["UpdateCompanyRequest"];

// The name's own line on the company record: the name and where the account
// stands, nothing else; what it is and the way in read on the marks row under
// it (CompanyMarks). Centred against the name rather than on the compact
// head's shared baseline, which drops a button low beside display type.
// Keyed by the record: a refused pick belongs to this account and must not
// carry over when the page moves to the next.
export function CompanyNameLine({ company }: Readonly<{ company: Company }>) {
  return (
    <div className="co-name-control">
      <CompanyLifecycleControl key={company.id} company={company} />
    </div>
  );
}

// The account's standing, drawn as the header's one real control beside the
// company's name. Deliberately NOT InlineChoice: that primitive is a value
// inside a line of text, sized to the text (24px) so the line does not move
// when it opens, and the header is not a line of text — the stage is the one
// thing a rep sets on an account, and as a pale inline chip it read as
// metadata a reader could not act on. So it is the design system's own
// dropdown worn as a filled button: `--controlHeight`, the same picker, the
// same PATCH (useCompanyFieldPatch) the rail's Details grid writes through.
// The grid keeps its inline field, because there the stage IS a value in a
// row of values.
export function CompanyLifecycleControl({
  company,
}: Readonly<{ company: Company }>) {
  const t = useT();
  // useCanWriteRecord, not useCanWrite: the grant and the seat say this ROLE
  // may change accounts, and the row says whether this one is theirs to change.
  // Gating on the grant alone offers an active control whose save is rejected.
  const canUpdate = useCanWriteRecord("company", company);
  const readOnlyReason = useCompanyReadOnlyReason(company);
  const patch = useCompanyFieldPatch(company);
  // What the reader picked, held until the record they picked it against has
  // answered. A refused save keeps it, so the button still says what they
  // chose while the refusal beside it says why it did not land — snapping back
  // to the stored stage would discard their answer and explain nothing.
  const [pending, setPending] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const [failure, setFailure] = useState<string | null>(null);
  const stored = company.lifecycle ?? "unknown";
  // A stage saved from somewhere else (the Details grid) supersedes a refused
  // pick here: the button shows what the record now holds, not the old error.
  const [shownFor, setShownFor] = useState(stored);
  if (shownFor !== stored) {
    setShownFor(stored);
    setPending(null);
    setFailure(null);
  }
  // Which stored stage a save was started against. A save still out when the
  // stage changes elsewhere settles against a record that has moved on, so
  // its refusal is not drawn over the stage that replaced it.
  const storedEpoch = useRef(0);
  useEffect(() => {
    storedEpoch.current += 1;
  }, [stored]);
  const label = (value: string) => t(LIFECYCLE_LABELS[value as Lifecycle]);
  if (!canUpdate || readOnlyReason) {
    // A reader who may not change the stage is shown the stage. A button that
    // looks pressable and then refuses is the defect InlineChoice was built
    // to avoid, and the header keeps that rule.
    return (
      <span title={readOnlyReason} data-testid="company-lifecycle">
        <Badge tone="accent">{label(stored)}</Badge>
      </span>
    );
  }
  const commit = async (next: string) => {
    // One write at a time: a second pick on top of an unanswered one would
    // send a version the first is about to move past.
    if (saving) {
      return;
    }
    // Choosing what is already stored is not an edit: no audit row for a
    // change that did not happen.
    if (next === stored) {
      setPending(null);
      setFailure(null);
      return;
    }
    setPending(next);
    setSaving(true);
    setFailure(null);
    const startedAt = storedEpoch.current;
    try {
      await patch({
        lifecycle: next as NonNullable<UpdateCompanyRequest["lifecycle"]>,
      });
      setPending(null);
    } catch (err) {
      if (storedEpoch.current === startedAt) {
        setFailure(problemMessageOf(err, t));
      }
    } finally {
      setSaving(false);
    }
  };
  return (
    <Field
      label={t("company.lifecycle")}
      // The button's face is the stage itself; a visible "Lifecycle" over it
      // would be the one control in the header naming itself twice. The label
      // still names it for assistive tech.
      labelHidden
      error={failure ?? undefined}
    >
      {(control) => (
        <Select
          {...control}
          appearance="button"
          // Named for the company record's own layout suite, which measures
          // this control's drawn size; neither the shared primitive's class
          // nor the German copy on its face can name it.
          testId="company-lifecycle"
          value={pending ?? stored}
          options={LIFECYCLE_OPTIONS.map((value) => ({
            value,
            label: label(value),
          }))}
          // Busy, not disabled: the button keeps focus and its full ink
          // while the write is out, and refuses the press itself. `commit`
          // turns away a pick that lands anyway (from the keyboard).
          aria-busy={saving || undefined}
          onChange={(next) => void commit(next)}
        />
      )}
    </Field>
  );
}
