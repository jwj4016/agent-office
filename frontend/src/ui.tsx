// Shared UI parts. Every screen builds on these so states, colours and
// keyboard behaviour stay the same everywhere (docs/ui-redesign-plan.ko.md).
import {
    AlertCircle, AlertTriangle, Bot, Check, CheckCircle2, CircleDashed, CircleDot, CircleSlash, Clock, Eye, GitFork, GitMerge,
    Hand, Info, Loader2, type LucideIcon, MessageCircleQuestion, PauseCircle, ShieldCheck, SkipForward, Stamp, User, Wallet, Wrench, X,
    XCircle,
} from 'lucide-react';
import {
    Children, cloneElement, createContext, type ButtonHTMLAttributes, isValidElement, type KeyboardEvent, type ReactElement, type ReactNode,
    useCallback, useContext, useEffect, useId, useRef, useState,
} from 'react';
import {errorText} from './api';
import {t} from './i18n';
import type {ActorKind, InboxKind, Issue, NodeKind, RunStatus, StepStatus} from './types';

export type Tone = '' | 'ok' | 'warn' | 'bad' | 'info' | 'ai' | 'human';

// ---------- icons ----------

// Icon draws one lucide icon. Icons next to visible text are decorative
// and hidden from screen readers; pass label when an icon stands alone.
export function Icon({icon: I, size = 16, label, className}: { icon: LucideIcon; size?: number; label?: string; className?: string }) {
    return <I size={size} strokeWidth={1.75} className={className} aria-hidden={label ? undefined : true} aria-label={label}
              role={label ? 'img' : undefined} focusable={false}/>;
}

// ---------- status ----------

export const runTone: Record<RunStatus, Tone> = {
    running: 'info', waiting: 'warn', paused: 'warn', succeeded: 'ok', failed: 'bad', interrupted: 'bad', cancelled: '',
};
const runIcon: Record<RunStatus, LucideIcon> = {
    running: CircleDot, waiting: Hand, paused: PauseCircle, succeeded: CheckCircle2, failed: AlertTriangle, interrupted: CircleSlash, cancelled: XCircle,
};
export const stepTone: Record<StepStatus, Tone> = {
    pending: '', running: 'info', verifying: 'info', waiting_human: 'warn', waiting_approval: 'warn',
    waiting_input: 'warn', succeeded: 'ok', skipped: '', failed: 'bad', interrupted: 'bad', cancelled: '', superseded: '',
};
export const stepIcon: Record<StepStatus, LucideIcon> = {
    pending: CircleDashed, running: Loader2, verifying: ShieldCheck, waiting_human: Hand, waiting_approval: Stamp, waiting_input: MessageCircleQuestion,
    succeeded: Check, skipped: SkipForward, failed: AlertTriangle, interrupted: CircleSlash, cancelled: X, superseded: Clock,
};

// StatusBadge is the one way to show a state: icon, words and colour.
export function StatusBadge({tone = '', icon, children, className = ''}: { tone?: Tone; icon?: LucideIcon; children: ReactNode; className?: string }) {
    return <span className={`badge ${tone} ${className}`}>{icon ? <Icon icon={icon} size={13}/> : null}{children}</span>;
}

export function RunBadge({status}: { status: RunStatus }) {
    return <StatusBadge tone={runTone[status]} icon={runIcon[status]}>{t.runStatus[status] ?? status}</StatusBadge>;
}

export function StepBadge({status}: { status: StepStatus }) {
    return <StatusBadge tone={stepTone[status]} icon={stepIcon[status]}>{t.stepStatus[status] ?? status}</StatusBadge>;
}

export function ActorBadge({kind}: { kind: ActorKind }) {
    return <StatusBadge tone={kind} icon={kind === 'ai' ? Bot : User}>{kind === 'ai' ? t.common.ai : t.common.human}</StatusBadge>;
}

export const nodeKindIcon: Record<NodeKind, LucideIcon> = {task: Wrench, review: Eye, approval: Stamp, condition: GitFork, join: GitMerge};

