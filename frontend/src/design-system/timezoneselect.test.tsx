/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { StoryProviders } from "../testing/story-utils";
import { TimezoneSelect } from "./timezoneselect";

afterEach(cleanup);

it("finds a zone by its former city name and by its IANA id", async () => {
  const user = userEvent.setup();
  const onChange = vi.fn();
  render(
    <StoryProviders>
      <TimezoneSelect
        aria-label="Time zone"
        value="Europe/Berlin"
        onChange={onChange}
      />
    </StoryProviders>,
  );
  const picker = screen.getByRole("combobox", { name: "Time zone" });
  await user.click(picker);
  await user.keyboard("Saigon{Enter}");
  expect(onChange).toHaveBeenLastCalledWith("Asia/Ho_Chi_Minh");

  await user.click(picker);
  await user.keyboard("america/new_york{Enter}");
  expect(onChange).toHaveBeenLastCalledWith("America/New_York");
});

it("shows a saved former zone name as the one current entry", async () => {
  const user = userEvent.setup();
  render(
    <StoryProviders>
      <TimezoneSelect
        aria-label="Time zone"
        value="Asia/Saigon"
        onChange={vi.fn()}
      />
    </StoryProviders>,
  );
  const picker = screen.getByRole("combobox", { name: "Time zone" });
  expect(picker).toHaveTextContent("Ho Chi Minh (Asia)");
  await user.click(picker);
  await user.keyboard("Ho Chi Minh");
  expect(screen.getAllByRole("option")).toHaveLength(1);
});

it("finds the zone by either id, whichever of the two was saved", async () => {
  const user = userEvent.setup();
  const onChange = vi.fn();
  const { rerender } = render(
    <StoryProviders>
      <TimezoneSelect aria-label="Time zone" value="UTC" onChange={onChange} />
    </StoryProviders>,
  );
  const picker = screen.getByRole("combobox", { name: "Time zone" });
  await user.click(picker);
  await user.keyboard("asia/saigon{Enter}");
  expect(onChange).toHaveBeenLastCalledWith("Asia/Ho_Chi_Minh");

  rerender(
    <StoryProviders>
      <TimezoneSelect
        aria-label="Time zone"
        value="Asia/Saigon"
        onChange={onChange}
      />
    </StoryProviders>,
  );
  await user.click(picker);
  await user.keyboard("asia/ho_chi_minh");
  expect(screen.getAllByRole("option")).toHaveLength(1);
  await user.keyboard("{Enter}");
  expect(onChange).toHaveBeenLastCalledWith("Asia/Saigon");
});
