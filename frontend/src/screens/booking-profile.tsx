import { useMutation, useQueryClient } from "@tanstack/react-query";
import { CalendarDays, Copy, ExternalLink } from "lucide-react";
import { useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Button, TextInput } from "../design-system/atoms";
import { useClipboardCopy } from "../design-system/clipboardcopy";
import { CompanyLogo } from "../design-system/companylogo";
import { ConfirmModal } from "../design-system/confirmmodal";
import { ErrorLine } from "../design-system/errorline";
import { FactList } from "../design-system/factlist";
import { Heading } from "../design-system/heading";
import { Panel, PanelBody, PanelIntro } from "../design-system/panel";
import { QueryGate } from "../design-system/querystates";
import { SettingList, SettingRow } from "../design-system/settingrow";
import { Switch } from "../design-system/switch";
import { useT } from "../i18n";
import { throwProblem, unwrap } from "../lib/problem";
import { navigate } from "../lib/router";
import { useBookingCalendar } from "./booking-calendar-state";
import { BookingBack } from "./booking-common";
import { useSchedulingProfile } from "./scheduling-profile-query";
import "./booking-profile.css";

type Profile = components["schemas"]["SchedulingProfile"];

export function BookingProfileScreen({
  embedded = false,
}: Readonly<{ embedded?: boolean }>) {
  const t = useT();
  const query = useSchedulingProfile();
  return (
    <div className={embedded ? "book-form" : "book-page"}>
      {!embedded && <BookingBack />}
      {!embedded && (
        <header className="book-heading">
          <div>
            <Heading as="h1" size="large">
              {t("scheduling.myLink")}
            </Heading>
            <p className="t-caption">{t("scheduling.linkIntro")}</p>
          </div>
          <Button onClick={() => navigate({ screen: "book", id: "contact" })}>
            <CalendarDays aria-hidden />
            {t("scheduling.new")}
          </Button>
        </header>
      )}
      <QueryGate pendingLabel={t("common.loading")} query={query}>
        {(profile) => <ProfileForm profile={profile} embedded={embedded} />}
      </QueryGate>
    </div>
  );
}

function ProfileForm({
  profile,
  embedded,
}: Readonly<{ profile: Profile; embedded: boolean }>) {
  const t = useT();
  const client = useQueryClient();
  const calendar = useBookingCalendar(profile.provider, false);
  const [replace, setReplace] = useState(false);
  const copy = useClipboardCopy(profile.public_url ?? "", {
    copy: t("scheduling.copyLink"),
    copied: t("scheduling.copied"),
    remedy: t("scheduling.copyFallback"),
  });
  const save = useMutation({
    mutationFn: async (
      value: Partial<Pick<Profile, "enabled" | "replace_link">>,
    ) => {
      const latest = await api.GET("/scheduling/profile");
      if (latest.error) throwProblem(latest.error);
      return unwrap(
        await api.PUT("/scheduling/profile", {
          body: { ...latest.data, ...value },
        }),
      );
    },
    onSuccess: (value) => client.setQueryData(["scheduling-profile"], value),
  });
  return (
    <div className="book-form">
      {/* On its own page the heading above names this lead card; in settings
          it is one titled panel among several. */}
      <Panel
        title={embedded ? t("scheduling.myLink") : undefined}
        tone={embedded ? undefined : "accent"}
      >
        <PanelBody className="form-stack">
          {embedded && <PanelIntro>{t("scheduling.linkIntro")}</PanelIntro>}
          {profile.enabled &&
            !calendar.connections.isPending &&
            !calendar.ready && (
              <ErrorLine>{t("scheduling.publicCalendarUnavailable")}</ErrorLine>
            )}
          <div className="booking-link">
            {profile.public_url ? (
              <TextInput
                aria-label={t("scheduling.myLink")}
                readOnly
                value={profile.public_url}
              />
            ) : (
              <p className="t-caption">
                {t(
                  profile.slug
                    ? "scheduling.publicUrlMissing"
                    : "scheduling.linkNotCreated",
                )}
              </p>
            )}
            <Button
              variant="primary"
              disabled={!profile.public_url}
              onClick={copy.copy}
            >
              <Copy aria-hidden />
              {copy.label}
            </Button>
            <Button onClick={() => navigate({ screen: "book", id: "preview" })}>
              <ExternalLink aria-hidden />
              {t("scheduling.preview")}
            </Button>
          </div>
          {copy.notice}
        </PanelBody>
        <SettingList bleed="settings">
          <SettingRow
            label={t("scheduling.acceptBookings")}
            description={t("scheduling.acceptBookingsHelp")}
            control={
              <Switch
                label={t("scheduling.acceptBookings")}
                labelHidden
                checked={profile.enabled}
                pending={save.isPending}
                // The refusal rides the switch, so at 390 it stays beside it.
                reason={
                  !profile.enabled && !profile.provider
                    ? t("scheduling.calendarSetupNeeded")
                    : undefined
                }
                onChange={(on) => save.mutate({ enabled: on })}
              />
            }
          />
          <SettingRow
            label={t("scheduling.replace")}
            description={t("scheduling.replaceShort")}
            control={
              <Button
                disabled={!profile.slug || save.isPending}
                onClick={() => setReplace(true)}
              >
                {t("scheduling.replaceAction")}
              </Button>
            }
          />
        </SettingList>
        {embedded && (
          <PanelBody>
            <BookingIdentity profile={profile} />
          </PanelBody>
        )}
        {save.error && !replace && (
          <PanelBody>
            <ErrorLine error={save.error} />
          </PanelBody>
        )}
        <ConfirmModal
          open={replace}
          onClose={() => setReplace(false)}
          title={t("scheduling.replace")}
          confirmLabel={t("scheduling.replace")}
          pending={save.isPending}
          onConfirm={() =>
            save.mutate(
              { replace_link: true },
              { onSuccess: () => setReplace(false) },
            )
          }
        >
          <p>{t("scheduling.replaceHelp")}</p>
          <ErrorLine error={save.error} />
        </ConfirmModal>
      </Panel>
      {!embedded && (
        <a href="#/settings/meetings">{t("scheduling.openSettings")}</a>
      )}
    </div>
  );
}

// Where the booking page's name and branding come from, since neither is
// edited here. The company shows as the guest's page draws it: the logo when
// there is one, else the name.
function BookingIdentity({ profile }: Readonly<{ profile: Profile }>) {
  const t = useT();
  const company = profile.company_name;
  return (
    <FactList
      facts={[
        {
          key: "name",
          term: t("scheduling.hostName"),
          value: profile.host_name || t("field.unset"),
          note: <a href="#/settings/account">{t("settings.tab.account")}</a>,
        },
        {
          key: "company",
          term: t("scheduling.brand"),
          value: company ? (
            <span className="booking-brand-mark">
              <CompanyLogo
                name={company}
                src={profile.logo_url}
                fallback={company}
              />
            </span>
          ) : (
            t("field.unset")
          ),
          note: <a href="#/settings/company">{t("settings.companyTitle")}</a>,
        },
      ]}
    />
  );
}
