/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import { Heading } from "../../design-system/heading";
import { LocaleProvider } from "../../i18n";
import { TaskName } from "./ai-task-name";

// Every screen names an AI task by its name. What the task does, and the key
// a log or a config file spells it with, sit behind that name.

afterEach(cleanup);

function renderName(node: React.ReactNode) {
  return render(<LocaleProvider initial="en">{node}</LocaleProvider>);
}

describe("TaskName", () => {
  it("shows the name and keeps the task's key out of the line", () => {
    renderName(
      <TaskName
        name="Assessing a sales report"
        task="report_interpret"
        mode="While the user waits"
        summary="Reads a sales report and says what it shows."
      />,
    );

    const trigger = screen.getByRole("button", {
      name: "Assessing a sales report: what it does",
    });
    expect(trigger).toHaveClass("evmark-trigger");
    expect(trigger).toHaveTextContent("Assessing a sales report");
    expect(screen.queryByText(/report_interpret/)).toBeNull();
  });

  it("opens what the task does and its key, with how it runs", async () => {
    const user = userEvent.setup();
    renderName(
      <TaskName
        name="Assessing a sales report"
        task="report_interpret"
        mode="While the user waits"
        summary="Reads a sales report and says what it shows."
        details={<p>More about the task</p>}
      />,
    );

    await user.click(
      screen.getByRole("button", { name: /Assessing a sales report/ }),
    );

    expect(
      await screen.findByText("Reads a sales report and says what it shows."),
    ).toBeInTheDocument();
    const key = screen.getByText("report_interpret");
    expect(key.tagName).toBe("CODE");
    expect(key.parentElement).toHaveTextContent(
      "report_interpret · While the user waits",
    );
    expect(screen.getByText("More about the task")).toBeInTheDocument();
  });

  it("opens the key alone when the task has no summary", async () => {
    const user = userEvent.setup();
    renderName(
      <TaskName name="Unknown task" task="nightly_batch" summary={undefined} />,
    );

    await user.click(screen.getByRole("button", { name: /Unknown task/ }));

    expect(await screen.findByText("nightly_batch")).toBeInTheDocument();
  });

  it("names a dialog titled by it with the task name alone", () => {
    renderName(
      <div role="dialog" aria-labelledby="sheet-title">
        <Heading size="large" id="sheet-title">
          <TaskName
            name="Assessing a sales report"
            task="report_interpret"
            summary="Reads a sales report and says what it shows."
            inTitle
          />
        </Heading>
      </div>,
    );

    expect(
      screen.getByRole("dialog", { name: "Assessing a sales report" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Assessing a sales report" }),
    ).toHaveAttribute("aria-expanded", "false");
  });

  it("is plain text when the name is the key and there is nothing to say", () => {
    renderName(
      <TaskName name="embeddings" task="embeddings" summary={undefined} />,
    );

    expect(screen.getByText("embeddings")).toBeInTheDocument();
    expect(screen.queryByRole("button")).toBeNull();
  });
});
