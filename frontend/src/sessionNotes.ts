// Limits and link rules mirror internal/session/config.go (Validate*, LinkURLs).
// The host only opens a URL that its own extraction finds in the session's
// notes or tags, so keep the two rules identical.
export const maxNotesLength = 500;
export const maxTags = 8;
export const maxTagLength = 120;
export const maxLinkLength = 500;

export type Segment = { text: string; url?: string };

const linkPattern = /https?:\/\/[^\s<>"'`]+/g;
const trailing = /[.,;:!?)\]}]+$/;

function validUrl(text: string): boolean {
  try {
    return new URL(text).host !== "";
  } catch {
    return false;
  }
}

// linkSegments splits text into plain runs and http(s) links.
export function linkSegments(text: string): Segment[] {
  const segments: Segment[] = [];
  let last = 0;
  for (const match of text.matchAll(linkPattern)) {
    const url = match[0].replace(trailing, "");
    if (!validUrl(url)) continue;
    const start = match.index ?? 0;
    if (start > last) segments.push({ text: text.slice(last, start) });
    segments.push({ text: url, url });
    last = start + url.length;
  }
  if (last < text.length) segments.push({ text: text.slice(last) });
  return segments;
}

// parseTags reads tags separated by spaces or commas, dropping empty entries and
// case-insensitive duplicates.
export function parseTags(input: string): string[] {
  const seen = new Set<string>();
  const tags: string[] = [];
  for (const raw of input.split(/[\s,]+/)) {
    const key = raw.toLowerCase();
    if (!raw || seen.has(key)) continue;
    seen.add(key);
    tags.push(raw);
  }
  return tags;
}

// linkProblem describes why a link cannot be saved, or "" when it can be (empty clears it).
export function linkProblem(link: string): string {
  if (!link) return "";
  if ([...link].length > maxLinkLength) return `The link is limited to ${maxLinkLength} characters.`;
  if (/\s/.test(link)) return "The link must not contain spaces.";
  return validUrl(link) && /^https?:\/\//i.test(link) ? "" : "The link must be an http or https address.";
}

// notesProblem describes why a notes and tags edit cannot be saved, or "" when it can.
export function notesProblem(notes: string, tags: string[]): string {
  if ([...notes].length > maxNotesLength) return `Notes are limited to ${maxNotesLength} characters.`;
  if (tags.length > maxTags) return `At most ${maxTags} tags are allowed.`;
  if (tags.some((tag) => [...tag].length > maxTagLength)) return `Each tag is limited to ${maxTagLength} characters.`;
  return "";
}
