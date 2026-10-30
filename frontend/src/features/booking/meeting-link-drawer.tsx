// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type ReactNode, useId, useState } from "react";
import type { components } from "../../api/schema";
import { AppLink } from "../../design-system/applink";
import {
  Button,
  Field,
  SegmentedControl,
  TextInput,
} from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { type Choice, ChoiceList } from "../../design-system/choicelist";
import { ConfirmModal } from "../../design-system/confirmmodal";
import {
  DrawerBody,
  DrawerFoot,
  DrawerHead,
} from "../../design-system/drawerbands";
import { ErrorLine } from "../../design-system/errorline";
import { Heading } from "../../design-system/heading";
import { Modal } from "../../design-system/modal";
import { ProviderMark } from "../../design-system/provider-mark";
import {
  type PluralTranslator,
  type Translator,
  useLocale,
  usePlural,
  useT,
} from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import {
  PROVIDER_VIDEO_APP,
  VIDEO_APP_NAME,
  type VideoApp,
} from "../../lib/booking-video";
import { useConnectors } from "../../lib/connectors";
import { usePatchSchedulingProfile } from "../../lib/scheduling-profile-query";
import { hoursSentence } from "./meeting-phrases";
import { useWorkingHours } from "./working-hours";
import "./meeting-cards.css";

type Profile = components["schemas"]["SchedulingProfile"];

// The limits crm.yaml gives SchedulingProfile.title and .location.
const TITLE_MAX = 200;
const LOCATION_MAX = 1000;
const LENGTHS = ["15", "30", "45", "60", "90", "other"] as const;
type Length = (typeof LENGTHS)[number];

/** How a booking is met: the calendar's own video app, the host's own place, or none. */
type Video = "app" | "own" | "none";
type VideoTile = VideoApp | "own" | "none";

// Each app's calendar and words: offered when bookings land in that calendar,
// pointed at Calendar when it is connected but not chosen, else at Connections.
const APP_TILE = {
  google_meet: {
    provider: "gcal",
    vendor: "google",
    help: "scheduling.video.meetHelp",
    other: "scheduling.video.meetOther",
    needs: "scheduling.video.meetNeeds",
    connect: "scheduling.connectGoogle",
  },
  microsoft_teams: {
    provider: "graphcal",
    vendor: "microsoft",
    help: "scheduling.video.teamsHelp",
    other: "scheduling.video.teamsOther",
    needs: "scheduling.video.teamsNeeds",
    connect: "scheduling.connectMicrosoft",
  },
} as const satisfies Record<
  VideoApp,
  Readonly<{
    provider: string;
    vendor: string;
    help: MessageKey;
    other: MessageKey;
    needs: MessageKey;
    connect: MessageKey;
  }>
>;

/**
 * Today's profile has no video field of its own, so the tiles map onto the two
 * it has. The app is video_call on with no location. The host's own place is
 * video_call off with one. None is off with neither. An absent video_call means
 * on.
 */
export function videoOf(
  profile: Pick<Profile, "video_call" | "location">,
): Video {
  if (profile.video_call !== false) return "app";
  return profile.location.trim() === "" ? "none" : "own";
}

type LinkDraft = Readonly<{
  title: string;
  length: Length;
  minutes: string;
  video: Video;
  location: string;
}>;

function draftOf(profile: Profile): LinkDraft {
  const minutes = String(profile.duration_minutes);
  return {
    title: profile.title,
    length: LENGTHS.find((length) => length === minutes) ?? "other",
    minutes,
    video: videoOf(profile),
    location: profile.location,
  };
}

function minutesOf(draft: LinkDraft): number {
  return Number(draft.length === "other" ? draft.minutes : draft.length);
}

/**
 * What a save sends, against the profile the drawer opened on. The video pair
 * is written when the tile changed or the own address was edited. It is also
 * written when a video app is saved over a stored address. Otherwise the booking
 * would send that address beside the generated link.
 */
export function linkChanges(
  original: Profile,
  draft: LinkDraft,
): Partial<Profile> {
  const changes: Partial<Profile> = {};
  const title = draft.title.trim();
  if (title !== original.title) changes.title = title;
  const minutes = minutesOf(draft);
  if (minutes !== original.duration_minutes) changes.duration_minutes = minutes;
  const before = videoOf(original);
  const location = draft.location.trim();
  if (
    draft.video !== before ||
    (draft.video === "own" && location !== original.location) ||
    (draft.video === "app" && original.location.trim() !== "")
  ) {
    changes.video_call = draft.video === "app";
    changes.location = draft.video === "own" ? location : "";
  }
  return changes;
}

