// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The one door to the browser's Web Storage. `storage-spelling.test.ts` fails
// any other file that reaches for `localStorage` or `sessionStorage`.

type StorageArea = "local" | "session";

// `seat`: what whoever is signed in on this browser did or holds, erased by
// `forgetSeat`; `device` (a preference) and `account` (named by one) stay.
type Lifetime = "device" | "seat" | "account";

type Declared = Readonly<{ area: StorageArea; lifetime: Lifetime }> &
  (Readonly<{ name: string }> | Readonly<{ prefix: string }>);

/** Every key the app keeps; a `prefix` declares a family of per-record keys.
 *  Names are what earlier builds wrote: renaming one strands stored values. */
export const STORAGE_KEYS = {
  locale: { name: "margince.locale", area: "local", lifetime: "device" },
  theme: { name: "margince.theme", area: "local", lifetime: "device" },
  sidebarCollapsed: {
    name: "margince.sidebarCollapsed",
    area: "local",
    lifetime: "device",
  },
  pageAsideCollapsed: {
    name: "margince.pageAside.collapsed",
    area: "local",
    lifetime: "device",
  },
  edgeLight: { name: "margince.edgeLight", area: "local", lifetime: "device" },
  // v2: widths saved under content sizing pin columns wrongly under shares.
  tableWidths: {
    prefix: "margince.table.widths.v2.",
    area: "local",
    lifetime: "device",
  },
  faultsSeen: {
    name: "margince.agent.faults-seen",
    area: "local",
    lifetime: "seat",
  },
  segregationNoteDismissed: {
    name: "margince.leads.segregationNoteDismissed",
    area: "local",
    lifetime: "seat",
  },
  importRun: { name: "margince.import.run", area: "local", lifetime: "seat" },
  platformDeclined: {
    prefix: "margince.first-run.platform-declined:",
    area: "local",
    lifetime: "account",
  },
  oauthAttempt: {
    name: "ob.connect.oauthAttempt",
    area: "session",
    lifetime: "seat",
  },
  overnightChoice: {
    name: "ob.connect.overnightChoice",
    area: "session",
    lifetime: "seat",
  },
  buyerRoomSession: {
    name: "margince.room.session",
    area: "session",
    lifetime: "seat",
  },
} as const satisfies Record<string, Declared>;

type Registered = (typeof STORAGE_KEYS)[keyof typeof STORAGE_KEYS];

type KeyFamily = Extract<Registered, { prefix: string }>;

/** A registered key, or one member of a registered family. */
export type StorageKey =
  | Extract<Registered, { name: string }>
  | Readonly<{ family: KeyFamily; member: string }>;

function locate(
  key: StorageKey,
): Readonly<{ name: string; area: StorageArea }> {
  return "family" in key
    ? { name: key.family.prefix + key.member, area: key.family.area }
    : key;
}

// Private windows, disabled site data, a full quota and runtimes without Web
// Storage answer `whenRefused`: an unkeepable value is absent, not an error.
function withStorage<T>(
  area: StorageArea,
  use: (storage: Storage) => T,
  whenRefused: T,
): T {
  try {
    const storage =
      area === "local" ? globalThis.localStorage : globalThis.sessionStorage;
    return storage ? use(storage) : whenRefused;
  } catch {
    return whenRefused;
  }
}

export function readStored(key: StorageKey): string | null {
  const { name, area } = locate(key);
  return withStorage(area, (storage) => storage.getItem(name), null);
}

export function writeStored(key: StorageKey, value: string): void {
  const { name, area } = locate(key);
  withStorage(area, (storage) => storage.setItem(name, value), undefined);
}

export function removeStored(key: StorageKey): void {
  const { name, area } = locate(key);
  withStorage(area, (storage) => storage.removeItem(name), undefined);
}

/** The stored JSON as `parse` shapes it, or null when there is none to read. */
export function readStoredJson<T>(
  key: StorageKey,
  parse: (value: unknown) => T | null,
): T | null {
  const raw = readStored(key);
  if (raw === null) {
    return null;
  }
  let value: unknown;
  try {
    value = JSON.parse(raw);
  } catch {
    // Text some earlier build wrote in another shape reads as never written.
    return null;
  }
  return parse(value);
}

function registeredAs(area: StorageArea, name: string): Registered | undefined {
  return Object.values(STORAGE_KEYS).find(
    (key) =>
      key.area === area &&
      ("name" in key ? key.name === name : name.startsWith(key.prefix)),
  );
}

/** Erase every `seat` key, in both areas and every family, at sign-out. */
export function forgetSeat(): void {
  for (const area of ["local", "session"] as const) {
    withStorage(
      area,
      (storage) => {
        const names = Array.from({ length: storage.length }, (_, index) =>
          storage.key(index),
        );
        for (const name of names) {
          if (name !== null && registeredAs(area, name)?.lifetime === "seat") {
            storage.removeItem(name);
          }
        }
      },
      undefined,
    );
  }
}
