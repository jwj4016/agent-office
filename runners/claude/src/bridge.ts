// JSONL bridge between Agent Office (Go) and the Claude Agent SDK.
//
// Go -> bridge (stdin), one JSON object per line:
//   {"type":"start", ...StartCommand}
//   {"type":"respond", "requestId", "decision"?, "answer"?}
//   {"type":"cancel"}
// bridge -> Go (stdout):
//   {"type":"event", "kind", "payload"}  kinds match internal/providers
// The last event is always kind "completed". Diagnostics go to stderr.

import type {
    CanUseTool,
    HookCallback,
    Options,
    PermissionResult,
    Query,
    SDKMessage,
} from '@anthropic-ai/claude-agent-sdk';
import {existsSync, realpathSync} from 'node:fs';
import {dirname, isAbsolute, join, relative, resolve} from 'node:path';
import {createInterface} from 'node:readline';
import type {Readable, Writable} from 'node:stream';

export type StartCommand = {
    type: 'start';
    prompt: string;
    instructions?: string;
    cwd?: string;
    model?: string;
    /** Tools that run without asking. Everything else goes through approval. */
    allowedTools?: string[];
    /** When false, tools outside allowedTools are denied instead of asked. */
    askApproval?: boolean;
    maxTurns?: number;
    maxBudgetUsd?: number;
    /** Extra folders the agent may write (the attempt's output folder). */
    writableDirs?: string[];
    /** When true the working folder itself is not writable; only writableDirs are. */
    readOnlyCwd?: boolean;
    /** Folders the agent may read without asking (bundled skills); never writable. */
    readableDirs?: string[];
    /** Local plugin folders loaded for this session only (e.g. the diagram skill). */
    plugins?: string[];
    /** A self-check [interpreter, script] that runs without asking on files in the work and output folders. */
    checkCommand?: string[];
};

type Question = { question: string; header?: string; options?: { label: string; description?: string }[]; multiSelect?: boolean };

type RespondCommand = { type: 'respond'; requestId: string; decision?: string; answer?: string };
type Command = StartCommand | RespondCommand | { type: 'cancel' };

// realpathNearest resolves symlinks of the deepest existing ancestor, so a
// path that does not exist yet is judged by where it would really land.
function realpathNearest(p: string): string {
    let cur = p;
    const rest: string[] = [];
    while (!existsSync(cur)) {
        const parent = dirname(cur);
        if (parent === cur) break;
        rest.unshift(cur.slice(parent.length).replace(/^[\\/]/, ''));
        cur = parent;
    }
    const base = existsSync(cur) ? realpathSync.native(cur) : cur;
    return rest.length ? join(base, ...rest) : base;
}

function inside(root: string, p: string): boolean {
    const rel = relative(root, p);
    return rel === '' || (!rel.startsWith('..') && !isAbsolute(rel));
}

const fileWriteTools = new Set(['Write', 'Edit', 'MultiEdit', 'NotebookEdit']);
const fileReadTools = new Set(['Read', 'Glob', 'Grep']);

// readAllowed reports whether a read-only file tool stays inside the
// granted readable folders (bundled skills): such reads need no approval.
export function readAllowed(cwd: string, readable: string[], toolName: string, toolInput: unknown): boolean {
    if (!fileReadTools.has(toolName) || readable.length === 0) return false;
    const ti = (toolInput ?? {}) as Record<string, unknown>;
    const target = ti.file_path ?? ti.path;
    if (typeof target !== 'string') return false;
    const real = realpathNearest(resolve(cwd, target));
    return readable.some((r) => inside(realpathNearest(resolve(r)), real));
}

// writeTarget returns where a file-writing tool would really write, or
// undefined for other tools.
function writeTarget(cwd: string, toolName: string, toolInput: unknown): string | undefined {
    if (!fileWriteTools.has(toolName)) return undefined;
    const ti = (toolInput ?? {}) as Record<string, unknown>;
    const target = ti.file_path ?? ti.notebook_path;
    return typeof target === 'string' ? realpathNearest(resolve(cwd, target)) : undefined;
}

