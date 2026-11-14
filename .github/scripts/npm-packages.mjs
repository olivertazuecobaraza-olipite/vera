#!/usr/bin/env node
// Genera los paquetes de npm de vera a partir de los binarios que compila GoReleaser.
//
//   node .github/scripts/npm-packages.mjs [dist] [salida]     (por defecto: dist  dist/npm)
//
// Patrón de esbuild/biome: un paquete por plataforma (vera-ia-darwin-arm64, …) con el
// binario dentro y campos os/cpu, y el paquete principal «vera-ia», que los declara como
// optionalDependencies y trae un lanzador. npm, pnpm, yarn y bun instalan solo el de tu
// plataforma, y no hace falta ningún script postinstall (pnpm y bun los bloquean por defecto).
//
// Lo usan release.yml (para publicar) y ci.yml (para probar la instalación en cada PR).
import { chmodSync, copyFileSync, mkdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { join } from "node:path";

const dist = process.argv[2] ?? "dist";
const out = process.argv[3] ?? join(dist, "npm");
const NAME = "vera-ia";

const meta = JSON.parse(readFileSync(join(dist, "metadata.json"), "utf8"));
const version = meta.version;
const artifacts = JSON.parse(readFileSync(join(dist, "artifacts.json"), "utf8"));

const common = {
  version,
  description: "Comandos de terminal con IA local (Ollama)",
  license: "MIT",
  author: "Oliver Tazueco",
  homepage: "https://github.com/olivertazuecobaraza-olipite/vera",
  repository: { type: "git", url: "git+https://github.com/olivertazuecobaraza-olipite/vera.git" },
  bugs: "https://github.com/olivertazuecobaraza-olipite/vera/issues",
};
const os = { darwin: "darwin", linux: "linux", windows: "win32" }; // GOOS → process.platform
const cpu = { amd64: "x64", arm64: "arm64" }; // GOARCH → process.arch

rmSync(out, { recursive: true, force: true });

const platforms = [];
for (const a of artifacts) {
  if (a.type !== "Binary" || !os[a.goos] || !cpu[a.goarch]) continue;
  const name = `${NAME}-${os[a.goos]}-${cpu[a.goarch]}`;
  const exe = a.goos === "windows" ? "vera.exe" : "vera";
  const dir = join(out, name);
  mkdirSync(join(dir, "bin"), { recursive: true });
  copyFileSync(a.path, join(dir, "bin", exe));
  chmodSync(join(dir, "bin", exe), 0o755);
  copyFileSync("LICENSE", join(dir, "LICENSE"));
  writeFileSync(
    join(dir, "package.json"),
    JSON.stringify(
      {
        name,
        ...common,
        description: `Binario de vera para ${os[a.goos]} ${cpu[a.goarch]}. Instala «${NAME}», no este paquete.`,
        os: [os[a.goos]],
        cpu: [cpu[a.goarch]],
        files: ["bin/", "LICENSE"],
        preferUnplugged: true, // yarn PnP: el binario tiene que estar en disco
      },
      null,
      2,
    ) + "\n",
  );
  platforms.push(name);
}
if (platforms.length !== 6) {
  console.error(`Se esperaban 6 binarios y hay ${platforms.length}: ${platforms.join(", ")}`);
  process.exit(1);
}
platforms.sort();

// Paquete principal: lanzador + dependencias opcionales con la versión exacta.
const main = join(out, NAME);
mkdirSync(join(main, "bin"), { recursive: true });
writeFileSync(
  join(main, "bin", "vera.js"),
  `#!/usr/bin/env node
// Lanzador de vera: busca el binario del paquete de tu plataforma y lo ejecuta.
"use strict";
const { spawnSync } = require("node:child_process");

const pkg = \`${NAME}-\${process.platform}-\${process.arch}\`;
const exe = process.platform === "win32" ? "vera.exe" : "vera";
let bin;
try {
  bin = require.resolve(\`\${pkg}/bin/\${exe}\`);
} catch {
  console.error(
    \`vera: no está instalado el binario para \${process.platform}/\${process.arch} (\${pkg}).\\n\` +
      "Reinstala sin omitir las dependencias opcionales (sin --no-optional / --omit=optional),\\n" +
      "o instala vera con otro método: https://github.com/olivertazuecobaraza-olipite/vera#instalación",
  );
  process.exit(1);
}
const r = spawnSync(bin, process.argv.slice(2), { stdio: "inherit", windowsHide: false });
if (r.error) {
  console.error(\`vera: no se pudo ejecutar \${bin}: \${r.error.message}\`);
  process.exit(1);
}
if (r.signal) process.kill(process.pid, r.signal);
process.exit(r.status ?? 1);
`,
);
chmodSync(join(main, "bin", "vera.js"), 0o755);
copyFileSync("LICENSE", join(main, "LICENSE"));
copyFileSync("README.md", join(main, "README.md"));
writeFileSync(
  join(main, "package.json"),
  JSON.stringify(
    {
      name: NAME,
      ...common,
      keywords: ["cli", "ai", "ollama", "terminal", "shell", "llm"],
      bin: { vera: "bin/vera.js", ia: "bin/vera.js" }, // «ia» es un alias de vera, como en install.sh
      files: ["bin/", "LICENSE", "README.md"],
      engines: { node: ">=18" },
      optionalDependencies: Object.fromEntries(platforms.map((p) => [p, version])),
    },
    null,
    2,
  ) + "\n",
);

// Orden de publicación: primero las plataformas, el principal al final.
writeFileSync(join(out, "order.txt"), [...platforms, NAME].join("\n") + "\n");
console.log(`Paquetes npm ${version} en ${out}: ${[...platforms, NAME].join(", ")}`);
