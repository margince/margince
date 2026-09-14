import { describe, expect, it, vi } from "vitest";
import { ProblemError } from "./common";
import { saveIndependentEdit } from "./independentedit";

type RecordValue = Record<string, unknown> & {
  id: string;
  version: number;
  masked_fields?: string[];
};
const skew = () => new ProblemError({ code: "version_skew", status: 409 });
const original: RecordValue = {
  id: "record",
  version: 1,
  name: "Before",
  cf_tier: null,
};

// A real compare-and-swap model: successful writes apply ONLY their body and
// only while their version is current. Tests inspect both the saved row and
// every attempted write, including a second writer landing during recovery.
function server(initial: RecordValue) {
  let current = structuredClone(initial);
  const read = vi.fn(async () => structuredClone(current));
  const write = vi.fn(
    async (patch: Record<string, unknown>, version: number) => {
      if (version !== current.version) throw skew();
      current = { ...current, ...patch, version: version + 1 };
      return current;
    },
  );
  return {
    read,
    write,
    current: () => current,
    change: (patch: Record<string, unknown>) => {
      current = { ...current, ...patch, version: current.version + 1 };
    },
  };
}

it("saves a custom field across an unrelated background update and sends only the edit", async () => {
  const api = server({
    ...original,
    version: 2,
    logo_url: "new-logo",
    name: "New name",
  });
  const saved = await saveIndependentEdit({
    opened: { ...original, original },
    patch: { name: "Before", cf_tier: "Growth" },
    ...api,
  });
  expect(saved).toMatchObject({
    name: "New name",
    logo_url: "new-logo",
    cf_tier: "Growth",
    version: 3,
  });
  expect(api.write.mock.calls).toEqual([
    [{ cf_tier: "Growth" }, 1],
    [{ cf_tier: "Growth" }, 2],
  ]);
});

it("refuses overlapping custom-field changes without a second write", async () => {
  const api = server({ ...original, version: 2, cf_tier: "Strategic" });
  await expect(
    saveIndependentEdit({
      opened: { ...original, original },
      patch: { cf_tier: "Growth" },
      ...api,
    }),
  ).rejects.toBeInstanceOf(ProblemError);
  expect(api.write).toHaveBeenCalledTimes(1);
  expect(api.current().cf_tier).toBe("Strategic");
});

it("merges separate address members and preserves fields outside the form", async () => {
  const before = {
    ...original,
    address: { city: "Old city", country: "DE", region: "Old region" },
  };
  const api = server({
    ...before,
    version: 2,
    address: { city: "Old city", country: "FR", region: "New region" },
  });
  await saveIndependentEdit({
    opened: { ...before, original: before },
    patch: { address: { city: "New city", country: "DE" } },
    ...api,
  });
  expect(api.current().address).toEqual({
    city: "New city",
    country: "FR",
    region: "New region",
  });
  expect(before.address.city).toBe("Old city");
});

it("preserves an explicit clear through recovery", async () => {
  const before = { ...original, cf_tier: "Strategic" };
  const api = server({ ...before, version: 2, name: "New name" });
  await saveIndependentEdit({
    opened: { ...before, original: before },
    patch: { cf_tier: null },
    ...api,
  });
  expect(api.current()).toMatchObject({ name: "New name", cf_tier: null });
});

it("uses the stored baseline when a custom field arrived after opening", async () => {
  const api = server({ ...original, version: 2, cf_tier: "Strategic" });
  await saveIndependentEdit({
    opened: { ...original, original, cf_tier: "Strategic" },
    patch: { cf_tier: null },
    ...api,
  });
  expect(api.current().cf_tier).toBeNull();
});

it("treats an email replacement as one field and refuses concurrent list edits", async () => {
  const before = { ...original, emails: [{ email: "old@example.test" }] };
  const api = server({
    ...before,
    version: 2,
    emails: [{ email: "other@example.test" }],
  });
  await expect(
    saveIndependentEdit({
      opened: { ...before, original: before },
      patch: { emails: [] },
      ...api,
    }),
  ).rejects.toBeInstanceOf(ProblemError);
  expect(api.current().emails).toEqual([{ email: "other@example.test" }]);
});

