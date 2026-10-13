import {type KeyboardEvent, type MouseEvent, useEffect, useMemo, useRef, useState} from 'react';
import type {Route} from '../App';
import {api} from '../api';
import {t} from '../i18n';
import {useLoad} from '../live';
import {buildOffice, describe, type Desk, type DeskState, type Office as OfficeModel} from '../office/model';
import {draw, hit, layout, SCENE_W} from '../office/scene';
import type {Project} from '../types';
import {ActorBadge, ErrorBox, REDUCED_MOTION, StepBadge, useAction, useDetail} from '../ui';
import {ArtifactViewer} from './ArtifactViewer';

const VIEW = 'ui.officeView';
const COLS = 3;

// useOffice loads everything the office shows and rebuilds it whenever
// the project records new events.
export function useOffice(projectId: string) {
    const team = useLoad(() => Promise.all([api.assignments(projectId), api.roles()]), [projectId], projectId);
    const runs = useLoad(() => api.runs(projectId), [projectId], projectId);
    const inbox = useLoad(() => api.inbox(), [], '*');
    const office = useMemo(() => team.data && runs.data
        ? buildOffice(team.data[0], team.data[1], runs.data, (inbox.data ?? []).filter((i) => i.projectId === projectId))
        : undefined, [team.data, runs.data, inbox.data, projectId]);
    return {office, error: team.error ?? runs.error ?? inbox.error, reload: runs.reload};
}

// useReducedMotion follows the app setting and the OS preference. Until
// the setting is loaded motion counts as reduced, so nothing moves first.
function useReducedMotion() {
    const settings = useLoad(() => api.settings(), []);
    const query = typeof window !== 'undefined' && window.matchMedia ? window.matchMedia('(prefers-reduced-motion: reduce)') : null;
    const [os, setOs] = useState(query?.matches ?? false);
    useEffect(() => {
        if (!query) return;
        const on = () => setOs(query.matches);
        query.addEventListener?.('change', on);
        return () => query.removeEventListener?.('change', on);
    }, [query]);
    return os || !settings.data || settings.data[REDUCED_MOTION] === 'true';
}

export function Office({project, onRoute}: { project: Project; onRoute: (r: Route) => void }) {
    const {office, error} = useOffice(project.id);
    const settings = useLoad(() => api.settings(), []);
    const [view, setView] = useState<'office' | 'list' | null>(null);
    const shown = view ?? (settings.data?.[VIEW] === 'list' ? 'list' : 'office');
    const setDetail = useDetail();
    // A window too narrow for the office shows the list instead.
    const box = useRef<HTMLDivElement>(null);
    const [narrow, setNarrow] = useState(false);
    useEffect(() => {
        const el = box.current;
        if (!el || typeof ResizeObserver === 'undefined') return;
        const ro = new ResizeObserver(() => setNarrow(el.clientWidth > 0 && el.clientWidth < SCENE_W + 2));
        ro.observe(el);
        return () => ro.disconnect();
    }, [office !== undefined]);
    const choose = (v: 'office' | 'list') => { setView(v); api.setSetting(VIEW, v).catch(() => {}); };
    const open = (d: Desk) => setDetail(<DeskDetail project={project} assignmentId={d.assignment.id} onRoute={onRoute}/>);

    if (error) return <ErrorBox error={error}/>;
    if (!office) return <p className="muted">{t.common.loading}</p>;
    if (office.desks.length === 0) {
        return <p className="muted">아직 담당자가 없습니다. ‘담당자’ 탭에서 역할과 담당자를 배정하면 사무실에 자리가 생깁니다.</p>;
    }
    return (
        <div className="stack" ref={box}>
            <div className="row between">
                <p className="muted small" style={{margin: 0}}>실제 실행 상태를 보여 줍니다. 캐릭터 동작은 표시일 뿐 상태를 정하지 않습니다.</p>
                <div className="segmented" role="group" aria-label="보기 방식">
                    <button className="btn small" aria-pressed={shown === 'office'} onClick={() => choose('office')}>사무실</button>
                    <button className="btn small" aria-pressed={shown === 'list'} onClick={() => choose('list')}>목록</button>
                </div>
            </div>
            {shown === 'office' && narrow ? <p className="notice small">창이 좁아 목록으로 보여 줍니다.</p> : null}
            {shown === 'office' && !narrow ? <OfficeCanvas office={office} onOpen={open}/> : <OfficeList office={office} onOpen={open}/>}
            {office.meetings.length > 1 ? (
                <p className="small">진행 중인 회의: {office.meetings.map((m) => m.title).join(', ')}</p>
            ) : null}
            <Legend/>
        </div>
    );
}

