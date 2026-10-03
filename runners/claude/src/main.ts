import {query} from '@anthropic-ai/claude-agent-sdk';
import {runBridge} from './bridge.js';

runBridge({
    input: process.stdin,
    output: process.stdout,
    query: ({prompt, options}) => query({prompt, options}),
    log: (msg) => process.stderr.write(msg.endsWith('\n') ? msg : msg + '\n'),
}).then(() => process.exit(0));
