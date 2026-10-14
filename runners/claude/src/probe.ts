// Probe checks that an installed bridge can really run: the SDK loads
// (main.ts imports it before calling probe) and the native Claude binary
// for this OS and CPU is installed and starts. It never calls the API.

import {execFileSync} from 'node:child_process';
import {existsSync, readFileSync} from 'node:fs';
import {join} from 'node:path';

export type ProbeResult = {
    ok: boolean;
    node: string;
    platform: string;
    sdkVersion?: string;
    binary?: string;
    binaryVersion?: string;
    error?: string;
};

// platformPackages lists the SDK's native packages that can serve this
// platform; Linux has glibc and musl builds.
export function platformPackages(platform: string, arch: string): string[] {
    const base = `@anthropic-ai/claude-agent-sdk-${platform}-${arch}`;
    return platform === 'linux' ? [base, base + '-musl'] : [base];
}

export function probe(
    root: string,
    platform: string = process.platform,
    arch: string = process.arch,
    run: (file: string) => string = (file) => execFileSync(file, ['--version'], {encoding: 'utf8', timeout: 30_000}),
): ProbeResult {
    const res: ProbeResult = {ok: false, node: process.version, platform: `${platform}-${arch}`};
    const modules = join(root, 'node_modules');
    try {
        res.sdkVersion = JSON.parse(readFileSync(join(modules, '@anthropic-ai', 'claude-agent-sdk', 'package.json'), 'utf8')).version;
    } catch {
        res.error = 'Claude Agent SDK가 설치되지 않았습니다';
        return res;
    }
    const exe = platform === 'win32' ? 'claude.exe' : 'claude';
    res.binary = platformPackages(platform, arch).map((p) => join(modules, ...p.split('/'), exe)).find((p) => existsSync(p));
    if (!res.binary) {
        res.error = `이 OS(${res.platform})용 Claude 실행 파일이 없습니다`;
        return res;
    }
    try {
        res.binaryVersion = run(res.binary).trim().split('\n')[0];
    } catch (e) {
        res.error = `Claude 실행 파일을 시작하지 못했습니다: ${(e as Error).message}`;
        return res;
    }
    res.ok = true;
    return res;
}