// writeAllowed reports whether a file-writing tool stays inside the work
// and output folders: such writes need no per-file approval.
export function writeAllowed(cwd: string, writable: string[], toolName: string, toolInput: unknown, readOnlyCwd = false): boolean {
    const real = writeTarget(cwd, toolName, toolInput);
    if (real === undefined) return false;
    const roots = writeRoots(cwd, writable, readOnlyCwd);
    return roots.some((r) => inside(r, real));
}

// writeRoots are the folders file-writing tools may touch.
function writeRoots(cwd: string, writable: string[], readOnlyCwd: boolean): string[] {
    return (readOnlyCwd ? writable : [cwd, ...writable]).map((r) => realpathNearest(resolve(r)));
}

// writeGuard is a PreToolUse hook: file-writing tools may only touch the
// working folder and the granted output folders, whatever the permission
// settings say. (Shell commands are not parsed; they go through approval.)
export function writeGuard(cwd: string, writable: string[], readOnlyCwd = false): HookCallback {
    const roots = writeRoots(cwd, writable, readOnlyCwd);
    return async (input) => {
        if (input.hook_event_name !== 'PreToolUse' || !fileWriteTools.has(input.tool_name)) return {};
        const ti = (input.tool_input ?? {}) as Record<string, unknown>;
        const target = ti.file_path ?? ti.notebook_path;
        if (typeof target !== 'string') return {};
        const real = realpathNearest(resolve(cwd, target));
        if (roots.some((r) => inside(r, real))) return {};
        return {
            hookSpecificOutput: {
                hookEventName: 'PreToolUse', permissionDecision: 'deny',
                permissionDecisionReason: `Agent Office 정책: 작업 폴더와 결과 폴더 밖에는 쓸 수 없습니다 (${target})`,
            },
        };
    };
}

