import {fireEvent, render, screen, waitFor} from '@testing-library/react';
import {beforeEach, describe, expect, it, vi} from 'vitest';
import {fakeApi} from '../test/fakeApi';

vi.mock('../api', () => ({api: fakeApi, errorText: (e: unknown) => String(e)}));

import {LiveProvider} from '../live';
import type {OrgView, Project} from '../types';
import {OrganizeService} from './Organize';

const project: Project = {
    id: 'prj-1', organizationId: 'org-default', name: '빌라노트', goal: '', instructions: '', workspacePath: '', budget: {},
    mode: 'auto', status: 'active', createdAt: '', updatedAt: '',
};
const ready: OrgView = {
    names: {'a-plan': '기획 담당', 'a-be': '백엔드 개발자', 'a-qa': '품질·보안 검증 담당'}, autoMode: true, hasBudget: false,
    allowed: [], suggestAllow: [{id: 'conn-claude', name: 'Claude'}, {id: 'conn-codex', name: 'Codex'}],
    proposal: {id: 'org-1', projectId: 'prj-1', status: 'ready', connectionId: 'conn-claude', model: '', error: '', startedAt: '', endedAt: '',
        plan: {departments: [
            {name: '개발팀', responsibility: '코드를 만든다', members: ['a-be'], lead: 'a-be', watches: []},
            {name: '검증팀', responsibility: '검증한다', members: ['a-qa'], lead: 'a-qa',
                watches: [{eventKinds: ['document.shared'], guidance: '개발팀 자료를 본다'}]},
        ], intakeDesk: null}},
};

beforeEach(() => vi.clearAllMocks());

describe('OrganizeService', () => {
    it('asks the AI for a proposal', async () => {
        fakeApi.proposeOrganization.mockResolvedValue(ready);
        render(<LiveProvider><OrganizeService project={project}/></LiveProvider>);
        fireEvent.click(await screen.findByRole('button', {name: 'AI에게 조직 제안받기'}));
        await waitFor(() => expect(fakeApi.proposeOrganization).toHaveBeenCalledWith('prj-1'));
    });

    it('shows the departments, warns about the budget and applies with the chosen connections', async () => {
        fakeApi.latestOrgProposal.mockResolvedValue(ready);
        fakeApi.applyOrganization.mockResolvedValue({departments: ['개발팀', '검증팀'], skipped: [], desk: '', autoStart: true, adoptedWork: 2, problems: []});
        render(<LiveProvider><OrganizeService project={project}/></LiveProvider>);
        const qa = await screen.findByRole('group', {name: '부서 검증팀'});
        expect(qa).toHaveTextContent('품질·보안 검증 담당(대표)');
        expect(qa).toHaveTextContent('자료 공유 — 개발팀 자료를 본다');
        expect(screen.getByText('서비스 예산이 없습니다')).toBeInTheDocument();
        await waitFor(() => expect(screen.getByLabelText('Codex')).toBeChecked());
        fireEvent.click(screen.getByLabelText('Codex'));
        fireEvent.click(screen.getByRole('button', {name: '적용'}));
        await waitFor(() => expect(fakeApi.applyOrganization).toHaveBeenCalledWith('prj-1', 'org-1', ['conn-claude']));
    });
});
