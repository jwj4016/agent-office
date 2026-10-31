import {fireEvent, render, screen, waitFor, within} from '@testing-library/react';
import {beforeEach, describe, expect, it, vi} from 'vitest';
import {fakeApi} from '../test/fakeApi';

vi.mock('../api', () => ({api: fakeApi, errorText: (e: unknown) => String(e)}));

import {LiveProvider} from '../live';
import type {Project} from '../types';
import {ServiceDeskSettings} from './IntakeDesk';

const project: Project = {
    id: 'prj-1', organizationId: 'org-default', name: '빌라노트', goal: '', instructions: '', workspacePath: '', budget: {},
    mode: 'review', status: 'active', createdAt: '', updatedAt: '',
};
const noDesk = {desk: {projectId: 'prj-1', name: '', assignmentId: '', connectionId: '', model: '', instructions: '', budget: {}, enabled: true},
    exists: false, suggestedConnectionId: 'conn-claude', suggestedModel: ''};

beforeEach(() => {
    vi.clearAllMocks();
    fakeApi.connections.mockResolvedValue([{id: 'conn-claude', name: 'Claude', usable: true}]);
    fakeApi.assignments.mockResolvedValue([
        {id: 'a-plan', projectId: 'prj-1', roleId: 'r', actorKind: 'ai', actorId: '', displayName: '기획 담당', connectionId: 'conn-claude', model: '', overrides: {}},
        {id: 'a-desk', projectId: 'prj-1', roleId: 'r-desk', actorKind: 'ai', actorId: '', displayName: '빌라노트 접수 담당', connectionId: 'conn-claude', model: '',
            overrides: {instructions: '합성 자료 단계임을 먼저 밝힌다'}},
    ]);
});

describe('ServiceDeskSettings', () => {
    it('lets the AI staff the desk, with the suggested connection named', async () => {
        fakeApi.deskView.mockResolvedValue(noDesk);
        render(<LiveProvider><ServiceDeskSettings project={project}/></LiveProvider>);
        expect(await screen.findByText(/가장 많이 쓰는 ‘Claude’/)).toBeInTheDocument();
        fireEvent.click(screen.getAllByRole('button', {name: 'AI에게 맡기기'}).at(-1)!);
        await waitFor(() => expect(fakeApi.staffDesk).toHaveBeenCalledWith('prj-1'));
    });

    it('creates a new desk assignee by hand, or picks an existing one', async () => {
        fakeApi.deskView.mockResolvedValue(noDesk);
        render(<LiveProvider><ServiceDeskSettings project={project}/></LiveProvider>);
        fireEvent.click(await screen.findByRole('button', {name: '새로 만들기'}));
        fireEvent.change(screen.getByLabelText(/담당자 지침/), {target: {value: '법률 판단은 대표에게'}});
        fireEvent.click(screen.getByRole('button', {name: '만들기'}));
        await waitFor(() => expect(fakeApi.createDeskAssignee).toHaveBeenCalledWith(
            {projectId: 'prj-1', name: '빌라노트 접수 담당', connectionId: 'conn-claude', model: '', instructions: '법률 판단은 대표에게'}));
        fireEvent.click(screen.getByRole('button', {name: '기존 인원 중 지정'}));
        fireEvent.change(screen.getByLabelText('접수 담당자'), {target: {value: 'a-plan'}});
        fireEvent.click(screen.getByRole('button', {name: '저장'}));
        await waitFor(() => expect(fakeApi.saveDesk).toHaveBeenCalledWith(expect.objectContaining({projectId: 'prj-1', assignmentId: 'a-plan', enabled: true})));
    });

    it('shows the current desk, how it was set up, and rewrites its instructions', async () => {
        fakeApi.deskView.mockResolvedValue({...noDesk, exists: true, desk: {...noDesk.desk, assignmentId: 'a-desk', name: '빌라노트 접수 담당'},
            assignee: {id: 'a-desk', projectId: 'prj-1', roleId: 'r-desk', actorKind: 'ai', actorId: '', displayName: '빌라노트 접수 담당',
                connectionId: 'conn-claude', model: '', overrides: {instructions: '합성 자료 단계임을 먼저 밝힌다'}},
            lastStaffing: {id: 's', projectId: 'prj-1', requestId: 'req-1', kind: 'create', connectionId: 'conn-claude', model: '', status: 'succeeded',
                result: {name: '빌라노트 접수 담당'}, error: '', startedAt: '2026-10-31T00:00:00Z', endedAt: '2026-10-31T00:00:10Z'}});
        render(<LiveProvider><ServiceDeskSettings project={project}/></LiveProvider>);
        const current = await screen.findByRole('group', {name: '지금 접수 담당'});
        expect(current).toHaveTextContent('빌라노트 접수 담당');
        expect(current).toHaveTextContent('요청을 받으며 자동으로');
        expect(current).toHaveTextContent('합성 자료 단계임을 먼저 밝힌다');
        fireEvent.click(within(current).getByRole('button', {name: 'AI로 지침 다시 쓰기'}));
        await waitFor(() => expect(fakeApi.rewriteDesk).toHaveBeenCalledWith('prj-1'));
    });
});
