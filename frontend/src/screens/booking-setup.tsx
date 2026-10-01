import { Panel, PanelBody } from "../design-system/panel";
import { useT } from "../i18n";

export function BookingSetup() {
  const t = useT();
  return (
    <Panel title={t("scheduling.setupCalendar")}>
      <PanelBody>
        <p>{t("scheduling.setupInSettings")}</p>
        <a href="#/settings/meetings" target="_blank" rel="noreferrer">
          {t("scheduling.openSettings")}
        </a>
      </PanelBody>
    </Panel>
  );
}
