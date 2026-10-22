// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Clock, ExternalLink, MapPin, Video } from "lucide-react";
import type { components } from "../../api/schema";
import { Button, Card } from "../../design-system/atoms";
import { CompanyLogo } from "../../design-system/companylogo";
import { ErrorLine } from "../../design-system/errorline";
import { Heading } from "../../design-system/heading";
import { Panel, PanelBody } from "../../design-system/panel";
import { SettingList, SettingRow } from "../../design-system/settingrow";
import { Switch } from "../../design-system/switch";
import { isMailbox } from "../../format/connectorprovider";
import { formatNumber } from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { PROVIDER_VIDEO_APP, VIDEO_APP_NAME } from "../../lib/booking-video";
import { useConnectors } from "../../lib/connectors";
import { navigate } from "../../lib/router";
import { usePatchSchedulingProfile } from "../../lib/scheduling-profile-query";
import { videoOf } from "./meeting-link-drawer";
import "./meeting-cards.css";

type Profile = components["schemas"]["SchedulingProfile"];

/**
 * The public booking page as a guest meets it, beside the settings that feed
 * it. Only the reminder is set here; the company and the host's name are read
 * from where they are kept, and each row links there.
 */
export function GuestViewCard({ profile }: Readonly<{ profile: Profile }>) {
  const t = useT();
  const connectors = useConnectors();
  const reminder = usePatchSchedulingProfile();
  const mailbox = connectors.data?.data.find(
    (item) => isMailbox(item.provider) && item.status !== "disconnected",
  );
  const noMailbox = connectors.isSuccess && mailbox === undefined;
  return (
    <Panel
      title={t("scheduling.guestView")}
      titleAction={
        <Button onClick={() => navigate({ screen: "book", id: "preview" })}>
          <ExternalLink aria-hidden />
          {t("scheduling.preview")}
        </Button>
      }
    >
      <PanelBody className="meeting-guest">
        <BookingPagePreview profile={profile} />
        <div>
          <SettingList>
            <SettingRow
              label={t("scheduling.companyRow")}
              description={t(companyHelp(profile))}
              control={
                <Button
                  onClick={() =>
                    navigate({ screen: "settings", id: "company" })
                  }
                >
                  {t(
                    profile.logo_url
                      ? "scheduling.change"
                      : "scheduling.addLogo",
                  )}
                </Button>
              }
            />
            <SettingRow
              label={t("scheduling.yourName")}
              description={t("scheduling.yourNameHelp", {
                name: profile.host_name || t("field.unset"),
              })}
              control={
                <Button
                  onClick={() =>
                    navigate({ screen: "settings", id: "account" })
                  }
                >
                  {t("scheduling.change")}
                </Button>
              }
            />
            <SettingRow
              label={t("scheduling.reminderRow")}
              description={
                mailbox
                  ? t("scheduling.reminderFrom", {
                      mailbox: mailbox.account_label ?? "",
                    })
                  : t("scheduling.reminderNoMailbox")
              }
              control={(control) => (
                <Switch
                  label={t("scheduling.reminderRow")}
                  labelHidden
                  describedBy={control["aria-describedby"]}
                  checked={profile.email_reminder ?? false}
                  disabled={noMailbox}
                  pending={reminder.isPending}
                  onChange={(on) => reminder.mutate({ email_reminder: on })}
                />
              )}
            />
          </SettingList>
          <ErrorLine error={reminder.error} />
        </div>
      </PanelBody>
    </Panel>
  );
}

function companyHelp(profile: Profile): MessageKey {
  if (!profile.company_name) return "scheduling.companyNoName";
  return profile.logo_url
    ? "scheduling.companyRowHelp"
    : "scheduling.companyNoLogo";
}

// A static miniature of the page's header and summary. Kept here rather than
// in the design system: it has one caller, and the guest page is its source.
function BookingPagePreview({ profile }: Readonly<{ profile: Profile }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const company = profile.company_name ?? "";
  const video = videoOf(profile);
  const app =
    video === "app" && profile.provider !== ""
      ? VIDEO_APP_NAME[PROVIDER_VIDEO_APP[profile.provider]]
      : null;
  return (
    <figure className="meeting-preview-figure">
      <figcaption className="sr-only">
        {t("scheduling.previewLabel")}
      </figcaption>
      <Card as="div" inset className="meeting-preview">
        {company && (
          <div className="meeting-preview-brand">
            {profile.logo_url && (
              <CompanyLogo
                name={company}
                src={profile.logo_url}
                fallback={company}
              />
            )}
            <span>{company}</span>
          </div>
        )}
        <span className="t-caption">{profile.host_name}</span>
        <Heading as="span" size="small">
          {profile.title}
        </Heading>
        <span className="meeting-preview-line">
          <Clock aria-hidden size={14} />
          {plural("scheduling.minutes", profile.duration_minutes, {
            count: formatNumber(profile.duration_minutes, locale),
          })}
        </span>
        {app && (
          <span className="meeting-preview-line">
            <Video aria-hidden size={14} />
            {app}
          </span>
        )}
        {video === "own" && (
          <span className="meeting-preview-line">
            <MapPin aria-hidden size={14} />
            {profile.location}
          </span>
        )}
      </Card>
    </figure>
  );
}
