// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { useState } from "react";
import { afterEach, expect, it } from "vitest";
import { Button } from "./atoms";
import { Switch } from "./switch";

afterEach(cleanup);

// The `pending` a caller passes answers one render late, so it cannot be the
// whole guard. These presses land in ONE task, which is what "however fast the
// reader presses" means and what a blocked main thread produces: no commit
// happens between them, so nothing the first press started has been drawn yet.

function Writer() {
  const [writes, setWrites] = useState(0);
  const [out, setOut] = useState(false);
  return (
    <>
      <Button
        pending={out}
        onClick={() => {
          setWrites((n) => n + 1);
          setOut(true);
        }}
      >
        Save
      </Button>
      <output>{writes}</output>
    </>
  );
}

it("sends one write however many presses land before the first is drawn", async () => {
  render(<Writer />);
  const save = screen.getByRole("button", { name: "Save" });

  save.click();
  save.click();
  save.click();

  await waitFor(() => expect(screen.getByRole("status").textContent).toBe("1"));
});

function Toggle() {
  const [flicks, setFlicks] = useState(0);
  const [on, setOn] = useState(false);
  return (
    <>
      <Switch
        label="Notify me"
        checked={on}
        pending={flicks > 0}
        onChange={(next) => {
          setFlicks((n) => n + 1);
          setOn(next);
        }}
      />
      <output>{flicks}</output>
    </>
  );
}

it("turns a setting once however many flicks land before the first is drawn", async () => {
  render(<Toggle />);
  const toggle = screen.getByRole("switch", { name: "Notify me" });

  toggle.click();
  toggle.click();

  await waitFor(() => expect(screen.getByRole("status").textContent).toBe("1"));
});

// The release is what keeps the latch from being a brick: a press that starts
// no write at all leaves nothing to report, and a control that refused every
// press after its first would be worse than the double submit it prevents.
function Counter() {
  const [presses, setPresses] = useState(0);
  return (
    <>
      <Button onClick={() => setPresses((n) => n + 1)}>Count</Button>
      <output>{presses}</output>
    </>
  );
}

it("takes the next press once the one before it has settled", async () => {
  render(<Counter />);
  const count = screen.getByRole("button", { name: "Count" });

  count.click();
  await waitFor(() => expect(screen.getByRole("status").textContent).toBe("1"));
  count.click();
  await waitFor(() => expect(screen.getByRole("status").textContent).toBe("2"));
});
