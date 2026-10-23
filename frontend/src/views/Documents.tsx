import {Archive, BadgeCheck, FileText, FolderUp, Link2, Link2Off, RefreshCw, Share2, X} from 'lucide-react';
import {useState} from 'react';
import {api} from '../api';
import {useLoad} from '../live';
import type {
    Department, DocumentVersion, ImpactCheck, NewDocumentVersion, Project, SharedDocument, WorkItem,
} from '../types';
import {
    Banner, Button, DetailHeader, EmptyState, ErrorBox, Field, Icon, Loading, Section, StatusBadge, shortTime, useAction, useDetail, useToast,
} from '../ui';

const stageLabel = (v: DocumentVersion) => (v.stage === 'final' ? '확정' : '초안');
const latest = (d: SharedDocument) => [...d.versions].reverse().find((v) => !v.withdrawnAt);
const kb = (n: number) => (n < 1024 ? `${n}B` : `${(n / 1024).toFixed(1)}KB`);

export function VersionBadge({v}: { v: DocumentVersion }) {
    if (v.withdrawnAt) return <StatusBadge tone="bad">{v.number}판 철회</StatusBadge>;
    return <StatusBadge tone={v.stage === 'final' ? 'ok' : 'info'}>{v.number}판 {stageLabel(v)}</StatusBadge>;
}

// VersionContent shows one shared copy's text and whether it is intact.
function VersionContent({project, versionId, onClose}: { project: Project; versionId: string; onClose: () => void }) {
    const c = useLoad(() => api.readDocumentVersion(project.id, versionId), [project.id, versionId]);
    if (c.error) return <ErrorBox error={c.error}/>;
    if (!c.data) return <Loading/>;
    return (
        <div className="card stack tight" aria-label={`${c.data.title} ${c.data.number}판 내용`}>
            <div className="row between nowrap">
                <strong className="small">{c.data.fileName} · {kb(c.data.size)}</strong>
                <Button size="sm" variant="ghost" iconOnly icon={X} aria-label="내용 닫기" onClick={onClose}/>
            </div>
            {!c.data.hashOk ? <Banner tone="bad">사본의 해시가 기록과 다릅니다. 파일이 바뀌었을 수 있습니다.</Banner> : null}
            {c.data.binary ? <p className="muted small">텍스트가 아닌 파일입니다.</p> : <div className="pre small">{c.data.content}</div>}
            {c.data.truncated ? <p className="muted xsmall">앞부분만 보여 줍니다.</p> : null}
        </div>
    );
}

// Documents is the service's shared library: material any department
// shared, by version, with its stage and who may see it.
export function Documents({project}: { project: Project }) {
    const departments = useLoad(() => api.departments(project.id), [project.id], project.id);
    const categories = useLoad(() => api.categories(project.id), [project.id], project.id);
    if (departments.error || categories.error) return <ErrorBox error={departments.error || categories.error}/>;
    if (!departments.data || !categories.data) return <Loading/>;
    return <Library project={project} departments={departments.data} categories={categories.data}/>;
}

