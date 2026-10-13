import {act, render, screen} from '@testing-library/react';
import {afterEach, beforeEach, describe, expect, it, vi} from 'vitest';
import {fakeApi} from './test/fakeApi';

vi.mock('./api', () => ({api: fakeApi, errorText: (e: unknown) => String(e)}));

import {LiveProvider, urgentEvent, useLive} from './live';
import type {StoredEvent} from './types';

function Version() {
    return <span data-testid="v">{useLive().version}</span>;
}

const ev = (kind: string): StoredEvent => ({sequence: 1, id: kind, projectId: 'prj-1', runId: 'run-1', kind, payload: {}, createdAt: ''});

let send: (evs: StoredEvent[]) => void = () => {};
beforeEach(() => {
    vi.useFakeTimers();
    fakeApi.onEvents.mockImplementation(((cb: (evs: StoredEvent[]) => void) => { send = cb; return () => {}; }) as never);
});
afterEach(() => vi.useRealTimers());

describe('LiveProvider', () => {
    it('batches ordinary events for about 100ms', () => {
        render(<LiveProvider><Version/></LiveProvider>);
        act(() => { send([ev('provider.tool')]); send([ev('artifact.created')]); });
        expect(screen.getByTestId('v')).toHaveTextContent('0');
        act(() => { vi.advanceTimersByTime(100); });
        expect(screen.getByTestId('v')).toHaveTextContent('1');
    });

    it('applies completions, failures and waits for a person at once', () => {
        render(<LiveProvider><Version/></LiveProvider>);
        act(() => { send([ev('provider.tool')]); send([ev('step.failed')]); });
        expect(screen.getByTestId('v')).toHaveTextContent('1'); // both, in one refresh
        act(() => { vi.advanceTimersByTime(200); });
        expect(screen.getByTestId('v')).toHaveTextContent('1');
    });

    it('knows which events are urgent', () => {
        for (const k of ['step.succeeded', 'step.waiting_approval', 'run.status', 'approval.requested', 'message.question', 'provider.completed']) {
            expect(urgentEvent.test(k)).toBe(true);
        }
        for (const k of ['step.running', 'provider.tool', 'provider.usage', 'message.handoff', 'artifact.created']) {
            expect(urgentEvent.test(k)).toBe(false);
        }
    });
});
