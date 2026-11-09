export type SetupStatus = {
  claimable: boolean;
  /** The jurisdiction pack codes this build carries, offered by the claim form. */
  jurisdictions: readonly string[];
};

// A failed status read keeps the availability screen instead of offering a claim.
export async function fetchSetupStatus(): Promise<SetupStatus> {
  try {
    const response = await fetch("/setup/status", {
      headers: { Accept: "application/json" },
    });
    if (!response.ok) return { claimable: false, jurisdictions: [] };
    const body: unknown = await response.json();
    // Only an explicit true may offer a form that creates an administrator.
    if (typeof body === "object" && body !== null && "claimable" in body) {
      const offered = "jurisdictions" in body ? body.jurisdictions : [];
      return {
        claimable: body.claimable === true,
        jurisdictions: Array.isArray(offered)
          ? offered.filter((code): code is string => typeof code === "string")
          : [],
      };
    }
    return { claimable: false, jurisdictions: [] };
  } catch {
    return { claimable: false, jurisdictions: [] };
  }
}