/**
 * The one booking link, edited in a drawer: its name, address, length and how
 * the meeting is held. Mounted only while open, so a cancelled draft is gone.
 */
export function MeetingLinkDrawer({
  open,
  onClose,
  profile,
  onChangeHours,
  returnFocusTo,
}: Readonly<{
  open: boolean;
  onClose: () => void;
  profile: Profile;
  /** Closes the drawer and brings the hours card into view. */
  onChangeHours: () => void;
  /** Where focus lands on close instead of the opener, when somewhere else asked. */
  returnFocusTo?: () => HTMLElement | null;
}>) {
  const t = useT();
  const titleId = useId();
  return (
    <Modal
      open={open}
      onClose={onClose}
      labelledBy={titleId}
      intent="drawer"
      returnFocusTo={returnFocusTo}
    >
      <DrawerHead className="meeting-drawer-heading">
        <Heading size="large" as="h2" className="t-h2" id={titleId}>
          {t("scheduling.drawerTitle")}
        </Heading>
        <p className="t-caption">{t("scheduling.drawerIntro")}</p>
      </DrawerHead>
      {open && (
        <LinkForm
          profile={profile}
          onClose={onClose}
          onChangeHours={onChangeHours}
        />
      )}
    </Modal>
  );
}

function LinkForm({
  profile,
  onClose,
  onChangeHours,
}: Readonly<{
  profile: Profile;
  onClose: () => void;
  onChangeHours: () => void;
}>) {
  const t = useT();
  const plural = usePlural();
  const [draft, setDraft] = useState(() => draftOf(profile));
  const [replacing, setReplacing] = useState(false);
  const save = usePatchSchedulingProfile();
  const minutes = minutesOf(draft);
  const lengthError =
    draft.length === "other" &&
    !(Number.isInteger(minutes) && minutes >= 15 && minutes <= 480)
      ? t("scheduling.lengthInvalid")
      : undefined;
  const locationError =
    draft.video === "own" && draft.location.trim() === ""
      ? t("scheduling.locationRequired")
      : undefined;
  const titleMissing = draft.title.trim() === "";
  const update = (next: Partial<LinkDraft>) => setDraft({ ...draft, ...next });
  const submit = () => {
    const changes = linkChanges(profile, draft);
    if (Object.keys(changes).length === 0) {
      onClose();
      return;
    }
    save.mutate(changes, { onSuccess: onClose });
  };
  return (
    <>
      <DrawerBody className="form-stack">
        <Field
          label={t("scheduling.meetingName")}
          hint={t("scheduling.meetingNameHelp")}
        >
          {(control) => (
            <TextInput
              {...control}
              required
              maxLength={TITLE_MAX}
              value={draft.title}
              onChange={(event) => update({ title: event.target.value })}
            />
          )}
        </Field>
        {profile.public_url && (
          <Field
            label={t("scheduling.linkAddress")}
            hint={t("scheduling.linkAddressHelp")}
          >
            {(control) => (
              <div className="meeting-address">
                <TextInput
                  {...control}
                  readOnly
                  value={profile.public_url ?? ""}
                />
                <Button onClick={() => setReplacing(true)}>
                  {t("scheduling.replaceAddress")}
                </Button>
              </div>
            )}
          </Field>
        )}
        <SegmentedControl
          label={t("scheduling.col.length")}
          options={LENGTHS}
          value={draft.length}
          labels={lengthLabels(t, plural)}
          onChange={(length) => update({ length })}
        />
        {draft.length === "other" ? (
          <Field
            label={t("scheduling.lengthMinutes")}
            hint={t("scheduling.lengthOtherHelp")}
            error={lengthError}
          >
            {(control) => (
              <TextInput
                {...control}
                type="number"
                inputMode="numeric"
                min={15}
                max={480}
                value={draft.minutes}
                onChange={(event) => update({ minutes: event.target.value })}
              />
            )}
          </Field>
        ) : (
          <p className="t-caption">{t("scheduling.lengthOtherHelp")}</p>
        )}
        <VideoChoice
          profile={profile}
          draft={draft}
          onChange={update}
          error={locationError}
        />
        <HoursSummary
          notice={profile.notice_minutes}
          onChangeHours={onChangeHours}
        />
        <ErrorLine error={save.error} />
      </DrawerBody>
      <DrawerFoot className="actions">
        <Button onClick={onClose} disabled={save.isPending}>
          {t("create.cancel")}
        </Button>
        <Button
          variant="primary"
          pending={save.isPending}
          disabled={!!lengthError || !!locationError || titleMissing}
          onClick={submit}
        >
          {t("scheduling.saveLink")}
        </Button>
      </DrawerFoot>
      <ReplaceAddressConfirm
        open={replacing}
        onClose={() => setReplacing(false)}
      />
    </>
  );
}