export const inboxKindIcon: Record<InboxKind, LucideIcon> = {
    task: Wrench, review: Eye, approval: Stamp, tool_approval: ShieldCheck, question: MessageCircleQuestion, budget: Wallet,
};

// ---------- messages ----------

const bannerIcon: Record<string, LucideIcon> = {bad: AlertCircle, warn: AlertTriangle, ok: CheckCircle2, info: Info, '': Info};

// Banner is a message with an optional title and next actions. Errors
// and warnings are announced; quieter notes are not.
export function Banner({tone = 'info', title, children, actions, icon, role}: {
    tone?: 'ok' | 'warn' | 'bad' | 'info' | ''; title?: ReactNode; children?: ReactNode; actions?: ReactNode; icon?: LucideIcon; role?: string;
}) {
    return (
        <div className={`banner ${tone}`} role={role ?? (tone === 'bad' ? 'alert' : undefined)}>
            <Icon icon={icon ?? bannerIcon[tone]} size={18}/>
            <div className="banner-body">
                {title ? <div className="banner-title">{title}</div> : null}
                {children ? <div>{children}</div> : null}
            </div>
            {actions ? <div className="banner-actions">{actions}</div> : null}
        </div>
    );
}

export function ErrorBox({error}: { error?: string | null }) {
    return error ? <Banner tone="bad">{error}</Banner> : null;
}

export function IssueList({issues}: { issues: Issue[] }) {
    if (!issues.length) return null;
    return (
        <ul className="issue-list" aria-label="검증 결과">
            {issues.map((i, n) => (
                <li key={n} className={i.severity}>
                    <Icon icon={i.severity === 'error' ? AlertCircle : AlertTriangle} size={14}/>
                    <span>{i.nodeId ? <strong>{i.nodeId}: </strong> : null}{i.message}{i.severity === 'run' ? ' (실행 전 해결 필요)' : ''}</span>
                </li>
            ))}
        </ul>
    );
}

// EmptyState explains why something is empty and offers the next step.
export function EmptyState({icon, title, children, actions, compact}: {
    icon?: LucideIcon; title: ReactNode; children?: ReactNode; actions?: ReactNode; compact?: boolean;
}) {
    return (
        <div className={`empty ${compact ? 'compact' : ''}`}>
            {icon ? <Icon icon={icon} size={compact ? 20 : 28} className="empty-icon"/> : null}
            <div className="empty-title">{title}</div>
            {children ? <div>{children}</div> : null}
            {actions ? <div className="empty-actions">{actions}</div> : null}
        </div>
    );
}

export function Skeleton({height = 16, width = '100%'}: { height?: number; width?: number | string }) {
    return <div className="skeleton" style={{height, width}} aria-hidden/>;
}

// Loading is the placeholder while a screen's data arrives.
export function Loading({rows = 3}: { rows?: number }) {
    return (
        <div className="stack" role="status" aria-label={t.common.loading}>
            {Array.from({length: rows}, (_, i) => <Skeleton key={i} height={i === 0 ? 24 : 56}/>)}
        </div>
    );
}

// ---------- buttons ----------

type ButtonProps = ButtonHTMLAttributes<HTMLButtonElement> & {
    variant?: 'primary' | 'secondary' | 'ghost' | 'danger' | 'danger-solid'; size?: 'sm' | 'md';
    icon?: LucideIcon; busy?: boolean; iconOnly?: boolean;
};

export function Button({variant = 'secondary', size = 'md', icon, busy, iconOnly, className = '', children, disabled, type = 'button', ...rest}: ButtonProps) {
    const v = variant === 'danger-solid' ? 'danger primary' : variant === 'secondary' ? '' : variant;
    const cls = ['btn', v, size === 'sm' ? 'small' : '', iconOnly ? 'icon' : '', className].filter(Boolean).join(' ');
    return (
        <button type={type} className={cls} disabled={disabled || busy} aria-busy={busy || undefined} {...rest}>
            {busy ? <Icon icon={Loader2} size={size === 'sm' ? 14 : 16} className="spin"/> : icon ? <Icon icon={icon} size={size === 'sm' ? 14 : 16}/> : null}
            {children}
        </button>
    );
}

