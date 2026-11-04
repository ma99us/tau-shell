const maxBytes = 1 << 20;

// Decodes an OSC 52 payload ("<targets>;<base64>"). Returns the text to copy, or
// undefined for read queries ("?"), empty/invalid data and oversized payloads,
// so a program can write the clipboard but never read it.
export function parseOsc52(payload: string): string | undefined {
  const split = payload.indexOf(";");
  if (split < 0) return undefined;
  const data = payload.slice(split + 1);
  if (!data || data === "?" || data.length > maxBytes * 1.4) return undefined;
  try {
    const binary = atob(data);
    const bytes = Uint8Array.from(binary, (char) => char.charCodeAt(0));
    return new TextDecoder("utf-8", { fatal: true }).decode(bytes);
  } catch {
    return undefined;
  }
}
