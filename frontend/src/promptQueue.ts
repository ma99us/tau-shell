import { hasCueSignal, type HarnessSignal } from "./attention.ts";

// Per-session prompt queue. It lives in the window only (nothing is saved), so a queued
// prompt can never outlive the conversation it was written for. An item is typed only when
// the harness reports a confirmed idle state, never while it works or waits for an answer.
export type QueueItem = { id: number; text: string };
export type SessionQueue = { items: QueueItem[]; sentAt: number; sawBusy: boolean; paused: string };

export const minGapMs = 2000;
// A turn that never showed as busy (very short, or missed between polls) is treated as finished after this.
export const graceMs = 10000;

export type QueueDecision = { action: "send" | "wait" | "pause"; reason: string };

const empty: SessionQueue = { items: [], sentAt: 0, sawBusy: false, paused: "" };

export function queueDecision(queue: SessionQueue, tool: string, running: boolean, signal: HarnessSignal, draft: string, now: number): QueueDecision {
  if (queue.items.length === 0) return { action: "wait", reason: "" };
  if (!running) return { action: "pause", reason: "the session is not running" };
  if (queue.paused) return { action: "wait", reason: queue.paused };
  if (!hasCueSignal(tool)) return { action: "pause", reason: "this harness has no reliable idle signal" };
  if (signal === "input") return { action: "wait", reason: "the harness is waiting for an answer" };
  if (signal === "working") return { action: "wait", reason: "the harness is working" };
  if (signal !== "idle") return { action: "wait", reason: "the harness shows no readable state" };
  if (draft.trim()) return { action: "wait", reason: "there is unsent text in the input line" };
  if (queue.sentAt > 0) {
    const since = now - queue.sentAt;
    if (since < minGapMs) return { action: "wait", reason: "the previous prompt was just sent" };
    if (!queue.sawBusy && since < graceMs) return { action: "wait", reason: "waiting for the previous prompt to start" };
  }
  return { action: "send", reason: "" };
}

let nextId = 1;
const queues = new Map<string, SessionQueue>();
const listeners = new Set<() => void>();
let version = 0;

function commit(id: string, queue: SessionQueue) {
  const before = queues.get(id)?.items ?? [];
  if (queue.items.length === 0 && !queue.paused) queues.delete(id);
  else queues.set(id, queue);
  if (before.length !== queue.items.length || before.some((item, index) => item.text !== queue.items[index].text)) persist?.(id, queue.items.map((item) => item.text));
  version += 1;
  listeners.forEach((listener) => listener());
}

export const subscribeQueues = (listener: () => void) => { listeners.add(listener); return () => { listeners.delete(listener); }; };
export const queuesVersion = () => version;
export const getQueue = (id: string): SessionQueue => queues.get(id) ?? empty;
export const queuedTotal = () => [...queues.values()].reduce((sum, queue) => sum + queue.items.length, 0);

let kick: (() => void) | undefined;
// main.tsx registers a scheduler pass so a prompt added to an idle session goes out at once.
export const setQueueKicker = (fn: (() => void) | undefined) => { kick = fn; };

// Queued text is saved so it survives a restart; only the text is stored, never the send state.
let persist: ((id: string, texts: string[]) => void) | undefined;
export const setQueuePersister = (fn: typeof persist) => { persist = fn; };
let forceSender: ((session: QueueSession, text: string) => string) | undefined;
export const setQueueForceSender = (fn: typeof forceSender) => { forceSender = fn; };

export const restoredNote = "restored after a restart; press Send to start sending";

// Puts saved prompts back, paused: nothing is typed into a harness until the user presses Send or Resume.
export function restoreQueues(saved: Record<string, string[]>) {
  for (const [id, texts] of Object.entries(saved)) {
    if (getQueue(id).items.length > 0 || texts.length === 0) continue;
    queues.set(id, { items: texts.map((text) => ({ id: nextId++, text })), sentAt: 0, sawBusy: false, paused: restoredNote });
  }
  version += 1;
  listeners.forEach((listener) => listener());
}

// Sends the next prompt now, skipping the idle wait, and lets the queue drain normally after it.
// Returns why it was refused, or "".
export function sendNextNow(session: QueueSession, now: number): string {
  const queue = getQueue(session.id);
  if (queue.items.length === 0) return "";
  if (!forceSender) return "sending is not available yet";
  if (!session.runningHere) return "the session is not running";
  const [head, ...rest] = queue.items;
  const refused = forceSender(session, head.text);
  if (refused) {
    commit(session.id, { ...queue, paused: "" });
    return refused;
  }
  commit(session.id, { items: rest, sentAt: now, sawBusy: false, paused: "" });
  return "";
}

export function enqueuePrompt(id: string, text: string) {
  const current = getQueue(id);
  commit(id, { ...current, items: [...current.items, { id: nextId++, text }] });
  kick?.();
}

export function removeQueued(id: string, itemId: number) {
  const current = getQueue(id);
  commit(id, { ...current, items: current.items.filter((item) => item.id !== itemId) });
}

export function updateQueued(id: string, itemId: number, text: string) {
  const current = getQueue(id);
  commit(id, { ...current, items: current.items.map((item) => item.id === itemId ? { ...item, text } : item) });
}

export function clearQueue(id: string) {
  commit(id, empty);
}

export function resumeQueue(id: string) {
  const current = getQueue(id);
  commit(id, { ...current, paused: "", sentAt: 0, sawBusy: false });
}

export type QueueSession = { id: string; tool: string; runningHere: boolean };
export type QueueHooks = {
  signalOf: (session: QueueSession) => HarnessSignal;
  draftOf: (session: QueueSession) => string;
  // Types the text and presses Enter; returns why it was refused, or "".
  send: (session: QueueSession, text: string) => string;
  sent: (session: QueueSession) => void;
};

// One scheduler pass over every non-empty queue. Called about once a second.
export function runQueues(sessions: QueueSession[], hooks: QueueHooks, now: number) {
  for (const session of sessions) {
    const queue = getQueue(session.id);
    if (queue.items.length === 0) continue;
    const signal = session.runningHere ? hooks.signalOf(session) : undefined;
    let current = queue;
    if (current.sentAt > 0 && !current.sawBusy && (signal === "working" || signal === "input")) {
      current = { ...current, sawBusy: true };
      queues.set(session.id, current);
    }
    const decision = queueDecision(current, session.tool, session.runningHere, signal, session.runningHere ? hooks.draftOf(session) : "", now);
    if (decision.action === "pause") {
      commit(session.id, { ...current, paused: decision.reason });
    } else if (decision.action === "send") {
      const [head, ...rest] = current.items;
      // A refusal (screen changed since the check) leaves the item waiting for the next pass.
      if (hooks.send(session, head.text) === "") {
        commit(session.id, { ...current, items: rest, sentAt: now, sawBusy: false });
        hooks.sent(session);
      }
    }
  }
}
