import type { components } from "../api/schema";

type SessionUser = components["schemas"]["MeResponse"]["user"];

// A missing display name leaves the stored email as the account label.
export function identityOf(user: SessionUser | undefined) {
  const name = user?.display_name ?? "";
  const email = user?.email ?? "";
  const label = name || email;
  return {
    id: user?.id ?? "",
    name,
    email,
    label,
    photo: user?.avatar_url,
    // Both lines in one sentence, for the state that has no room to print them.
    spoken: name && email ? `${name} — ${email}` : label,
  };
}

export type Identity = ReturnType<typeof identityOf>;
