/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import {
  bookingConnection,
  bookingHours,
  bookingProfile,
} from "../../testing/book.testkit";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  type RouteMap,
  StoryProviders,
} from "../../testing/story-utils";
import { linkChanges, MeetingLinkDrawer, videoOf } from "./meeting-link-drawer";

type Profile = components["schemas"]["SchedulingProfile"];

// The fixture is a legacy profile: video_call never written, and a typed
// location kept beside the generated video link.
const legacy: Profile = bookingProfile;

function mount(profile: Profile, overrides: RouteMap = {}) {
  const writes: unknown[] = [];
  installFetchStub({
    "GET /me": meRoute({ activity: ["read", "create"] }),
    "GET /connectors": () => jsonResponse({ data: [bookingConnection] }),
    "GET /me/working-hours": () => jsonResponse(bookingHours),
    "GET /scheduling/profile": () => jsonResponse(profile),
    "PUT /scheduling/profile": (body) => {
      writes.push(body);
      return jsonResponse({ ...profile, ...(body as Partial<Profile>) });
    },
    ...overrides,
  });
  const onClose = vi.fn();
  render(
    <StoryProviders>
      <MeetingLinkDrawer
        open
        onClose={onClose}
        profile={profile}
        onChangeHours={vi.fn()}
      />
    </StoryProviders>,
  );
  return { writes, onClose };
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

it("maps each video tile onto the video_call and location pair the profile keeps", () => {
  const draft = {
    title: legacy.title,
    length: "30" as const,
    minutes: "30",
    location: "",
  };
  const app = { ...legacy, video_call: true, location: "" };
  expect(
    linkChanges(app, { ...draft, video: "own", location: " Room 4 " }),
  ).toEqual({ video_call: false, location: "Room 4" });
  expect(linkChanges(app, { ...draft, video: "none" })).toEqual({
    video_call: false,
    location: "",
  });
  const own = { ...legacy, video_call: false, location: "Room 4" };
  expect(linkChanges(own, { ...draft, video: "app" })).toEqual({
    video_call: true,
    location: "",
  });
  expect(videoOf({ video_call: false, location: "  " })).toBe("none");
});

it("clears a legacy address when the video app is saved, and leaves it out of a title-only edit's video pair", () => {
  const draft = {
    title: "Intro call",
    length: "30" as const,
    minutes: "30",
    video: videoOf(legacy),
    location: legacy.location,
  };
  expect(linkChanges(legacy, draft)).toEqual({
    title: "Intro call",
    use_default_title: false,
    video_call: true,
    location: "",
  });
  const app = { ...legacy, video_call: true, location: "" };
  expect(linkChanges(app, { ...draft, location: "" })).toEqual({
    title: "Intro call",
    use_default_title: false,
  });
});

it("opens a legacy profile on the calendar's video app without showing the address it kept", async () => {
  mount(legacy);
  const meet = await screen.findByRole("radio", { name: /Google Meet/ });
  expect(meet).toHaveProperty("checked", true);
  expect(
    screen.getByRole("radio", { name: /Your own link or address/ }),
  ).toHaveProperty("checked", false);
  expect(screen.queryByText(/Saved address/)).toBeNull();
});

it("offers Teams only to a host whose bookings go to a Microsoft calendar", async () => {
  mount(legacy);
  const teams = await screen.findByRole("radio", { name: /Microsoft Teams/ });
  expect(teams).toHaveProperty("disabled", true);
  expect(screen.getByText(/Needs a Microsoft 365 calendar/)).toBeTruthy();
  expect(
    screen.getByRole("link", { name: "Connect Microsoft 365" }),
  ).toBeTruthy();
  expect(screen.getByRole("radio", { name: /Google Meet/ })).toHaveProperty(
    "disabled",
    false,
  );
});

it("saves the host's own address over the latest profile", async () => {
  const user = userEvent.setup();
  const app = { ...legacy, video_call: true, location: "" };
  const { writes, onClose } = mount(app);
  await user.click(
    await screen.findByRole("radio", { name: /Your own link or address/ }),
  );
  const save = screen.getByRole("button", { name: "Save link" });
  expect(save).toHaveProperty("disabled", true);
  await user.type(
    screen.getByRole("textbox", { name: "Link or address" }),
    "https://zoom.example/ada",
  );
  await user.click(save);
  await waitFor(() => expect(writes).toHaveLength(1));
  expect(writes[0]).toMatchObject({
    video_call: false,
    location: "https://zoom.example/ada",
    title: app.title,
    slug: app.slug,
  });
  await waitFor(() => expect(onClose).toHaveBeenCalled());
});

it("takes any other length from 15 minutes to 8 hours and nothing outside it", async () => {
  const user = userEvent.setup();
  const { writes } = mount(legacy);
  await user.click(await screen.findByRole("button", { name: "Other" }));
  const minutes = screen.getByRole("spinbutton", { name: "Length in minutes" });
  const save = screen.getByRole("button", { name: "Save link" });
  for (const outside of ["10", "481"]) {
    await user.clear(minutes);
    await user.type(minutes, outside);
    expect(save).toHaveProperty("disabled", true);
    expect(
      screen.getByText("Enter a length from 15 to 480 minutes."),
    ).toBeTruthy();
  }
  await user.clear(minutes);
  await user.type(minutes, "120");
  expect(save).toHaveProperty("disabled", false);
  await user.click(save);
  await waitFor(() => expect(writes).toHaveLength(1));
  expect(writes[0]).toMatchObject({ duration_minutes: 120 });
  expect(writes[0]).toMatchObject({ location: "", video_call: true });
});

it("replaces the address only after the confirmation", async () => {
  const user = userEvent.setup();
  const { writes } = mount(legacy);
  await user.click(
    await screen.findByRole("button", { name: "Replace address" }),
  );
  expect(writes).toHaveLength(0);
  const confirm = screen.getByRole("dialog", {
    name: "Replace link address?",
  });
  await user.click(
    within(confirm).getByRole("button", { name: "Replace address" }),
  );
  await waitFor(() => expect(writes).toHaveLength(1));
  expect(writes[0]).toMatchObject({ replace_link: true, slug: legacy.slug });
});

it("saves a removal from the existing additional meeting lengths", async () => {
  const user = userEvent.setup();
  const { writes, onClose } = mount({
    ...legacy,
    video_call: true,
    location: "",
    offered_lengths: [15, 30, 45, 60, 90],
  });
  const offered = await screen.findByRole("textbox", {
    name: "Additional meeting lengths",
  });
  await user.clear(offered);
  await user.type(offered, "15, 45, 60, 90");
  await user.click(screen.getByRole("button", { name: "Save link" }));
  await waitFor(() => expect(writes).toHaveLength(1));
  expect(writes[0]).toMatchObject({ offered_lengths: [15, 45, 60, 90] });
  await waitFor(() => expect(onClose).toHaveBeenCalledTimes(1));
});

it("saves the visible additional lengths even when no change event reached React", async () => {
  const user = userEvent.setup();
  const { writes, onClose } = mount({
    ...legacy,
    video_call: true,
    location: "",
    offered_lengths: [15, 30, 45, 60, 90],
  });
  const offered = await screen.findByRole<HTMLInputElement>("textbox", {
    name: "Additional meeting lengths",
  });
  offered.value = "15, 45, 60, 90";
  await user.click(screen.getByRole("button", { name: "Save link" }));
  await waitFor(() => expect(writes).toHaveLength(1));
  expect(writes[0]).toMatchObject({ offered_lengths: [15, 45, 60, 90] });
  expect(onClose).toHaveBeenCalledTimes(1);
});

it.each([
  "10",
  "481",
  "30, 30",
  "no",
  "15.5",
  Array.from({ length: 17 }, (_, i) => String(i + 15)).join(","),
])(
  "refuses visible invalid lengths %s and accepts a corrected edit",
  async (value) => {
    const user = userEvent.setup();
    const { writes, onClose } = mount({
      ...legacy,
      video_call: true,
      location: "",
      offered_lengths: [15, 30],
    });
    const offered = await screen.findByRole<HTMLInputElement>("textbox", {
      name: "Additional meeting lengths",
    });
    offered.value = value;
    await user.click(screen.getByRole("button", { name: "Save link" }));
    expect(writes).toHaveLength(0);
    expect(onClose).not.toHaveBeenCalled();
    expect(offered.getAttribute("aria-invalid")).toBe("true");
    expect(offered.value).toBe(value);
    offered.value = "15, 45";
    await user.click(screen.getByRole("button", { name: "Save link" }));
    await waitFor(() => expect(writes).toHaveLength(1));
    expect(writes[0]).toMatchObject({ offered_lengths: [15, 45] });
  },
);

it("clears every additional length when the visible field is empty", async () => {
  const user = userEvent.setup();
  const { writes } = mount({
    ...legacy,
    video_call: true,
    location: "",
    offered_lengths: [15, 45],
  });
  const offered = await screen.findByRole<HTMLInputElement>("textbox", {
    name: "Additional meeting lengths",
  });
  offered.value = "";
  await user.click(screen.getByRole("button", { name: "Save link" }));
  await waitFor(() => expect(writes).toHaveLength(1));
  expect(writes[0]).toMatchObject({ offered_lengths: [] });
});

it("keeps live validation when additional lengths are typed", async () => {
  const user = userEvent.setup();
  mount({ ...legacy, video_call: true, location: "" });
  const offered = await screen.findByRole("textbox", {
    name: "Additional meeting lengths",
  });
  await user.type(offered, "10");
  expect(offered.getAttribute("aria-invalid")).toBe("true");
  await user.type(offered, "0");
  expect(offered.getAttribute("aria-invalid")).toBeNull();
  await user.type(offered, "0");
  expect(offered.getAttribute("aria-invalid")).toBe("true");
});

it("saves the visible title, custom minutes and location with the lengths", async () => {
  const user = userEvent.setup();
  const { writes } = mount({
    ...legacy,
    video_call: false,
    location: "Room 1",
    duration_minutes: 75,
  });
  const title = await screen.findByRole<HTMLInputElement>("textbox", {
    name: "Meeting name",
  });
  title.value = "Discovery";
  screen.getByRole<HTMLInputElement>("spinbutton", {
    name: "Length in minutes",
  }).value = "120";
  screen.getByRole<HTMLInputElement>("textbox", {
    name: "Link or address",
  }).value = "Room 2";
  await user.click(screen.getByRole("button", { name: "Save link" }));
  await waitFor(() => expect(writes).toHaveLength(1));
  expect(writes[0]).toMatchObject({
    title: "Discovery",
    duration_minutes: 120,
    location: "Room 2",
    video_call: false,
  });
});

it("keeps corrected values and clears the old length error when the server refuses the save", async () => {
  const user = userEvent.setup();
  const request = vi.fn(() =>
    jsonResponse(
      {
        status: 422,
        code: "validation_error",
        title: "Invalid booking profile",
      },
      422,
    ),
  );
  const { onClose } = mount(
    { ...legacy, video_call: true, location: "" },
    { "PUT /scheduling/profile": request },
  );
  const offered = await screen.findByRole<HTMLInputElement>("textbox", {
    name: "Additional meeting lengths",
  });
  offered.value = "10";
  await user.click(screen.getByRole("button", { name: "Save link" }));
  expect(offered.getAttribute("aria-invalid")).toBe("true");
  offered.value = "15, 45";
  await user.click(screen.getByRole("button", { name: "Save link" }));
  await waitFor(() => expect(request).toHaveBeenCalledTimes(1));
  expect(await screen.findByText("Invalid booking profile")).toBeTruthy();
  expect(onClose).not.toHaveBeenCalled();
  expect(offered.value).toBe("15, 45");
  expect(offered.getAttribute("aria-invalid")).toBeNull();
});
