// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A lead leaving a note on somebody else's day.

import { useState } from "react";
import {
  Button,
  Field,
  SegmentedControl,
  Textarea,
} from "../../design-system/atoms";
import { Panel, PanelBody } from "../../design-system/panel";
import { useFieldFaults } from "../../design-system/savepage";
import { Select, type SelectOption } from "../../design-system/select";
import { useToast } from "../../design-system/toast";
import { useLocale, useT } from "../../i18n";
import { blankRequired } from "../../lib/blankrequired";
import { problemCodeOf } from "../../lib/problem";
import { useCoachTeammate, type WorklistItem } from "./worklist.queries";

// COACH_KINDS is the contract's vocabulary, spelled once. A kind the server
// gains and this list lacks is not offered, so no control refuses its press.
const COACH_KINDS = [
  "coach_reply_aging",
  "coach_deal_needs_next_step",
  "coach_review_backlog",
  "coach_general",
] as const;

type CoachKind = (typeof COACH_KINDS)[number];

/**
 * The deals a next-step note can name: each deal on the day being read, once.
 * A deal whose name the reader may not see is left out, not offered unnamed.
 */
export function coachableDeals(
  queue: readonly WorklistItem[],
): readonly SelectOption[] {
  const named = new Map<string, string>();
  for (const item of queue) {
    const subject = item.subject;
    if (subject?.type === "deal" && subject.label) {
      named.set(subject.id, subject.label);
    }
  }
  return [...named].map(([value, label]) => ({ value, label }));
}

// Leave a note on this contact's queue. The kind is the headline Margince
// writes, and the note is the coach's own words beneath it.
//
// One panel in both states, so opening it does not change the page's shape.
export function CoachControl({
  owner,
  name,
  deals,
}: Readonly<{
  owner: string;
  /** The deals of their day, as coachableDeals reads them off the queue. */
  deals: readonly SelectOption[];
  /**
   * Whose day this is, by name, from the roster the page has already read.
   * Absent where the roster cannot name the id; the title then names nobody.
   */
  name?: string | null;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const toast = useToast();
  const [faults, faultsRoot] = useFieldFaults<HTMLDivElement>();
  const [open, setOpen] = useState(false);
  const [kind, setKind] = useState<CoachKind>("coach_general");
  const [note, setNote] = useState("");
  const [deal, setDeal] = useState("");
  // Only a next-step note names a deal, and it cannot be sent without one.
  const namesDeal = kind === "coach_deal_needs_next_step";
  const dealId = namesDeal ? deal : "";
  const coach = useCoachTeammate();
  const title = name
    ? t("worklist.manager.coachTitle", { name })
    : t("worklist.manager.coachTitleUnnamed");

  if (!open) {
    return (
      <Panel
        className="worklist-coach"
        title={title}
        // What a note does, said before the reader spends a press on it.
        actions={
          <Button variant="primary" onClick={() => setOpen(true)}>
            {t("worklist.manager.coach")}
          </Button>
        }
      >
        <PanelBody>
          <p className="t-body">{t("worklist.manager.coachIntro")}</p>
        </PanelBody>
      </Panel>
    );
  }
  return (
    <Panel
      className="worklist-coach"
      title={title}
      actions={
        <>
          <Button variant="ghost" onClick={() => setOpen(false)}>
            {t("worklist.manager.cancel")}
          </Button>
          <Button
            variant="primary"
            disabled={coach.isPending}
            onClick={() => {
              const blank = namesDeal
                ? blankRequired(locale, {
                    deal: [t("worklist.manager.coachDeal"), dealId],
                  })
                : new Map<string, string>();
              if (faults.show(blank)) {
                return;
              }
              coach.mutate(
                { recipientUserId: owner, kind, note, dealId },
                {
                  onSuccess: () => {
                    setOpen(false);
                    setNote("");
                    setDeal("");
                    toast.show(t("worklist.manager.coached"));
                  },
                  onError: (failure) => coachRefused(failure, name, t, toast),
                },
              );
            }}
          >
            {t("worklist.manager.coachConfirm")}
          </Button>
        </>
      }
    >
      {/* `.form-stack` spaces the fields. One `PanelBody`, because the kind,
          the deal and the note are one question, not sections. */}
      <PanelBody>
        <div className="form-stack" ref={faultsRoot}>
          {/* The visible label is also the strip's accessible name, so both
            readers learn what the four buttons choose between. */}
          <div className="field">
            <span className="t-label">{t("worklist.manager.coachAbout")}</span>
            <SegmentedControl
              options={COACH_KINDS}
              value={kind}
              onChange={setKind}
              label={t("worklist.manager.coachAbout")}
              labels={{
                coach_reply_aging: t("worklist.manager.kind.reply_aging"),
                coach_deal_needs_next_step: t(
                  "worklist.manager.kind.next_step",
                ),
                coach_review_backlog: t("worklist.manager.kind.review_backlog"),
                coach_general: t("worklist.manager.kind.general"),
              }}
            />
          </div>
          {namesDeal && (
            <Field
              label={t("worklist.manager.coachDeal")}
              required
              error={faults.of("deal")}
              hint={
                deals.length === 0
                  ? t("worklist.manager.coachNoDeal")
                  : undefined
              }
            >
              {(control) => (
                <Select
                  {...control}
                  options={deals}
                  value={deal}
                  onChange={(next) => {
                    setDeal(next);
                    faults.edited("deal");
                  }}
                  disabled={deals.length === 0}
                />
              )}
            </Field>
          )}
          <Field label={t("worklist.manager.note")}>
            {(control) => (
              <Textarea
                {...control}
                value={note}
                maxLength={500}
                rows={3}
                onChange={(event) => setNote(event.target.value)}
              />
            )}
          </Field>
        </div>
      </PanelBody>
    </Panel>
  );
}

/**
 * What a refused note says. A 403 `permission_denied` means the caller may not
 * coach this contact, so retrying cannot help, and the toast says so by name.
 * It reads the problem code, never the localized message text. Without a name
 * it falls back to the generic sentence.
 */
function coachRefused(
  failure: unknown,
  name: string | null | undefined,
  t: ReturnType<typeof useT>,
  toast: ReturnType<typeof useToast>,
): void {
  const refused = problemCodeOf(failure) === "permission_denied";
  toast.show(
    refused && name
      ? t("worklist.manager.coachRefused", { name })
      : t("worklist.manager.coachFailed"),
    { tone: "danger" },
  );
}
