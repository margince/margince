import type { components } from "../../api/schema";
import { DataTable, type DataTableColumn } from "../../design-system/datatable";
import { PanelGroupHead } from "../../design-system/panel";
import { providerName } from "../../format/connectorprovider";
import { formatDateTime } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import { CAPTURE_CONCERNS, concernKey } from "../../lib/captureconcern";

type MemberConcern = components["schemas"]["CaptureConnectionConcern"];
type SharedConcern = components["schemas"]["CaptureSharedMailboxConcern"];
// A shared mailbox has no member, so its row names the mailbox in their place.
type Concern = SharedConcern & { display_name?: string | null };

export function CaptureConnectionsHealth({
  rows,
  shared = [],
  zone,
}: Readonly<{
  rows: MemberConcern[];
  shared?: SharedConcern[];
  zone: string;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const all: (Concern & { shared: boolean })[] = [
    ...rows.map((row) => ({ ...row, shared: false })),
    ...shared.map((row) => ({ ...row, shared: true })),
  ];
  if (all.length === 0) return null;
  const columns: DataTableColumn<(typeof all)[number]>[] = [
    {
      key: "member",
      header: t("captureConcern.member"),
      render: (row) =>
        row.shared ? t("captureConcern.sharedMailbox") : row.display_name,
    },
    {
      key: "provider",
      header: t("captureConcern.provider"),
      render: (row) => providerName(row.provider, t),
    },
    {
      key: "mailbox",
      header: t("captureConcern.mailbox"),
      render: (row) => row.account_label,
    },
    {
      key: "condition",
      header: t("captureConcern.condition"),
      render: (row) =>
        Object.hasOwn(CAPTURE_CONCERNS, row.concern)
          ? t(concernKey(row.concern, row.error_class))
          : row.concern,
    },
    {
      key: "since",
      header: t("captureConcern.sinceLabel"),
      render: (row) =>
        row.since ? formatDateTime(row.since, locale, zone) : null,
    },
    {
      key: "lastSuccess",
      header: t("captureConcern.lastSuccess"),
      render: (row) =>
        row.last_success_at
          ? formatDateTime(row.last_success_at, locale, zone)
          : null,
    },
  ];
  return (
    <>
      <PanelGroupHead title={t("captureConcern.connections")} level="h3" />
      <DataTable
        bleed
        fold
        label={t("captureConcern.connections")}
        columns={columns}
        rows={all}
        rowKey={(row) => row.connection_id}
      />
    </>
  );
}
