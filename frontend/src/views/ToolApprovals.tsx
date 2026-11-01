import {AlertTriangle, CheckCheck, ExternalLink, ShieldCheck, ShieldX, Trash2} from 'lucide-react';
import {useState} from 'react';
import {api} from '../api';
import {useLoad} from '../live';
import type {InboxItem, Project, ToolRequest} from '../types';
import {Button, ErrorBox, Icon, Loading, relativeTime, Section, shortTime, StatusBadge, useAction, useToast} from '../ui';

// What each tool does, in words (Claude's tools and Codex's requests).
const toolLabel: Record<string, string> = {
    Read: '파일 읽기', Write: '파일 쓰기', Edit: '파일 수정', MultiEdit: '파일 수정', NotebookEdit: '노트북 수정',
    Bash: '명령 실행', Glob: '파일 찾기', Grep: '내용 검색', LS: '폴더 보기', WebFetch: '웹 페이지 읽기', WebSearch: '웹 검색',
    command: '명령 실행', file_change: '파일 변경',
};

const baseName = (path: string) => path.split(/[\\/]/).pop() || path;

// toolSummary says what a tool approval asks: what it does, on which file
// (named short), or the command in full (it is read before allowing it).
export function toolSummary(t: ToolRequest | undefined, detail = '') {
    if (!t) return {label: '도구 사용', target: '', command: detail};
    const label = toolLabel[t.name] ?? t.name;
    if (t.path) return {label, target: t.input ? `${t.input} (${baseName(t.path)})` : baseName(t.path), command: ''};
    return {label, target: '', command: t.command ?? ''};
}

// groupToolApprovals keeps items in order but puts the tool approvals of
// one attempt together, where the first of them was.
export function groupToolApprovals(items: InboxItem[]): (InboxItem | InboxItem[])[] {
    const out: (InboxItem | InboxItem[])[] = [];
    const byAttempt = new Map<string, InboxItem[]>();
    for (const it of items) {
        if (it.kind !== 'tool_approval') {
            out.push(it);
            continue;
        }
        const group = byAttempt.get(it.attemptId);
        if (group) group.push(it);
        else {
            const g = [it];
            byAttempt.set(it.attemptId, g);
            out.push(g);
        }
    }
    return out;
}

