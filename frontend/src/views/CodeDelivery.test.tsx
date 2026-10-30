import {fireEvent, render, screen, waitFor} from '@testing-library/react';
import {beforeEach, describe, expect, it, vi} from 'vitest';
import {fakeApi} from '../test/fakeApi';

vi.mock('../api', () => ({api: fakeApi, errorText: (e: unknown) => String(e)}));

import {LiveProvider} from '../live';
import type {FinalCode, Project, RunDetail} from '../types';
import {CodeHandover} from './CodeDelivery';

const project: Project = {
    id: 'prj-1', organizationId: 'org-default', name: '빌라노트', goal: '', instructions: '', workspacePath: '', budget: {},
    mode: 'review', status: 'active', createdAt: '', updatedAt: '',
};
const run: RunDetail = {
    id: 'run-1', projectId: 'prj-1', versionId: 'wv-1', versionNumber: 1, title: 'MVP', status: 'succeeded', paused: false, steps: [],
    repo: {kind: 'base', path: '/data/projects/prj-1/repo'},
};
const final: FinalCode = {
    runId: 'run-1', repo: '/data/projects/prj-1/repo', appRepo: true, base: 'b0', commit: '11097fdb90cba85b',
    steps: [{stepId: 'web', title: '웹 클라이언트 구현', commit: '11097fdb90cba85b'}],
    stat: {files: 37, insertions: 15251, deletions: 0}, included: false, deliveries: [],
};

beforeEach(() => vi.clearAllMocks());

describe('CodeHandover', () => {
    it('copies the code into a chosen folder and makes it the work folder', async () => {
        fakeApi.finalCode.mockResolvedValue(final);
        fakeApi.chooseFolder.mockResolvedValue('/Users/me/dev/villanote');
        fakeApi.deliverCode.mockResolvedValue({method: 'folder', folder: '/Users/me/dev/villanote', workspace: true, at: ''});
        render(<LiveProvider><CodeHandover project={project} run={run}/></LiveProvider>);

        expect(await screen.findByText('아직 서비스 기준 코드에 들어 있지 않습니다')).toBeInTheDocument();
        expect(screen.getByText(/파일 37개/)).toBeInTheDocument();
        const copy = screen.getByRole('button', {name: '폴더로 꺼내기'});
        expect(copy).toBeDisabled();
        fireEvent.click(screen.getByRole('button', {name: '폴더 선택…'}));
        await waitFor(() => expect(copy).toBeEnabled());
        fireEvent.click(copy);
        await waitFor(() => expect(fakeApi.deliverCode).toHaveBeenCalledWith(
            {projectId: 'prj-1', runId: 'run-1', method: 'folder', folder: '/Users/me/dev/villanote', useAsWorkspace: true}));
        expect(fakeApi.finalCode).toHaveBeenCalledTimes(2); // reloaded after the delivery
    });

    it('offers branches for a person’s repository and lists past deliveries', async () => {
        fakeApi.finalCode.mockResolvedValue({...final, appRepo: false, deliveries: [
            {method: 'advance', branch: 'agent-office/result-run1', note: '작업 폴더에 커밋되지 않은 변경이 있어 main 브랜치에는 반영하지 않았습니다', at: '2026-10-30T14:00:00Z'},
        ]});
        render(<LiveProvider><CodeHandover project={project} run={{...run, repo: {kind: 'base', path: '/Users/me/app'}}}/></LiveProvider>);
        expect(await screen.findByRole('button', {name: '결과 브랜치 만들기'})).toBeInTheDocument();
        expect(screen.queryByRole('button', {name: '폴더로 꺼내기'})).not.toBeInTheDocument();
        expect(screen.getByRole('list', {name: '전달 기록'})).toHaveTextContent('커밋되지 않은 변경');
        fireEvent.click(screen.getByRole('button', {name: '결과 브랜치 만들기'}));
        await waitFor(() => expect(fakeApi.deliverCode).toHaveBeenCalledWith(expect.objectContaining({method: 'branch'})));
    });

    it('shows nothing for a run that changed no code', async () => {
        fakeApi.finalCode.mockResolvedValue({...final, steps: [], commit: undefined});
        const {container} = render(<LiveProvider><CodeHandover project={project} run={run}/></LiveProvider>);
        await waitFor(() => expect(fakeApi.finalCode).toHaveBeenCalled());
        expect(container).toBeEmptyDOMElement();
    });
});
