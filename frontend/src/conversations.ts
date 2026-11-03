import type { Conversation } from "./bridge";

// Matches the query against the title, the ID and the model, ignoring case.
export function filterConversations(list: Conversation[], query: string): Conversation[] {
  const needle = query.trim().toLowerCase();
  if (!needle) return list;
  return list.filter((item) => [item.title, item.harnessSessionId, item.model].some((field) => field.toLowerCase().includes(needle)));
}

export function conversationDays(item: Pick<Conversation, "firstDay" | "lastDay">): string {
  if (!item.firstDay) return "no recorded usage";
  return item.firstDay === item.lastDay ? item.firstDay : `${item.firstDay} to ${item.lastDay}`;
}
