import type { components } from "../../api/schema";
import { DataTable, type DataTableColumn } from "../../design-system/datatable";
import { PanelGroupHead } from "../../design-system/panel";
import { providerName } from "../../format/connectorprovider";
import { formatDateTime } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import { CAPTURE_CONCERNS } from "../../lib/captureconcern";

type Concern = components["schemas"]["CaptureConnectionConcern"];

export function CaptureConnectionsHealth({
  rows,
  zone,
}: Readonly<{ rows: Concern[]; zone: string }>) {
  const t = useT();
  const { locale } = useLocale();
  if (rows.length === 0) return null;
  const columns: DataTableColumn<Concern>[] = [
    {
      key: "member",
      header: t("captureConcern.member"),
      render: (row) => row.display_name,
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
          ? t(CAPTURE_CONCERNS[row.concern])
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
        rows={rows}
        rowKey={(row) => row.connection_id}
      />
    </>
  );
}
