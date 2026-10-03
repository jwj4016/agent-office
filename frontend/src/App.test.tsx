import {fireEvent, render, screen} from '@testing-library/react';
import {describe, expect, it, vi} from 'vitest';
import App, {EVENT_PING} from './App';

const handlers = new Map<string, (data: unknown) => void>();

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
}));

describe('App', () => {
    it('shows both the binding reply and the emitted event', async () => {
        render(<App/>);
        fireEvent.click(screen.getByRole('button', {name: '연결 확인'}));

        expect(await screen.findByText('응답: #1 연결 확인')).toBeInTheDocument();
        expect(screen.getByTestId('event')).toHaveTextContent('이벤트: #1 2026-10-03T00:00:00Z');
    });
});
