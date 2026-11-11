// Which IDE the "Open in IDE" button opens: the session's last pick, else the
// last pick anywhere, else the best match the host offered first.
export type IdeOption = { id: string; name: string; icon: string; matched: boolean };

const globalKey = "taushell.ide.last.v1";
const sessionKey = (sessionId: string) => `taushell.ide.session.${sessionId}.v1`;

type Store = Pick<Storage, "getItem" | "setItem">;

export function chooseIde(options: IdeOption[], sessionPick: string | null, globalPick: string | null): IdeOption | undefined {
  return options.find((option) => option.id === sessionPick) ?? options.find((option) => option.id === globalPick) ?? options[0];
}

export function readIdePick(store: Store, sessionId: string, options: IdeOption[]): IdeOption | undefined {
  let session: string | null = null;
  let global: string | null = null;
  try {
    session = store.getItem(sessionKey(sessionId));
    global = store.getItem(globalKey);
  } catch { /* the pick is a convenience */ }
  return chooseIde(options, session, global);
}

export function writeIdePick(store: Store, sessionId: string, id: string) {
  try {
    store.setItem(sessionKey(sessionId), id);
    store.setItem(globalKey, id);
  } catch { /* the pick is a convenience */ }
}
