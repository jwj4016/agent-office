import type {Options, SDKMessage} from '@anthropic-ai/claude-agent-sdk';
import assert from 'node:assert/strict';
import {PassThrough} from 'node:stream';
import {test} from 'node:test';
import {type QueryFn, runBridge, writeGuard} from './bridge.js';
import {mkdirSync, mkdtempSync, symlinkSync} from 'node:fs';
import {tmpdir} from 'node:os';
import {join} from 'node:path';

type Ev = { type: 'event'; kind: string; payload: any };

// harness wires a bridge to in-memory streams and collects emitted events.
function harness(query: QueryFn) {
    const input = new PassThrough();
    const output = new PassThrough();
    const events: Ev[] = [];
    let buf = '';
    const waiters: Array<() => void> = [];
    output.on('data', (chunk) => {
        buf += chunk;
        let i;
        while ((i = buf.indexOf('\n')) >= 0) {
            events.push(JSON.parse(buf.slice(0, i)));
            buf = buf.slice(i + 1);
            waiters.splice(0).forEach((w) => w());
        }
    });
    const send = (cmd: object) => input.write(JSON.stringify(cmd) + '\n');
    const waitFor = async (kind: string) => {
        while (!events.some((e) => e.kind === kind)) await new Promise<void>((r) => waiters.push(r));
        return events.find((e) => e.kind === kind)!;
    };
    const done = runBridge({input, output, query});
    return {events, send, waitFor, done};
}

const sys = {type: 'system', subtype: 'init', session_id: 'sess-1', model: 'claude-test'} as unknown as SDKMessage;
const text = (t: string) =>
    ({type: 'assistant', parent_tool_use_id: null, message: {content: [{type: 'text', text: t}]}}) as unknown as SDKMessage;
const result = (r: string) =>
    ({
        type: 'result', subtype: 'success', is_error: false, result: r,
        usage: {input_tokens: 11, output_tokens: 7}, total_cost_usd: 0.0012,
    }) as unknown as SDKMessage;

test('success maps to normalized events and passes isolation options', async () => {
    let seen: Options | undefined;
    const h = harness(async function* ({options}) {
        seen = options;
        yield sys;
        yield text('pong');
        yield result('pong');
    });
    h.send({type: 'start', prompt: 'ping', instructions: '짧게 답한다', allowedTools: ['Read']});
    await h.done;
    assert.deepEqual(h.events.map((e) => e.kind), ['started', 'message', 'usage', 'completed']);
    assert.deepEqual(h.events.at(-1)!.payload, {status: 'succeeded', text: 'pong'});
    assert.equal(h.events[2].payload.costUsd, 0.0012);
    assert.deepEqual(seen!.settingSources, []);
    assert.deepEqual(seen!.systemPrompt, {type: 'preset', preset: 'claude_code', append: '짧게 답한다'});
});

test('tool approval round trip, accept and decline', async () => {
    for (const decision of ['accept', 'decline']) {
        let outcome: string | undefined;
        const h = harness(async function* ({options}) {
            yield sys;
            const r = await options.canUseTool!('Bash', {command: 'npm test'}, {signal: new AbortController().signal} as any);
            outcome = r?.behavior;
            yield result('done');
        });
        h.send({type: 'start', prompt: 'x', askApproval: true});
        const req = await h.waitFor('approval_request');
        assert.equal(req.payload.action, 'Bash');
        h.send({type: 'respond', requestId: req.payload.requestId, decision});
        await h.done;
        assert.equal(outcome, decision === 'accept' ? 'allow' : 'deny');
        assert.equal(h.events.find((e) => e.kind === 'request_resolved')!.payload.decision, decision);
    }
});

test('tools outside policy are denied without asking when approval is off', async () => {
    let outcome: string | undefined;
    const h = harness(async function* ({options}) {
        const r = await options.canUseTool!('Bash', {}, {signal: new AbortController().signal} as any);
        outcome = r?.behavior;
        yield result('done');
    });
    h.send({type: 'start', prompt: 'x', askApproval: false});
    await h.done;
    assert.equal(outcome, 'deny');
    assert.ok(!h.events.some((e) => e.kind === 'approval_request'));
});

