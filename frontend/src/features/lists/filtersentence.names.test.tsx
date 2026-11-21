// @vitest-environment happy-dom
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import type { VocabularyField } from "../../lib/filterdata";
import { type Node, newGroup, newLeaf } from "../../lib/segmentpredicate";
import { recordNamesRoutes } from "../../testing/recordnames.testkit";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import {
  filterSentence,
  referencedIdsOf,
  useSentenceWords,
} from "./filtersentence";

afterEach(cleanup);

function field(name: string, references: VocabularyField["references"]) {
  return {
    name,
    type: "id",
    operators: ["in", "eq"],
    custom: false,
    references,
  } satisfies VocabularyField;
}

const FIELDS: VocabularyField[] = [
  field("company_id", "company"),
  field("owner_id", "app_user"),
  field("team_id", "team"),
  field("stage_id", "stage"),
  field("pipeline_id", "pipeline"),
  field("project_id", "project"),
  field("product_id", "product"),
];

function page(data: object[]) {
  return () =>
    jsonResponse({ data, page: { next_cursor: null, has_more: false } });
}

function stubCatalogues() {
  installFetchStub({
    "GET /me": meRoute({ company: ["read"], product: ["read"] }),
    ...recordNamesRoutes({ company: { "co-1": "Demo GmbH" } }),
    "GET /users/names": () =>
      jsonResponse({ data: [{ id: "u-1", display_name: "Alice Meyer" }] }),
    "GET /teams": page([{ id: "tm-1", name: "Sales DACH" }]),
    "GET /stages": page([{ id: "s-1", name: "Proposal" }]),
    "GET /pipelines": page([{ id: "pl-1", name: "New business" }]),
    "GET /projects": page([{ id: "pr-1", name: "Fleet rollout" }]),
    "GET /products": page([{ id: "pd-1", name: "Telematics" }]),
  });
}

function Sentence({ tree }: Readonly<{ tree: Node }>) {
  const words = useSentenceWords(referencedIdsOf([tree], FIELDS));
  return <p data-testid="sentence">{filterSentence(tree, FIELDS, words)}</p>;
}

function show(tree: Node) {
  render(
    <StoryProviders>
      <Sentence tree={tree} />
    </StoryProviders>,
  );
}

describe("a sentence through the real name hooks", () => {
  it.each([
    ["company_id", "co-1", "Company is Demo GmbH"],
    ["owner_id", "u-1", "Owner is Alice Meyer"],
    ["team_id", "tm-1", "team id is Sales DACH"],
    ["stage_id", "s-1", "Stage is Proposal"],
    ["pipeline_id", "pl-1", "Pipeline is New business"],
    ["project_id", "pr-1", "Project is Fleet rollout"],
    ["product_id", "pd-1", "Product is Telematics"],
  ])("names a %s clause", async (name, id, sentence) => {
    stubCatalogues();
    show(newGroup("and", [newLeaf(name, "in", [id])]));
    expect(await screen.findByText(sentence)).toBeInTheDocument();
  });

  it("counts a company the reader may not see, and never names it", async () => {
    stubCatalogues();
    show(newGroup("and", [newLeaf("company_id", "in", ["co-hidden"])]));
    expect(
      await screen.findByText("Company is any of 1 company"),
    ).toBeInTheDocument();
    expect(screen.queryByText(/co-hidden|Demo GmbH/)).not.toBeInTheDocument();
  });

  it("counts a product that is not in the catalogue", async () => {
    stubCatalogues();
    show(newGroup("and", [newLeaf("product_id", "in", ["pd-gone"])]));
    expect(await screen.findByTestId("sentence")).toHaveTextContent(
      /1 product/,
    );
    expect(screen.queryByText(/pd-gone/)).not.toBeInTheDocument();
  });
});