function OfficeCanvas({office, onOpen}: { office: OfficeModel; onOpen: (d: Desk) => void }) {
    const reduced = useReducedMotion();
    const wrap = useRef<HTMLDivElement>(null);
    const canvas = useRef<HTMLCanvasElement>(null);
    const [scale, setScale] = useState(2);
    const [selected, setSelected] = useState(0);
    const [tick, setTick] = useState(0);
    const [visible, setVisible] = useState(true);
    const joy = useJoy(office, reduced);
    const l = useMemo(() => layout(office.desks.length), [office.desks.length]);
    const sel = Math.min(selected, office.desks.length - 1);

    // Whole-number scale that fits the available width.
    useEffect(() => {
        const el = wrap.current;
        if (!el || typeof ResizeObserver === 'undefined') return;
        const ro = new ResizeObserver(() => setScale(Math.max(1, Math.min(4, Math.floor(el.clientWidth / SCENE_W)))));
        ro.observe(el);
        return () => ro.disconnect();
    }, []);

    // Animation runs only while the office is on screen and motion is allowed.
    useEffect(() => {
        const el = wrap.current;
        const onVis = () => setVisible(!document.hidden);
        document.addEventListener('visibilitychange', onVis);
        let io: IntersectionObserver | undefined;
        if (el && typeof IntersectionObserver !== 'undefined') {
            io = new IntersectionObserver(([e]) => setVisible(e.isIntersecting && !document.hidden));
            io.observe(el);
        }
        return () => { document.removeEventListener('visibilitychange', onVis); io?.disconnect(); };
    }, []);
    const animate = !reduced && visible;
    useEffect(() => {
        if (!animate) { setTick(0); return; }
        const id = window.setInterval(() => setTick((n) => (n + 1) % 960), 170);
        return () => window.clearInterval(id);
    }, [animate]);

    useEffect(() => {
        const c = canvas.current;
        const ctx = c?.getContext?.('2d');
        if (!c || !ctx) return;
        const dpr = window.devicePixelRatio || 1;
        c.width = SCENE_W * scale * dpr;
        c.height = l.height * scale * dpr;
        ctx.setTransform(scale * dpr, 0, 0, scale * dpr, 0, 0);
        ctx.imageSmoothingEnabled = false;
        draw(ctx, office, l, {tick: animate ? tick : 0, selected: sel, joy});
    }, [office, l, scale, tick, animate, sel, joy]);

    const move = (to: number) => setSelected(Math.max(0, Math.min(office.desks.length - 1, to)));
    const onKey = (e: KeyboardEvent) => {
        const keys: Record<string, () => void> = {
            ArrowRight: () => move(sel + 1), ArrowLeft: () => move(sel - 1),
            ArrowDown: () => move(sel + COLS), ArrowUp: () => move(sel - COLS),
            Home: () => move(0), End: () => move(office.desks.length - 1),
            Enter: () => onOpen(office.desks[sel]), ' ': () => onOpen(office.desks[sel]),
        };
        const fn = keys[e.key];
        if (fn) { e.preventDefault(); fn(); }
    };
    const onClick = (e: MouseEvent<HTMLCanvasElement>) => {
        const rect = e.currentTarget.getBoundingClientRect();
        const i = hit(office, l, (e.clientX - rect.left) / scale, (e.clientY - rect.top) / scale);
        if (i >= 0) { setSelected(i); onOpen(office.desks[i]); }
    };
    const current = office.desks[sel];

    return (
        <div ref={wrap} className="office">
            <canvas ref={canvas} className="office-canvas" tabIndex={0} role="application" aria-roledescription="사무실"
                    aria-label="사무실. 화살표 키로 담당자를 고르고 Enter로 상세를 엽니다."
                    aria-describedby="office-selected"
                    style={{width: SCENE_W * scale, height: l.height * scale}} onKeyDown={onKey} onClick={onClick}/>
            <p id="office-selected" className="small" aria-live="polite">{current ? describe(current) : ''}</p>
        </div>
    );
}

// useJoy remembers which desks just finished, for a two-second hop.
function useJoy(office: OfficeModel, reduced: boolean) {
    const prev = useRef<Map<string, DeskState>>(new Map());
    const [joy, setJoy] = useState<Set<string>>(new Set());
    useEffect(() => {
        const fresh: string[] = [];
        for (const d of office.desks) {
            const was = prev.current.get(d.assignment.id);
            if (d.state === 'done' && was !== undefined && was !== 'done') fresh.push(d.assignment.id);
            prev.current.set(d.assignment.id, d.state);
        }
        if (!fresh.length || reduced) return;
        setJoy((s) => new Set([...s, ...fresh]));
        const id = window.setTimeout(() => setJoy((s) => new Set([...s].filter((x) => !fresh.includes(x)))), 2000);
        return () => window.clearTimeout(id);
    }, [office, reduced]);
    return joy;
}

const tone: Record<DeskState, string> = {idle: '', working: 'info', meeting: 'info', waiting: 'warn', checking: 'info', done: 'ok', problem: 'bad'};