function Library({project, departments, categories}: { project: Project; departments: Department[]; categories: string[] }) {
    const [query, setQuery] = useState('');
    const [category, setCategory] = useState('');
    const [visibleTo, setVisibleTo] = useState('');
    const [withdrawn, setWithdrawn] = useState(false);
    const list = useLoad(() => api.documents(project.id, {query, category, visibleTo, includeWithdrawn: withdrawn}),
        [project.id, query, category, visibleTo, withdrawn], project.id);
    const setDetail = useDetail();
    const archived = project.status === 'archived';
    const deptName = (id: string) => departments.find((d) => d.id === id)?.name ?? '';
    const open = (d: SharedDocument) => setDetail(<DocumentDetail key={d.id} project={project} documentId={d.id} departments={departments}/>);
    return (
        <div className="stack loose">
            <p className="muted">부서가 공유한 초안과 결과입니다. 공유한 사본은 바뀌지 않고, 새 내용은 새 판으로 올라갑니다. 업무를 끝내지 않아도 공유할 수 있습니다.</p>
            {project.workspacePath ? (
                <ShareFileForm project={project} departments={departments} categories={categories} disabled={archived} onShared={list.reload}/>
            ) : <Banner tone="info">작업 폴더가 없는 서비스입니다. 업무 실행 결과에서 ‘공유’를 눌러 자료를 올리세요.</Banner>}
            <Section title={<><Icon icon={FileText} size={18}/>공유 자료</>} label="공유 자료" count={list.data?.length}>
                <div className="toolbar" role="group" aria-label="자료 조회 조건">
                    <input value={query} placeholder="제목·파일 이름 검색" aria-label="자료 검색" onChange={(e) => setQuery(e.target.value)}/>
                    <label className="row nowrap small">분류
                        <select value={category} onChange={(e) => setCategory(e.target.value)}>
                            <option value="">전체</option>
                            {categories.map((c) => <option key={c} value={c}>{c}</option>)}
                        </select></label>
                    <label className="row nowrap small">볼 수 있는 부서
                        <select value={visibleTo} onChange={(e) => setVisibleTo(e.target.value)}>
                            <option value="">모든 부서</option>
                            {departments.map((d) => <option key={d.id} value={d.id}>{d.name}</option>)}
                        </select></label>
                    <label className="check small"><input type="checkbox" checked={withdrawn} onChange={(e) => setWithdrawn(e.target.checked)}/>철회된 자료 포함</label>
                </div>
                <ErrorBox error={list.error}/>
                {!list.data ? <Loading/> : list.data.length === 0 ? (
                    <EmptyState compact icon={FileText} title="공유된 자료가 없습니다">업무 실행 결과나 작업 폴더의 파일을 공유하면 여기에 모입니다.</EmptyState>
                ) : (
                    <div className="table-wrap">
                        <table className="list">
                            <thead><tr><th>자료</th><th>작성 부서</th><th>최신 판</th><th>공유 범위</th><th>바뀐 때</th></tr></thead>
                            <tbody>
                            {list.data.map((d) => {
                                const v = latest(d);
                                return (
                                    <tr key={d.id}>
                                        <td><button className="btn ghost small" onClick={() => open(d)} aria-label={`${d.title} 자료 보기`}><strong>{d.title}</strong></button>
                                            {d.categories.length ? <div className="muted xsmall">{d.categories.join(', ')}</div> : null}</td>
                                        <td className="small">{deptName(d.ownerDepartmentId) || <span className="muted">—</span>}</td>
                                        <td>{d.status === 'withdrawn' ? <StatusBadge tone="bad">철회됨</StatusBadge> : v ? <VersionBadge v={v}/> : null}</td>
                                        <td className="small">{d.scope === 'service' ? '서비스 전체' : d.scopeDepartments.map(deptName).join(', ')}</td>
                                        <td className="small nowrap">{shortTime(d.updatedAt)}</td>
                                    </tr>
                                );
                            })}
                            </tbody>
                        </table>
                    </div>
                )}
            </Section>
        </div>
    );
}

// DocumentFields are the fields of a new shared document.
function DocumentFields({value, onChange, departments, categories, ownerLocked}: {
    value: NewDocumentVersion; onChange: (v: NewDocumentVersion) => void; departments: Department[]; categories: string[]; ownerLocked?: boolean;
}) {
    const set = (patch: Partial<NewDocumentVersion>) => onChange({...value, ...patch});
    const cats = value.categories ?? [];
    const scopeDepts = value.scopeDepartments ?? [];
    return (
        <>
            <Field label="자료 제목"><input value={value.title ?? ''} onChange={(e) => set({title: e.target.value})}/></Field>
            {!ownerLocked ? (
                <Field label="작성 부서" optional>
                    <select value={value.ownerDepartmentId ?? ''} onChange={(e) => set({ownerDepartmentId: e.target.value})}>
                        <option value="">지정 안 함</option>
                        {departments.map((d) => <option key={d.id} value={d.id}>{d.name}</option>)}
                    </select></Field>
            ) : null}
            <fieldset className="stack tight">
                <legend className="field-label">분류 <span className="optional">(선택)</span></legend>
                <div className="row">
                    {categories.map((c) => (
                        <label key={c} className="check"><input type="checkbox" checked={cats.includes(c)}
                                                               onChange={(e) => set({categories: e.target.checked ? [...cats, c] : cats.filter((x) => x !== c)})}/>{c}</label>
                    ))}
                </div>
            </fieldset>
            <fieldset className="stack tight">
                <legend className="field-label">공유 범위</legend>
                <label className="check"><input type="radio" name="scope" checked={(value.scope ?? 'service') === 'service'}
                                                onChange={() => set({scope: 'service'})}/>서비스 전체</label>
                <label className="check"><input type="radio" name="scope" checked={value.scope === 'departments'}
                                                onChange={() => set({scope: 'departments'})}/>지정한 부서만</label>
                {value.scope === 'departments' ? (
                    <div className="row">
                        {departments.map((d) => (
                            <label key={d.id} className="check"><input type="checkbox" checked={scopeDepts.includes(d.id)}
                                                                       onChange={(e) => set({scopeDepartments: e.target.checked ? [...scopeDepts, d.id] : scopeDepts.filter((x) => x !== d.id)})}/>{d.name}</label>
                        ))}
                    </div>
                ) : null}
            </fieldset>
        </>
    );
}

