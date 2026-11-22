// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type ReactNode, type Ref, useId, useRef, useState } from "react";
import type { components } from "../../api/schema";
import { AppLink } from "../../design-system/applink";
import { Button, Field, TextInput } from "../../design-system/atoms";
import { type Choice, ChoiceList } from "../../design-system/choicelist";
import {
  DrawerBody,
  DrawerFoot,
  DrawerHead,
} from "../../design-system/drawerbands";
import { ErrorLine } from "../../design-system/errorline";
import { Heading } from "../../design-system/heading";
import { Modal } from "../../design-system/modal";
import { ProviderMark } from "../../design-system/provider-mark";
import { Select, type SelectOption } from "../../design-system/select";
import {
  type PluralTranslator,
  type Translator,
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
import { BookingAliasField } from "./booking-alias";
import { HoursSummary, ReplaceAddressConfirm } from "./meeting-link-actions";
import "./meeting-cards.css";

export { ReplaceAddressConfirm } from "./meeting-link-actions";

type Profile = components["schemas"]["SchedulingProfile"];

// The limits crm.yaml gives SchedulingProfile.title and .location.
const TITLE_MAX = 200;
const LOCATION_MAX = 1000;
const MIN_MINUTES = 15;
const MAX_MINUTES = 480;
const MAX_OFFERED = 16;
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

// An absent video_call keeps the generated-video default.
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
  offered?: string;
  video: Video;
  location: string;
}>;

function draftOf(profile: Profile): LinkDraft {
  const minutes = String(profile.duration_minutes);
  return {
    title: profile.title,
    length: LENGTHS.find((length) => length === minutes) ?? "other",
    minutes,
    offered: (profile.offered_lengths ?? []).join(", "),
    video: videoOf(profile),
    location: profile.location,
  };
}

function parseLengths(value: string): number[] {
  return value
    .split(",")
    .map((part) => part.trim())
    .filter(Boolean)
    .map(Number);
}

// The one problem a list of extra lengths has, named so the field says which.
function lengthsProblem(lengths: number[]): MessageKey | undefined {
  if (
    !lengths.every(
      (length) =>
        Number.isInteger(length) &&
        length >= MIN_MINUTES &&
        length <= MAX_MINUTES,
    )
  ) {
    return "scheduling.lengthInvalid";
  }
  if (new Set(lengths).size !== lengths.length) {
    return "scheduling.lengthRepeated";
  }
  return lengths.length > MAX_OFFERED ? "scheduling.lengthsTooMany" : undefined;
}

function minutesOf(draft: LinkDraft): number {
  return Number(draft.length === "other" ? draft.minutes : draft.length);
}

// Saving a video app clears a stored address so guests never get both.
export function linkChanges(
  original: Profile,
  draft: LinkDraft,
): Partial<Profile> {
  const changes: Partial<Profile> = {};
  const title = draft.title.trim();
  if (title !== original.title) {
    changes.title = title;
    changes.use_default_title = false;
  }
  const minutes = minutesOf(draft);
  if (minutes !== original.duration_minutes) changes.duration_minutes = minutes;
  const offered = parseLengths(
    draft.offered ?? (original.offered_lengths ?? []).join(", "),
  );
  if (
    JSON.stringify(offered) !== JSON.stringify(original.offered_lengths ?? [])
  )
    changes.offered_lengths = offered;
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

// Mounting the form only while open discards cancelled drafts.
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
    !(
      Number.isInteger(minutes) &&
      minutes >= MIN_MINUTES &&
      minutes <= MAX_MINUTES
    )
      ? t("scheduling.lengthInvalid")
      : undefined;
  const titleInput = useRef<HTMLInputElement>(null);
  const minutesInput = useRef<HTMLInputElement>(null);
  const offeredInput = useRef<HTMLInputElement>(null);
  const locationInput = useRef<HTMLInputElement>(null);
  const offeredProblem = lengthsProblem(parseLengths(draft.offered ?? ""));
  const locationError =
    draft.video === "own" && draft.location.trim() === ""
      ? t("scheduling.locationRequired")
      : undefined;
  const titleMissing = draft.title.trim() === "";
  const update = (next: Partial<LinkDraft>) => setDraft({ ...draft, ...next });
  const submit = () => {
    const submitted = {
      ...draft,
      title: titleInput.current?.value ?? draft.title,
      minutes: minutesInput.current?.value ?? draft.minutes,
      offered: offeredInput.current?.value ?? draft.offered,
      location: locationInput.current?.value ?? draft.location,
    };
    setDraft(submitted);
    const minutes = minutesOf(submitted);
    if (
      !submitted.title.trim() ||
      !Number.isInteger(minutes) ||
      minutes < MIN_MINUTES ||
      minutes > MAX_MINUTES ||
      lengthsProblem(parseLengths(submitted.offered ?? "")) !== undefined ||
      (submitted.video === "own" && !submitted.location.trim())
    )
      return;
    const changes = linkChanges(profile, submitted);
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
          error={titleMissing ? t("record.fieldRequired") : undefined}
        >
          {(control) => (
            <TextInput
              {...control}
              required
              maxLength={TITLE_MAX}
              ref={titleInput}
              defaultValue={draft.title}
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
        {profile.slug && <BookingAliasField profile={profile} />}
        <Field label={t("scheduling.col.length")}>
          {(control) => (
            <Select
              {...control}
              options={lengthOptions(t, plural)}
              value={draft.length}
              onChange={(next) => {
                const length = LENGTHS.find((preset) => preset === next);
                if (length !== undefined) {
                  update({ length });
                }
              }}
            />
          )}
        </Field>
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
                ref={minutesInput}
                min={MIN_MINUTES}
                max={MAX_MINUTES}
                defaultValue={draft.minutes}
                onChange={(event) => update({ minutes: event.target.value })}
              />
            )}
          </Field>
        ) : (
          <p className="t-caption">{t("scheduling.lengthOtherHelp")}</p>
        )}
        <Field
          label={t("scheduling.offeredLengths")}
          hint={t("scheduling.offeredLengthsHelp")}
          error={
            offeredProblem && t(offeredProblem, { max: String(MAX_OFFERED) })
          }
        >
          {(control) => (
            <TextInput
              {...control}
              ref={offeredInput}
              defaultValue={draft.offered ?? ""}
              onChange={(event) => update({ offered: event.target.value })}
            />
          )}
        </Field>
        <VideoChoice
          profile={profile}
          draft={draft}
          onChange={update}
          error={locationError}
          locationRef={locationInput}
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
        <Button variant="primary" pending={save.isPending} onClick={submit}>
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

function lengthOptions(
  t: Translator,
  plural: PluralTranslator,
): SelectOption[] {
  return LENGTHS.map((length) => ({
    value: length,
    label:
      length === "other"
        ? t("scheduling.lengthOther")
        : plural("scheduling.minutes", Number(length), { count: length }),
  }));
}

function VideoChoice({
  profile,
  draft,
  onChange,
  error,
  locationRef,
}: Readonly<{
  profile: Profile;
  draft: LinkDraft;
  locationRef: Ref<HTMLInputElement>;
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
              ref={locationRef}
              placeholder={t("scheduling.locationExample")}
              maxLength={LOCATION_MAX}
              defaultValue={draft.location}
              onChange={(event) => onChange({ location: event.target.value })}
            />
          )}
        </Field>
      )}
    </>
  );
}
