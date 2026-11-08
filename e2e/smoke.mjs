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
import { readFileSync, existsSync, rmSync } from "node:fs";
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

export function versionFailures(report, expectedVersion) {
  const failures = [];
  if (report.version !== expectedVersion) failures.push(`bridge version is ${report.version}, expected ${expectedVersion}`);
  const displayed = expectedVersion === "dev" ? "dev build" : `v${expectedVersion}`;
  if (report.displayedVersion !== displayed) failures.push(`Home version is '${report.displayedVersion}', expected '${displayed}'`);
  return failures;
}

export function lifecycleFailures(prepare, restore) {
  const failures = [];
  if (!prepare) return ["lifecycle prepare report is missing"];
  if (!prepare.home) failures.push("prepare phase did not render Home");
  if (prepare.seedStarted !== true) failures.push("prepare phase did not start the fixture session");
  if (prepare.seedStopped !== true) failures.push("prepare phase did not stop the fixture session cleanly");
  if (prepare.hasResumeId !== true) failures.push("prepare phase did not persist a resumable harness ID");
  if (prepare.confirmationCount !== 0) failures.push(`prepare phase showed ${prepare.confirmationCount} close confirmations, expected none`);
  if (!restore) return [...failures, "lifecycle restore report is missing"];
  if (!restore.home) failures.push("restore phase did not render Home");
  if (restore.autoRestored !== true) failures.push("fixture session did not auto-restore on startup");
  if (restore.hasResumeId !== true) failures.push("restored fixture has no harness session ID");
  if (restore.confirmationCount !== 1) failures.push(`shutdown showed ${restore.confirmationCount} confirmations, expected exactly one`);
  if (restore.shutdownFinished !== true) failures.push("shutdown progress did not finish");
  if (restore.allStopped !== true) failures.push("shutdown progress did not report all fixture sessions stopped");
  for (const report of [prepare, restore]) {
    if (report.banner) failures.push(`${report.phase} phase showed an error banner: ${report.banner}`);
    if (report.missing?.length) failures.push(`${report.phase} phase is missing bridge methods: ${report.missing.join(", ")}`);
    if (report.error) failures.push(`${report.phase} lifecycle error: ${report.error}`);
  }
  return failures;
}

async function runLifecyclePhase({ exe, phase, methods, timeoutMs }) {
  const reportPath = path.join(tmpdir(), `taushell-lifecycle-${process.pid}-${phase}.json`);
  rmSync(reportPath, { force: true });
  const child = spawn(exe, [], {
    stdio: "ignore",
    env: { ...process.env, TAUSHELL_SMOKE_OUT: reportPath, TAUSHELL_SMOKE_METHODS: methods.join(","), TAUSHELL_SMOKE_LIFECYCLE: phase },
  });
  let exitCode = null;
  let spawnError = null;
  child.on("exit", (code) => { exitCode = code; });
  child.on("error", (error) => { spawnError = error; exitCode = -1; });
  const deadline = Date.now() + timeoutMs;
  try {
    while (!existsSync(reportPath) && exitCode === null && Date.now() < deadline) await sleep(250);
    if (!existsSync(reportPath)) throw new Error(spawnError ? `could not start ${phase} app: ${spawnError.message}` : `${phase} app did not write its lifecycle report`);
    while (exitCode === null && Date.now() < deadline) await sleep(250);
    if (exitCode === null) throw new Error(`${phase} app did not exit within ${timeoutMs}ms`);
    if (exitCode !== 0) throw new Error(`${phase} app exited with code ${exitCode}`);
    return JSON.parse(readFileSync(reportPath, "utf8"));
  } finally {
    if (exitCode === null) child.kill();
    rmSync(reportPath, { force: true });
  }
}

async function run({ exe, logPath, expectedVersion, timeoutMs = 120000, lifecycle = false }) {
  const failures = [];
  const fail = (message) => { failures.push(message); console.error(`FAIL: ${message}`); };
  const here = path.dirname(fileURLToPath(import.meta.url));
  const methods = bridgeMethods(readFileSync(path.join(here, "..", "frontend", "src", "bridge.ts"), "utf8"));

  if (lifecycle) {
    let prepare = null;
    let restore = null;
    try {
      prepare = await runLifecyclePhase({ exe, phase: "prepare", methods, timeoutMs });
    } catch (error) { fail(error.message); }
    if (prepare) {
      try {
        restore = await runLifecyclePhase({ exe, phase: "restore", methods, timeoutMs });
      } catch (error) { fail(error.message); }
    }
    for (const message of lifecycleFailures(prepare, restore)) fail(message);
    if (prepare?.seedStarted && prepare?.seedStopped && prepare?.hasResumeId && prepare?.confirmationCount === 0) {
      console.log("ok: fixture session started, saved a resume ID, and stopped without a close prompt");
    }
    if (restore?.autoRestored) console.log("ok: eligible fixture session auto-restored on startup");
    if (restore?.confirmationCount === 1) console.log("ok: closing with a running session asked for confirmation exactly once");
    if (restore?.shutdownFinished && restore?.allStopped) console.log("ok: shutdown stopped the fixture session and completed");
    if (expectedVersion) {
      for (const report of [prepare, restore].filter(Boolean)) {
        for (const message of versionFailures(report, expectedVersion)) fail(`${report.phase}: ${message}`);
      }
    }
  } else {
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
        if (expectedVersion) {
          const mismatches = versionFailures(report, expectedVersion);
          for (const message of mismatches) fail(message);
          if (mismatches.length === 0) console.log(`ok: bridge and Home display version ${expectedVersion}`);
        }
      }
    } finally {
      if (exited === null) child.kill();
      rmSync(reportPath, { force: true });
    }
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
  run({ exe: args.exe, logPath: args.log, expectedVersion: args.version, lifecycle: args.lifecycle === "true" }).then((failures) => {
    if (failures.length > 0) { console.error(`${failures.length} check(s) failed`); process.exit(1); }
    console.log("smoke test passed");
  }).catch((error) => { console.error(`FAIL: ${error.message}`); process.exit(1); });
}