// OfficeList is the plain alternative: every desk, its state in words and
// the same detail (and actions) as the office.
export function OfficeList({office, onOpen}: { office: OfficeModel; onOpen: (d: Desk) => void }) {
    return (
        <table className="list" aria-label="담당자 상태">
            <thead><tr><th>담당자</th><th>상태</th><th>지금 업무</th><th/></tr></thead>
            <tbody>
            {office.desks.map((d) => (
                <tr key={d.assignment.id}>
                    <td>{d.assignment.displayName} <ActorBadge kind={d.assignment.actorKind}/>
                        {d.roleName ? <div className="muted small">{d.roleName}</div> : null}</td>
                    <td><span className={`badge ${tone[d.state]}`}>{d.label}</span></td>
                    <td className="small">{d.summary || <span className="muted">없음</span>}
                        {d.work.length > 1 ? <span className="muted"> 외 {d.work.length - 1}건</span> : null}</td>
                    <td><button className="btn small" onClick={() => onOpen(d)}
                                aria-label={`${d.assignment.displayName} 상세`}>상세</button></td>
                </tr>
            ))}
            </tbody>
        </table>
    );
}

function Legend() {
    const items: [string, string][] = [
        ['모니터 켜짐·진행 막대', '작업 중'], ['회의실 자리', '회의 중'], ['주황 카드', '사람 응답·내 할 일 대기'],
        ['체크리스트', '결과 확인 중'], ['초록 체크·책상 위 문서', '완료·결과 있음'], ['빨간 표시', '문제 발생 (상세에서 다시 시도)'],
    ];
    return (
        <details className="small">
            <summary>그림 설명</summary>
            <ul>{items.map(([k, v]) => <li key={k}>{k}: {v}</li>)}</ul>
            <p className="muted">가슴의 보라 표시는 AI, 주황 표시는 사람입니다. 사람은 앱에서 남긴 기록만 보여 주며 자리 밖 행동은 추정하지 않습니다.</p>
        </details>
    );
}

// DeskDetail is the right-hand panel for one assignment: its real state,
// the steps behind it and what can be done about each.
export function DeskDetail({project, assignmentId, onRoute}: { project: Project; assignmentId: string; onRoute: (r: Route) => void }) {
    const {office, error, reload} = useOffice(project.id);
    const conns = useLoad(() => api.connections(), []);
    const setDetail = useDetail();
    const action = useAction();
    if (error) return <ErrorBox error={error}/>;
    const d = office?.desks.find((x) => x.assignment.id === assignmentId);
    if (!office) return <p className="muted">{t.common.loading}</p>;
    if (!d) return <p className="muted">이 담당자는 더 이상 없습니다.</p>;
    const a = d.assignment;
    const conn = (conns.data ?? []).find((c) => c.id === a.connectionId);
    const back = () => setDetail(<DeskDetail project={project} assignmentId={assignmentId} onRoute={onRoute}/>);
    return (
        <div className="stack">
            <div className="row between">
                <h2 style={{margin: 0}}>{a.displayName}</h2>
                <button className="btn small" onClick={() => setDetail(null)}>{t.common.close}</button>
            </div>
            <div className="row"><ActorBadge kind={a.actorKind}/><span className={`badge ${tone[d.state]}`}>{d.label}</span></div>
            <div className="small">
                {d.roleName ? <div>역할: {d.roleName}</div> : null}
                {a.actorKind === 'ai'
                    ? <div>연결: {conn ? `${conn.name}${conn.usable ? '' : ' (확인 필요)'}` : '없음'}{a.model ? ` · ${a.model}` : ''}</div>
                    : <div>앱에서 처리한 일만 표시합니다.</div>}
            </div>
            <ErrorBox error={action.error}/>
            {d.work.length === 0 ? <p className="muted small">지금 맡은 업무가 없습니다.</p> : null}
            {d.work.map((w) => (
                <section key={`${w.runId}/${w.stepId}/${w.participant}`} className="card stack" aria-label={w.stepTitle}>
                    <div className="row between">
                        <strong>{w.stepTitle}</strong>
                        <StepBadge status={w.status}/>
                    </div>
                    <div className="muted small">{w.runTitle} · v{w.versionNumber}{w.participant ? ' · 회의 참여' : ''}</div>
                    {w.error ? <div className="small" style={{color: 'var(--danger)'}}>{w.error}</div> : null}
                    {w.inbox.length ? <div className="small">기다리는 일: {w.inbox.map((k) => t.inboxKind[k]).join(', ')}</div> : null}
                    <div className="row">
                        <button className="btn small" onClick={() => onRoute({view: 'project', projectId: project.id, tab: 'runs', runId: w.runId})}>실행 보기</button>
                        {w.inbox.length ? <button className="btn small primary" onClick={() => onRoute({view: 'inbox'})}>내 할 일에서 처리</button> : null}
                        {w.canRetry ? (
                            <button className="btn small" disabled={action.busy}
                                    onClick={() => action.run(async () => { await api.retryStep(project.id, w.runId, w.stepId); reload(); })}>다시 시도</button>
                        ) : null}
                        {w.artifacts.map((art) => (
                            <button key={art.id} className="btn small"
                                    onClick={() => setDetail(<ArtifactViewer projectId={project.id} artifactId={art.id} onClose={back}/>)}>
                                {art.outputKey} v{art.version}</button>
                        ))}
                    </div>
                </section>
            ))}
        </div>
    );
}
