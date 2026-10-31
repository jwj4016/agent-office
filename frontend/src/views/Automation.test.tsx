import {fireEvent, render, screen, waitFor} from '@testing-library/react';
import {type ReactNode} from 'react';
import {beforeEach, describe, expect, it, vi} from 'vitest';
import {fakeApi} from '../test/fakeApi';

vi.mock('../api', () => ({api: fakeApi, errorText: (e: unknown) => String(e)}));

import {LiveProvider} from '../live';
import type {Department, Project} from '../types';
import {DetailProvider, ToastProvider} from '../ui';
import {AutomationPanel, rulesOf} from './Automation';

const project: Project = {id: 'prj-1', organizationId: 'org-default', name: '캠페인', goal: '', instructions: '', workspacePath: '',
    budget: {}, mode: 'review', status: 'active', createdAt: '', updatedAt: ''};
const dev: Department = {id: 'dev', projectId: 'prj-1', name: '개발팀', responsibility: '', leadAssignmentId: 'a1', memberIds: ['a1'], templateId: '',
    automation: {}, limits: {}, status: 'active', createdAt: '', updatedAt: ''};

const Shell = ({children}: { children: ReactNode }) => (
    <LiveProvider><ToastProvider><DetailProvider value={() => {}}>{children}</DetailProvider></ToastProvider></LiveProvider>
);

beforeEach(() => {
    vi.clearAllMocks();
    fakeApi.categories.mockResolvedValue(['개발', '비용']);
    fakeApi.connections.mockResolvedValue([{id: 'conn-1', name: '테스트 연결'}]);
    fakeApi.workflowChoices.mockResolvedValue([{versionId: 'wv-1', workflowId: 'wf-1', title: '정기 배포', number: 2, createdAt: ''}]);
});

describe('department automation', () => {
    it('starts with everything off', () => {
        expect(rulesOf(dev)).toEqual({autoStart: false, categories: [], connectionIds: [], outputTypes: [], workflowVersionIds: [], autoShare: false,
            autoCollab: false});
    });

    it('saves what the department lets happen and warns in review mode', async () => {
        fakeApi.setDepartmentAutomation.mockResolvedValue(dev);
        render(<Shell><AutomationPanel project={project} department={dev}/></Shell>);
        expect(await screen.findByText(/검토 모드입니다/)).toBeInTheDocument();
        fireEvent.click(screen.getByRole('checkbox', {name: /업무 자동 시작/}));
        fireEvent.click(screen.getByRole('checkbox', {name: /참고용 협업 요청/}));
        fireEvent.click(screen.getByRole('checkbox', {name: '개발'}));
        fireEvent.click(screen.getByRole('checkbox', {name: '테스트 연결'}));
        fireEvent.click(screen.getByRole('checkbox', {name: '정기 배포 2판'}));
        fireEvent.click(screen.getByRole('button', {name: '저장'}));
        await waitFor(() => expect(fakeApi.setDepartmentAutomation).toHaveBeenCalledWith('prj-1', 'dev', {autoStart: true, categories: ['개발'],
            connectionIds: ['conn-1'], outputTypes: [], workflowVersionIds: ['wv-1'], autoShare: false, autoCollab: true}));
    });
});
