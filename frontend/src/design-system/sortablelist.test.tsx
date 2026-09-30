/** @vitest-environment happy-dom */
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { moveKey, orderAtPointer, SortableList } from "./sortablelist";

afterEach(cleanup);

type Row = Readonly<{ key: string; name: string }>;

const ROWS: readonly Row[] = [
  { key: "a", name: "Alpha" },
  { key: "b", name: "Bravo" },
  { key: "c", name: "Charlie" },
];

const labels = {
  handle: (item: Row, position: number, count: number) =>
    `Move ${item.name}, ${position} of ${count}`,
  moved: (item: Row, position: number, count: number) =>
    `${item.name} is ${position} of ${count}`,
};

function Harness({
  onReorder,
  busy,
}: Readonly<{ onReorder?: (keys: string[]) => void; busy?: boolean }>) {
  const [rows, setRows] = useState(ROWS);
  return (
    <SortableList
      label="Rows"
      items={rows}
      busy={busy}
      labels={labels}
      onReorder={
        onReorder &&
        ((keys) => {
          onReorder(keys);
          setRows(keys.flatMap((key) => ROWS.filter((r) => r.key === key)));
        })
      }
      renderItem={(item) => item.name}
    />
  );
}

describe("orderAtPointer", () => {
  // Rows stacked 40px apart, midpoints at 20, 60 and 100.
  const midpoint = (key: string) => ({ a: 20, b: 60, c: 100 })[key] ?? 0;

  it("puts the carried row before the first other row whose midpoint is below the pointer", () => {
    expect(orderAtPointer(["a", "b", "c"], "c", 50, midpoint)).toEqual([
      "a",
      "c",
      "b",
    ]);
  });

  it("puts it last once the pointer is below every other row", () => {
    expect(orderAtPointer(["a", "b", "c"], "a", 140, midpoint)).toEqual([
      "b",
      "c",
      "a",
    ]);
  });

  it("leaves the order alone while the pointer stays between its neighbours", () => {
    expect(orderAtPointer(["a", "b", "c"], "b", 70, midpoint)).toEqual([
      "a",
      "b",
      "c",
    ]);
  });
});

describe("moveKey", () => {
  it("takes the key out and puts it back at the index", () => {
    expect(moveKey(["a", "b", "c"], "a", 2)).toEqual(["b", "c", "a"]);
    expect(moveKey(["a", "b", "c"], "c", 0)).toEqual(["c", "a", "b"]);
  });
});

describe("SortableList", () => {
  it("moves a row with the arrow keys, says where it landed, and keeps focus on its grip", async () => {
    const user = userEvent.setup();
    const onReorder = vi.fn();
    render(<Harness onReorder={onReorder} />);
    screen.getByRole("button", { name: "Move Charlie, 3 of 3" }).focus();
    await user.keyboard("{ArrowUp}");
    expect(onReorder).toHaveBeenLastCalledWith(["a", "c", "b"]);
    expect(screen.getByText("Charlie is 2 of 3")).toBeTruthy();
    expect(document.activeElement).toBe(
      screen.getByRole("button", { name: "Move Charlie, 2 of 3" }),
    );
  });

  it("commits nothing past either end", async () => {
    const user = userEvent.setup();
    const onReorder = vi.fn();
    render(<Harness onReorder={onReorder} />);
    screen.getByRole("button", { name: "Move Alpha, 1 of 3" }).focus();
    await user.keyboard("{ArrowUp}");
    expect(onReorder).not.toHaveBeenCalled();
  });

  it("refuses a move while a write is out, without dropping the grip", async () => {
    const user = userEvent.setup();
    const onReorder = vi.fn();
    render(<Harness onReorder={onReorder} busy />);
    const grip = screen.getByRole("button", { name: "Move Bravo, 2 of 3" });
    expect(grip.getAttribute("aria-disabled")).toBe("true");
    grip.focus();
    await user.keyboard("{ArrowDown}");
    expect(onReorder).not.toHaveBeenCalled();
    expect(document.activeElement).toBe(grip);
  });

  // happy-dom lays every row out at zero height, so a pointer at 100 is below
  // them all and carries the row to the end.
  const dragBravoToTheEnd = () => {
    const grip = screen.getByRole("button", { name: "Move Bravo, 2 of 3" });
    fireEvent.pointerDown(grip, { pointerId: 1, button: 0 });
    fireEvent.pointerMove(globalThis.window, { pointerId: 1, clientY: 100 });
  };

  it("commits a pointer drag when it lands", () => {
    const onReorder = vi.fn();
    render(<Harness onReorder={onReorder} />);
    dragBravoToTheEnd();
    fireEvent.pointerUp(globalThis.window, { pointerId: 1 });
    expect(onReorder).toHaveBeenCalledWith(["a", "c", "b"]);
  });

  it("drops a drag that lands after a write has started", () => {
    const onReorder = vi.fn();
    const { rerender } = render(<Harness onReorder={onReorder} />);
    dragBravoToTheEnd();
    rerender(<Harness onReorder={onReorder} busy />);
    fireEvent.pointerUp(globalThis.window, { pointerId: 1 });
    expect(onReorder).not.toHaveBeenCalled();
  });

  it("draws no grip for a reader who may not reorder", () => {
    render(<Harness />);
    expect(screen.getByText("Bravo")).toBeTruthy();
    expect(screen.queryByRole("button")).toBeNull();
  });
});
