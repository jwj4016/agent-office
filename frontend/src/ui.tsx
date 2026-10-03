import {createContext, type ReactNode, useContext, useState} from 'react';
import {errorText} from './api';
import {t} from './i18n';
import type {ActorKind, Issue, RunStatus, StepStatus} from './types';

const runTone: Record<RunStatus, string> = {
    running: 'info', waiting: 'warn', paused: 'warn', succeeded: 'ok', failed: 'bad', interrupted: 'bad', cancelled: '',
};
const stepTone: Record<StepStatus, string> = {
    pending: '', running: 'info', verifying: 'info', waiting_human: 'warn', waiting_approval: 'warn',
    waiting_input: 'warn', succeeded: 'ok', skipped: '', failed: 'bad', interrupted: 'bad', cancelled: '', superseded: '',
};

export function RunBadge({status}: { status: RunStatus }) {
    return <span className={`badge ${runTone[status] ?? ''}`}>{t.runStatus[status] ?? status}</span>;
}

export function StepBadge({status}: { status: StepStatus }) {
    return <span className={`badge ${stepTone[status] ?? ''}`}>{t.stepStatus[status] ?? status}</span>;
}

export function ActorBadge({kind}: { kind: ActorKind }) {
    return <span className={`badge ${kind}`}>{kind === 'ai' ? t.common.ai : t.common.human}</span>;
}

export function ErrorBox({error}: { error?: string | null }) {
    return error ? <div className="alert" role="alert">{error}</div> : null;
}

export function IssueList({issues}: { issues: Issue[] }) {
    if (!issues.length) return null;
    return (
        <ul className="issue-list" aria-label="검증 결과">
            {issues.map((i, n) => (
                <li key={n} className={i.severity}>
                    {i.nodeId ? <strong>{i.nodeId}: </strong> : null}{i.message}
                    {i.severity === 'run' ? ' (실행 전 해결 필요)' : ''}
                </li>
            ))}
        </ul>
    );
}

// useAction wraps a button handler with busy and error state.
export function useAction() {
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState<string | null>(null);
    const run = async <T, >(fn: () => Promise<T>): Promise<T | undefined> => {
        setBusy(true);
        setError(null);
        try {
            return await fn();
        } catch (e) {
            setError(errorText(e));
            return undefined;
        } finally {
            setBusy(false);
        }
    };
    return {busy, error, setError, run};
}

// ConfirmButton asks for a second click instead of a browser dialog.
export function ConfirmButton({label, confirmLabel, onConfirm, className = 'btn', disabled}: {
    label: string; confirmLabel: string; onConfirm: () => void; className?: string; disabled?: boolean;
}) {
    const [armed, setArmed] = useState(false);
    if (!armed) return <button className={className} disabled={disabled} onClick={() => setArmed(true)}>{label}</button>;
    return (
        <span className="row">
            <button className={`${className} primary`} onClick={() => { setArmed(false); onConfirm(); }}>{confirmLabel}</button>
            <button className="btn" onClick={() => setArmed(false)}>{t.common.cancel}</button>
        </span>
    );
}

// The right-hand detail pane is filled by whichever view is active.
const DetailContext = createContext<(node: ReactNode | null) => void>(() => {});
export const DetailProvider = DetailContext.Provider;
export const useDetail = () => useContext(DetailContext);

export function shortTime(iso?: string) {
    if (!iso) return '';
    const d = new Date(iso);
    return isNaN(d.getTime()) ? iso : d.toLocaleString('ko-KR', {month: 'numeric', day: 'numeric', hour: '2-digit', minute: '2-digit'});
}

export const errMsg = errorText;
