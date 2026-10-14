// Screen measurements for performance runs (see perf.go). They only run
// when the app was started with AGENT_OFFICE_PERF; the normal app never
// schedules any of this.
import {api} from './api';

export type LagStats = { n: number; p50: number; p95: number; max: number };

// lagStats summarises event-loop delays in milliseconds.
export function lagStats(samples: number[]): LagStats {
    if (samples.length === 0) return {n: 0, p50: 0, p95: 0, max: 0};
    const s = [...samples].sort((a, b) => a - b);
    const at = (q: number) => Math.round(s[Math.min(s.length - 1, Math.floor(q * s.length))]);
    return {n: s.length, p50: at(0.5), p95: at(0.95), max: Math.round(s[s.length - 1])};
}

// startLagProbe measures how late a 100ms timer fires (time the screen
// could not respond) and reports a summary every reportMs.
export function startLagProbe(intervalMs = 100, reportMs = 10_000): () => void {
    let samples: number[] = [];
    let expected = performance.now() + intervalMs;
    const tick = setInterval(() => {
        const now = performance.now();
        samples.push(Math.max(0, now - expected));
        expected = now + intervalMs;
    }, intervalMs);
    const report = setInterval(() => {
        api.perfMark('lag', JSON.stringify({...lagStats(samples), hidden: document.hidden})).catch(() => {});
        samples = [];
    }, reportMs);
    return () => {
        clearInterval(tick);
        clearInterval(report);
    };
}

// markReady records the first screen with data, after it is painted. A
// hidden window never paints (no animation frames), so it is recorded at
// once and marked hidden.
export function markReady() {
    const mark = () => api.perfMark('ui-ready', JSON.stringify({
        sinceNavigationMs: Math.round(performance.now()), hidden: document.hidden,
    })).catch(() => {});
    if (document.hidden) setTimeout(mark, 0);
    else requestAnimationFrame(() => setTimeout(mark, 0));
}
