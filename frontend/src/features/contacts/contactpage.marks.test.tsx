/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import {
  installFetchStub,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { ContactMarks } from "./contactpage.marks";
import { recordedContact } from "./contactpage.marks.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

it("keeps record access without inferring personal work from the last inbound", () => {
  installFetchStub({ "GET /me": meRoute({ contact: ["read"] }) });
  render(
    <StoryProviders>
      <ContactMarks view={recordedContact} />
    </StoryProviders>,
  );
  expect(
    screen.getByRole("button", { name: /Who can see this contact/ }),
  ).toBeVisible();
  expect(screen.queryByText("Your move")).not.toBeInTheDocument();
});
