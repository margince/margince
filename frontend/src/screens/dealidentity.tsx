// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The one line of facts under a deal's name, and the facts it draws.
//
// Its own file rather than a section of the deal screen: the screen is at the
// tree's file-length ceiling, and this is a self-contained presentational row
// with its own type, read by the deal page and by the 360's tests and stories.

import type { ReactNode } from "react";
import type { components } from "../api/schema";
import {
  IdentityFact,
  IdentityLine,
  IdentityMeta,
} from "../design-system/identityline";
import { FieldGuard } from "../design-system/rbac";
import { formatMoney } from "../format/format";
import { type Locale, useT } from "../i18n";
import {
  type WonWithoutContract,
  WonWithoutContractFact,
} from "./dealwinreason";
import {
  EntityRef,
  rosterOwnerName,
  useRoster,
  useRosterPartial,
} from "./entityref";

type Deal = components["schemas"]["Deal"];

// What the identity line reads off a deal. Every field is optional because
// every one of them is a fact a deal can lack or a reader can be refused.
export type DealIdentity = Pick<
  Deal,
  | "amount_minor"
  | "currency"
  | "stage_id"
  | "owner_id"
  | "company_id"
  | "partner_company_id"
  | "partner_attribution"
  | "masked_fields"
> &
  WonWithoutContract;

/**
 * The one line of facts under a deal's name: what it is worth, where it sits
 * on the board, whose deal it is, and — when one brought it — which partner.
 *
 * It is the design system's `IdentityLine`, the same row the account and the
 * contact draw under their own names, so the three records read the same way.
 * These facts used to stand in a labelled box beside the verbs instead
 * (`DealFacts`), which made the deal the only record whose head answered "what
 * is this" in a different shape from every other — and cost the verbs the room
 * they need beside a name at the record rung.
 *
 * The partner was editable in the form and rendered nowhere, so a deal that a
 * partner sourced looked identical to one we won alone. That is the fact the
 * commission is computed from, and a figure a partner is paid on has to be
 * visible on the record it came from.
 *
 * Each reference goes through EntityRef, which resolves the name and links to
 * the record — and withholds both when the reader may not open it, which is
 * why the ids are not printed as a fallback. A withheld fact NAMES the field
 * it withholds: on a line of joined facts a bare mask says only "something
 * here is hidden", and the amount, the company and the partner are three
 * different things to be refused.
 */
export function DealIdentityLine({
  deal,
  stages,
  locale,
}: Readonly<{
  // The facts this line draws and no more. A presentational row does not need
  // a whole `Deal` to say what one is worth, and asking for one makes every
  // story and test that draws the line assemble a record it does not read.
  deal: DealIdentity;
  // The stages the PAGE already sorted for the board, not a second read.
  stages: readonly { id: string; name: string }[];
  locale: Locale;
}>) {
  const t = useT();
  // Only asked for when there is an owner to name: an unowned deal needs no
  // roster read to say so.
  const roster = useRoster("user", Boolean(deal.owner_id));
  const partial = useRosterPartial("user", Boolean(deal.owner_id));
  const masked = deal.masked_fields ?? [];
  // An em dash rather than the stage id: a deal in overlay mode carries no
  // native pipeline row, and printing a UUID where a stage name goes reads as
  // a fault.
  const stage = stages.find((candidate) => candidate.id === deal.stage_id);
  return (
    <IdentityMeta>
      <IdentityLine>
        {masked.includes("company_id") ? (
          <IdentityFact>
            {t("create.company")} <FieldGuard mode="masked" />
          </IdentityFact>
        ) : (
          deal.company_id && (
            <IdentityFact>
              <EntityRef kind="company" id={deal.company_id} />
            </IdentityFact>
          )
        )}
        <IdentityFact>
          {/* A masked amount NAMES the field, like every other refusal on this
              line: a lone lock among joined facts says only that something
              here is hidden, and "no value recorded" and "you may not see the
              value" are different statements about a deal. */}
          {masked.includes("amount_minor") ? (
            <>
              {t("deals.amount")} <FieldGuard mode="masked" />
            </>
          ) : (
            dealAmount(deal, locale)
          )}
        </IdentityFact>
        <IdentityFact>{stage?.name ?? "—"}</IdentityFact>
        <IdentityFact quiet>
          {t("list.owner")}:{" "}
          {rosterOwnerName(
            deal.owner_id,
            roster,
            partial,
            t,
            t("co.pulse.unowned"),
          )}
        </IdentityFact>
        {masked.includes("partner_company_id") ? (
          // No attribution word here: what the partner did is withheld WITH
          // the partner, so naming one would decide what a partner nobody
          // could see is owed.
          <IdentityFact>
            {t("deal.partnerCompany")} <FieldGuard mode="masked" />
          </IdentityFact>
        ) : (
          deal.partner_company_id && (
            <IdentityFact>
              {/* Sourced and influenced are paid differently, so the line says
                  which one rather than a neutral "partner: X" that hides the
                  distinction the commission turns on. */}
              {t(
                deal.partner_attribution === "influenced"
                  ? "deal.partnerInfluenced"
                  : "deal.partnerSourced",
              )}{" "}
              <EntityRef kind="company" id={deal.partner_company_id} />
            </IdentityFact>
          )
        )}
        <WonWithoutContractFact deal={deal} />
      </IdentityLine>
    </IdentityMeta>
  );
}

// The value, or an em dash when the deal carries none. The masked case is the
// caller's, because a refusal on this line is written as the field's name
// beside the mark rather than as the mark alone.
function dealAmount(deal: DealIdentity, locale: Locale): ReactNode {
  if (deal.amount_minor == null || !deal.currency) {
    return "—";
  }
  return formatMoney(deal.amount_minor, deal.currency, locale);
}
