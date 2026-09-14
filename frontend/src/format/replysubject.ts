// Re: is normalized; forward markers remain part of the selected subject.
// The shared server corpus includes Unicode whitespace (including NEXT LINE).
export function replySubject(subject: string): string {
  const trimmed = subject.replace(/^[\s\u0085]+|[\s\u0085]+$/g, "");
  return `Re: ${trimmed.replace(/^(?:re:[\s\u0085]*)+/i, "")}`;
}
