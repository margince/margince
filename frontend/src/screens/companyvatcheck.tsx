// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useCan } from "../app/capability";
import { Badge, Button, Card } from "../design-system/atoms";
import { formatDate } from "../format/format";
import { viewerZone } from "../format/timezone";
import { type Locale, useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { problemMessageOf, QueryGate, throwProblem } from "./common";
// The card's own rows, defined beside the rest of the company record's.
import "./company360.css";

type VatCheck = components["schemas"]["OrganizationVatCheck"];
type VatStatus = VatCheck["status"];

/** What each verdict is called, and how strongly to say it. `unavailable` is
 * the register declining rather than a finding about the company, so it stays
 * untoned — colouring it would report an outage as a problem with the account. */
const VERDICTS: Readonly<
  Record<VatStatus, { label: MessageKey; tone?: "success" | "danger" }>
> = {
  valid: { label: "co.vat.status.valid", tone: "success" },
  invalid: { label: "co.vat.status.invalid", tone: "danger" },
  unavailable: { label: "co.vat.status.unavailable" },
};

/** isConsultation reports whether a body carries the three things the contract
 * makes required. The date is checked for PARSEABILITY rather than presence:
 * `formatDate` throws on a value `Date` cannot read, and a throw here is a
 * blank company record rather than a blank card. */
function isConsultation(body: VatCheck | undefined): body is VatCheck {
  return (
    body !== undefined &&
    typeof body.status === "string" &&
    typeof body.vat_number === "string" &&
    typeof body.checked_at === "string" &&
    !Number.isNaN(new Date(body.checked_at).getTime())
  );
}

/**
 * VatCheckCard shows what the EU VAT register answered about this company, and
 * the receipt for having asked.
 *
 * The receipt is the half that carries weight. A business treating a sale as
 * intra-EU has to show it verified its counterpart, and what a tax authority
 * accepts is the consultation number the register issues — tied to the number
 * consulted and the day it was asked. So the number, the date and the receipt
 * are shown together, and the date is the REGISTER's rather than ours.
 *
 * The registered name is shown beside the verdict because a number that
 * validates to a company nobody recognises is the finding: a copied imprint
 * states somebody else's VAT ID, and only the name exposes it.
 */
export function VatCheckCard({
  orgId,
}: Readonly<{ orgId: string }>): ReactNode {
  const t = useT();
  const { locale } = useLocale();
  const zone = viewerZone();

  // 404 is the honest "never consulted", and leaves the card saying so rather
  // than reporting a failure nobody caused.
  const check = useQuery({
    queryKey: ["org-vat-check", orgId],
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
      // A body that carries neither a verdict nor a date is not a
      // consultation, whatever its status code said. It is a fault rather than
      // an absence: reporting it as "never consulted" would state a business
      // fact about the company on the strength of a broken response, and
      // rendering it would put an unparseable date through the formatter and
      // take the whole record page down.
      if (!isConsultation(data)) {
        throw new Error(
          "the VAT register answered with something this app cannot read",
        );
      }
      return data;
    },
  });

  return (
    <Card title={t("co.vat.title")} sub={t("co.vat.sub")}>
      <QueryGate query={check} pendingLabel={t("co.vat.title")}>
        {(answer) => (
          <>
            {answer === null ? (
              <p className="t-caption">{t("co.vat.never")}</p>
            ) : (
              <VatVerdict check={answer} locale={locale} zone={zone} />
            )}
            {/* Offered in BOTH states, and the unconsulted one is the state it
                matters in most: a number the crawl never checked showed a
                reader "never consulted" and no way to change that. */}
            <AskTheRegister orgId={orgId} consulted={answer !== null} />
          </>
        )}
      </QueryGate>
    </Card>
  );
}

/**
 * AskTheRegister consults the register again about the number on record.
 *
 * Nothing re-asks on a schedule — a verdict going stale is not an event the
 * product can observe — so without this a stored answer stood forever, and a rep
 * who knew a registration had changed at the registry could not act on it.
 *
 * The two refusals are shown as themselves. Asking again too soon says to wait,
 * because the answer already on the card still stands; having no number to
 * consult says to enter one, which is a different thing for the reader to do.
 */
function AskTheRegister({
  orgId,
  consulted,
}: Readonly<{ orgId: string; consulted: boolean }>): ReactNode {
  const t = useT();
  const queryClient = useQueryClient();
  const canEdit = useCan("organization", "update");
  const ask = useMutation({
    // The organization is a VARIABLE rather than a closure over this render:
    // a click landing between the commit and React Query re-arming the options
    // would otherwise run against the previous render's id.
    mutationFn: async (id: string) => {
      const { error } = await api.POST("/organizations/{id}/vat-check", {
        params: { path: { id } },
      });
      if (error) {
        throwProblem(error);
      }
    },
    // The consultation is QUEUED, so there is nothing to read back yet. The
    // card is invalidated anyway: the answer lands out of band, and a reader
    // who navigates back is the one who sees it.
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: ["org-vat-check", orgId] }),
  });

  if (!canEdit) {
    return null;
  }
  return (
    <div className="co-card-actions">
      <Button
        variant="ghost"
        disabled={ask.isPending}
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

/** VatVerdict draws one consultation: what was asked, what came back, and what
 * proves it was asked. */
function VatVerdict({
  check,
  locale,
  zone,
}: Readonly<{
  check: VatCheck;
  locale: Locale;
  zone: string;
}>): ReactNode {
  const t = useT();
  // A status this build has no name for is a server newer than this tab, not a
  // reason to take the company page down with it. The rest of the consultation
  // — the number, the date, the receipt — is still worth reading, so the
  // unknown verdict renders as the register's own word, untoned.
  const verdict = VERDICTS[check.status];
  return (
    <dl className="co-vat">
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