// ShareFileForm shares a file of the service's work folder.
function ShareFileForm({project, departments, categories, disabled, onShared}: {
    project: Project; departments: Department[]; categories: string[]; disabled: boolean; onShared: () => void;
}) {
    const [open, setOpen] = useState(false);
    const [path, setPath] = useState('');
    const [doc, setDoc] = useState<NewDocumentVersion>({scope: 'service'});
    const action = useAction();
    const toast = useToast();
    if (!open) return <div><Button icon={FolderUp} disabled={disabled} onClick={() => setOpen(true)}>작업 폴더 파일 공유</Button></div>;
    const share = () => action.run(async () => {
        const v = await api.shareWorkspaceFile(project.id, path, doc);
        toast(`${v.fileName}을(를) ${v.number}판으로 공유했습니다`);
        setOpen(false); setPath(''); setDoc({scope: 'service'});
        onShared();
    });
    return (
        <form className="card stack" aria-label="작업 폴더 파일 공유" onSubmit={(e) => { e.preventDefault(); share(); }}>
            <Field label="파일 경로" hint={`작업 폴더(${project.workspacePath}) 안의 경로. 저장이 끝난 파일만 공유하세요.`}>
                <input value={path} placeholder="docs/plan.md" onChange={(e) => setPath(e.target.value)}/></Field>
            <DocumentFields value={doc} onChange={setDoc} departments={departments} categories={categories}/>
            <ErrorBox error={action.error}/>
            <div className="form-actions">
                <Button type="submit" variant="primary" icon={Share2} busy={action.busy} disabled={!path.trim() || !doc.title?.trim()}>공유</Button>
                <Button onClick={() => setOpen(false)}>취소</Button>
            </div>
        </form>
    );
}

