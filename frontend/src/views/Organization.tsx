import {useEffect, useState} from 'react';
import {api} from '../api';
import {t} from '../i18n';
import {useLoad} from '../live';
import type {Role} from '../types';
import {ConfirmButton, ErrorBox, useAction} from '../ui';

// roleTree orders roles parent-first with their depth for indentation.
export function roleTree(roles: Role[]): { role: Role; depth: number }[] {
    const byParent = new Map<string, Role[]>();
    const ids = new Set(roles.map((r) => r.id));
    for (const r of roles) {
        const parent = r.parentRoleId && ids.has(r.parentRoleId) ? r.parentRoleId : '';
        byParent.set(parent, [...(byParent.get(parent) ?? []), r]);
    }
    const out: { role: Role; depth: number }[] = [];
    const walk = (parent: string, depth: number) => {
        for (const r of (byParent.get(parent) ?? []).sort((a, b) => a.name.localeCompare(b.name, 'ko'))) {
            out.push({role: r, depth});
            walk(r.id, depth + 1);
        }
    };
    walk('', 0);
    return out;
}

const emptyRole = {id: '', parentRoleId: '', name: '', mission: '', instructions: ''};

export function Organization() {
    const org = useLoad(() => api.organization(), []);
    const roles = useLoad(() => api.roles(), []);
    const [company, setCompany] = useState({name: '', instructions: ''});
    useEffect(() => { if (org.data) setCompany({name: org.data.name, instructions: org.data.instructions}); }, [org.data]);
    const [editing, setEditing] = useState<typeof emptyRole | null>(null);
    const action = useAction();

    const saveRole = async () => {
        if (!editing) return;
        const saved = await action.run(() => api.saveRole(editing));
        if (saved) {
            setEditing(null);
            roles.reload();
        }
    };

    const tree = roleTree(roles.data ?? []);
    return (
        <div className="stack">
            <h1>{t.nav.organization}</h1>
            <p className="muted">역할은 책임과 지침입니다. 서비스마다 이 역할을 AI 또는 사람에게 맡깁니다.</p>
            <section className="card stack" aria-label="회사">
                <h2>회사</h2>
                <label className="field"><span>회사 이름</span>
                    <input value={company.name} onChange={(e) => setCompany({...company, name: e.target.value})}/></label>
                <label className="field"><span>회사 지침 (모든 역할에 가장 먼저 적용)</span>
                    <textarea value={company.instructions} onChange={(e) => setCompany({...company, instructions: e.target.value})}/></label>
                <div className="row">
                    <button className="btn primary" disabled={action.busy}
                            onClick={() => action.run(async () => { await api.updateOrganization(company.name, company.instructions); org.reload(); })}>
                        {t.common.save}</button>
                </div>
            </section>
            <section className="stack" aria-label="역할">
                <div className="row between">
                    <h2>역할</h2>
                    <button className="btn" onClick={() => setEditing({...emptyRole})}>역할 추가</button>
                </div>
                <ErrorBox error={roles.error || action.error}/>
                {tree.length === 0 ? <p className="muted small">아직 역할이 없습니다. 예: 기획자, CTO, 개발자, 법률 검토</p> : null}
                <ul className="stack" style={{listStyle: 'none', padding: 0, margin: 0}}>
                    {tree.map(({role, depth}) => (
                        <li key={role.id} className="card row between" style={{marginLeft: depth * 20}}>
                            <span>
                                <strong>{role.name}</strong>
                                {role.mission ? <span className="muted small"> · {role.mission}</span> : null}
                            </span>
                            <span className="row">
                                <button className="btn small" onClick={() => setEditing({
                                    id: role.id, parentRoleId: role.parentRoleId, name: role.name, mission: role.mission, instructions: role.instructions,
                                })}>{t.common.edit}</button>
                                <ConfirmButton label={t.common.delete} confirmLabel="삭제하기" className="btn small danger"
                                               onConfirm={() => action.run(async () => { await api.deleteRole(role.id); roles.reload(); })}/>
                            </span>
                        </li>
                    ))}
                </ul>
            </section>
            {editing ? (
                <form className="card stack" aria-label="역할 편집" onSubmit={(e) => { e.preventDefault(); saveRole(); }}>
                    <h2>{editing.id ? '역할 편집' : '새 역할'}</h2>
                    <label className="field"><span>이름</span>
                        <input value={editing.name} autoFocus onChange={(e) => setEditing({...editing, name: e.target.value})}/></label>
                    <label className="field"><span>상위 역할 (지침·보고 관계)</span>
                        <select value={editing.parentRoleId} onChange={(e) => setEditing({...editing, parentRoleId: e.target.value})}>
                            <option value="">없음 (최상위)</option>
                            {tree.filter(({role}) => role.id !== editing.id).map(({role, depth}) => (
                                <option key={role.id} value={role.id}>{' '.repeat(depth * 2)}{role.name}</option>
                            ))}
                        </select></label>
                    <label className="field"><span>책임</span>
                        <input value={editing.mission} onChange={(e) => setEditing({...editing, mission: e.target.value})}/></label>
                    <label className="field"><span>지침</span>
                        <textarea value={editing.instructions} onChange={(e) => setEditing({...editing, instructions: e.target.value})}/></label>
                    <div className="row">
                        <button className="btn primary" type="submit" disabled={action.busy || !editing.name.trim()}>{t.common.save}</button>
                        <button className="btn" type="button" onClick={() => setEditing(null)}>{t.common.cancel}</button>
                    </div>
                </form>
            ) : null}
        </div>
    );
}
