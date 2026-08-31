// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ShieldAlert, ShieldCheck, ShieldQuestion } from "lucide-react";
import { type ReactNode, useEffect, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useCan } from "../app/capability";
import { Badge, Button } from "../design-system/atoms";
import { Popover } from "../design-system/popover";
import { formatDate } from "../format/format";
import { viewerZone } from "../format/timezone";
import { type Locale, useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { problemMessageOf, throwProblem } from "./common";
import "./companyvatmark.css";

type VatCheck = components["schemas"]["OrganizationVatCheck"];
type VatStatus = VatCheck["status"];

/** The key every reader of this consultation registers, so the check and the
 * profile field that states the number settle together. */
export function vatCheckKey(orgId: string) {
  return ["org-vat-check", orgId] as const;
}

/** What each verdict is called, which glyph carries it, and how strongly to say
 * it. `unavailable` is the register declining rather than a finding about the
 * company, so it stays untoned — colouring it would report an outage as a
 * problem with the account. */
const VERDICTS: Readonly<
  Record<
    VatStatus,
    { label: MessageKey; tone?: "success" | "danger"; icon: ReactNode }
  >
> = {
  valid: {
    label: "co.vat.status.valid",
    tone: "success",
    icon: <ShieldCheck aria-hidden />,
  },
  invalid: {
    label: "co.vat.status.invalid",
    tone: "danger",
    icon: <ShieldAlert aria-hidden />,
  },
  unavailable: {
    label: "co.vat.status.unavailable",
    icon: <ShieldQuestion aria-hidden />,
  },
};

/** isConsultation reports whether a body carries the three things the contract
 * makes required. The date is checked for PARSEABILITY rather than presence:
 * `formatDate` throws on a value `Date` cannot read, and a throw here would
 * take the whole rail down rather than one mark. */
function isConsultation(body: VatCheck | undefined): body is VatCheck {
  return (
    body !== undefined &&
    typeof body.status === "string" &&
    typeof body.vat_number === "string" &&
    typeof body.checked_at === "string" &&
    !Number.isNaN(new Date(body.checked_at).getTime())
  );
}

/** When the mark looks again after a person asked. The register usually answers
 * within a second or two, so the first look is soon; the later two are for a
 * service being slow, and there is no fourth because a reader who has not been
 * answered by then is better served by the page they will open next than by a
 * tab that keeps asking. */
const VAT_POLL_DELAYS_MS = [1_500, 5_000, 15_000];

/** sameNumber decides whether the consulted number and the stated one are the
 * same VAT ID, on the terms the SERVER uses: it upper-cases and drops spaces,
 * dots, hyphens and slashes before splitting a number into country and body
 * (`vatcheck.splitVAT`), so `de 811 907 980` and `DE811907980` are one number.
 *
 * A raw string comparison would report the record's number as having "moved"
 * because somebody typed a space, and send a reader to spend a consultation
 * re-asking about a number that did not change. Two writers of one rule, and
 * this one says so rather than pretending it is the only reader of the format —
 * a mirror in a comment beats a false alarm on a screen. */
function sameNumber(consulted: string, stated: string): boolean {
  const normalize = (raw: string) =>
    raw
      .trim()
      .toUpperCase()
      .replaceAll(/[ ./-]/g, "");
  return normalize(consulted) === normalize(stated);
}

/** markName is what the mark is called, and the three cases are three different
 * facts about the company.
 *
 * The one worth spelling out is the third. A status this build has no name for
 * is a server newer than this tab — an ordinary state during a deploy — and it
 * must NOT fall through to "not checked yet": a consultation happened, and
 * announcing it as an absence would tell a reader the opposite of the truth at
 * the only moment they are looking. The register's own word carries instead,
 * untranslated because this build has no translation for it. */
function markName(
  answer: VatCheck | null,
  verdict: { label: MessageKey } | null | undefined,
  t: (key: MessageKey, vars?: Record<string, string>) => string,
): string {
  if (answer === null) {
    return t("co.vat.markUnchecked");
  }
  return t("co.vat.markVerdict", {
    verdict: verdict ? t(verdict.label) : answer.status,
  });
}

/**
 * VatMark is the whole VAT surface: one glyph beside the number that says
 * whether the register recognises it, opening to the receipt and the verb.
 *
 * It replaces a card two tabs away behind a collapsed section. That card held
 * everything this does and a reader looking straight at the VAT ID never found
 * it — the check belongs where the number is, or it is a feature nobody uses.
 *
 * The glyph carries the VERDICT rather than a bare "check this" action, because
 * whether a stated number holds up is the fact a reader wants at a glance; the
 * consultation is what they want occasionally. A number a copied imprint took
 * from another company is the finding this exists for, and it should not take a
 * click to see.
 */
export function VatMark({
  orgId,
  stated,
}: Readonly<{ orgId: string; stated: string }>): ReactNode {
  const t = useT();
  const { locale } = useLocale();
  const zone = viewerZone();

  // 404 is the honest "never consulted", and leaves the mark saying so rather
  // than reporting a failure nobody caused.
  const check = useQuery({
    queryKey: vatCheckKey(orgId),
    queryFn: async () => {
      const { data, error, response } = await api.GET(
        "/organizations/{id}/vat-check",
        { params: { path: { id: orgId } } },
      );
      if (response.status === 404) {
        return null;
      }
      if (error) {
        throwProblem(error);
      }
      // A body carrying neither a verdict nor a readable date is not a
      // consultation, whatever its status code said. Reporting it as "never
      // consulted" would state a business fact about the company on the
      // strength of a broken response.
      if (!isConsultation(data)) {
        throw new Error(
          "the VAT register answered with something this app cannot read",
        );
      }
      return data;
    },
  });

  // A read still in flight draws nothing rather than a third state: the mark
  // sits inside a value the reader is already looking at, and a glyph that
  // appears a beat later under the eye is worse than one that appears with the
  // row.
  if (check.isPending) {
    return null;
  }
  // A read that FAILED is a different fact from one that has not answered, and
  // collapsing them would hide a stored verdict behind silence. The mark says
  // it could not ask and offers the retry, because "we do not know right now"
  // is honest where drawing nothing implies there is nothing to know.
  if (check.isError) {
    return (
      <span className="vatmark">
        <button
          type="button"
          className="vatmark-glyph vatmark-retry"
          data-tone="none"
          onClick={() => void check.refetch()}
        >
          <ShieldQuestion aria-hidden />
          <span className="sr-only">{t("co.vat.markUnreadable")}</span>
        </button>
      </span>
    );
  }
  const answer = check.data ?? null;
  const verdict = answer === null ? null : VERDICTS[answer.status];

  return (
    <Popover
      className="vatmark"
      label={
        <span className="vatmark-glyph" data-tone={verdict?.tone ?? "none"}>
          {verdict?.icon ?? <ShieldQuestion aria-hidden />}
          {/* The glyph is the whole visible label, so the name is spoken here.
              As TEXT rather than an aria-label, because Popover's trigger takes
              its name from its children — and it carries the VERDICT, not just
              the verb: "VAT ID: not valid" is the fact a sighted reader gets
              from the colour, while "check VAT ID" would make them press a
              control to learn it. */}
          <span className="sr-only">{markName(answer, verdict, t)}</span>
        </span>
      }
    >
      <div className="vatmark-panel">
        {answer === null ? (
          <p className="t-caption">{t("co.vat.never")}</p>
        ) : (
          <VatReceipt check={answer} locale={locale} zone={zone} />
        )}
        <AskTheRegister orgId={orgId} consulted={answer !== null} />
        {/* The number this mark answers for, last: a receipt names what it was
            issued for, and the row above the mark can be edited after a check
            — so a mark that showed only a verdict could sit beside a number
            nobody ever consulted. */}
        {answer !== null && sameNumber(answer.vat_number, stated) === false && (
          <p className="t-caption vatmark-stale">{t("co.vat.numberMoved")}</p>
        )}
      </div>
    </Popover>
  );
}

/** VatReceipt is what the register answered and the proof it was asked. The
 * consultation number is the half that carries weight: a business treating a
 * sale as intra-EU has to show it verified its counterpart, and what a tax
 * authority accepts is that receipt, tied to the number and the day. */
function VatReceipt({
  check,
  locale,
  zone,
}: Readonly<{
  check: VatCheck;
  locale: Locale;
  zone: string;
}>): ReactNode {
  const t = useT();
  // A status this build has no name for is a server newer than this tab. The
  // rest of the consultation is still worth reading, so the unknown verdict
  // renders as the register's own word, untoned.
  const verdict = VERDICTS[check.status];
  return (
    <dl className="vatmark-receipt">
      <div>
        <dt>{t("co.vat.verdict")}</dt>
        <dd>
          {verdict ? (
            <Badge tone={verdict.tone}>{t(verdict.label)}</Badge>
          ) : (
            <Badge>{check.status}</Badge>
          )}
        </dd>
      </div>
      <div>
        <dt>{t("co.vat.number")}</dt>
        <dd>{check.vat_number}</dd>
      </div>
      {/* A number that validates to a company nobody recognises is the finding:
          a copied imprint states somebody else's VAT ID, and only the name the
          register holds exposes it. */}
      {check.registered_name && (
        <div>
          <dt>{t("co.vat.registeredName")}</dt>
          <dd>{check.registered_name}</dd>
        </div>
      )}
      {check.registered_address && (
        <div>
          <dt>{t("co.vat.registeredAddress")}</dt>
          <dd>{check.registered_address}</dd>
        </div>
      )}
      <div>
        <dt>{t("co.vat.checkedAt")}</dt>
        <dd>{formatDate(check.checked_at, locale, zone)}</dd>
      </div>
      <div>
        <dt>{t("co.vat.receipt")}</dt>
        <dd>
          {check.consultation_number ? (
            <code>{check.consultation_number}</code>
          ) : (
            <span className="t-caption">{t("co.vat.noReceipt")}</span>
          )}
        </dd>
      </div>
    </dl>
  );
}

/**
 * AskTheRegister consults the register again about the number on record.
 *
 * Nothing re-asks on a schedule — a verdict going stale is not an event the
 * product can observe — so without this a stored answer stood forever, and a
 * rep who knew a registration had changed at the registry could not act on it.
 */
function AskTheRegister({
  orgId,
  consulted,
}: Readonly<{ orgId: string; consulted: boolean }>): ReactNode {
  const t = useT();
  const queryClient = useQueryClient();
  const canEdit = useCan("organization", "update");
  // The organization whose answer we are waiting for, or null. Set by the
  // mutation and cleared once the poll below has run its course.
  const [waitingFor, setWaitingFor] = useState<string | null>(null);
  const ask = useMutation({
    // The organization is a VARIABLE rather than a closure over this render: a
    // click landing between the commit and React Query re-arming the options
    // would otherwise run against the previous render's id.
    mutationFn: async (id: string) => {
      const { error, response } = await api.POST(
        "/organizations/{id}/vat-check",
        { params: { path: { id } } },
      );
      // `response.ok` as well as `error`, and the second is what catches a
      // failure here: this endpoint answers 202 with NO BODY, so a bodiless
      // non-2xx leaves openapi-fetch nothing to parse and `error` comes back
      // falsy. Checked alone it would report a refused request as asked.
      if (error || !response.ok) {
        throwProblem(error);
      }
      return id;
    },
    // Deliberately NOT invalidating here. The consultation is QUEUED — 202,
    // no body — and the worker has not asked the register yet, so a refetch on
    // success re-reads the OLD verdict and caches it as though it were the
    // answer to this request. The reader would then watch a stale word settle
    // under a line saying the register had been asked.
    //
    // Waiting is the honest surface instead: the panel says the answer appears
    // once it replies, and the poll below is what makes that true. Keyed on
    // what the mutation RETURNED rather than this render's orgId, so a click in
    // React Query's re-arming window cannot poll the company the reader was
    // looking at a moment ago.
    onSuccess: (asked) => setWaitingFor(asked),
  });

  // The register answers out of band, so the only way the mark learns is to
  // look again. A few spaced attempts rather than a subscription: the whole
  // exchange is one HTTP call the worker makes, usually inside a second or two,
  // and a socket for that would be machinery nobody needs. The attempts stop
  // whether or not an answer arrived — a poll that ran forever would keep
  // asking about a company the reader left.
  useEffect(() => {
    if (waitingFor === null) {
      return;
    }
    const timers = VAT_POLL_DELAYS_MS.map((delay, attempt) =>
      setTimeout(() => {
        // refetchQueries, not invalidateQueries: the second marks the entry
        // stale and refetches only where an observer is currently mounted, and
        // this poll lives inside a popover panel the reader may have closed. A
        // request that answered nothing because a panel shut is the shape of
        // bug this whole change exists to stop.
        void queryClient.refetchQueries({ queryKey: vatCheckKey(waitingFor) });
        if (attempt === VAT_POLL_DELAYS_MS.length - 1) {
          setWaitingFor(null);
        }
      }, delay),
    );
    return () => {
      for (const timer of timers) {
        clearTimeout(timer);
      }
    };
  }, [waitingFor, queryClient]);

  if (!canEdit) {
    return null;
  }
  return (
    <div className="vatmark-ask">
      <Button
        variant="ghost"
        small
        pending={ask.isPending}
        onClick={() => ask.mutate(orgId)}
      >
        {t(consulted ? "co.vat.askAgain" : "co.vat.askNow")}
      </Button>
      {ask.isSuccess && <p className="t-caption">{t("co.vat.asked")}</p>}
      {ask.error !== null && (
        <p className="t-caption" role="status">
          {problemMessageOf(ask.error, t)}
        </p>
      )}
    </div>
  );
}
