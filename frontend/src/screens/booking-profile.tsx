import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { CalendarDays, Copy, ExternalLink } from "lucide-react";
import { useMemo, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { navigate } from "../app/router";
import {
  Badge,
  Button,
  Checkbox,
  Field,
  TextInput,
} from "../design-system/atoms";
import { useClipboardCopy } from "../design-system/clipboardcopy";
import { ConfirmModal } from "../design-system/confirmmodal";
import { ErrorLine } from "../design-system/errorline";
import { Heading } from "../design-system/heading";
import { Panel, PanelBody } from "../design-system/panel";
import { Select } from "../design-system/select";
import { useT } from "../i18n";
import { useBookingCalendar } from "./booking-calendar-state";
import { BookingCalendars } from "./booking-calendars";
import { BookingBack } from "./booking-common";
import { QueryGate, throwProblem } from "./common";

type Profile = components["schemas"]["SchedulingProfile"];

export function BookingProfileScreen() {
  const t = useT();
  const query = useQuery({
    queryKey: ["scheduling-profile"],
    queryFn: async () => {
      const { data, error } = await api.GET("/scheduling/profile");
      if (error) throwProblem(error);
      return data;
    },
  });
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
  const [form, setForm] = useState(profile);
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
    mutationFn: async (value: Profile) => {
      const { data, error } = await api.PUT("/scheduling/profile", {
        body: value,
      });
      if (error) throwProblem(error);
      return data;
    },
    onSuccess: (value) => client.setQueryData(["scheduling-profile"], value),
  });
  return (
    <div className="book-profile-grid">
      <Panel title={t("scheduling.myLink")} tone="accent">
        <PanelBody>
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
            <Button
              disabled={!profile.slug}
              onClick={() => navigate({ screen: "book", id: profile.slug })}
            >
              <ExternalLink aria-hidden />
              {t("scheduling.preview")}
            </Button>
          </div>
          {copy.notice}
          <Button disabled={!profile.public_url} onClick={copySignature.copy}>
            {copySignature.label}
          </Button>
          {copySignature.notice}
          {profile.public_url && (
            <p>
              <a href={profile.public_url}>{t("scheduling.signature")}</a>
            </p>
          )}
          <div className="book-actions">
            <Button
              disabled={save.isPending}
              onClick={() =>
                save.mutate({ ...profile, enabled: !profile.enabled })
              }
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
                { ...profile, replace_link: true },
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
      <Panel title={t("scheduling.settings")}>
        <PanelBody>
          <form
            className="book-form"
            onSubmit={(e) => {
              e.preventDefault();
              save.mutate(form);
            }}
          >
            <Field label={t("scheduling.provider")}>
              {(control) => (
                <Select
                  {...control}
                  value={form.provider}
                  options={[
                    { value: "", label: t("scheduling.chooseProvider") },
                    { value: "gcal", label: "Google Calendar" },
                    { value: "graphcal", label: "Microsoft Outlook" },
                  ]}
                  onChange={(value) => {
                    if (
                      value === "" ||
                      value === "gcal" ||
                      value === "graphcal"
                    )
                      setForm({
                        ...form,
                        provider: value,
                        calendar_id: "primary",
                        blocking_calendars: [],
                      });
                  }}
                />
              )}
            </Field>
            <BookingCalendars
              provider={form.provider}
              calendar={form.calendar_id}
              blocking={form.blocking_calendars ?? []}
              onCalendar={(calendar_id) => setForm({ ...form, calendar_id })}
              onBlocking={(blocking_calendars) =>
                setForm({ ...form, blocking_calendars })
              }
            />
            <Field label={t("scheduling.hostName")}>
              {(control) => (
                <TextInput
                  {...control}
                  required
                  value={form.host_name ?? ""}
                  onChange={(e) =>
                    setForm({ ...form, host_name: e.target.value })
                  }
                />
              )}
            </Field>
            <Field label={t("scheduling.companyName")}>
              {(control) => (
                <TextInput
                  {...control}
                  value={form.company_name ?? ""}
                  onChange={(e) =>
                    setForm({ ...form, company_name: e.target.value })
                  }
                />
              )}
            </Field>
            <Field label={t("scheduling.logo")}>
              {(control) => (
                <TextInput
                  {...control}
                  type="url"
                  value={form.logo_url ?? ""}
                  onChange={(e) =>
                    setForm({ ...form, logo_url: e.target.value })
                  }
                />
              )}
            </Field>
            <Field label={t("scheduling.subject")}>
              {(control) => (
                <TextInput
                  {...control}
                  required
                  value={form.title}
                  onChange={(e) => setForm({ ...form, title: e.target.value })}
                />
              )}
            </Field>
            <Field label={t("scheduling.location")}>
              {(control) => (
                <TextInput
                  {...control}
                  value={form.location}
                  onChange={(e) =>
                    setForm({ ...form, location: e.target.value })
                  }
                />
              )}
            </Field>
            <Checkbox
              label={t("scheduling.emailReminder")}
              checked={form.email_reminder ?? false}
              onChange={(event) =>
                setForm({ ...form, email_reminder: event.target.checked })
              }
            />
            <p className="t-caption">{t("scheduling.reminderHelp")}</p>
            <div className="book-policy-fields">
              <Field label={t("scheduling.duration")}>
                {(control) => (
                  <TextInput
                    {...control}
                    type="number"
                    min={15}
                    max={480}
                    value={form.duration_minutes}
                    onChange={(e) =>
                      setForm({
                        ...form,
                        duration_minutes: Number(e.target.value),
                      })
                    }
                  />
                )}
              </Field>
              <Field label={t("scheduling.notice")}>
                {(control) => (
                  <TextInput
                    {...control}
                    type="number"
                    min={0}
                    max={10080}
                    value={form.notice_minutes}
                    onChange={(e) =>
                      setForm({
                        ...form,
                        notice_minutes: Number(e.target.value),
                      })
                    }
                  />
                )}
              </Field>
              <Field label={t("scheduling.buffer")}>
                {(control) => (
                  <TextInput
                    {...control}
                    type="number"
                    min={0}
                    max={120}
                    value={form.buffer_minutes}
                    onChange={(e) =>
                      setForm({
                        ...form,
                        buffer_minutes: Number(e.target.value),
                      })
                    }
                  />
                )}
              </Field>
              <Field label={t("scheduling.horizon")}>
                {(control) => (
                  <TextInput
                    {...control}
                    type="number"
                    min={1}
                    max={90}
                    value={form.horizon_days}
                    onChange={(e) =>
                      setForm({ ...form, horizon_days: Number(e.target.value) })
                    }
                  />
                )}
              </Field>
            </div>
            <div className="book-actions">
              <Button type="submit" variant="primary" disabled={save.isPending}>
                {t("scheduling.save")}
              </Button>
              <Button
                onClick={() => navigate({ screen: "settings", id: "account" })}
              >
                {t("scheduling.hours")}
              </Button>
            </div>
          </form>
          <ErrorLine error={save.error} />
        </PanelBody>
      </Panel>
    </div>
  );
}