// DocumentDetail shows a document's versions: read, confirm, withdraw,
// and upload a new version from the work folder.
function DocumentDetail({project, documentId, departments}: { project: Project; documentId: string; departments: Department[] }) {
    const d = useLoad(() => api.document(project.id, documentId), [project.id, documentId], project.id);
    const setDetail = useDetail();
    const action = useAction();
    const toast = useToast();
    const [reading, setReading] = useState<string | null>(null);
    const [reason, setReason] = useState('');
    const [path, setPath] = useState('');
    const archived = project.status === 'archived';
    if (d.error) return <><DetailHeader title="자료" onClose={() => setDetail(null)}/><ErrorBox error={d.error}/></>;
    if (!d.data) return <Loading/>;
    const doc = d.data;
    const active = doc.status === 'active' && !archived;
    const deptName = (id: string) => departments.find((x) => x.id === id)?.name ?? '';
    const run = (fn: () => Promise<unknown>, done: string) => action.run(async () => { await fn(); d.reload(); toast(done); });
    return (
        <div className="stack">
            <DetailHeader title={doc.title} onClose={() => setDetail(null)}>
                <span className="row small">
                    {doc.status === 'withdrawn' ? <StatusBadge tone="bad">철회됨</StatusBadge> : null}
                    <span className="muted">{deptName(doc.ownerDepartmentId) || '작성 부서 없음'} · {doc.scope === 'service' ? '서비스 전체 공유' : `${doc.scopeDepartments.map(deptName).join(', ')}에만 공유`}</span>
                </span>
            </DetailHeader>
            <ErrorBox error={action.error}/>
            <ol className="stack tight" aria-label="판 목록" style={{listStyle: 'none', padding: 0, margin: 0}}>
                {[...doc.versions].reverse().map((v) => (
                    <li key={v.id} className="card stack tight">
                        <div className="row between">
                            <span className="row nowrap"><VersionBadge v={v}/><span className="muted xsmall">{v.fileName} · {kb(v.size)} · {shortTime(v.createdAt)}</span></span>
                            <span className="row nowrap">
                                <Button size="sm" variant="ghost" icon={FileText} onClick={() => setReading(reading === v.id ? null : v.id)}>내용</Button>
                                {active && !v.withdrawnAt && v.stage === 'draft' ? (
                                    <Button size="sm" icon={BadgeCheck} busy={action.busy} aria-label={`${v.number}판 확정`}
                                            onClick={() => run(() => api.confirmDocumentVersion(project.id, v.id, ''), `${v.number}판을 확정했습니다`)}>확정</Button>
                                ) : null}
                            </span>
                        </div>
                        <span className="muted xsmall">출처: {v.sourceKind === 'artifact' ? '업무 실행 결과' : `작업 폴더 ${v.sourcePath}`}
                            {v.confirmedAt ? ` · ${shortTime(v.confirmedAt)} 확정${v.confirmationNote ? ` (${v.confirmationNote})` : ''}` : ''}</span>
                        {reading === v.id ? <VersionContent project={project} versionId={v.id} onClose={() => setReading(null)}/> : null}
                    </li>
                ))}
            </ol>
            {active && project.workspacePath ? (
                <form className="row nowrap" aria-label="새 판 올리기" onSubmit={(e) => {
                    e.preventDefault();
                    run(() => api.shareWorkspaceFile(project.id, path, {documentId: doc.id}), '새 판을 올렸습니다');
                    setPath('');
                }}>
                    <input value={path} placeholder="작업 폴더 안의 파일 경로" aria-label="새 판 파일 경로" onChange={(e) => setPath(e.target.value)}/>
                    <Button type="submit" size="sm" icon={RefreshCw} disabled={!path.trim()} busy={action.busy}>새 판 올리기</Button>
                </form>
            ) : null}
            {active ? (
                <form className="stack tight" aria-label="자료 철회" onSubmit={(e) => {
                    e.preventDefault();
                    run(() => api.withdrawDocument(project.id, doc.id, '', reason), '자료를 철회했습니다');
                }}>
                    <input value={reason} placeholder="철회 이유 (이 자료를 쓴 업무에 전달됩니다)" aria-label="철회 이유" onChange={(e) => setReason(e.target.value)}/>
                    <div><Button type="submit" size="sm" icon={Archive} className="btn small danger" disabled={!reason.trim()} busy={action.busy}>자료 철회</Button></div>
                </form>
            ) : null}
        </div>
    );
}

// ShareResult shares one run result, as a new document or a new version
// of one this work already shared.
export function ShareResult({project, artifactId, item, departments, categories, onDone}: {
    project: Project; artifactId: string; item: WorkItem; departments: Department[]; categories: string[]; onDone: () => void;
}) {
    const mine = useLoad(() => api.documents(project.id, {}), [project.id], project.id);
    const [target, setTarget] = useState('');
    const [doc, setDoc] = useState<NewDocumentVersion>({title: item.title, categories: item.categories, scope: 'service'});
    const action = useAction();
    const toast = useToast();
    const own = (mine.data ?? []).filter((d) => d.workItemId === item.id && d.status === 'active');
    const share = () => action.run(async () => {
        const v = await api.shareArtifact(project.id, artifactId, target ? {documentId: target} : {...doc, workItemId: item.id});
        toast(`${v.number}판으로 공유했습니다`);
        onDone();
    });
    return (
        <form className="card stack" aria-label="결과 공유" onSubmit={(e) => { e.preventDefault(); share(); }}>
            <strong className="small">이 결과를 공유 자료로 올리기</strong>
            {own.length ? (
                <Field label="올릴 곳">
                    <select value={target} onChange={(e) => setTarget(e.target.value)}>
                        <option value="">새 자료</option>
                        {own.map((d) => <option key={d.id} value={d.id}>{d.title}의 새 판</option>)}
                    </select></Field>
            ) : null}
            {!target ? <DocumentFields value={doc} onChange={setDoc} departments={departments} categories={categories} ownerLocked/> : null}
            <p className="muted xsmall">업무는 그대로 진행 중입니다. 초안으로 공유되며, 확정은 자료함에서 따로 합니다.</p>
            <ErrorBox error={action.error}/>
            <div className="form-actions">
                <Button type="submit" variant="primary" icon={Share2} busy={action.busy} disabled={!target && !doc.title?.trim()}>공유</Button>
                <Button onClick={onDone}>취소</Button>
            </div>
        </form>
    );
}

