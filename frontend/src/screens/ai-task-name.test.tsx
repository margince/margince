/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import { LocaleProvider } from "../i18n";
import { TaskName } from "./ai-task-name";

// A task's name on a settings screen opens what the task does, so a reader
// choosing a model for it, or reading what it cost, knows what it is.

afterEach(cleanup);

describe("TaskName", () => {
  it("opens the task's summary from its name", async () => {
    const user = userEvent.setup();
    render(
      <LocaleProvider initial="en">
        <TaskName
          name="Website triage"
          summary="Decides what an email domain's website is."
        />
      </LocaleProvider>,
    );

    await user.click(screen.getByRole("button", { name: /Website triage/ }));

    expect(
      await screen.findByText("Decides what an email domain's website is."),
    ).toBeInTheDocument();
  });

  it("is plain text when the task has nothing to say", () => {
    render(
      <LocaleProvider initial="en">
        <TaskName name="embeddings" summary={undefined} />
      </LocaleProvider>,
    );

    expect(screen.getByText("embeddings")).toBeInTheDocument();
    expect(screen.queryByRole("button")).toBeNull();
  });
});
