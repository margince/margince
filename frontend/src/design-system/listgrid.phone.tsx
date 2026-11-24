// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// biome-ignore-all lint/a11y/noRedundantRoles: display:block drops implicit table roles

import { Fragment, type ReactNode, useMemo } from "react";
import { useFoldedViewport } from "../lib/viewport";
import type { ListColumn } from "./listcolumn";

type PhonePlace = "title" | "end" | "menu" | "caption";

function phonePlace<Row>(column: ListColumn<Row>): PhonePlace | undefined {
  if (column.fixed) {
    return "title";
  }
  return column.verbs === "menu" ? "menu" : column.phone;
}

/**
 * The columns the grid draws. A list with a `phone` place draws phone rows at
 * the fold, in declared order: the reader's ticks and order are a desktop's.
 */
export function useDrawnColumns<Row>(
  columns: readonly ListColumn<Row>[],
  placed: readonly ListColumn<Row>[],
  hidden: ReadonlySet<string>,
): { phone: boolean; shown: readonly ListColumn<Row>[] } {
  const folded = useFoldedViewport();
  return useMemo(() => {
    const at = (place: PhonePlace) =>
      columns.filter((column) => phonePlace(column) === place);
    if (!folded || at("end").length + at("caption").length === 0) {
      return {
        phone: false,
        shown: placed.filter((column) => !hidden.has(column.key)),
      };
    }
    return {
      phone: true,
      shown: [
        ...at("title"),
        ...at("end").slice(0, 1),
        ...at("menu"),
        ...at("caption"),
      ],
    };
  }, [columns, placed, hidden, folded]);
}

// The phone row's grid places elements; bare text would land in the first free track.
export function asElement(content: ReactNode): ReactNode {
  return typeof content === "string" || typeof content === "number" ? (
    <span>{content}</span>
  ) : (
    content
  );
}

export function phoneContent<Row>(
  column: ListColumn<Row>,
  row: Row,
): ReactNode {
  const said = column.folded?.(row);
  return said === undefined ? column.cell(row) : said;
}

/** A phone row's cells: line one's, then one caption cell; CSS leaves out a fact that renders nothing. */
export function phoneCells<Row>(
  row: Row,
  shown: readonly ListColumn<Row>[],
  identity: (column: ListColumn<Row>) => ReactNode,
): ReactNode[] {
  const cells: ReactNode[] = [];
  const facts: ReactNode[] = [];
  for (const column of shown) {
    const place = phonePlace(column);
    if (place === "title") {
      cells.push(identity(column));
      continue;
    }
    const content = phoneContent(column, row);
    if (place === "caption") {
      facts.push(
        <Fragment key={column.key}>
          {facts.length > 0 && " "}
          <span className="sr-only">{column.header}</span>
          <span className="lt-fact">{content}</span>
        </Fragment>,
      );
    } else {
      cells.push(
        <td
          key={column.key}
          role="cell"
          className={cellClass(column)}
          data-phone={place}
        >
          {content}
        </td>,
      );
    }
  }
  if (facts.length > 0) {
    cells.push(
      <td key="caption" role="cell" data-phone="caption">
        {facts}
      </td>,
    );
  }
  return cells;
}

/**
 * Numeric alignment, and the marker the phone layout uses to promote the
 * identity column to the card's heading. Spelled once so th and td agree.
 */
export function cellClass<Row>(column: ListColumn<Row>): string | undefined {
  const names = [
    column.numeric ? "lt-num" : column.verbs === "menu" ? "lt-rowmenu" : "",
    column.fixed ? "lt-identity" : "",
  ].filter(Boolean);
  return names.length > 0 ? names.join(" ") : undefined;
}
