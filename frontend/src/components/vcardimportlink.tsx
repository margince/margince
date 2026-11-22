// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useId } from "react";
import { AppLink } from "../design-system/applink";
import { Button } from "../design-system/atoms";
import { useT } from "../i18n";
import { IMPORT_ID } from "../lib/importpage";
import { routeHash } from "../lib/router";
import { hashWithParams } from "../lib/urlstate";
import { useActionRefusal } from "../lib/writerefusal";

/** The address dial that names the company an import files its new contacts under. */
export const VCARD_COMPANY_DIAL = "company";

/**
 * Why this seat is refused the vCard import, or undefined when it is not. The
 * import writes a new contact or updates the one a card matches, so it asks
 * both grants (contacts/vcardimport.go).
 */
export function useVCardImportRefusal(): string | undefined {
  const create = useActionRefusal({ object: "contact", action: "create" });
  const update = useActionRefusal({ object: "contact", action: "update" });
  return create ?? update;
}

/** The import page's address, for one company's Contacts tab when `companyId` is named. */
export function vcardImportHref(companyId?: string): string {
  const page = routeHash({ screen: "contacts", id: IMPORT_ID });
  return companyId === undefined
    ? page
    : hashWithParams(page, new Map([[VCARD_COMPANY_DIAL, companyId]]));
}

/**
 * The link a contact follows to import address cards. A handed-over card is
 * first-party data, because the contact gave it, which justifies storing their
 * details. A human pressing this link makes the import write rather than stage.
 * So the verb lives on the contact list beside the other ways a contact comes
 * to exist, not in a settings page.
 */
export function VCardImportAction({
  refusalId,
  companyId,
}: Readonly<{
  // The page's sentence saying why the import is refused, when it is.
  refusalId?: string;
  // The company whose Contacts tab the import came from.
  companyId?: string;
}>) {
  const t = useT();
  if (refusalId !== undefined) {
    return (
      <Button reasonId={refusalId} data-testid="vcard-import">
        {t("vcardImport.action")}
      </Button>
    );
  }
  return (
    <AppLink
      className="btn btn-ghost"
      data-testid="vcard-import"
      href={vcardImportHref(companyId)}
    >
      {t("vcardImport.action")}
    </AppLink>
  );
}

/** The import verb on a company's Contacts tab, with the sentence that says why a seat is refused it. */
export function CompanyVCardImport({
  companyId,
}: Readonly<{ companyId: string }>) {
  const refusalId = useId();
  const refusal = useVCardImportRefusal();
  return (
    <>
      <VCardImportAction
        companyId={companyId}
        refusalId={refusal === undefined ? undefined : refusalId}
      />
      {refusal !== undefined && (
        <span className="t-caption" id={refusalId}>
          {refusal}
        </span>
      )}
    </>
  );
}
