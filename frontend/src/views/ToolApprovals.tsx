import {CheckCheck, ExternalLink, ShieldCheck, ShieldX} from 'lucide-react';
import {useState} from 'react';
import {api} from '../api';
import type {InboxItem, ToolRequest} from '../types';
import {Button, ErrorBox, Icon, relativeTime, shortTime, StatusBadge, useAction, useToast} from '../ui';

// What each tool does, in words (Claude's tools and Codex's requests).
const toolLabel: Record<string, string> = {
    Read: '파일 읽기', Write: '파일 쓰기', Edit: '파일 수정', MultiEdit: '파일 수정', NotebookEdit: '노트북 수정',
    Bash: '명령 실행', Glob: '파일 찾기', Grep: '내용 검색', LS: '폴더 보기', WebFetch: '웹 페이지 읽기', WebSearch: '웹 검색',
    command: '명령 실행', file_change: '파일 변경',
};

const baseName = (path: string) => path.split(/[\\/]/).pop() || path;

// toolSummary says what a tool approval asks: what it does and on what.
export function toolSummary(t: ToolRequest | undefined, detail = '') {
    if (!t) return {label: '도구 사용', target: detail};
    const label = toolLabel[t.name] ?? t.name;
    if (t.path) return {label, target: t.input ? `${t.input} (${baseName(t.path)})` : baseName(t.path)};
    return {label, target: t.command ?? ''};
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
    const open = items.filter((it) => !decided[it.approvalId!]);
    const decide = async (list: InboxItem[], accept: boolean) => {
        const res = await action.run(async () => {
            for (const it of list) {
                await api.decideTool(it.projectId, it.approvalId!, accept);
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
            <p className="small" style={{margin: 0}}>담당 AI가 아래 작업을 하려고 허락을 기다립니다.</p>
            <ul className="stack tight" style={{listStyle: 'none', padding: 0, margin: 0}} aria-label="요청">
                {items.map((it) => {
                    const s = toolSummary(it.tool, it.detail);
                    const done = decided[it.approvalId!];
                    return (
                        <li key={it.approvalId} className="card subtle tight row nowrap" aria-label={`${s.label} ${s.target}`}>
                            <div className="stack tight" style={{flex: 1, minWidth: 0}}>
                                <span><strong>{s.label}</strong>{s.target ? <> · <span className="truncate" title={it.tool?.path || it.tool?.command || it.detail}>{s.target}</span></> : null}</span>
                                <details className="xsmall muted">
                                    <summary>자세히</summary>
                                    <span className="mono" style={{wordBreak: 'break-all'}}>{it.detail}</span>
                                </details>
                            </div>
                            {done ? <StatusBadge tone={done === 'accepted' ? 'ok' : ''}>{done === 'accepted' ? '허용함' : '거절함'}</StatusBadge> : (
                                <span className="row nowrap">
                                    <Button size="sm" variant="primary" icon={ShieldCheck} disabled={action.busy} aria-label={`${s.label} ${s.target} 허용`}
                                            onClick={() => decide([it], true)}>허용</Button>
                                    <Button size="sm" variant="danger" icon={ShieldX} disabled={action.busy} aria-label={`${s.label} ${s.target} 거절`}
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
