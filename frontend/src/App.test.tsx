import {fireEvent, render, screen, waitFor} from '@testing-library/react';
import {beforeEach, describe, expect, it, vi} from 'vitest';
import App, {EVENT_PING} from './App';

const handlers = new Map<string, (data: unknown) => void>();
const settings: Record<string, string> = {};

vi.mock('../wailsjs/runtime/runtime', () => ({
    EventsOn: (name: string, cb: (data: unknown) => void) => {
        handlers.set(name, cb);
        return () => handlers.delete(name);
    },
}));

vi.mock('../wailsjs/go/main/App', () => ({
    Ping: vi.fn(async (message: string) => {
        const res = {sequence: 1, message, at: '2026-10-03T00:00:00Z'};
        handlers.get(EVENT_PING)?.(res);
        return res;
    }),
    SystemStatus: vi.fn(async () => ({dataDir: '/data', schemaVersion: 1, secretsPersistent: false, error: ''})),
    GetSettings: vi.fn(async () => ({...settings})),
    SetSetting: vi.fn(async (key: string, value: string) => {
        settings[key] = value;
    }),
}));

beforeEach(() => {
    for (const k of Object.keys(settings)) delete settings[k];
});

describe('App', () => {
    it('shows both the binding reply and the emitted event', async () => {
        render(<App/>);
        fireEvent.click(screen.getByRole('button', {name: '연결 확인'}));

        expect(await screen.findByText('응답: #1 연결 확인')).toBeInTheDocument();
        expect(screen.getByTestId('event')).toHaveTextContent('이벤트: #1 2026-10-03T00:00:00Z');
    });

    it('restores the reduced-motion setting on a fresh mount', async () => {
        const first = render(<App/>);
        fireEvent.click(screen.getByLabelText('모션 줄이기'));
        await waitFor(() => expect(settings['ui.reducedMotion']).toBe('true'));
        first.unmount();

        render(<App/>);
        await waitFor(() => expect(screen.getByLabelText('모션 줄이기')).toBeChecked());
    });

    it('says when keys only live in session memory', async () => {
        render(<App/>);
        expect(await screen.findByTestId('status')).toHaveTextContent('이번 세션 메모리만');
    });
});
