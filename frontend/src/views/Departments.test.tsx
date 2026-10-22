import {createEvent, fireEvent, render, screen, waitFor, within} from '@testing-library/react';
import {type ReactNode, useState} from 'react';
import {beforeEach, describe, expect, it, vi} from 'vitest';
import {fakeApi} from '../test/fakeApi';

vi.mock('../api', () => ({api: fakeApi, errorText: (e: unknown) => String(e)}));

import {LiveProvider} from '../live';
import type {Assignment, Department, DepartmentTemplate, Project, Role} from '../types';
import {DetailProvider, ToastProvider} from '../ui';
import {Organization} from './Organization';
import {Team, TEMPLATE_MIME} from './Team';

const project: Project = {id: 'prj-1', organizationId: 'org-default', name: '캠페인', goal: '', instructions: '', workspacePath: '',
    budget: {}, mode: 'review', status: 'active', createdAt: '', updatedAt: ''};

const role = (id: string, name: string): Role => ({
    id, name, parentRoleId: '', organizationId: 'org-default', mission: '', instructions: '', outputDefaults: [], policy: {}, appearance: {}, updatedAt: '',
});
const template = (id: string, name: string, roleIds: string[]): DepartmentTemplate => ({
    id, organizationId: 'org-default', name, responsibility: `${name}의 책임`, roleIds, updatedAt: '',
});
const dept = (id: string, name: string, memberIds: string[], leadAssignmentId = ''): Department => ({
    id, projectId: 'prj-1', name, responsibility: '', leadAssignmentId, memberIds, templateId: '', automation: {}, limits: {},
    status: 'active', createdAt: '', updatedAt: '',
});
const asg = (id: string, name: string, roleId: string): Assignment => ({
    id, projectId: 'prj-1', roleId, actorKind: 'ai', actorId: '', displayName: name, connectionId: '', model: '', overrides: {},
});

function Shell({children}: { children: ReactNode }) {
    const [detail, setDetail] = useState<ReactNode | null>(null);
    return (
        <LiveProvider>
            <ToastProvider>
                <DetailProvider value={setDetail}>
                    {children}
                    {detail ? <aside aria-label="상세">{detail}</aside> : null}
                </DetailProvider>
            </ToastProvider>
        </LiveProvider>
    );
}

beforeEach(() => {
    vi.clearAllMocks();
    fakeApi.roles.mockResolvedValue([role('r-mkt', '마케터'), role('r-dev', '개발자')]);
    fakeApi.departmentTemplates.mockResolvedValue([template('t-mkt', '마케팅팀', ['r-mkt']), template('t-dev', '개발팀', ['r-dev'])]);
    fakeApi.assignments.mockResolvedValue([asg('a-mkt', '마케팅 AI', 'r-mkt'), asg('a-dev', '개발 AI', 'r-dev')]);
    fakeApi.departments.mockResolvedValue([dept('d-dev', '개발팀', ['a-dev'], 'a-dev')]);
    fakeApi.connections.mockResolvedValue([]);
});

describe('company department templates', () => {
    it('lists templates with their roles and saves the chosen roles', async () => {
        fakeApi.saveDepartmentTemplate.mockResolvedValue(template('t-mkt', '마케팅팀', ['r-mkt', 'r-dev']));
        render(<Shell><Organization/></Shell>);
        const section = await screen.findByRole('region', {name: '부서 템플릿'});
        expect(await within(section).findByText('마케팅팀')).toBeInTheDocument();
        fireEvent.click(within(section).getByRole('button', {name: '마케팅팀 템플릿 편집'}));
        const form = await screen.findByRole('form', {name: '부서 템플릿 편집'});
        fireEvent.click(within(form).getByRole('checkbox', {name: '개발자'}));
        fireEvent.click(within(form).getByRole('button', {name: '저장'}));
        await waitFor(() => expect(fakeApi.saveDepartmentTemplate).toHaveBeenCalledWith(expect.objectContaining({
            id: 't-mkt', name: '마케팅팀', roleIds: ['r-mkt', 'r-dev'],
        })));
    });
});

describe('service departments', () => {
    it('imports a template with the button', async () => {
        fakeApi.importDepartmentTemplate.mockResolvedValue(dept('d-mkt', '마케팅팀', ['a-mkt'], 'a-mkt'));
        render(<Shell><Team project={project}/></Shell>);
        fireEvent.click(await screen.findByRole('button', {name: '마케팅팀 템플릿 가져오기'}));
        await waitFor(() => expect(fakeApi.importDepartmentTemplate).toHaveBeenCalledWith('prj-1', 't-mkt'));
    });

    it('imports a template dropped onto the department list', async () => {
        fakeApi.importDepartmentTemplate.mockResolvedValue(dept('d-mkt', '마케팅팀', []));
        render(<Shell><Team project={project}/></Shell>);
        await screen.findByRole('button', {name: '개발팀 템플릿 가져오기'});
        const zone = screen.getByLabelText(/템플릿을 끌어다 놓으면/);
        const data = {getData: (k: string) => (k === TEMPLATE_MIME ? 't-dev' : ''), types: [TEMPLATE_MIME], dropEffect: ''};
        const over = createEvent.dragOver(zone);
        Object.defineProperty(over, 'dataTransfer', {value: data});
        fireEvent(zone, over);
        expect(zone).toHaveClass('over');
        const drop = createEvent.drop(zone);
        Object.defineProperty(drop, 'dataTransfer', {value: data});
        fireEvent(zone, drop);
        await waitFor(() => expect(fakeApi.importDepartmentTemplate).toHaveBeenCalledWith('prj-1', 't-dev'));
    });

    it('moves a member from another department and picks the lead among members', async () => {
        fakeApi.saveDepartment.mockResolvedValue(dept('d-new', '기획팀', ['a-dev'], 'a-dev'));
        render(<Shell><Team project={project}/></Shell>);
        await screen.findByRole('button', {name: '개발팀 부서 편집'});
        fireEvent.click(screen.getByRole('button', {name: '부서 만들기'}));
        const form = await screen.findByRole('form', {name: '부서 편집'});
        fireEvent.change(within(form).getByLabelText('이름'), {target: {value: '기획팀'}});
        expect(within(form).getByText('(현재 개발팀)')).toBeInTheDocument();
        fireEvent.click(within(form).getByRole('checkbox', {name: /개발 AI/}));
        expect(within(form).getByText('(개발팀에서 옮겨집니다)')).toBeInTheDocument();
        fireEvent.change(within(form).getByLabelText(/대표 담당자/), {target: {value: 'a-dev'}});
        fireEvent.click(within(form).getByRole('button', {name: '저장'}));
        await waitFor(() => expect(fakeApi.saveDepartment).toHaveBeenCalledWith(expect.objectContaining({
            projectId: 'prj-1', name: '기획팀', memberIds: ['a-dev'], leadAssignmentId: 'a-dev',
        })));
    });
});