// ToolApprovalCard is one AI step's tool requests: the step once, each
// request on a line in words, and allowing them one by one or all at once.
export function ToolApprovalCard({items, onDone, onOpenRun}: { items: InboxItem[]; onDone: () => void; onOpenRun: () => void }) {
    const first = items[0];
    const action = useAction();
    const toast = useToast();
    const [decided, setDecided] = useState<Record<string, 'accepted' | 'declined'>>({});
    const [remember, setRemember] = useState<Record<string, boolean>>({});
    const open = items.filter((it) => !decided[it.approvalId!]);
    const decide = async (list: InboxItem[], accept: boolean) => {
        const res = await action.run(async () => {
            for (const it of list) {
                await api.decideTool(it.projectId, it.approvalId!, accept, accept && !!remember[it.approvalId!]);
                setDecided((d) => ({...d, [it.approvalId!]: accept ? 'accepted' : 'declined'}));
            }
            return true;
        });
        if (res) {
            if (list.length > 1) toast(`‘${first.stepTitle}’ 요청 ${list.length}건을 허용했습니다`);
            onDone();
        }
    };
    return (
        <article className="card stack" aria-label={`${first.projectName} ${first.stepTitle} 도구 사용 승인`}>
            <div className="row nowrap" style={{alignItems: 'flex-start'}}>
                <span className="icon-wrap warn"><Icon icon={ShieldCheck} size={18}/></span>
                <div className="stack tight" style={{flex: 1, minWidth: 0}}>
                    <div className="row">
                        <StatusBadge tone="warn">도구 사용 승인 {items.length}건</StatusBadge>
                        <span className="spacer"/>
                        <span className="muted xsmall" title={shortTime(first.since)}>{relativeTime(first.since)}</span>
                    </div>
                    <h3 style={{fontSize: 'var(--fs-lg)'}}>{first.stepTitle}</h3>
                    <div className="row small muted">
                        <span>서비스 <strong style={{color: 'var(--text)'}}>{first.projectName}</strong></span>
                        <span>· {first.runTitle} v{first.versionNumber}</span>
                        <span>· 시도 {first.generation}-{first.attempt}</span>
                        <Button size="sm" variant="ghost" icon={ExternalLink} onClick={onOpenRun}>실행 보기</Button>
                    </div>
                </div>
            </div>
            <p className="small" style={{margin: 0}}>담당 AI가 아래 작업을 하려고 허락을 기다립니다. 안전한 명령은 앱이 자동으로 허용했고, 아래는 확인이 필요한 것만 남았습니다.</p>
            <ul className="stack tight" style={{listStyle: 'none', padding: 0, margin: 0}} aria-label="요청">
                {items.map((it) => {
                    const s = toolSummary(it.tool, it.detail);
                    const done = decided[it.approvalId!];
                    return (
                        <li key={it.approvalId} className="card subtle tight tool-request" aria-label={`${s.label} ${s.target || s.command}`}>
                            <div className="tool-request-body">
                                <span><strong>{s.label}</strong>{s.target ? <> · <span className="tool-target" title={it.tool?.path}>{s.target}</span></> : null}</span>
                                {s.command ? <code className="tool-command">{s.command}</code> : null}
                                {it.tool?.risks?.length ? (
                                    <ul className="stack tight small" style={{listStyle: 'none', padding: 0, margin: 0}} aria-label="확인이 필요한 이유">
                                        {it.tool.risks.map((r) => <li key={r} className="row nowrap" style={{alignItems: 'flex-start'}}>
                                            <Icon icon={AlertTriangle} size={14}/><span>{r}</span></li>)}
                                    </ul>
                                ) : null}
                                {!done && it.tool?.rules?.length ? (
                                    <label className="check block small">
                                        <input type="checkbox" checked={!!remember[it.approvalId!]}
                                               onChange={(e) => setRemember((m) => ({...m, [it.approvalId!]: e.target.checked}))}/>
                                        이런 명령은 앞으로 이 서비스에서 항상 허용<span className="muted"> ({it.tool.rules.join(', ')})</span>
                                    </label>
                                ) : !done && it.tool?.risks?.length ? <span className="xsmall muted">이 위험은 항상 허용할 수 없어 매번 확인합니다.</span> : null}
                                <details className="xsmall muted">
                                    <summary>자세히</summary>
                                    <span className="mono" style={{wordBreak: 'break-all'}}>{it.detail}</span>
                                </details>
                            </div>
                            {done ? <StatusBadge tone={done === 'accepted' ? 'ok' : ''}>{done === 'accepted' ? '허용함' : '거절함'}</StatusBadge> : (
                                <span className="row nowrap tool-request-actions">
                                    <Button size="sm" variant="primary" icon={ShieldCheck} disabled={action.busy} aria-label={`${s.label} ${s.target || s.command} 허용`}
                                            onClick={() => decide([it], true)}>허용</Button>
                                    <Button size="sm" variant="danger" icon={ShieldX} disabled={action.busy} aria-label={`${s.label} ${s.target || s.command} 거절`}
                                            onClick={() => decide([it], false)}>거절</Button>
                                </span>
                            )}
                        </li>
                    );
                })}
            </ul>
            {open.length > 1 ? (
                <div className="form-actions">
                    <Button variant="primary" icon={CheckCheck} busy={action.busy} onClick={() => decide(open, true)}>모두 허용 ({open.length})</Button>
                </div>
            ) : null}
            {first.instructions ? (
                <details className="small">
                    <summary>업무 지침 보기</summary>
                    <p className="pre">{first.instructions}</p>
                </details>
            ) : null}
            <ErrorBox error={action.error}/>
        </article>
    );
}

// ToolRules lists the kinds of command a service lets its AI steps run
// without asking (kept from "always allow" on an approval).
export function ToolRules({project}: { project: Project }) {
    const rules = useLoad(() => api.toolRules(project.id), [project.id], project.id);
    const action = useAction();
    const toast = useToast();
    return (
        <Section title="명령 자동 허용">
            <div className="card stack">
                <p className="muted small">작업 폴더 안에서 파일 보기·테스트·빌드·로컬 서버 같은 안전한 명령은 앱이 자동으로 허용합니다.
                    관리자 권한, 작업 폴더 밖에 쓰거나 지우기, 열쇠·비밀번호 파일 접근은 항상 묻습니다.
                    아래는 승인할 때 ‘항상 허용’으로 남긴 규칙입니다.</p>
                {rules.error ? <ErrorBox error={rules.error}/> : !rules.data ? <Loading rows={1}/> : rules.data.length === 0
                    ? <p className="small muted">아직 규칙이 없습니다.</p>
                    : (
                        <ul className="stack tight" style={{listStyle: 'none', padding: 0, margin: 0}} aria-label="자동 허용 규칙">
                            {rules.data.map((r) => (
                                <li key={r.id} className="row small">
                                    <code>{r.key}</code><span className="muted">{relativeTime(r.createdAt)}</span><span className="spacer"/>
                                    <Button size="sm" variant="ghost" icon={Trash2} busy={action.busy} aria-label={`${r.key} 규칙 지우기`}
                                            onClick={() => action.run(async () => { await api.deleteToolRule(project.id, r.id); toast(`‘${r.key}’ 규칙을 지웠습니다`); rules.reload(); })}>지우기</Button>
                                </li>
                            ))}
                        </ul>
                    )}
                <ErrorBox error={action.error}/>
            </div>
        </Section>
    );
}
