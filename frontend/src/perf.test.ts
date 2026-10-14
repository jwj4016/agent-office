import {afterEach, beforeEach, describe, expect, it, vi} from 'vitest';

const perfMark = vi.fn(async (_n: string, _d: string) => {});
vi.mock('./api', () => ({api: {perfMark: (n: string, d: string) => perfMark(n, d)}}));

import {lagStats, startLagProbe} from './perf';

describe('perf', () => {
    beforeEach(() => vi.useFakeTimers());
    afterEach(() => {
        vi.useRealTimers();
        perfMark.mockClear();
    });

    it('summarises lag samples', () => {
        expect(lagStats([])).toEqual({n: 0, p50: 0, p95: 0, max: 0});
        const s = Array.from({length: 100}, (_, i) => i);
        expect(lagStats(s)).toEqual({n: 100, p50: 50, p95: 95, max: 99});
    });

    it('reports lag every interval and stops cleanly', () => {
        const stop = startLagProbe(100, 1000);
        vi.advanceTimersByTime(1000);
        expect(perfMark).toHaveBeenCalledTimes(1);
        const [name, data] = perfMark.mock.calls[0];
        expect(name).toBe('lag');
        expect(JSON.parse(data).n).toBeGreaterThanOrEqual(9);
        stop();
        vi.advanceTimersByTime(5000);
        expect(perfMark).toHaveBeenCalledTimes(1);
    });
});
