import assert from "node:assert/strict";
import { execFileSync, spawn, spawnSync } from "node:child_process";
import { chmodSync, copyFileSync, existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";
import { checkFrontendVersion, compareVersions, normalizeVersion, setFrontendVersion } from "./frontend-version.mjs";

const script = join(dirname(fileURLToPath(import.meta.url)), "frontend-version.mjs");
const root = dirname(dirname(script));

function fixture(t, version = "1.0.1") {
  const directory = mkdtempSync(join(tmpdir(), "taushell-frontend-version-"));
  t.after(() => rmSync(directory, { recursive: true, force: true }));
  const frontend = join(directory, "frontend");
  mkdirSync(frontend);
  const pkg = { name: "taushell-desktop", version, dependencies: { example: "^1.0.0" } };
  const lock = { name: pkg.name, version, lockfileVersion: 3, packages: { "": { ...pkg }, "node_modules/example": { version: "1.0.0" } } };
  writeFileSync(join(frontend, "package.json"), `${JSON.stringify(pkg, null, 2)}\n`);
  writeFileSync(join(frontend, "package-lock.json"), `${JSON.stringify(lock, null, 2)}\n`);
  return directory;
}

function json(directory, file) {
  return JSON.parse(readFileSync(join(directory, "frontend", file), "utf8"));
}

function git(directory, env, ...args) {
  return execFileSync("git", args, { cwd: directory, env, encoding: "utf8", stdio: ["ignore", "pipe", "pipe"] }).trim();
}

function repository(t) {
  const directory = fixture(t);
  const remote = join(directory, "remote.git");
  const env = { ...process.env, GIT_CONFIG_NOSYSTEM: "1", GIT_CONFIG_GLOBAL: join(directory, "unused.gitconfig"), GIT_TERMINAL_PROMPT: "0" };
  git(directory, env, "init", "--bare", remote);
  git(directory, env, "init");
  git(directory, env, "config", "user.name", "Release Test");
  git(directory, env, "config", "user.email", "release-test@example.invalid");
  git(directory, env, "add", "frontend");
  git(directory, env, "commit", "-m", "Initial fixture", "-m", "Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>");
  git(directory, env, "branch", "-M", "main");
  git(directory, env, "remote", "add", "origin", remote);
  git(directory, env, "push", "-u", "origin", "main");
  return { directory, remote, env };
}

function clone(repo, name) {
  const directory = join(repo.directory, name);
  git(repo.directory, repo.env, "clone", "--branch", "main", repo.remote, directory);
  return directory;
}

function run(repo, checkout, version) {
  return execFileSync(process.execPath, [script, "commit", version, checkout, "main"], {
    cwd: repo.directory, env: repo.env, encoding: "utf8", stdio: ["ignore", "pipe", "pipe"],
  });
}

function runAsync(repo, checkout, version) {
  return new Promise((resolve, reject) => {
    const child = spawn(process.execPath, [script, "commit", version, checkout, "main"], {
      cwd: repo.directory, env: repo.env, stdio: ["ignore", "pipe", "pipe"],
    });
    let output = "";
    child.stdout.on("data", (chunk) => { output += chunk; });
    child.stderr.on("data", (chunk) => { output += chunk; });
    child.on("error", reject);
    child.on("close", (code) => code === 0 ? resolve(output) : reject(new Error(output)));
  });
}

test("release versions are normalized and invalid npm versions are rejected", () => {
  assert.equal(normalizeVersion("v1.2.3-rc.1"), "1.2.3-rc.1");
  assert.equal(normalizeVersion("1.2.3+build.01"), "1.2.3+build.01");
  for (const value of ["dev", "1.2", "v01.2.3", "1.2.3-01", "1.2.3-", "1.2.3-rc+build", "vv1.2.3"]) {
    assert.throws(() => normalizeVersion(value), /Invalid release version/);
  }
});

test("version comparison follows prerelease precedence without numeric overflow", () => {
  const ordered = ["1.0.0-alpha", "1.0.0-alpha.1", "1.0.0-alpha.beta", "1.0.0-beta", "1.0.0-beta.2", "1.0.0-beta.11", "1.0.0-rc.1", "1.0.0"];
  for (let index = 1; index < ordered.length; index++) {
    assert.equal(compareVersions(ordered[index - 1], ordered[index]), -1);
    assert.equal(compareVersions(ordered[index], ordered[index - 1]), 1);
  }
  assert.equal(compareVersions("1.0.0+build.1", "1.0.0+build.2"), 0);
  assert.equal(compareVersions("999999999999999999999.0.0", "1000000000000000000000.0.0"), -1);
});

test("setting a release version updates exactly the three manifest versions", (t) => {
  const directory = fixture(t);
  const beforePackage = json(directory, "package.json");
  const beforeLock = json(directory, "package-lock.json");
  assert.equal(setFrontendVersion(directory, "v2.3.4-rc1"), "2.3.4-rc1");
  beforePackage.version = "2.3.4-rc1";
  beforeLock.version = "2.3.4-rc1";
  beforeLock.packages[""].version = "2.3.4-rc1";
  assert.deepEqual(json(directory, "package.json"), beforePackage);
  assert.deepEqual(json(directory, "package-lock.json"), beforeLock);
  assert.doesNotThrow(() => checkFrontendVersion(directory, "v2.3.4-rc1"));
  assert.throws(() => checkFrontendVersion(directory, "2.3.4"), /Frontend version mismatch/);
  for (const file of ["package.json", "package-lock.json"]) {
    assert.doesNotMatch(readFileSync(join(directory, "frontend", file), "utf8"), /\r/);
  }
});

test("invalid lockfiles fail before either manifest is changed", (t) => {
  const directory = fixture(t);
  const file = join(directory, "frontend", "package.json");
  const before = readFileSync(file, "utf8");
  for (const invalid of ['{"name":"taushell-desktop"}\n', "null\n", "[]\n"]) {
    writeFileSync(join(directory, "frontend", "package-lock.json"), invalid);
    assert.throws(() => setFrontendVersion(directory, "2.0.0"), /root version fields/);
    assert.equal(readFileSync(file, "utf8"), before);
  }
});

test("npm accepts synchronized release metadata without installing dependencies", (t) => {
  const directory = fixture(t);
  const pkg = json(directory, "package.json");
  const lock = json(directory, "package-lock.json");
  delete pkg.dependencies;
  delete lock.packages[""].dependencies;
  delete lock.packages["node_modules/example"];
  writeFileSync(join(directory, "frontend", "package.json"), `${JSON.stringify(pkg, null, 2)}\n`);
  writeFileSync(join(directory, "frontend", "package-lock.json"), `${JSON.stringify(lock, null, 2)}\n`);
  setFrontendVersion(directory, "v1.2.3+build.9");
  const command = process.platform === "win32" ? "pwsh" : "npm";
  const args = process.platform === "win32"
    ? ["-NoProfile", "-Command", "npm ci --offline --ignore-scripts --no-audit --no-fund; exit $LASTEXITCODE"]
    : ["ci", "--offline", "--ignore-scripts", "--no-audit", "--no-fund"];
  const result = spawnSync(command, args, { cwd: join(directory, "frontend"), encoding: "utf8" });
  assert.equal(result.status, 0, result.stderr || result.error?.message);
  checkFrontendVersion(directory, "1.2.3+build.9");
});

test("release workflows stamp before building and commit only after a tag release", () => {
  const release = readFileSync(join(root, ".github", "workflows", "release.yml"), "utf8");
  const ci = readFileSync(join(root, ".github", "workflows", "ci.yml"), "utf8");
  const block = (text, name) => {
    const start = text.indexOf(`      - name: ${name}\n`);
    assert.notEqual(start, -1, `Missing workflow step: ${name}`);
    const end = text.indexOf("\n      - ", start + 1);
    return text.slice(start, end === -1 ? undefined : end);
  };
  assert.ok(release.indexOf("- name: Set frontend release version") < release.indexOf("- name: Frontend build and tests"));
  assert.match(block(release, "Set frontend release version"), /frontend-version\.mjs set \$env:VERSION/);
  assert.ok(release.indexOf("- name: Publish GitHub release") < release.indexOf("- name: Commit frontend release version"));
  assert.match(block(release, "Checkout default branch for frontend version"), /if: github\.event_name == 'push'/);
  assert.match(block(release, "Commit frontend release version"), /if: github\.event_name == 'push'/);
  assert.match(block(release, "Commit frontend release version"), /frontend-version\.mjs commit \$env:VERSION/);
  assert.ok(ci.indexOf("- name: Set frontend version for the release smoke test") < ci.indexOf("- name: Frontend build, token check and tests"));
  assert.match(block(ci, "Set frontend version for the release smoke test"), /frontend-version\.mjs set 0\.0\.0-ci/);
});

test("Windows version resources keep the release core and full product version", () => {
  const check = [
    "import importlib.util",
    "import struct",
    "from pathlib import Path",
    "spec = importlib.util.spec_from_file_location('taushell_gen_icon', Path('build') / 'windows' / 'gen_icon.py')",
    "module = importlib.util.module_from_spec(spec)",
    "spec.loader.exec_module(module)",
    "for version in ('1.2.3', '1.2.3-rc.4', '1.2.3+build.9'):",
    "    data = module.build_version_info(version)",
    "    offset = data.index(struct.pack('<I', 0xFEEF04BD))",
    "    _, _, ms, ls = struct.unpack_from('<4I', data, offset)",
    "    assert (ms >> 16, ms & 0xFFFF, ls >> 16, ls & 0xFFFF) == (1, 2, 3, 0), version",
    "    assert version.encode('utf-16-le') in data, version",
  ].join("\n");
  const result = spawnSync("python", ["-B", "-c", check], { cwd: root, encoding: "utf8" });
  assert.equal(result.status, 0, result.stderr || result.error?.message);
});

test("Android release version codes ignore prerelease and build suffixes", { skip: process.platform !== "win32" }, () => {
  const workflow = readFileSync(join(root, ".github", "workflows", "release.yml"), "utf8");
  const lines = workflow.split("\n");
  const parts = lines.find((line) => line.trim().startsWith("$parts = "))?.trim();
  const code = lines.find((line) => line.trim().startsWith("$code = "))?.trim();
  assert.ok(parts && code, "Android version parsing is missing");
  for (const version of ["1.2.3", "1.2.3-rc.4", "1.2.3+build.9"]) {
    const result = spawnSync("pwsh", ["-NoProfile", "-Command", `$bare = '${version}'; ${parts}; ${code}; Write-Output $code`], { encoding: "utf8" });
    assert.equal(result.status, 0, result.stderr || result.error?.message);
    assert.equal(result.stdout.trim(), "10203");
  }
});

test("local release builds set or check the version before any build work", { skip: process.platform !== "win32" }, (t) => {
  const directory = fixture(t);
  const scripts = join(directory, "scripts");
  mkdirSync(scripts);
  const release = join(scripts, "build-release.ps1");
  copyFileSync(script, join(scripts, "frontend-version.mjs"));
  copyFileSync(join(root, "scripts", "build-release.ps1"), release);
  const output = join(directory, "release-output");
  const mismatch = spawnSync("pwsh", ["-NoProfile", "-File", release, "-Version", "2.0.0", "-SkipFrontend", "-OutputDir", output], { encoding: "utf8" });
  assert.notEqual(mismatch.status, 0);
  assert.match(mismatch.stdout + mismatch.stderr, /Frontend version mismatch/);
  assert.equal(existsSync(output), false);
  const wrapper = join(directory, "mock-release.ps1");
  writeFileSync(wrapper, [
    "param([string]$Script, [string]$OutputDir)",
    "function npm { Write-Output 'MOCKED_NPM'; $global:LASTEXITCODE = 43 }",
    "& $Script -Version v2.0.0 -SkipPackage -OutputDir $OutputDir",
    "",
  ].join("\n"));
  const built = spawnSync("pwsh", ["-NoProfile", "-File", wrapper, release, output], { encoding: "utf8" });
  assert.notEqual(built.status, 0);
  assert.match(built.stdout + built.stderr, /MOCKED_NPM/);
  assert.match(built.stdout + built.stderr, /npm ci failed/);
  assert.equal(json(directory, "package.json").version, "2.0.0");
  assert.equal(json(directory, "package-lock.json").version, "2.0.0");
  assert.equal(json(directory, "package-lock.json").packages[""].version, "2.0.0");
  assert.equal(existsSync(output), false);
});

test("successful tag updates are committed once and older tags cannot downgrade main", (t) => {
  const repo = repository(t);
  const checkout = clone(repo, "release checkout");
  assert.match(run(repo, checkout, "v1.1.0-rc.1"), /Committed frontend version/);
  assert.equal(json(checkout, "package.json").version, "1.1.0-rc.1");
  assert.equal(json(checkout, "package-lock.json").packages[""].version, "1.1.0-rc.1");
  assert.deepEqual(git(checkout, repo.env, "diff", "--name-only", "HEAD^", "HEAD").split(/\r?\n/).sort(),
    ["frontend/package-lock.json", "frontend/package.json"]);
  assert.match(git(checkout, repo.env, "log", "-1", "--format=%B"),
    /Co-authored-by: Copilot <223556219\+Copilot@users\.noreply\.github\.com>/);
  assert.match(run(repo, checkout, "v1.1.0"), /Committed frontend version/);
  const latest = git(checkout, repo.env, "rev-parse", "HEAD");
  assert.match(run(repo, checkout, "v1.1.0"), /already synchronized/);
  assert.match(run(repo, checkout, "v1.1.0-rc.2"), /Skipping .* newer/);
  assert.equal(git(checkout, repo.env, "rev-parse", "HEAD"), latest);
  assert.equal(json(checkout, "package.json").version, "1.1.0");
});

test("a dirty checkout is rejected without changing its files", (t) => {
  const repo = repository(t);
  const checkout = clone(repo, "dirty");
  const file = join(checkout, "frontend", "package.json");
  const changed = `${readFileSync(file, "utf8")}\n`;
  writeFileSync(file, changed);
  assert.throws(() => run(repo, checkout, "v1.2.0"), /requires a clean checkout/);
  assert.equal(readFileSync(file, "utf8"), changed);
});

test("a rejected push is reported and does not change the default branch", (t) => {
  const repo = repository(t);
  const checkout = clone(repo, "protected");
  const hook = join(repo.remote, "hooks", "pre-receive");
  writeFileSync(hook, "#!/bin/sh\nprintf 'test branch protection\\n' >&2\nexit 1\n");
  chmodSync(hook, 0o755);
  assert.throws(() => run(repo, checkout, "v1.2.0"), /Could not push the frontend version commit.*test branch protection/s);
  const final = clone(repo, "unchanged");
  assert.equal(json(final, "package.json").version, "1.0.1");
  assert.equal(json(final, "package-lock.json").version, "1.0.1");
});

test("concurrent tag updates preserve the newest version", async (t) => {
  const repo = repository(t);
  const first = clone(repo, "first");
  const second = clone(repo, "second");
  await Promise.all([runAsync(repo, first, "v1.2.0"), runAsync(repo, second, "v1.3.0")]);
  const final = clone(repo, "final");
  assert.equal(json(final, "package.json").version, "1.3.0");
  assert.equal(json(final, "package-lock.json").version, "1.3.0");
  assert.equal(json(final, "package-lock.json").packages[""].version, "1.3.0");
});