test('cancel aborts a running query', async () => {
    const h = harness(async function* ({options}) {
        yield sys;
        await new Promise((_, reject) =>
            options.abortController!.signal.addEventListener('abort', () => reject(new Error('aborted'))));
    });
    h.send({type: 'start', prompt: 'x'});
    await h.waitFor('started');
    h.send({type: 'cancel'});
    await h.done;
    assert.deepEqual(h.events.at(-1)!.payload, {status: 'cancelled'});
    assert.equal(h.events.filter((e) => e.kind === 'completed').length, 1);
});

test('error result is a failure even if text claimed success', async () => {
    const h = harness(async function* () {
        yield text('완료했습니다!');
        yield {type: 'result', subtype: 'error_max_turns', is_error: true, errors: [], usage: {input_tokens: 1, output_tokens: 1}, total_cost_usd: 0} as unknown as SDKMessage;
    });
    h.send({type: 'start', prompt: 'x'});
    await h.done;
    assert.deepEqual(h.events.at(-1)!.payload, {status: 'failed', error: 'error_max_turns'});
});

test('AskUserQuestion becomes inbox questions and answers flow back', async () => {
    let updated: any;
    const h = harness(async function* ({options}) {
        yield sys;
        const r = await options.canUseTool!('AskUserQuestion', {
            questions: [
                {question: '대상 플랫폼은?', header: '플랫폼', options: [{label: '웹', description: ''}, {label: '모바일', description: ''}], multiSelect: false},
                {question: '출시 지역은?', header: '지역', options: [{label: '한국', description: ''}, {label: '전세계', description: ''}], multiSelect: false},
            ],
        }, {signal: new AbortController().signal} as any);
        updated = r && r.behavior === 'allow' ? r.updatedInput : undefined;
        yield result('ok');
    });
    h.send({type: 'start', prompt: 'x', askApproval: true, writableDirs: ['/tmp/out']});
    const q1 = await h.waitFor('question');
    assert.match(q1.payload.detail, /대상 플랫폼은\?/);
    h.send({type: 'respond', requestId: q1.payload.requestId, answer: '웹'});
    while (h.events.filter((e) => e.kind === 'question').length < 2) await new Promise((r) => setTimeout(r, 5));
    const q2 = h.events.filter((e) => e.kind === 'question')[1];
    h.send({type: 'respond', requestId: q2.payload.requestId, answer: '한국'});
    await h.done;
    assert.deepEqual(updated.answers, {'대상 플랫폼은?': '웹', '출시 지역은?': '한국'});
});

test('writable dirs are passed as additional directories', async () => {
    let seen: Options | undefined;
    const h = harness(async function* ({options}) {
        seen = options;
        yield result('ok');
    });
    h.send({type: 'start', prompt: 'x', writableDirs: ['/data/attempt/out']});
    await h.done;
    assert.deepEqual(seen!.additionalDirectories, ['/data/attempt/out']);
});

test('write guard keeps file tools inside the work and output folders', async () => {
    const root = mkdtempSync(join(tmpdir(), 'guard-'));
    const work = join(root, 'work'), out = join(root, 'out'), outside = join(root, 'outside');
    for (const d of [work, out, outside]) mkdirSync(d);
    symlinkSync(outside, join(work, 'escape'));
    const guard = writeGuard(work, [out]);
    const decide = async (tool: string, file_path: string) => {
        const r: any = await guard({hook_event_name: 'PreToolUse', tool_name: tool, tool_input: {file_path}} as any, 'id', {signal: new AbortController().signal});
        return r.hookSpecificOutput?.permissionDecision ?? 'allow';
    };
    assert.equal(await decide('Write', 'src/new.ts'), 'allow');            // relative, inside work
    assert.equal(await decide('Edit', join(out, 'spec.md')), 'allow');     // granted output folder
    assert.equal(await decide('Write', join(outside, 'x.md')), 'deny');    // outside
    assert.equal(await decide('Write', '../outside/x.md'), 'deny');        // relative escape
    assert.equal(await decide('Write', 'escape/x.md'), 'deny');            // symlink escape
    assert.equal(await decide('Read', join(outside, 'x.md')), 'allow');    // not a write tool
});

test('the guard is installed as a PreToolUse hook', async () => {
    let seen: Options | undefined;
    const h = harness(async function* ({options}) {
        seen = options;
        yield result('ok');
    });
    h.send({type: 'start', prompt: 'x', cwd: '/tmp', writableDirs: ['/tmp/out']});
    await h.done;
    assert.equal(seen!.hooks!.PreToolUse!.length, 1);
});
