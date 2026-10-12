// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Disclosure } from "../design-system/atoms";
import { PanelBody } from "../design-system/panel";
import { useT } from "../i18n";
import { SectionSummary } from "./companyrailshared";
import { useListsAvailable, useRecordLists } from "./lists.queries";
import { RecordListsBody } from "./recordlists";

/**
 * The lists this account is on that the reader may find, as one slice of the
 * rail, drawn by the record pages' shared block. Nothing while lists are
 * switched off, rather than a slice that could only answer 404.
 */
export function ListsSection({ companyId }: Readonly<{ companyId: string }>) {
  if (!useListsAvailable()) {
    return null;
  }
  return <CompanyLists companyId={companyId} />;
}

function CompanyLists({ companyId }: Readonly<{ companyId: string }>) {
  const t = useT();
  const lists = useRecordLists("company", companyId);
  return (
    <Disclosure
      className="co-sect"
      summary={
        <SectionSummary
          title={t("lists.record.title")}
          count={lists.data?.data.length}
        />
      }
    >
      <PanelBody>
        <RecordListsBody entityType="company" entityId={companyId} />
      </PanelBody>
    </Disclosure>
  );
}
