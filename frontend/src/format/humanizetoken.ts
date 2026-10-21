// Unknown wire values keep their wording, with readable word separators.
export function humanizeToken(token: string): string {
  return token.replace(/_/g, " ");
}
