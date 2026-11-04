// End-to-end smoke test for a built TauShell (Node 22+, no dependencies).
//
//   node e2e/smoke.mjs --exe <taushell.exe> --log <taushell.log> [--version 1.2.3]
//
// scripts/smoke-test.ps1 runs this inside an isolated profile. The app is
// started with TAUSHELL_SMOKE_OUT set, which makes it run a self check in its
// own WebView once the UI loads (cmd/taushell/smoke_windows.go) and write a
// report. (WebView2 remote debugging was tried first; it does not open a port
// under Wails, so the self check runs in-process instead.) Checks:
//   1. the Home screen renders and no error banner shows;
//   2. every method declared in frontend/src/bridge.ts is bound on the Go side
//      (window.go.main.App), so a UI/host mismatch fails here, not for a user;
//   3. the host reports the expected version;
//   4. the diagnostics log has no frontend failure or panic.
import { spawn } from "node:child_process";
import { readFileSync, existsSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { tmpdir } from "node:os";
import path from "node:path";

export function bridgeMethods(source) {
  const start = source.indexOf("type WailsApp = {");
  if (start < 0) throw new Error("WailsApp type not found in bridge.ts");
  const end = source.indexOf("\n};", start);
  return [...source.slice(start, end).matchAll(/^\s{2}([A-Za-z]\w*)\(/gm)].map((match) => match[1]);
}

const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));

async function run({ exe, logPath, expectedVersion, timeoutMs = 120000 }) {
  const failures = [];
  const fail = (message) => { failures.push(message); console.error(`FAIL: ${message}`); };
  const here = path.dirname(fileURLToPath(import.meta.url));
  const methods = bridgeMethods(readFileSync(path.join(here, "..", "frontend", "src", "bridge.ts"), "utf8"));
  const reportPath = path.join(tmpdir(), `taushell-smoke-${process.pid}.json`);

  const child = spawn(exe, [], {
    stdio: "ignore",
    env: { ...process.env, TAUSHELL_SMOKE_OUT: reportPath, TAUSHELL_SMOKE_METHODS: methods.join(",") },
  });
  let exited = null;
  child.on("exit", (code) => { exited = code; });

  try {
    const deadline = Date.now() + timeoutMs;
    while (!existsSync(reportPath) && exited === null && Date.now() < deadline) await sleep(500);
    if (!existsSync(reportPath)) {
      fail(exited !== null ? `app exited early with code ${exited}` : "timed out waiting for the UI self check report");
    } else {
      const report = JSON.parse(readFileSync(reportPath, "utf8"));
      if (report.home) console.log("ok: Home screen rendered");
      else fail("Home screen did not render");
      if (report.banner) fail(`error banner is showing: ${report.banner}`);
      if (report.missing.length > 0) fail(`bridge methods missing from the host: ${report.missing.join(", ")}`);
      else console.log(`ok: ${methods.length} bridge methods are bound`);
    }
  } finally {
    if (exited === null) child.kill();
  }

  if (existsSync(logPath)) {
    const lines = readFileSync(logPath, "utf8").split("\n");
    const bad = lines.filter((line) => /"msg":"frontend failure"|panic/i.test(line));
    for (const line of bad) fail(`diagnostics log: ${line.slice(0, 400)}`);
    if (bad.length === 0) console.log("ok: diagnostics log has no frontend failures or panics");
    if (expectedVersion) {
      const started = lines.find((line) => line.includes('"msg":"desktop startup beginning"'));
      const version = started && JSON.parse(started).version;
      if (version === expectedVersion) console.log(`ok: host reports version ${version}`);
      else fail(`host version is ${version}, expected ${expectedVersion}`);
    }
  } else {
    fail(`diagnostics log not found at ${logPath}`);
  }
  return failures;
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const args = Object.fromEntries(process.argv.slice(2).reduce((pairs, value, index, all) => {
    if (value.startsWith("--")) pairs.push([value.slice(2), all[index + 1]]);
    return pairs;
  }, []));
  run({ exe: args.exe, logPath: args.log, expectedVersion: args.version }).then((failures) => {
    if (failures.length > 0) { console.error(`${failures.length} check(s) failed`); process.exit(1); }
    console.log("smoke test passed");
  }).catch((error) => { console.error(`FAIL: ${error.message}`); process.exit(1); });
}