// WorkDocuments is what a work item reads: linked shared versions (the
// next run is given them), changes to check, and linking more.
export function WorkDocuments({project, item}: { project: Project; item: WorkItem }) {
    const links = useLoad(() => api.workDocuments(project.id, item.id), [project.id, item.id], project.id);
    const checks = useLoad(() => api.impactChecks(project.id, item.id, 'open'), [project.id, item.id], project.id);
    const visible = useLoad(() => api.documents(project.id, {visibleTo: item.departmentId}), [project.id, item.departmentId], project.id);
    const [pick, setPick] = useState('');
    const action = useAction();
    const archived = project.status === 'archived';
    if (links.error || checks.error) return <ErrorBox error={links.error || checks.error}/>;
    if (!links.data || !checks.data) return <Loading/>;
    const linked = new Set(links.data.map((l) => l.document.id));
    const candidates = (visible.data ?? []).filter((d) => !linked.has(d.id) && latest(d));
    const close = (c: ImpactCheck, status: 'resolved' | 'dismissed') =>
        action.run(async () => { await api.closeImpactCheck(project.id, c.id, status, ''); checks.reload(); });
    return (
        <Section title="참고 공유 자료" count={links.data.length}>
            {checks.data.map((c) => (
                <Banner key={c.id} tone="warn" title="사용한 자료가 바뀌었습니다"
                        actions={<span className="row nowrap">
                            <Button size="sm" busy={action.busy} onClick={() => close(c, 'resolved')}>확인함</Button>
                            <Button size="sm" variant="ghost" busy={action.busy} onClick={() => close(c, 'dismissed')}>무시</Button>
                        </span>}>{c.summary}</Banner>
            ))}
            {links.data.length === 0 ? <p className="muted small">연결한 자료가 없습니다. 연결한 자료는 다음 실행에 함께 전달되고 그 판으로 고정됩니다.</p> : (
                <ul className="stack tight" aria-label="연결한 자료" style={{listStyle: 'none', padding: 0, margin: 0}}>
                    {links.data.map((l) => (
                        <li key={l.version.id} className="row between nowrap">
                            <span className="row nowrap small"><Icon icon={Link2} size={14}/><strong>{l.document.title}</strong><VersionBadge v={l.version}/>
                                {l.newer ? <StatusBadge tone="warn">{l.newer}판 있음</StatusBadge> : null}</span>
                            <span className="row nowrap">
                                {l.newer ? (
                                    <Button size="sm" disabled={archived} busy={action.busy}
                                            onClick={() => action.run(async () => {
                                                const d = await api.document(project.id, l.document.id);
                                                const v = latest(d);
                                                if (v) await api.linkDocument(project.id, item.id, v.id);
                                                links.reload();
                                            })}>{l.newer}판으로 바꾸기</Button>
                                ) : null}
                                <Button size="sm" variant="ghost" iconOnly icon={Link2Off} aria-label={`${l.document.title} 연결 해제`} disabled={archived}
                                        onClick={() => action.run(async () => { await api.unlinkDocument(project.id, item.id, l.version.id); links.reload(); })}/>
                            </span>
                        </li>
                    ))}
                </ul>
            )}
            <form className="row nowrap" aria-label="자료 연결" onSubmit={(e) => {
                e.preventDefault();
                const d = candidates.find((x) => x.id === pick);
                const v = d && latest(d);
                if (v) action.run(async () => { await api.linkDocument(project.id, item.id, v.id); setPick(''); links.reload(); });
            }}>
                <select value={pick} disabled={archived} aria-label="연결할 자료" onChange={(e) => setPick(e.target.value)}>
                    <option value="">연결할 자료 선택 (최신 판)</option>
                    {candidates.map((d) => <option key={d.id} value={d.id}>{d.title} · {latest(d)?.number}판 {latest(d) ? stageLabel(latest(d)!) : ''}</option>)}
                </select>
                <Button type="submit" size="sm" icon={Link2} disabled={archived || !pick} busy={action.busy}>연결</Button>
            </form>
            <ErrorBox error={action.error}/>
        </Section>
    );
}