// ConfirmButton asks for a second click instead of a browser dialog.
export function ConfirmButton({label, confirmLabel, onConfirm, className = 'btn', disabled, icon}: {
    label: string; confirmLabel: string; onConfirm: () => void; className?: string; disabled?: boolean; icon?: LucideIcon;
}) {
    const [armed, setArmed] = useState(false);
    if (!armed) {
        return (
            <button type="button" className={className} disabled={disabled} onClick={() => setArmed(true)}>
                {icon ? <Icon icon={icon} size={className.includes('small') ? 14 : 16}/> : null}{label}
            </button>
        );
    }
    return (
        <span className="row nowrap" role="group" aria-label={`${label} 확인`}>
            <button type="button" className={`${className} primary`} autoFocus onClick={() => { setArmed(false); onConfirm(); }}>{confirmLabel}</button>
            <button type="button" className={`btn ${className.includes('small') ? 'small' : ''}`} onClick={() => setArmed(false)}>{t.common.cancel}</button>
        </span>
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

// ---------- layout ----------

// PageHeader holds the screen title. The title takes focus after a route
// change (App), so screen-reader users hear where they are.
export function PageHeader({eyebrow, title, desc, actions, badges}: {
    eyebrow?: ReactNode; title: ReactNode; desc?: ReactNode; actions?: ReactNode; badges?: ReactNode;
}) {
    return (
        <header className="page-header">
            <div className="titles">
                {eyebrow ? <div className="eyebrow">{eyebrow}</div> : null}
                <h1 id="page-title" tabIndex={-1}>{title}{badges}</h1>
                {desc ? <p className="desc">{desc}</p> : null}
            </div>
            {actions ? <div className="actions">{actions}</div> : null}
        </header>
    );
}

export function Section({title, count, actions, children, label, className = ''}: {
    title?: ReactNode; count?: number; actions?: ReactNode; children: ReactNode; label?: string; className?: string;
}) {
    return (
        <section className={`section ${className}`} aria-label={label ?? (typeof title === 'string' ? title : undefined)}>
            {title || actions ? (
                <div className="section-head">
                    {title ? <h2>{title}{count !== undefined ? <span className="count">{count}</span> : null}</h2> : <span/>}
                    {actions ? <div className="row">{actions}</div> : null}
                </div>
            ) : null}
            {children}
        </section>
    );
}

// Field gives one control a visible label and an optional hint or error,
// linked with aria-describedby. The hint is not part of the label.
export function Field({label, hint, error, optional, children, className = ''}: {
    label: ReactNode; hint?: ReactNode; error?: string | null; optional?: boolean; children: ReactElement; className?: string;
}) {
    const id = useId();
    const hintId = hint ? `${id}-hint` : undefined;
    const errId = error ? `${id}-err` : undefined;
    const child = Children.only(children);
    const props = (isValidElement(child) ? child.props : {}) as { id?: string };
    const controlId = props.id ?? id;
    const describedBy = [hintId, errId].filter(Boolean).join(' ') || undefined;
    return (
        <div className={`field ${className}`}>
            <label className="field-label" htmlFor={controlId}>{label}{optional ? <span className="optional"> (선택)</span> : null}</label>
            {cloneElement(child as ReactElement<Record<string, unknown>>, {id: controlId, 'aria-describedby': describedBy, 'aria-invalid': error ? true : undefined})}
            {hint ? <p id={hintId} className="field-hint">{hint}</p> : null}
            {error ? <p id={errId} className="field-error">{error}</p> : null}
        </div>
    );
}

export function ProgressBar({done, total, tone = '', label}: { done: number; total: number; tone?: Tone; label?: string }) {
    const pct = total ? Math.round((done / total) * 100) : 0;
    const text = label ?? `${done}/${total}`;
    return (
        <div className="progress" role="progressbar" aria-valuemin={0} aria-valuemax={total} aria-valuenow={done} aria-valuetext={`${total}개 중 ${done}개 완료`}>
            <div className="progress-track"><div className={`progress-fill ${tone}`} style={{transform: `scaleX(${pct / 100})`}}/></div>
            <span className="progress-label">{text}</span>
        </div>
    );
}

export type TabItem<T extends string> = { id: T; label: string; icon?: LucideIcon; badge?: ReactNode };

// Tabs follow the WAI tabs pattern: one tab in the Tab order, arrows,
// Home and End move between tabs, and the choice activates at once.
export function Tabs<T extends string>({items, value, onChange, label}: {
    items: TabItem<T>[]; value: T; onChange: (id: T) => void; label: string;
}) {
    const refs = useRef<Record<string, HTMLButtonElement | null>>({});
    const onKey = (e: KeyboardEvent, i: number) => {
        const to = {ArrowRight: i + 1, ArrowLeft: i - 1, Home: 0, End: items.length - 1}[e.key];
        if (to === undefined) return;
        e.preventDefault();
        const next = items[(to + items.length) % items.length];
        onChange(next.id);
        refs.current[next.id]?.focus();
    };
    return (
        <div role="tablist" aria-label={label} className="tabs">
            {items.map((it, i) => (
                <button key={it.id} ref={(el) => { refs.current[it.id] = el; }} role="tab" className="tab" type="button"
                        aria-selected={value === it.id} tabIndex={value === it.id ? 0 : -1}
                        onClick={() => onChange(it.id)} onKeyDown={(e) => onKey(e, i)}>
                    {it.icon ? <Icon icon={it.icon}/> : null}{it.label}{it.badge}
                </button>
            ))}
        </div>
    );
}

export function Stepper({steps, current}: { steps: string[]; current: number }) {
    return (
        <ol className="stepper" aria-label="진행 단계">
            {steps.map((s, i) => (
                <li key={s} className={i < current ? 'done' : ''} aria-current={i === current ? 'step' : undefined}>
                    <span className="step-no">{i < current ? <Icon icon={Check} size={12}/> : i + 1}</span>{s}
                </li>
            ))}
        </ol>
    );
}

// ---------- toast ----------

type ToastMsg = { id: number; text: string; tone: 'ok' | 'bad' };
const ToastContext = createContext<(text: string, tone?: 'ok' | 'bad') => void>(() => {});
export const useToast = () => useContext(ToastContext);

// ToastProvider shows short confirmations. They never take focus and are
// read out politely; each disappears after four seconds.
export function ToastProvider({children}: { children: ReactNode }) {
    const [items, setItems] = useState<ToastMsg[]>([]);
    const seq = useRef(0);
    const show = useCallback((text: string, tone: 'ok' | 'bad' = 'ok') => {
        const id = ++seq.current;
        setItems((xs) => [...xs.slice(-2), {id, text, tone}]);
        window.setTimeout(() => setItems((xs) => xs.filter((x) => x.id !== id)), 4000);
    }, []);
    return (
        <ToastContext.Provider value={show}>
            {children}
            <div className="toasts" role="status" aria-live="polite">
                {items.map((m) => (
                    <div key={m.id} className={`toast ${m.tone === 'bad' ? 'bad' : ''}`}>
                        <Icon icon={m.tone === 'bad' ? AlertCircle : CheckCircle2}/>{m.text}
                    </div>
                ))}
            </div>
        </ToastContext.Provider>
    );
}

// ---------- detail pane ----------

// The right-hand detail pane is filled by whichever view is active.
const DetailContext = createContext<(node: ReactNode | null) => void>(() => {});
export const DetailProvider = DetailContext.Provider;
export const useDetail = () => useContext(DetailContext);

// DetailPane is the right-hand pane (a drawer on narrow windows). Keyboard
// users land in it when it opens — not on every update, which would pull
// focus out of a field being typed in — and Escape closes it.
export function DetailPane({detail, onClose}: { detail: ReactNode | null; onClose: () => void }) {
    const aside = useRef<HTMLElement>(null);
    const open = detail !== null;
    useEffect(() => { if (open) aside.current?.focus(); }, [open]);
    if (!open) return null;
    return (
        <>
            <div className="scrim" onClick={onClose} aria-hidden/>
            <aside ref={aside} className="detail" aria-label="상세" tabIndex={-1}
                   onKeyDown={(e) => { if (e.key === 'Escape') onClose(); }}>{detail}</aside>
        </>
    );
}

// DetailHeader is the title row of a detail pane with its close button.
export function DetailHeader({title, onClose, children}: { title: ReactNode; onClose: () => void; children?: ReactNode }) {
    return (
        <div className="detail-head">
            <div className="stack tight" style={{minWidth: 0}}>
                <h2>{title}</h2>
                {children}
            </div>
            <Button variant="ghost" size="sm" iconOnly icon={X} aria-label={t.common.close} title={t.common.close} onClick={onClose}/>
        </div>
    );
}

// ---------- time ----------

export function shortTime(iso?: string) {
    if (!iso) return '';
    const d = new Date(iso);
    return isNaN(d.getTime()) ? iso : d.toLocaleString('ko-KR', {month: 'numeric', day: 'numeric', hour: '2-digit', minute: '2-digit'});
}

// relativeTime says how long ago, falling back to the date after a week.
export function relativeTime(iso?: string, now = Date.now()) {
    if (!iso) return '';
    const d = new Date(iso).getTime();
    if (isNaN(d)) return iso;
    const min = Math.round((now - d) / 60000);
    if (min < 1) return '방금';
    if (min < 60) return `${min}분 전`;
    const h = Math.round(min / 60);
    if (h < 24) return `${h}시간 전`;
    const days = Math.round(h / 24);
    return days < 7 ? `${days}일 전` : shortTime(iso);
}

export const errMsg = errorText;

// ---------- display settings ----------

export const REDUCED_MOTION = 'ui.reducedMotion';
export const THEME = 'ui.theme';
export type ThemePref = 'system' | 'light' | 'dark';

// applyMotion mirrors the app's '모션 줄이기' setting on the document so CSS
// stops animations as it does for the OS preference.
export function applyMotion(reduced: boolean) {
    document.documentElement.dataset.reducedMotion = reduced ? 'true' : 'false';
}

// useTheme is the theme in effect ('light' or 'dark'), following
// changes made by applyTheme, for parts that style themselves in script.
export function useTheme(): 'light' | 'dark' {
    const read = () => (document.documentElement.dataset.theme === 'dark' ? 'dark' : 'light');
    const [theme, setTheme] = useState<'light' | 'dark'>(read);
    useEffect(() => {
        if (typeof MutationObserver === 'undefined') return;
        const mo = new MutationObserver(() => setTheme(read()));
        mo.observe(document.documentElement, {attributes: true, attributeFilter: ['data-theme']});
        return () => mo.disconnect();
    }, []);
    return theme;
}

let themeQuery: MediaQueryList | null = null;
let themeListener: (() => void) | null = null;

// applyTheme sets data-theme to light or dark. 'system' follows the OS,
// including later changes.
export function applyTheme(pref: string | undefined) {
    const p: ThemePref = pref === 'light' || pref === 'dark' ? pref : 'system';
    if (themeQuery && themeListener) themeQuery.removeEventListener?.('change', themeListener);
    themeQuery = typeof window !== 'undefined' && window.matchMedia ? window.matchMedia('(prefers-color-scheme: dark)') : null;
    const set = () => {
        document.documentElement.dataset.theme = p === 'system' ? (themeQuery?.matches ? 'dark' : 'light') : p;
    };
    themeListener = p === 'system' ? set : null;
    if (themeListener) themeQuery?.addEventListener?.('change', themeListener);
    set();
}