it("does not mistake reordered JSON keys for a field change", async () => {
  const before = {
    ...original,
    emails: [{ email: "a@example.test", position: 0 }],
  };
  const api = server(before);
  await saveIndependentEdit({
    opened: { ...before, original: before },
    patch: {
      emails: [{ position: 0, email: "a@example.test" }],
      cf_tier: "Growth",
    },
    ...api,
  });
  expect(api.write.mock.calls).toEqual([[{ cf_tier: "Growth" }, 1]]);
});

it("protects against another independent write between read and retry", async () => {
  const api = server({ ...original, version: 2 });
  api.read.mockImplementationOnce(async () => {
    const read = { ...api.current() };
    api.change({ name: "Second writer" });
    return read;
  });
  await saveIndependentEdit({
    opened: { ...original, original },
    patch: { cf_tier: "Growth" },
    ...api,
  });
  expect(api.write.mock.calls.map(([, version]) => version)).toEqual([1, 2, 3]);
  expect(api.current()).toMatchObject({
    name: "Second writer",
    cf_tier: "Growth",
  });
});

it("refuses a same-field write that races with the retry", async () => {
  const api = server({ ...original, version: 2 });
  api.read.mockImplementationOnce(async () => {
    const read = { ...api.current() };
    api.change({ cf_tier: "Strategic" });
    return read;
  });
  await expect(
    saveIndependentEdit({
      opened: { ...original, original },
      patch: { cf_tier: "Growth" },
      ...api,
    }),
  ).rejects.toBeInstanceOf(ProblemError);
  expect(api.write).toHaveBeenCalledTimes(2);
  expect(api.current().cf_tier).toBe("Strategic");
});

const money = [["currency", "amount_minor", "expected_arr_minor"]];
it("retains the supplied amount when only currency changes", async () => {
  const before = { ...original, currency: "EUR", amount_minor: 10000 };
  const api = server(before);
  await saveIndependentEdit({
    opened: { ...before, original: before },
    patch: { currency: "USD", amount_minor: 10000 },
    groups: money,
    ...api,
  });
  expect(api.write.mock.calls[0][0]).toEqual({
    currency: "USD",
    amount_minor: 10000,
  });
});

it("refuses repricing against a currency changed by someone else", async () => {
  const before = { ...original, currency: "EUR", amount_minor: 10000 };
  const api = server({ ...before, version: 2, currency: "JPY" });
  await expect(
    saveIndependentEdit({
      opened: { ...before, original: before },
      patch: { amount_minor: 20000 },
      groups: money,
      ...api,
    }),
  ).rejects.toBeInstanceOf(ProblemError);
  expect(api.write).toHaveBeenCalledTimes(1);
});

describe("refusals stay refusals", () => {
  it.each([
    new Error("lost response"),
    new ProblemError({ code: "duplicate_email", status: 409 }),
    new ProblemError({ status: 403 }),
  ])("does not retry %s", async (error) => {
    const api = server(original);
    api.write.mockRejectedValue(error);
    await expect(
      saveIndependentEdit({
        opened: { ...original, original },
        patch: { cf_tier: "Growth" },
        ...api,
      }),
    ).rejects.toBe(error);
    expect(api.read).not.toHaveBeenCalled();
    expect(api.write).toHaveBeenCalledTimes(1);
  });

  it("does not rebase onto fields that have become hidden", async () => {
    const api = server({ ...original, version: 2, masked_fields: ["cf_tier"] });
    await expect(
      saveIndependentEdit({
        opened: { ...original, original },
        patch: { cf_tier: "Growth" },
        ...api,
      }),
    ).rejects.toBeInstanceOf(ProblemError);
    expect(api.write).toHaveBeenCalledTimes(1);
  });

  it("bounds retries when a record keeps changing", async () => {
    const api = server(original);
    api.write.mockRejectedValue(skew());
    await expect(
      saveIndependentEdit({
        opened: { ...original, original },
        patch: { cf_tier: "Growth" },
        ...api,
      }),
    ).rejects.toBeInstanceOf(ProblemError);
    expect(api.write).toHaveBeenCalledTimes(3);
  });
});
