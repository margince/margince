/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { en } from "../../i18n/en";
import { MessengerAudience } from "./messenger.audience";
import { MessengerStory } from "./messenger.stories.testkit";
import { messengerContext, messengerScope } from "./messenger.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});
it("shows workspace access and compares joined recipients with the assigned team", async () => {
  const user = userEvent.setup();
  render(
    <MessengerStory>
      <MessengerAudience
        scope={messengerScope}
        context={{
          ...messengerContext,
          conversation: {
            ...messengerContext.conversation,
            access: "workspace",
            members: [
              messengerContext.sender,
              {
                id: "U_APP",
                name: "Calendar app",
                bot: true,
                active: true,
                external: false,
              },
            ],
          },
        }}
        changed={false}
        onReview={() => {}}
      />
    </MessengerStory>,
  );
  expect(
    screen.getByText(en["messenger.accessWorkspace"], { exact: false }),
  ).toBeVisible();
  await user.click(
    screen.getByRole("button", { name: en["messenger.viewAudience"] }),
  );
  expect(screen.getByText(en["messenger.joinedOnly"])).toBeVisible();
  await screen.findByText(en["messenger.teamNotJoined"]);
  expect(screen.getByText(en["messenger.teamOutside"])).toBeVisible();
  expect(screen.getByText(en["messenger.appMember"])).toBeVisible();
  expect(
    screen.queryByText(en["messenger.inactiveMember"]),
  ).not.toBeInTheDocument();
});
