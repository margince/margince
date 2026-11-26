// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { QueryClient } from "@tanstack/react-query";
import { useId, useState } from "react";
import { EntityRef } from "../../components/entityref";
import { Button, Modal } from "../../design-system/atoms";
import { Heading } from "../../design-system/heading";
import type { Toast } from "../../design-system/toast";
import { formatNumber } from "../../format/format";
import { type Translator, useLocale, usePlural, useT } from "../../i18n";
import { readCompanyPairs, useCompanyPairs } from "../../lib/dedupe.queries";
import { navigate } from "../../lib/router";
import { companyPairsKey } from "../../lib/workkeys";

/**
 * The company page's door to the other companies an open duplicate pair puts
 * beside it, drawn only while one exists. A reader who may not read the pairs
 * gets an error from the read and sees no door.
 */
export function CompanyDuplicatesEntry({
  companyId,
}: Readonly<{ companyId: string }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const headingId = useId();
  const [open, setOpen] = useState(false);
  const others = useCompanyPairs(companyId).data ?? [];
  if (others.length === 0) return null;
  return (
    <>
      <Button
        onClick={() => setOpen(true)}
        data-testid="company-duplicates-entry"
      >
        {plural("company.duplicates.entry", others.length, {
          count: formatNumber(others.length, locale),
        })}
      </Button>
      <Modal
        open={open}
        onClose={() => setOpen(false)}
        labelledBy={headingId}
        intent="form"
      >
        <Heading size="large" id={headingId} className="t-h2 modal-title">
          {t("company.duplicates.title")}
        </Heading>
        <p>{t("company.duplicates.lead")}</p>
        <ul>
          {others.map((id) => (
            <li key={id}>
              <EntityRef kind="company" id={id} />
            </li>
          ))}
        </ul>
        <div className="actions">
          <Button onClick={() => setOpen(false)}>{t("common.close")}</Button>
        </div>
      </Modal>
    </>
  );
}

/**
 * Tells the creator of a company that another one already carries a similar
 * name, with the way to open it. The pair is filed on the server by the
 * create, so this only asks which one it filed. A failed read says nothing:
 * the company page still shows its duplicates.
 */
export async function announceCompanyDuplicates(
  client: QueryClient,
  toast: Toast,
  t: Translator,
  companyId: string,
): Promise<void> {
  const others = await client
    .fetchQuery({
      queryKey: [...companyPairsKey, companyId],
      queryFn: () => readCompanyPairs(companyId),
      staleTime: 0,
    })
    // The page's own entry shows the pairs, so a failed read here is no news.
    .catch(() => []);
  const [existing] = others;
  if (existing === undefined) return;
  toast.show(t("company.duplicates.created"), {
    tone: "warning",
    action: {
      kind: "open",
      label: t("dedupe.viewExisting"),
      onAct: () => navigate({ screen: "companies", id: existing }),
    },
  });
}
