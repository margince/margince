import { useMutation, useQueryClient } from "@tanstack/react-query";
import { CalendarDays, Copy, ExternalLink } from "lucide-react";
import { useMemo, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { navigate } from "../app/router";
import { Badge, Button, TextInput } from "../design-system/atoms";
import { useClipboardCopy } from "../design-system/clipboardcopy";
import { ConfirmModal } from "../design-system/confirmmodal";
import { ErrorLine } from "../design-system/errorline";
import { Heading } from "../design-system/heading";
import { Panel, PanelBody } from "../design-system/panel";
import { useT } from "../i18n";
import { useBookingCalendar } from "./booking-calendar-state";
import { BookingBack } from "./booking-common";
import { QueryGate, throwProblem } from "./common";
import { useSchedulingProfile } from "./scheduling-profile-query";

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
        {(profile) => (
          <ProfileForm
            key={JSON.stringify(profile)}
            profile={profile}
            embedded={embedded}
          />
        )}
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
  const signature = useMemo(() => {
    const anchor = document.createElement("a");
    anchor.href = profile.public_url ?? "";
    anchor.textContent = t("scheduling.signature");
    return anchor.outerHTML;
  }, [profile.public_url, t]);
  const copySignature = useClipboardCopy(
    `${t("scheduling.signature")} — ${profile.public_url ?? ""}`,
    {
      copy: t("scheduling.copySignature"),
      copied: t("scheduling.copied"),
      remedy: t("scheduling.copyFallback"),
    },
    undefined,
    signature,
  );
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
      const { data, error } = await api.PUT("/scheduling/profile", {
        body: { ...latest.data, ...value },
      });
      if (error) throwProblem(error);
      return data;
    },
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
          <div className="book-link">
            <TextInput
              aria-label={t("scheduling.myLink")}
              readOnly
              value={profile.public_url ?? ""}
            />
          </div>
          {!profile.public_url && (
            <p className="t-caption">{t("scheduling.publicUrlMissing")}</p>
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
            <Button disabled={!profile.public_url} onClick={copySignature.copy}>
              {copySignature.label}
            </Button>
            {profile.public_url && (
              <a href={profile.public_url}>{t("scheduling.signature")}</a>
            )}
          </div>
          {copySignature.notice}
          <div className="book-actions">
            <Button
              disabled={save.isPending}
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
      {!embedded && (
        <a href="#/settings/meetings">{t("scheduling.openSettings")}</a>
      )}
    </div>
  );
}
