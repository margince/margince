// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useId, useState } from "react";
import { Button, Field, Modal, Textarea } from "../design-system/atoms";
import { Callout, type CalloutTone } from "../design-system/callout";
import { ChoiceList } from "../design-system/choicelist";
import { CopyableText, useClipboardCopy } from "../design-system/clipboardcopy";
import { Heading } from "../design-system/heading";
import { formatNumber } from "../format/format";
import { formatDayLong } from "../format/format.dates";
import { type Translator, useLocale, usePlural, useT } from "../i18n";
import type { TrialStanding } from "../lib/license-posture";
import { useMe } from "../lib/session";
import {
  TRIAL_ASKS,
  type TrialAsk,
  trialMailtoHref,
} from "../lib/trialcontact";
import { useViewerZone } from "../lib/viewerzone";
import "./trialcontact.css";

/** The tone the trial is told in: calm, then a warning, then a fault. */
export function trialTone(standing: TrialStanding): CalloutTone {
  if (standing.ended) {
    return "danger";
  }
  return standing.closing ? "warning" : "info";
}

/**
 * The trial as one standing notice, with the way out beside it: the dialog
 * that writes the email to the Margince team. `seatsUsed` goes into that email
 * when the reader may see it.
 */
export function TrialNotice({
  standing,
  seatsUsed,
}: Readonly<{ standing: TrialStanding; seatsUsed?: number }>) {
  const t = useT();
  const { locale } = useLocale();
  const plural = usePlural();
  const [open, setOpen] = useState(false);
  const day = useTrialDay(standing.endsAt);
  return (
    <>
      <Callout
        kind="standing"
        tone={trialTone(standing)}
        title={
          standing.ended
            ? t("trial.notice.endedTitle")
            : plural("trial.notice.daysLeft", standing.daysLeft, {
                count: formatNumber(standing.daysLeft, locale),
              })
        }
        actions={
          <Button variant="primary" onClick={() => setOpen(true)}>
            {t("trial.contact.title")}
          </Button>
        }
      >
        {t(standing.ended ? "trial.notice.endedBody" : "trial.notice.body", {
          date: day,
        })}
      </Callout>
      {open && (
        <TrialContactDialog
          standing={standing}
          seatsUsed={seatsUsed}
          onClose={() => setOpen(false)}
        />
      )}
    </>
  );
}

/** The day the trial ends, as the reader's calendar names it; "" unknown. */
export function useTrialDay(endsAt: string | undefined): string {
  const { locale } = useLocale();
  const zone = useViewerZone();
  return endsAt === undefined ? "" : formatDayLong(endsAt, locale, zone);
}

/**
 * The email to the Margince team, written in the dialog and sent from the
 * reader's own mail app. The reader picks what would help; the facts the team
 * needs to answer (who, when the trial ends, how many seats) are filled in.
 *
 * Without a `standing` the trial has ended and its day is not this reader's to
 * know: a seat without `license:read` reaches the dialog from the lock screen.
 */
export function TrialContactDialog({
  standing,
  seatsUsed,
  onClose,
}: Readonly<{
  standing?: TrialStanding;
  seatsUsed?: number;
  onClose: () => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const heading = useId();
  const me = useMe();
  const day = useTrialDay(standing?.endsAt);
  const ended = standing?.ended ?? true;
  const [ask, setAsk] = useState<TrialAsk>("license");
  const [note, setNote] = useState("");
  const mail = trialMail(t, {
    ask,
    note,
    company: me.data?.workspace_name ?? "",
    name: me.data?.user.display_name ?? "",
    day,
    ended,
    seatsUsed:
      seatsUsed === undefined ? undefined : formatNumber(seatsUsed, locale),
  });
  const copy = useClipboardCopy(`${mail.subject}\n\n${mail.body}`, {
    copy: t("trial.contact.copy"),
    copied: t("trial.contact.copied"),
    remedy: t("trial.contact.copyRemedy"),
  });
  return (
    <Modal open onClose={onClose} labelledBy={heading} intent="form">
      <Heading size="large" id={heading} className="modal-title">
        {t("trial.contact.title")}
      </Heading>
      <div className="form-stack">
        <TrialStatus standing={standing} day={day} />
        <ChoiceList
          legend={t("trial.contact.legend")}
          layout="cards"
          value={ask}
          onChange={setAsk}
          choices={TRIAL_ASKS.map((value) => ({
            value,
            label: t(`trial.contact.ask.${value}`),
            description: t(`trial.contact.ask.${value}.hint`),
          }))}
        />
        <Field
          label={t("trial.contact.note")}
          hint={t("trial.contact.optional")}
        >
          {(control) => (
            <Textarea
              {...control}
              grow
              value={note}
              onChange={(event) => setNote(event.target.value)}
            />
          )}
        </Field>
        <section className="trialmail" aria-labelledby={`${heading}-mail`}>
          <Heading size="small" as="h3" id={`${heading}-mail`}>
            {t("trial.contact.preview")}
          </Heading>
          <dl className="trialmail-head">
            <dt>{t("trial.contact.to")}</dt>
            <dd>{t("trial.contact.team")}</dd>
            <dt>{t("trial.contact.subject")}</dt>
            <dd>
              <b>{mail.subject}</b>
            </dd>
          </dl>
          <CopyableText text={mail.body} label={t("trial.contact.preview")} />
        </section>
        {copy.notice}
      </div>
      <div className="actions">
        <Button onClick={copy.copy} pending={copy.pending}>
          {copy.label}
        </Button>
        <a className="btn btn-primary" href={trialMailtoHref(mail)}>
          {t("trial.contact.open")}
        </a>
      </div>
    </Modal>
  );
}

/** Where the trial stands, at the head of the dialog. */
function TrialStatus({
  standing,
  day,
}: Readonly<{ standing?: TrialStanding; day: string }>) {
  const t = useT();
  if (standing === undefined) {
    return (
      <Callout
        kind="standing"
        tone="danger"
        title={t("trial.notice.endedTitle")}
      >
        {t("trial.locked.body")}
      </Callout>
    );
  }
  return (
    <Callout
      kind="standing"
      tone={trialTone(standing)}
      title={t(
        standing.ended ? "trial.contact.endedOn" : "trial.contact.endsOn",
        { date: day },
      )}
    >
      {t(standing.ended ? "trial.notice.endedBody" : "trial.notice.body", {
        date: day,
      })}
    </Callout>
  );
}

/** The subject and body of the email, in the reader's language. */
export function trialMail(
  t: Translator,
  facts: Readonly<{
    ask: TrialAsk;
    note: string;
    company: string;
    name: string;
    day: string;
    ended: boolean;
    /** Already formatted for the reader, absent when they may not see it. */
    seatsUsed?: string;
  }>,
): Readonly<{ subject: string; body: string }> {
  const lines = [
    t("trial.mail.greeting"),
    "",
    t(`trial.mail.ask.${facts.ask}`),
    "",
    t("trial.mail.company", { company: facts.company }),
  ];
  if (facts.day !== "") {
    lines.push(
      t(facts.ended ? "trial.mail.ended" : "trial.mail.ends", {
        date: facts.day,
      }),
    );
  }
  if (facts.seatsUsed !== undefined) {
    lines.push(t("trial.mail.seats", { seats: facts.seatsUsed }));
  }
  const note = facts.note.trim();
  if (note !== "") {
    lines.push("", note);
  }
  lines.push("", t("trial.mail.signoff"), facts.name);
  return {
    subject: t(`trial.mail.subject.${facts.ask}`, { company: facts.company }),
    body: lines.join("\n"),
  };
}
