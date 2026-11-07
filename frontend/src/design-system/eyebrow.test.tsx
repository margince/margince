/** @vitest-environment happy-dom */
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { Button } from "./atoms";
import { Eyebrow } from "./eyebrow";

afterEach(cleanup);
it("keeps a group heading separate from its action", () => {
  const clear = vi.fn();
  render(
    <LocaleProvider initial="en">
      <Eyebrow as="h2" action={<Button onClick={clear}>Clear</Button>}>
        Recent
      </Eyebrow>
    </LocaleProvider>,
  );
  const heading = screen.getByRole("heading", { level: 2, name: "Recent" });
  const button = screen.getByRole("button", { name: "Clear" });
  expect(heading.contains(button)).toBe(false);
  fireEvent.click(button);
  expect(clear).toHaveBeenCalledOnce();
});
