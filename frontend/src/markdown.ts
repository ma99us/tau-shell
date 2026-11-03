// A small, read-only Markdown reader. It builds a tree of plain nodes that React renders as
// text, so no HTML from the file is ever inserted into the page, links are shown as text
// (never opened or followed), and images are not loaded.
export type Inline =
  | { kind: "text"; text: string }
  | { kind: "code"; text: string }
  | { kind: "strong"; children: Inline[] }
  | { kind: "em"; children: Inline[] }
  | { kind: "link"; label: string; href: string };

export type Block =
  | { kind: "heading"; level: number; inline: Inline[] }
  | { kind: "paragraph"; inline: Inline[] }
  | { kind: "code"; text: string }
  | { kind: "list"; ordered: boolean; items: Inline[][] }
  | { kind: "quote"; inline: Inline[] }
  | { kind: "rule" };

const inlinePattern = /`([^`]+)`|\*\*([^*]+)\*\*|(?<![\w*])\*([^*\s][^*]*)\*(?![\w*])|!?\[([^\]]*)\]\(([^)\s]*)[^)]*\)/g;

export function parseInline(text: string): Inline[] {
  const out: Inline[] = [];
  let last = 0;
  for (const match of text.matchAll(inlinePattern)) {
    const index = match.index ?? 0;
    if (index > last) out.push({ kind: "text", text: text.slice(last, index) });
    if (match[1] !== undefined) out.push({ kind: "code", text: match[1] });
    else if (match[2] !== undefined) out.push({ kind: "strong", children: parseInline(match[2]) });
    else if (match[3] !== undefined) out.push({ kind: "em", children: parseInline(match[3]) });
    else if (match[0].startsWith("!")) out.push({ kind: "text", text: `[image: ${match[4] || match[5]}]` });
    else out.push({ kind: "link", label: match[4] || match[5], href: match[5] });
    last = index + match[0].length;
  }
  if (last < text.length) out.push({ kind: "text", text: text.slice(last) });
  return out;
}

export function parseMarkdown(source: string): Block[] {
  const lines = source.replace(/\r\n?/g, "\n").split("\n");
  const blocks: Block[] = [];
  let paragraph: string[] = [];
  const flush = () => {
    if (paragraph.length) blocks.push({ kind: "paragraph", inline: parseInline(paragraph.join(" ")) });
    paragraph = [];
  };
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i];
    const fence = /^\s{0,3}(```|~~~)/.exec(line);
    if (fence) {
      flush();
      const body: string[] = [];
      i++;
      while (i < lines.length && !lines[i].trimStart().startsWith(fence[1])) body.push(lines[i++]);
      blocks.push({ kind: "code", text: body.join("\n") });
      continue;
    }
    const heading = /^(#{1,6})\s+(.*?)\s*#*\s*$/.exec(line);
    const listItem = /^\s{0,6}([-*+]|\d+[.)])\s+(.*)$/.exec(line);
    if (heading) { flush(); blocks.push({ kind: "heading", level: heading[1].length, inline: parseInline(heading[2]) }); }
    else if (/^\s{0,3}([-*_])(\s*\1){2,}\s*$/.test(line)) { flush(); blocks.push({ kind: "rule" }); }
    else if (listItem) {
      flush();
      const ordered = /\d/.test(listItem[1]);
      const last = blocks[blocks.length - 1];
      if (last?.kind === "list" && last.ordered === ordered) last.items.push(parseInline(listItem[2]));
      else blocks.push({ kind: "list", ordered, items: [parseInline(listItem[2])] });
    } else if (/^\s{0,3}>/.test(line)) {
      flush();
      blocks.push({ kind: "quote", inline: parseInline(line.replace(/^\s{0,3}>\s?/, "")) });
    } else if (!line.trim()) flush();
    else paragraph.push(line.trim());
  }
  flush();
  return blocks;
}