// shellWords splits a command line into words. Only plain words and
// quoted strings are accepted: anything the shell would expand, chain or
// redirect makes it return undefined.
export function shellWords(line: string): string[] | undefined {
    const words: string[] = [];
    let cur: string | undefined;
    for (let i = 0; i < line.length; i++) {
        const c = line[i];
        if (c === ' ' || c === '\t') {
            if (cur !== undefined) words.push(cur);
            cur = undefined;
        } else if (c === "'" || c === '"') {
            const end = line.indexOf(c, i + 1);
            if (end < 0) return undefined;
            const quoted = line.slice(i + 1, end);
            if (c === '"' && /[$`\\!]/.test(quoted)) return undefined;
            cur = (cur ?? '') + quoted;
            i = end;
        } else if (/[A-Za-z0-9/._\-+=,@:%]/.test(c)) {
            cur = (cur ?? '') + c;
        } else {
            return undefined;
        }
    }
    if (cur !== undefined) words.push(cur);
    return words;
}

const bareInterpreters = new Set(['python3', 'python']);

// checkCommandAllowed reports whether a Bash command is exactly the
// granted self-check: its interpreter (or a bare python3/python), an
// optional -I, its script, then only --offline and files inside the work
// and output folders.
export function checkCommandAllowed(cwd: string, writable: string[], check: string[] | undefined, toolName: string, toolInput: unknown): boolean {
    if (toolName !== 'Bash' || !check || check.length !== 2) return false;
    const command = (toolInput as { command?: unknown } | undefined)?.command;
    if (typeof command !== 'string') return false;
    const words = shellWords(command.trim());
    if (!words || words.length < 3) return false;
    const [exe, ...rest] = words;
    if (exe !== check[0] && !bareInterpreters.has(exe)) return false;
    if (rest[0] === '-I') rest.shift();
    const script = rest.shift();
    if (!script || realpathNearest(resolve(cwd, script)) !== realpathNearest(resolve(check[1]))) return false;
    const roots = writeRoots(cwd, writable, false);
    const files = rest.filter((a) => a !== '--offline');
    return files.length > 0 && files.every((f) => !f.startsWith('-') && roots.some((r) => inside(r, realpathNearest(resolve(cwd, f)))));
}

export type QueryFn = (params: { prompt: string; options: Options }) => Query | AsyncIterable<SDKMessage>;

export type BridgeIO = {
    input: Readable;
    output: Writable;
    query: QueryFn;
    log?: (msg: string) => void;
};

export function runBridge({input, output, query, log = () => {}}: BridgeIO): Promise<void> {
    let completed = false;
    const write = (kind: string, payload: unknown) => {
        if (completed) return;
        if (kind === 'completed') completed = true;
        output.write(JSON.stringify({type: 'event', kind, payload}) + '\n');
    };

    const abort = new AbortController();
    const pending = new Map<string, (r: RespondCommand) => void>();
    let nextRequest = 0;
    let started = false;
    let finish!: () => void;
    const done = new Promise<void>((resolve) => (finish = resolve));

    const ask = (kind: 'approval_request' | 'question', payload: Record<string, unknown>) =>
        new Promise<RespondCommand>((resolve, reject) => {
            const requestId = `claude-${++nextRequest}`;
            pending.set(requestId, resolve);
            abort.signal.addEventListener('abort', () => reject(new Error('cancelled')), {once: true});
            write(kind, {requestId, ...payload});
        });

    const start = async (cmd: StartCommand) => {
        const allowed = new Set(cmd.allowedTools ?? []);
        const canUseTool: CanUseTool = async (toolName, toolInput, opts): Promise<PermissionResult> => {
            if (toolName === 'AskUserQuestion') {
                // The agent asks the person: each question becomes an inbox
                // question and the answers go back as the tool's input.
                const questions = ((toolInput as { questions?: Question[] }).questions ?? []);
                const answers: Record<string, string> = {};
                for (const q of questions) {
                    const options = (q.options ?? []).map((o) => o.label);
                    const detail = options.length ? `${q.question}\n(선택지: ${options.join(' / ')})` : q.question;
                    const r = await ask('question', {action: 'question', detail, options});
                    write('request_resolved', {requestId: r.requestId, decision: 'answered'});
                    answers[q.question] = r.answer ?? '';
                }
                return {behavior: 'allow', updatedInput: {...toolInput, answers}};
            }
            if (allowed.has(toolName)) return {behavior: 'allow', updatedInput: toolInput};
            // Loading a skill from the plugins this attempt was given, and
            // reading those bundled files, is part of the task.
            if (toolName === 'Skill' && (cmd.plugins ?? []).length > 0) return {behavior: 'allow', updatedInput: toolInput};
            if (readAllowed(cmd.cwd ?? process.cwd(), cmd.readableDirs ?? [], toolName, toolInput)) {
                return {behavior: 'allow', updatedInput: toolInput};
            }
            if (checkCommandAllowed(cmd.cwd ?? process.cwd(), cmd.writableDirs ?? [], cmd.checkCommand, toolName, toolInput)) {
                return {behavior: 'allow', updatedInput: toolInput};
            }
            // Writing inside the work or output folder is the task itself;
            // the PreToolUse guard still blocks every other location.
            if (writeAllowed(cmd.cwd ?? process.cwd(), cmd.writableDirs ?? [], toolName, toolInput, cmd.readOnlyCwd)) {
                return {behavior: 'allow', updatedInput: toolInput};
            }
            if (!cmd.askApproval) {
                return {behavior: 'deny', message: 'Agent Office 정책에서 허용되지 않은 도구입니다.'};
            }
            const full = opts.title ?? `${toolName} ${JSON.stringify(toolInput)}`;
            const detail = full.length > 500 ? full.slice(0, 500) + '…' : full;
            const r = await ask('approval_request', {action: toolName, detail, options: ['accept', 'decline']});
            write('request_resolved', {requestId: r.requestId, decision: r.decision === 'accept' ? 'accept' : 'decline'});
            return r.decision === 'accept'
                ? {behavior: 'allow', updatedInput: toolInput}
                : {behavior: 'deny', message: '사용자가 거절했습니다.'};
        };

        const options: Options = {
            abortController: abort,
            cwd: cmd.cwd,
            additionalDirectories: [...(cmd.writableDirs ?? []), ...(cmd.readableDirs ?? [])],
            hooks: {PreToolUse: [{hooks: [writeGuard(cmd.cwd ?? process.cwd(), cmd.writableDirs ?? [], cmd.readOnlyCwd)]}]},
            model: cmd.model,
            canUseTool,
            allowedTools: cmd.allowedTools,
            includePartialMessages: true,
            maxTurns: cmd.maxTurns,
            maxBudgetUsd: cmd.maxBudgetUsd,
            // Isolation: never pick up the user's own Claude Code settings,
            // which could silently widen tool permissions. Plugins come only
            // from the app (cmd.plugins), never from the user's installs.
            settingSources: [],
            plugins: (cmd.plugins ?? []).map((path) => ({type: 'local' as const, path})),
            systemPrompt: cmd.instructions
                ? {type: 'preset', preset: 'claude_code', append: cmd.instructions}
                : {type: 'preset', preset: 'claude_code'},
            stderr: (data) => log(data),
        };

        let lastText = '';
        try {
            for await (const msg of query({prompt: cmd.prompt, options})) {
                switch (msg.type) {
                    case 'system':
                        if (msg.subtype === 'init') write('started', {sessionId: msg.session_id, model: msg.model});
                        break;
                    case 'stream_event': {
                        const ev = msg.event;
                        if (msg.parent_tool_use_id === null && ev.type === 'content_block_delta' && ev.delta.type === 'text_delta') {
                            write('message_delta', {text: ev.delta.text});
                        }
                        break;
                    }
                    case 'assistant':
                        if (msg.parent_tool_use_id !== null) break;
                        for (const block of msg.message.content) {
                            if (block.type === 'text') {
                                lastText = block.text;
                                write('message', {text: block.text});
                            } else if (block.type === 'tool_use') {
                                write('tool', {name: block.name, status: 'requested'});
                            }
                        }
                        break;
                    case 'result': {
                        // Input read from or written to the prompt cache is
                        // input too: without it a token budget sees a small
                        // part of what the call processed.
                        write('usage', {
                            inputTokens: msg.usage.input_tokens + (msg.usage.cache_creation_input_tokens ?? 0)
                                + (msg.usage.cache_read_input_tokens ?? 0),
                            outputTokens: msg.usage.output_tokens,
                            costUsd: msg.total_cost_usd,
                        });
                        if (msg.subtype === 'success' && !msg.is_error) {
                            write('completed', {status: 'succeeded', text: msg.result || lastText});
                        } else {
                            const error = msg.subtype === 'success' ? msg.result : msg.errors.join('; ') || msg.subtype;
                            write('completed', {status: 'failed', error});
                        }
                        break;
                    }
                }
            }
            if (abort.signal.aborted) write('completed', {status: 'cancelled'});
            else write('completed', {status: 'failed', error: 'stream ended without a result'});
        } catch (err) {
            if (abort.signal.aborted) write('completed', {status: 'cancelled'});
            else write('completed', {status: 'failed', error: err instanceof Error ? err.message : String(err)});
        }
    };

    const rl = createInterface({input});
    rl.on('line', (line) => {
        if (!line.trim()) return;
        let cmd: Command;
        try {
            cmd = JSON.parse(line);
        } catch {
            log(`unreadable command: ${line.slice(0, 200)}`);
            return;
        }
        switch (cmd.type) {
            case 'start':
                if (started) {
                    log('ignoring second start');
                    return;
                }
                started = true;
                start(cmd).finally(() => {
                    rl.close();
                    finish();
                });
                break;
            case 'respond': {
                const resolve = pending.get(cmd.requestId);
                pending.delete(cmd.requestId);
                if (resolve) resolve(cmd);
                else log(`no pending request ${cmd.requestId}`);
                break;
            }
            case 'cancel':
                abort.abort();
                if (!started) {
                    write('completed', {status: 'cancelled'});
                    rl.close();
                    finish();
                }
                break;
        }
    });
    // Go closing stdin before a result means the app went away: stop work.
    rl.on('close', () => {
        if (!completed) abort.abort();
        if (!started) finish();
    });
    return done;
}