function lengthLabels(
  t: Translator,
  plural: PluralTranslator,
): Record<Length, string> {
  const minutes = (count: string) =>
    plural("scheduling.minutes", Number(count), { count });
  return {
    "15": minutes("15"),
    "30": minutes("30"),
    "45": minutes("45"),
    "60": minutes("60"),
    "90": minutes("90"),
    other: t("scheduling.lengthOther"),
  };
}

function VideoChoice({
  profile,
  draft,
  onChange,
  error,
}: Readonly<{
  profile: Profile;
  draft: LinkDraft;
  onChange: (next: Partial<LinkDraft>) => void;
  error: string | undefined;
}>) {
  const t = useT();
  const connectors = useConnectors();
  const live = (connectors.data?.data ?? []).filter(
    (item) => item.status !== "disconnected",
  );
  const eventApp =
    profile.provider === "" ? null : PROVIDER_VIDEO_APP[profile.provider];
  const account =
    live.find((item) => item.provider === profile.provider)?.account_label ??
    "";
  const appTile = (app: VideoApp): Choice<VideoTile> => {
    const copy = APP_TILE[app];
    let description: ReactNode = t(copy.help, { account });
    if (eventApp !== app) {
      const connected = live.some((item) => item.provider === copy.provider);
      description = connected ? (
        t(copy.other)
      ) : (
        <>
          {t(copy.needs)}{" "}
          <AppLink href="#/settings/connections">{t(copy.connect)}</AppLink>
        </>
      );
    }
    return {
      value: app,
      label: VIDEO_APP_NAME[app],
      description,
      disabled: eventApp !== app,
      mark: <ProviderMark providerKey={copy.vendor} />,
    };
  };
  const value: VideoTile | "" =
    draft.video === "app" ? (eventApp ?? "") : draft.video;
  return (
    <>
      <ChoiceList<VideoTile>
        legend={t("scheduling.videoCall")}
        layout="cards"
        choices={[
          appTile("google_meet"),
          appTile("microsoft_teams"),
          {
            value: "own",
            label: t("scheduling.video.own"),
            description: t("scheduling.video.ownHelp"),
          },
          {
            value: "none",
            label: t("scheduling.video.none"),
            description: t("scheduling.video.noneHelp"),
          },
        ]}
        value={value}
        onChange={(tile) =>
          onChange({ video: tile === "own" || tile === "none" ? tile : "app" })
        }
      />
      {draft.video === "own" && (
        <Field label={t("scheduling.locationLabel")} error={error}>
          {(control) => (
            <TextInput
              {...control}
              required
              placeholder={t("scheduling.locationExample")}
              maxLength={LOCATION_MAX}
              value={draft.location}
              onChange={(event) => onChange({ location: event.target.value })}
            />
          )}
        </Field>
      )}
    </>
  );
}

/** The hours a booking through this link is offered in, in one sentence. */
function HoursSummary({
  notice,
  onChangeHours,
}: Readonly<{ notice: number; onChangeHours: () => void }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const hours = useWorkingHours().data?.working_hours;
  if (!hours) return null;
  return (
    <Callout
      title={hoursSentence(t, plural, locale, hours, notice)}
      actions={
        <Button variant="link" onClick={onChangeHours}>
          {t("scheduling.changeHours")}
        </Button>
      }
    />
  );
}

/** Replacing the link's address, asked first: the old address stops working. */
export function ReplaceAddressConfirm({
  open,
  onClose,
}: Readonly<{ open: boolean; onClose: () => void }>) {
  const t = useT();
  const replace = usePatchSchedulingProfile();
  return (
    <ConfirmModal
      open={open}
      onClose={() => {
        replace.reset();
        onClose();
      }}
      title={t("scheduling.replaceConfirmTitle")}
      confirmLabel={t("scheduling.replaceAddress")}
      pending={replace.isPending}
      onConfirm={() =>
        replace.mutate({ replace_link: true }, { onSuccess: onClose })
      }
    >
      <p>{t("scheduling.replaceHelp")}</p>
      <ErrorLine error={replace.error} />
    </ConfirmModal>
  );
}
