import assert from 'node:assert/strict';
import {mkdirSync, mkdtempSync, writeFileSync} from 'node:fs';
import {tmpdir} from 'node:os';
import {join} from 'node:path';
import {test} from 'node:test';
import {platformPackages, probe} from './probe.js';

// fakeInstall lays out node_modules the way npm installs the SDK.
function fakeInstall(nativePkg?: string, exe = 'claude'): string {
    const root = mkdtempSync(join(tmpdir(), 'probe 시험-'));
    const sdk = join(root, 'node_modules', '@anthropic-ai', 'claude-agent-sdk');
    mkdirSync(sdk, {recursive: true});
    writeFileSync(join(sdk, 'package.json'), JSON.stringify({version: '9.9.9'}));
    if (nativePkg) {
        const dir = join(root, 'node_modules', '@anthropic-ai', nativePkg);
        mkdirSync(dir, {recursive: true});
        writeFileSync(join(dir, exe), '');
    }
    return root;
}

test('probe runs the native binary for this platform', () => {
    const root = fakeInstall('claude-agent-sdk-darwin-arm64');
    const res = probe(root, 'darwin', 'arm64', () => '2.1.288 (Claude Code)\n');
    assert.equal(res.ok, true, res.error ?? "");
    assert.equal(res.sdkVersion, '9.9.9');
    assert.equal(res.binaryVersion, '2.1.288 (Claude Code)');
});

test('probe accepts the musl build on Linux and claude.exe on Windows', () => {
    assert.deepEqual(platformPackages('linux', 'x64'), ['@anthropic-ai/claude-agent-sdk-linux-x64', '@anthropic-ai/claude-agent-sdk-linux-x64-musl']);
    assert.equal(probe(fakeInstall('claude-agent-sdk-linux-x64-musl'), 'linux', 'x64', () => 'v').ok, true);
    assert.equal(probe(fakeInstall('claude-agent-sdk-win32-x64', 'claude.exe'), 'win32', 'x64', () => 'v').ok, true);
});

test('probe reports a missing SDK, a binary for another CPU and a binary that does not start', () => {
    const empty = mkdtempSync(join(tmpdir(), 'probe-'));
    assert.match(probe(empty, 'darwin', 'arm64').error!, /SDK/);
    const other = probe(fakeInstall('claude-agent-sdk-darwin-x64'), 'darwin', 'arm64', () => 'v');
    assert.equal(other.ok, false);
    assert.match(other.error!, /darwin-arm64/);
    const broken = probe(fakeInstall('claude-agent-sdk-darwin-arm64'), 'darwin', 'arm64', () => {
        throw new Error('Bad CPU type');
    });
    assert.equal(broken.ok, false);
    assert.match(broken.error!, /Bad CPU type/);
});
