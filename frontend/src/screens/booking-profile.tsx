import { useMutation, useQueryClient } from "@tanstack/react-query";
import { CalendarDays, Copy, ExternalLink } from "lucide-react";
import { useState } from "react";
import type { components } from "../api/schema";
import { Badge, Button, TextInput } from "../design-system/atoms";
import { useClipboardCopy } from "../design-system/clipboardcopy";
import { ConfirmModal } from "../design-system/confirmmodal";
import { ErrorLine } from "../design-system/errorline";
import { Heading } from "../design-system/heading";
import { Panel, PanelBody } from "../design-system/panel";
import { QueryGate } from "../design-system/querystates";
import { useT } from "../i18n";
import { navigate } from "../lib/router";
import { useBookingCalendar } from "./booking-calendar-state";
import { BookingBack } from "./booking-common";
import {
  putSchedulingChanges,
  useSchedulingProfile,
} from "./scheduling-profile-query";

type Profile = components["schemas"]["SchedulingProfile"];

export function BookingProfileScreen() {
  const t = useT();
  const query = useSchedulingProfile();
  return (
    <div className="book-page">
      <BookingBack />
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
      <QueryGate pendingLabel={t("common.loading")} query={query}>
        {(profile) => (
          <ProfileForm key={JSON.stringify(profile)} profile={profile} />
        )}
      </QueryGate>
    </div>
  );
}

function ProfileForm({ profile }: Readonly<{ profile: Profile }>) {
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
    mutationFn: (value: Partial<Pick<Profile, "enabled" | "replace_link">>) =>
      putSchedulingChanges(value),
    onSuccess: (value) => client.setQueryData(["scheduling-profile"], value),
  });
  return (
    <div className="book-form">
      <Panel title={t("scheduling.myLink")} tone="accent">
        <PanelBody>
          <p className="t-caption">{t("scheduling.linkIntro")}</p>
          <Badge tone={profile.enabled ? "success" : "default"}>
            {t(profile.enabled ? "scheduling.active" : "scheduling.paused")}
          </Badge>
          {profile.enabled &&
            !calendar.connections.isPending &&
            !calendar.ready && (
              <ErrorLine>{t("scheduling.publicCalendarUnavailable")}</ErrorLine>
            )}
          {profile.public_url && (
            <div className="book-link">
              <TextInput
                aria-label={t("scheduling.myLink")}
                readOnly
                value={profile.public_url ?? ""}
              />
            </div>
          )}
          {!profile.public_url && (
            <p className="t-caption">
              {t(
                profile.slug
                  ? "scheduling.publicUrlMissing"
                  : "scheduling.linkNotCreated",
              )}
            </p>
          )}
          <div className="book-actions">
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
          <div className="book-actions">
            {/* The refusal rides the button, so at 390 it stays beside the
                verb it explains rather than under the next one. */}
            <Button
              disabled={save.isPending}
              reason={
                !profile.enabled && !profile.provider
                  ? t("scheduling.calendarSetupNeeded")
                  : undefined
              }
              onClick={() => save.mutate({ enabled: !profile.enabled })}
            >
              {t(profile.enabled ? "scheduling.pause" : "scheduling.resume")}
            </Button>
            <Button
              disabled={!profile.slug || save.isPending}
              onClick={() => setReplace(true)}
            >
              {t("scheduling.replace")}
            </Button>
          </div>
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
          <ErrorLine error={save.error} />
        </PanelBody>
      </Panel>
      <a href="#/settings/meetings">{t("scheduling.openSettings")}</a>
    </div>
  );
}
