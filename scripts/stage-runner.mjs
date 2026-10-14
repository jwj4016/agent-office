#!/usr/bin/env node
// Stages the Claude bridge for a package: the built dist/ (without tests)
// plus production node_modules holding the native Claude binary for one
// OS and CPU. The app finds it as `claude-runner/dist/main.js` next to the
// executable (Windows, Linux) or in Contents/Resources (macOS).
//
//   node scripts/stage-runner.mjs <out-dir> [--os darwin|win32|linux] [--cpu arm64|x64]
//
// runners/claude must be built first (npm ci && npm run build).

import {execFileSync} from 'node:child_process';
import {cpSync, existsSync, mkdirSync, readdirSync, rmSync, statSync} from 'node:fs';
import {dirname, join, resolve} from 'node:path';
import {fileURLToPath} from 'node:url';

const args = process.argv.slice(2);
const opt = (name, def) => {
    const i = args.indexOf('--' + name);
    return i >= 0 ? args[i + 1] : def;
};
const out = args[0] && !args[0].startsWith('--') ? resolve(args[0]) : undefined;
if (!out) {
    console.error('usage: stage-runner.mjs <out-dir> [--os darwin|win32|linux] [--cpu arm64|x64]');
    process.exit(2);
}
const os = opt('os', process.platform);
const cpu = opt('cpu', process.arch);
const src = join(dirname(fileURLToPath(import.meta.url)), '..', 'runners', 'claude');
if (!existsSync(join(src, 'dist', 'main.js'))) {
    console.error(`${src}/dist/main.js is missing: run npm ci && npm run build in runners/claude`);
    process.exit(1);
}

rmSync(out, {recursive: true, force: true});
mkdirSync(join(out, 'dist'), {recursive: true});
for (const f of readdirSync(join(src, 'dist'))) {
    if (f.endsWith('.js') && !f.endsWith('.test.js')) cpSync(join(src, 'dist', f), join(out, 'dist', f));
}
for (const f of ['package.json', 'package-lock.json']) cpSync(join(src, f), join(out, f));

const npmArgs = ['ci', '--omit=dev', '--ignore-scripts', '--no-audit', '--no-fund', `--os=${os}`, `--cpu=${cpu}`];
if (os === 'linux') npmArgs.push('--libc=glibc');
const npm = process.platform === 'win32' ? 'npm.cmd' : 'npm';
execFileSync(npm, npmArgs, {cwd: out, stdio: 'inherit', shell: process.platform === 'win32'});

// Keep only this platform's native Claude binary (each one is ~220MB).
const scope = join(out, 'node_modules', '@anthropic-ai');
const native = `claude-agent-sdk-${os}-${cpu}`;
for (const d of readdirSync(scope)) {
    if (d.startsWith('claude-agent-sdk-') && d !== native) rmSync(join(scope, d), {recursive: true, force: true});
}
const exe = join(scope, native, os === 'win32' ? 'claude.exe' : 'claude');
if (!existsSync(exe)) {
    console.error(`missing native Claude binary: ${exe}`);
    process.exit(1);
}
// package.json stays: it marks dist/ as ES modules.
rmSync(join(out, 'package-lock.json'), {force: true});

const size = (p) => statSync(p).isDirectory() ? readdirSync(p).reduce((n, f) => n + size(join(p, f)), 0) : statSync(p).size;
console.log(`staged claude-runner for ${os}-${cpu}: ${out} (${Math.round(size(out) / 1e6)} MB)`);
