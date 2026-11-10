// Fails when screen code hard-codes colors, font sizes or radii instead of using
// the tokens in src/theme/ (base.css, dark.css, light.css), and when the dark and light
// themes do not define exactly the same color tokens. Run: npm run check:tokens
import { readFileSync, readdirSync, statSync } from "node:fs";
import { join, relative, sep } from "node:path";
import { fileURLToPath } from "node:url";

const srcDir = fileURLToPath(new URL("../src/", import.meta.url));
const themeFiles = ["base.css", "dark.css", "light.css"].map((name) => join(srcDir, "theme", name));
const skip = new Set(themeFiles);
const rules = [
  [/#[0-9a-fA-F]{3,8}\b/, "hex color"],
  [/\brgba?\(/, "rgb()/rgba() color"],
  [/font-size:\s*[1-9]\d*px/, "literal font-size"],
  [/border-radius:\s*[1-9]\d*px/, "literal border-radius"],
  [/fontSize\s*:\s*[`"']?\d/, "literal fontSize"],
];

const problems = [];
const walk = (dir) => {
  for (const name of readdirSync(dir)) {
    const path = join(dir, name);
    if (statSync(path).isDirectory()) { walk(path); continue; }
    if (!/\.(css|tsx?)$/.test(name) || skip.has(path)) continue;
    readFileSync(path, "utf8").split(/\r?\n/).forEach((line, index) => {
      for (const [pattern, label] of rules) {
        if (pattern.test(line)) problems.push(`${relative(srcDir, path).split(sep).join("/")}:${index + 1}: ${label}: ${line.trim()}`);
      }
    });
  }
};
walk(srcDir);

// Both themes must define the same tokens, so a color added for one cannot be missing in the other.
const definedIn = (file) => new Set([...readFileSync(join(srcDir, "theme", file), "utf8").matchAll(/^\s*(--[\w-]+):/gm)].map((match) => match[1]));
const dark = definedIn("dark.css"), light = definedIn("light.css");
for (const name of dark) if (!light.has(name)) problems.push(`theme/light.css: missing token ${name} (defined in dark.css)`);
for (const name of light) if (!dark.has(name)) problems.push(`theme/dark.css: missing token ${name} (defined in light.css)`);
if (problems.length) {
  console.error(`Hard-coded theme values (use tokens from src/theme/):\n${problems.join("\n")}`);
  process.exit(1);
}
console.log("Theme tokens OK");
