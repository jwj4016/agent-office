import {useEffect, useState} from 'react';
import {api} from '../api';
import {t} from '../i18n';
import {useLoad} from '../live';
import type {Role} from '../types';
import {Building2, CornerDownRight, Network, Pencil, Plus, UserCog} from 'lucide-react';
import {Button, ConfirmButton, DetailHeader, EmptyState, ErrorBox, Field, Icon, PageHeader, Section, useAction, useDetail, useToast} from '../ui';

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
type RoleForm = typeof emptyRole;

export function Organization() {
    const org = useLoad(() => api.organization(), []);
    const roles = useLoad(() => api.roles(), []);
    const [company, setCompany] = useState({name: '', instructions: ''});
    useEffect(() => { if (org.data) setCompany({name: org.data.name, instructions: org.data.instructions}); }, [org.data]);
    const action = useAction();
    const toast = useToast();
    const setDetail = useDetail();
    const dirty = !!org.data && (company.name !== org.data.name || company.instructions !== org.data.instructions);

    const edit = (r: RoleForm) => setDetail(<RoleEditor key={r.id || 'new'} initial={r} roles={roles.data ?? []}
                                                       onDone={() => { setDetail(null); roles.reload(); }}/>);

    const tree = roleTree(roles.data ?? []);
    return (
        <div className="page narrow" style={{maxWidth: 880}}>
            <PageHeader title={t.nav.organization} desc="역할은 책임과 지침입니다. 서비스마다 이 역할을 AI 또는 사람에게 맡깁니다."/>
            <Section title={<><Icon icon={Building2} size={18}/>회사</>} label="회사">
                <form className="card stack" onSubmit={(e) => {
                    e.preventDefault();
                    action.run(async () => { await api.updateOrganization(company.name, company.instructions); org.reload(); toast('회사 정보를 저장했습니다'); });
                }}>
                    <Field label="회사 이름"><input value={company.name} onChange={(e) => setCompany({...company, name: e.target.value})}/></Field>
                    <Field label="회사 지침" optional hint="모든 역할에 가장 먼저 적용됩니다.">
                        <textarea value={company.instructions} onChange={(e) => setCompany({...company, instructions: e.target.value})}/></Field>
                    <div className="form-actions">
                        <Button variant="primary" type="submit" busy={action.busy} disabled={!dirty}>{t.common.save}</Button>
                        {dirty ? <span className="save-state dirty">저장하지 않은 변경이 있습니다</span> : null}
                    </div>
                </form>
            </Section>
            <Section title={<><Icon icon={Network} size={18}/>역할</>} count={tree.length} label="역할"
                     actions={<Button icon={Plus} onClick={() => edit({...emptyRole})}>역할 추가</Button>}>
                <ErrorBox error={roles.error || action.error}/>
                {roles.data && tree.length === 0 ? (
                    <EmptyState compact icon={Network} title="아직 역할이 없습니다"
                                actions={<Button variant="primary" icon={Plus} onClick={() => edit({...emptyRole})}>역할 추가</Button>}>
                        예: 기획자, CTO, 개발자, 법률 검토
                    </EmptyState>
                ) : null}
                {tree.length ? (
                    <ul className="attention" style={{listStyle: 'none', padding: 0, margin: 0}}>
                        {tree.map(({role, depth}) => (
                            <li key={role.id} className="attention-item" style={{paddingLeft: 16 + depth * 24}}>
                                {depth > 0 ? <Icon icon={CornerDownRight} size={14} className="muted"/> : null}
                                <span className={`icon-wrap ${depth === 0 ? 'info' : ''}`}><Icon icon={UserCog} size={16}/></span>
                                <div className="body">
                                    <span className="title">{role.name}</span>
                                    {role.mission ? <span className="meta">{role.mission}</span> : null}
                                </div>
                                <span className="row nowrap">
                                    <Button size="sm" icon={Pencil} onClick={() => edit({
                                        id: role.id, parentRoleId: role.parentRoleId, name: role.name, mission: role.mission, instructions: role.instructions,
                                    })}>{t.common.edit}</Button>
                                    <ConfirmButton label={t.common.delete} confirmLabel="삭제하기" className="btn small danger"
                                                   onConfirm={() => action.run(async () => { await api.deleteRole(role.id); roles.reload(); toast(`‘${role.name}’ 역할을 삭제했습니다`); })}/>
                                </span>
                            </li>
                        ))}
                    </ul>
                ) : null}
            </Section>
        </div>
    );
}

// RoleEditor edits one role in the detail pane.
function RoleEditor({initial, roles, onDone}: { initial: RoleForm; roles: Role[]; onDone: () => void }) {
    const [editing, setEditing] = useState<RoleForm>(initial);
    const action = useAction();
    const toast = useToast();
    const setDetail = useDetail();
    const tree = roleTree(roles);
    const save = async () => {
        const saved = await action.run(() => api.saveRole(editing));
        if (saved) {
            toast(`‘${editing.name}’ 역할을 저장했습니다`);
            onDone();
        }
    };
    return (
        <form className="stack" aria-label="역할 편집" onSubmit={(e) => { e.preventDefault(); save(); }}>
            <DetailHeader title={editing.id ? '역할 편집' : '새 역할'} onClose={() => setDetail(null)}/>
            <Field label="이름"><input value={editing.name} autoFocus onChange={(e) => setEditing({...editing, name: e.target.value})}/></Field>
            <Field label="상위 역할" hint="상위 역할의 지침이 먼저 적용되고, 보고 관계가 됩니다.">
                <select value={editing.parentRoleId} onChange={(e) => setEditing({...editing, parentRoleId: e.target.value})}>
                    <option value="">없음 (최상위)</option>
                    {tree.filter(({role}) => role.id !== editing.id).map(({role, depth}) => (
                        <option key={role.id} value={role.id}>{'\u00a0'.repeat(depth * 3)}{role.name}</option>
                    ))}
                </select></Field>
            <Field label="책임" optional hint="한 줄로 이 역할이 맡는 일"><input value={editing.mission} onChange={(e) => setEditing({...editing, mission: e.target.value})}/></Field>
            <Field label="지침" optional><textarea value={editing.instructions} rows={8} onChange={(e) => setEditing({...editing, instructions: e.target.value})}/></Field>
            <ErrorBox error={action.error}/>
            <div className="form-actions">
                <Button variant="primary" type="submit" busy={action.busy} disabled={!editing.name.trim()}>{t.common.save}</Button>
                <Button onClick={() => setDetail(null)}>{t.common.cancel}</Button>
            </div>
        </form>
    );
}
