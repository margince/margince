/** @vitest-environment happy-dom */
import { en } from "../../i18n/en";
import "@testing-library/jest-dom/vitest";
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it } from "vitest";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { MessagingServiceSwitch } from "./messagingserviceswitch";

afterEach(cleanup);
it("requires an explicit reviewed workspace switch and preserves the service on a refused request", async () => {
  const choices: unknown[] = [];
  installFetchStub({
    "GET /me": meRoute(
      { channel_connection: ["read", "delete"] },
      { seat: "full" },
    ),
    "GET /messenger/service": () =>
      jsonResponse({ providers: ["dispact"], active_provider: "dispact" }),
    "PUT /messenger/service": (body) => {
      choices.push(body);
      return jsonResponse(
        { status: 403, code: "permission_denied", title: "Permission denied" },
        403,
      );
    },
  });
  const user = userEvent.setup();
  render(
    <StoryProviders>
      <MessagingServiceSwitch />
    </StoryProviders>,
  );
  await screen.findByRole("button", {
    name: en["messenger.switchService"],
  });
  await waitFor(() =>
    expect(
      screen.getByRole("combobox", { name: en["messenger.serviceTitle"] }),
    ).toBeEnabled(),
  );
  await user.click(
    screen.getByRole("combobox", { name: en["messenger.serviceTitle"] }),
  );
  await user.click(await screen.findByRole("option", { name: "Slack" }));
  await user.click(
    screen.getByRole("combobox", { name: en["channelProfile.distribution"] }),
  );
  await user.click(
    await screen.findByRole("option", { name: en["channelProfile.internal"] }),
  );
  expect(choices).toEqual([]);
  const change = screen.getByRole("button", {
    name: en["messenger.switchService"],
  });
  expect(change).toBeEnabled();
  await user.click(change);
  const dialog = await screen.findByRole("dialog");
  expect(within(dialog).getByText(/History stays/)).toBeVisible();
  await user.click(
    within(dialog).getByRole("button", { name: en["messenger.useService"] }),
  );
  await waitFor(() =>
    expect(choices).toEqual([
      { provider: "slack", slack_distribution: "internal" },
    ]),
  );
  expect(await within(dialog).findByRole("alert")).toBeVisible();
  expect(screen.getAllByText("Dispact").length).toBeGreaterThan(0);
});

it("requires review before disconnecting the selected workspace service", async () => {
  let disconnected = 0;
  installFetchStub({
    "GET /me": meRoute(
      { channel_connection: ["read", "delete"] },
      { seat: "full" },
    ),
    "GET /messenger/service": () =>
      jsonResponse({ providers: ["dispact"], active_provider: "dispact" }),
    "DELETE /messenger/service/dispact": () => {
      disconnected++;
      return new Response(null, { status: 204 });
    },
  });
  const user = userEvent.setup();
  render(
    <StoryProviders>
      <MessagingServiceSwitch />
    </StoryProviders>,
  );
  const button = await screen.findByRole("button", {
    name: en["messenger.disconnectService"],
  });
  await waitFor(() => expect(button).toBeEnabled());
  await user.click(button);
  expect(disconnected).toBe(0);
  const dialog = await screen.findByRole("dialog");
  expect(within(dialog).getByText(/History/)).toBeVisible();
  await user.click(
    within(dialog).getByRole("button", {
      name: en["messenger.disconnectService"],
    }),
  );
  await waitFor(() => expect(disconnected).toBe(1));
});
