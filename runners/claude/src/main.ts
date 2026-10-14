import {query} from '@anthropic-ai/claude-agent-sdk';
import {dirname, join} from 'node:path';
import {fileURLToPath} from 'node:url';
import {runBridge} from './bridge.js';
import {probe} from './probe.js';

if (process.argv.includes('--probe')) {
    // dist/main.js -> the runner folder holding node_modules
    const res = probe(join(dirname(fileURLToPath(import.meta.url)), '..'));
    process.stdout.write(JSON.stringify(res) + '\n');
    process.exit(res.ok ? 0 : 1);
}

runBridge({
    input: process.stdin,
    output: process.stdout,
    query: ({prompt, options}) => query({prompt, options}),
    log: (msg) => process.stderr.write(msg.endsWith('\n') ? msg : msg + '\n'),
}).then(() => process.exit(0));
