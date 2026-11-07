import { spawnSync } from "node:child_process";
import { readFileSync, writeFileSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const root = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const versionPattern = /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)|\+([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?$/;
const packagePaths = [join("frontend", "package.json"), join("frontend", "package-lock.json")];

function parseVersion(value) {
  if (typeof value !== "string") throw new Error("Version must be a string");
  const version = value.startsWith("v") ? value.slice(1) : value;
  const match = versionPattern.exec(version);
  if (!match) throw new Error(`Invalid release version '${value}'; expected x.y.z with an optional prerelease or build suffix`);
  const prerelease = match[4]?.split(".") ?? [];
  if (prerelease.some((part) => /^\d+$/.test(part) && part.length > 1 && part.startsWith("0"))) {
    throw new Error(`Invalid release version '${value}'; numeric prerelease identifiers cannot have leading zeroes`);
  }
  return { version, numbers: match.slice(1, 4), prerelease };
}

export function normalizeVersion(value) {
  return parseVersion(value).version;
}

function compareNumbers(left, right) {
  if (left.length !== right.length) return left.length < right.length ? -1 : 1;
  return left === right ? 0 : left < right ? -1 : 1;
}

export function compareVersions(left, right) {
  const a = parseVersion(left);
  const b = parseVersion(right);
  for (let index = 0; index < a.numbers.length; index++) {
    const comparison = compareNumbers(a.numbers[index], b.numbers[index]);
    if (comparison) return comparison;
  }
  if (!a.prerelease.length || !b.prerelease.length) {
    return a.prerelease.length === b.prerelease.length ? 0 : a.prerelease.length ? -1 : 1;
  }
  for (let index = 0; index < Math.min(a.prerelease.length, b.prerelease.length); index++) {
    const x = a.prerelease[index];
    const y = b.prerelease[index];
    const xNumeric = /^\d+$/.test(x);
    const yNumeric = /^\d+$/.test(y);
    if (xNumeric && yNumeric) {
      const comparison = compareNumbers(x, y);
      if (comparison) return comparison;
    } else if (xNumeric !== yNumeric) {
      return xNumeric ? -1 : 1;
    } else if (x !== y) {
      return x < y ? -1 : 1;
    }
  }
  return a.prerelease.length === b.prerelease.length ? 0 : a.prerelease.length < b.prerelease.length ? -1 : 1;
}

function readJSON(path) {
  const text = readFileSync(path, "utf8");
  try {
    return { text, value: JSON.parse(text) };
  } catch (error) {
    throw new Error(`Invalid JSON in ${path}`, { cause: error });
  }
}

function readManifests(directory) {
  const paths = packagePaths.map((path) => join(directory, path));
  const [pkg, lock] = paths.map(readJSON);
  const lockRoot = lock.value && typeof lock.value === "object" ? lock.value.packages?.[""] : undefined;
  if (!pkg.value || typeof pkg.value !== "object" || !lock.value || typeof lock.value !== "object" ||
      typeof pkg.value.version !== "string" || typeof lock.value.version !== "string" ||
      typeof lockRoot?.version !== "string" || typeof pkg.value.name !== "string" || !pkg.value.name ||
      pkg.value.name !== lock.value.name || pkg.value.name !== lockRoot.name) {
    throw new Error("Frontend package and lockfile must contain matching package names and root version fields");
  }
  return { paths, pkg, lock, lockRoot };
}

function versions(manifests) {
  return [manifests.pkg.value.version, manifests.lock.value.version, manifests.lockRoot.version];
}

export function setFrontendVersion(directory, requestedVersion) {
  const version = normalizeVersion(requestedVersion);
  const manifests = readManifests(directory);
  manifests.pkg.value.version = version;
  manifests.lock.value.version = version;
  manifests.lockRoot.version = version;
  for (const [index, document] of [manifests.pkg, manifests.lock].entries()) {
    const text = `${JSON.stringify(document.value, null, 2)}\n`;
    if (text !== document.text) writeFileSync(manifests.paths[index], text, "utf8");
  }
  return version;
}

export function checkFrontendVersion(directory, requestedVersion) {
  const version = normalizeVersion(requestedVersion);
  const actual = versions(readManifests(directory));
  if (actual.some((value) => value !== version)) {
    throw new Error(`Frontend version mismatch: expected ${version}; package, lockfile, and lockfile root are ${actual.join(", ")}`);
  }
}

function git(directory, args, allowFailure = false) {
  const result = spawnSync("git", args, { cwd: directory, encoding: "utf8", windowsHide: true });
  if (result.error) throw new Error(`Could not run git ${args[0]}: ${result.error.message}`, { cause: result.error });
  if (result.status !== 0 && !allowFailure) {
    throw new Error(`git ${args[0]} failed (exit ${result.status}): ${safeGitError(result.stderr)}`);
  }
  return result;
}

function safeGitError(message) {
  return message.trim().replace(/(https?:\/\/)[^/\s@]+@/g, "$1[redacted]@");
}

export function commitFrontendVersion(directory, requestedVersion, branch) {
  const version = normalizeVersion(requestedVersion);
  const checkout = resolve(directory);
  if (!branch) throw new Error("Default branch is required");
  git(checkout, ["check-ref-format", "--branch", branch]);
  if (git(checkout, ["status", "--porcelain", "--untracked-files=no"]).stdout.trim()) {
    throw new Error("Version synchronization requires a clean checkout");
  }
  git(checkout, ["config", "user.name", "github-actions[bot]"]);
  git(checkout, ["config", "user.email", "41898282+github-actions[bot]@users.noreply.github.com"]);

  for (let attempt = 1; attempt <= 3; attempt++) {
    git(checkout, ["fetch", "--no-tags", "origin", `refs/heads/${branch}`]);
    const base = git(checkout, ["rev-parse", "FETCH_HEAD"]).stdout.trim();
    git(checkout, ["switch", "--detach", "FETCH_HEAD"]);
    const current = versions(readManifests(checkout));
    if (current.some((value) => value !== current[0])) {
      throw new Error(`Default-branch frontend versions disagree: ${current.join(", ")}`);
    }
    if (compareVersions(version, current[0]) < 0) {
      console.log(`Skipping ${version}: default branch already has newer frontend version ${current[0]}`);
      return;
    }
    setFrontendVersion(checkout, version);
    git(checkout, ["add", "--", ...packagePaths]);
    const diff = git(checkout, ["diff", "--cached", "--quiet", "--", ...packagePaths], true);
    if (diff.status === 0) {
      console.log(`Frontend version ${version} is already synchronized`);
      return;
    }
    if (diff.status !== 1) throw new Error(`git diff failed (exit ${diff.status}): ${safeGitError(diff.stderr)}`);
    git(checkout, ["commit", "-m", `chore: sync frontend version to ${version}`,
      "-m", "Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"]);
    const pushed = git(checkout, ["push", "origin", `HEAD:refs/heads/${branch}`], true);
    if (pushed.status === 0) {
      console.log(`Committed frontend version ${version} to ${branch}`);
      return;
    }
    git(checkout, ["fetch", "--no-tags", "origin", `refs/heads/${branch}`]);
    const latest = git(checkout, ["rev-parse", "FETCH_HEAD"]).stdout.trim();
    if (latest === base || attempt === 3) {
      throw new Error(`Could not push the frontend version commit to ${branch}: ${safeGitError(pushed.stderr)}`);
    }
    console.log(`Default branch advanced while syncing ${version}; retrying against its latest commit`);
  }
}

function main(args) {
  const [command, version, directory, branch, ...extra] = args;
  if (!command || !version || extra.length || (command === "commit" ? !directory || !branch : !!branch)) {
    throw new Error("Usage: frontend-version.mjs set|check <version> [checkout] | commit <version> <checkout> <default-branch>");
  }
  if (command === "set") {
    console.log(`Frontend version set to ${setFrontendVersion(directory ? resolve(directory) : root, version)}`);
  } else if (command === "check") {
    checkFrontendVersion(directory ? resolve(directory) : root, version);
    console.log(`Frontend version ${normalizeVersion(version)} verified`);
  } else if (command === "commit") {
    commitFrontendVersion(directory, version, branch);
  } else {
    throw new Error(`Unknown frontend version command '${command}'`);
  }
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  main(process.argv.slice(2));
}
