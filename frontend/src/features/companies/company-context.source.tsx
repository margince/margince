import { RefreshCw } from "lucide-react";
import { Button } from "../../design-system/atoms";
import { Panel, PanelBody, PanelIntro } from "../../design-system/panel";
import { WriteRefused } from "../../design-system/querystates";
import { SettingList, SettingRow } from "../../design-system/settingrow";
import { useT } from "../../i18n";
import type { CompanyInput } from "./company-context";

/**
 * Where we read it from, and the verb that reads it again.
 *
 * Its own card, because it answers a different question. The card above says
 * what we hold; this one says which site we hold it from and offers the reading.
 * The trust rule sits here too: website text never becomes instructions. That
 * is a promise about the read, and on the profile card it reads as a caption.
 */
export function CompanySourceCard({
  form,
  canEdit,
  refreshing,
  failure,
  onEdit,
  onRefresh,
}: Readonly<{
  form: CompanyInput;
  canEdit: boolean;
  refreshing: boolean;
  /** What went wrong with the last read, in words a reader can act on. */
  failure: string | null;
  onEdit: () => void;
  onRefresh: () => void;
}>) {
  const t = useT();
  const website = (form.website ?? "").trim();
  return (
    <Panel title={t("settings.companySourceTitle")}>
      <PanelBody>
        <PanelIntro>{t("settings.companyTrust")}</PanelIntro>
      </PanelBody>
      <SettingList bleed="settings">
        <SettingRow
          label={t("settings.companyWebsite")}
          description={t("settings.companyWebsiteHint")}
          value={website === "" ? t("field.unset") : website}
          control={
            canEdit ? (
              <Button
                variant="ghost"
                aria-label={t("settings.companyEditField", {
                  field: t("settings.companyWebsite"),
                })}
                onClick={onEdit}
              >
                {t("settings.companyEdit")}
              </Button>
            ) : null
          }
        />
        {/* Reading the website rewrites this profile, so the server asks for
            the same grant as a save. */}
        {canEdit && (
          <SettingRow
            label={t("settings.companyRefreshRow")}
            description={t("settings.companyRefreshHint")}
            control={
              <Button
                variant="primary"
                reason={
                  website === ""
                    ? t("settings.companyWebsiteRequired")
                    : undefined
                }
                pending={refreshing}
                onClick={onRefresh}
              >
                <RefreshCw aria-hidden /> {t("settings.companyRefresh")}
              </Button>
            }
          />
        )}
      </SettingList>
      {failure !== null && (
        <PanelBody>
          <WriteRefused
            titleKey="settings.companyRefreshFailed"
            message={failure}
          />
        </PanelBody>
      )}
    </Panel>
  );
}
