import type { components } from "../api/schema";
import { Panel, PanelBody, PanelRow } from "../design-system/panel";
import { formatNumber } from "../format/format";
import { useLocale, usePlural, useT } from "../i18n";
import { useExtensionReports } from "../lib/extensionreports";

type Report = components["schemas"]["ExtensionReport"];

function Figures({ report }: Readonly<{ report: Report }>) {
  const t = useT();
  const { locale } = useLocale();
  if (report.kind === "count") {
    const group = report.groups[0];
    return (
      <PanelBody>
        {group?.count == null
          ? t("extensionReports.withheld")
          : formatNumber(group.count, locale)}
      </PanelBody>
    );
  }
  return (
    <>
      {report.groups.map((group, index) => (
        // A withheld group has no key, so the position is its only identity.
        <PanelRow key={group.key ?? `group-${index}`}>
          <span>
            {group.withheld
              ? t("extensionReports.withheld")
              : (group.key ?? t("extensionReports.noValue"))}
          </span>
          {group.count != null && (
            <span>{formatNumber(group.count, locale)}</span>
          )}
        </PanelRow>
      ))}
    </>
  );
}

/**
 * Reports an installed extension registers, one panel each. Reports and Home
 * both draw it, so a registered number reads the same on both. Nothing draws
 * when no extension registers one.
 */
export function ExtensionReports() {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const query = useExtensionReports();
  if (query.isError) {
    return <p className="sub">{t("extensionReports.failed")}</p>;
  }
  return (
    <>
      {(query.data?.data ?? []).map((report) => (
        <Panel key={report.name} title={report.label}>
          <Figures report={report} />
          {report.excluded_by_permission != null &&
            report.excluded_by_permission > 0 && (
              <PanelBody>
                {plural(
                  "extensionReports.excluded",
                  report.excluded_by_permission,
                  {
                    count: formatNumber(report.excluded_by_permission, locale),
                  },
                )}
              </PanelBody>
            )}
        </Panel>
      ))}
    </>
  );
}
