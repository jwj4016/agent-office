import {createContext, type ReactNode, useCallback, useContext, useEffect, useRef, useState} from 'react';
import {api, errorText} from './api';
import type {StoredEvent} from './types';

type Live = {
    // version increments when stored events arrive (all projects).
    version: number;
    // projectVersion increments per project.
    projectVersion: Record<string, number>;
    // streaming text per step attempt (cleared when the attempt ends).
    deltas: Record<string, string>;
};

const LiveContext = createContext<Live>({version: 0, projectVersion: {}, deltas: {}});

const terminalStep = /^step\.(succeeded|failed|cancelled|interrupted|superseded|verifying)$/;

// LiveProvider turns Go events into re-render signals. Events are applied
// in batches so a burst causes one refresh, not one per event.
export function LiveProvider({children}: { children: ReactNode }) {
    const [live, setLive] = useState<Live>({version: 0, projectVersion: {}, deltas: {}});
    const pending = useRef<StoredEvent[]>([]);
    const timer = useRef<number | undefined>(undefined);

    useEffect(() => {
        const flush = () => {
            timer.current = undefined;
            const evs = pending.current;
            pending.current = [];
            setLive((prev) => {
                const projectVersion = {...prev.projectVersion};
                const deltas = {...prev.deltas};
                for (const ev of evs) {
                    projectVersion[ev.projectId] = (projectVersion[ev.projectId] ?? 0) + 1;
                    if (ev.stepAttemptId && terminalStep.test(ev.kind)) delete deltas[ev.stepAttemptId];
                }
                return {version: prev.version + 1, projectVersion, deltas};
            });
        };
        const offEvents = api.onEvents((evs) => {
            pending.current.push(...evs);
            if (timer.current === undefined) timer.current = window.setTimeout(flush, 100);
        });
        const offDeltas = api.onDeltas((ds) => {
            setLive((prev) => {
                const deltas = {...prev.deltas};
                for (const d of ds) deltas[d.stepAttemptId] = (deltas[d.stepAttemptId] ?? '') + d.text;
                return {...prev, deltas};
            });
        });
        return () => {
            offEvents();
            offDeltas();
            window.clearTimeout(timer.current);
        };
    }, []);

    return <LiveContext.Provider value={live}>{children}</LiveContext.Provider>;
}

export const useLive = () => useContext(LiveContext);

// useLoad runs load() now and again whenever the watched project (or, for
// '*', any project) records new events.
export function useLoad<T>(load: () => Promise<T>, deps: unknown[], watch?: string) {
    const live = useLive();
    const signal = watch === '*' ? live.version : watch ? live.projectVersion[watch] ?? 0 : 0;
    const [state, setState] = useState<{ data?: T; error?: string; loading: boolean }>({loading: true});
    const seq = useRef(0);
    const reload = useCallback(() => {
        const mine = ++seq.current;
        load().then(
            (data) => mine === seq.current && setState({data, loading: false}),
            (e) => mine === seq.current && setState((s) => ({...s, error: errorText(e), loading: false})),
        );
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, deps);
    useEffect(() => {
        reload();
    }, [reload, signal]);
    return {...state, reload};
}
