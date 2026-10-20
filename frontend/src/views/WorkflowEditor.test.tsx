import {fireEvent, render, screen, waitFor} from '@testing-library/react';
import {beforeEach, describe, expect, it, vi} from 'vitest';
import {type ReactNode, useState} from 'react';
import {fakeApi} from '../test/fakeApi';

vi.mock('../api', () => ({api: fakeApi, errorText: (e: unknown) => String(e)}));

import {LiveProvider} from '../live';
import type {Project, Workflow} from '../types';
import {WorkflowEditor} from './WorkflowEditor';
import {DetailPane, DetailProvider} from '../ui';

const project: Project = {
    id: 'prj-1', organizationId: 'org-default', name: '게임', goal: '', instructions: '', workspacePath: '', budget: {},
    mode: 'review', status: 'active', createdAt: '', updatedAt: '',
};
const workflow: Workflow = {
    id: 'wf-1', projectId: 'prj-1', title: '흐름', revision: 3, updatedAt: '',
    draft: {schemaVersion: 1, title: '흐름', nodes: [{id: 'plan', title: '기획', kind: 'task', dependsOn: [], outputs: [{key: 'spec', type: 'markdown'}]}]},
};

beforeEach(() => {
    vi.clearAllMocks();
    fakeApi.workflow.mockResolvedValue(workflow);
});

const renderEditor = () => render(
    <LiveProvider><WorkflowEditor project={project} workflowId="wf-1" onStarted={() => {}} onClose={() => {}}/></LiveProvider>);

describe('WorkflowEditor', () => {
    it('opens the assistant from the toolbar and previews without saving the proposal', async () => {
        const proposal = {...workflow.draft, nodes: [...workflow.draft.nodes, {id: 'qa', title: '품질 확인', kind: 'task', dependsOn: ['plan']}]};
        fakeApi.assists.mockResolvedValue([{id: 'ast-preview', projectId: project.id, workflowId: workflow.id, kind: 'edit', status: 'ready',
            request: '품질 확인 추가', baseRevision: 3, baseWorkflow: workflow.draft, createdAt: '2026-10-20',
            result: {workflow: proposal, notes: '검토를 추가했습니다', issues: [], changes: {added: ['품질 확인'], changed: [], removed: []}}}]);
        function Host() {
            const [detail, setDetail] = useState<ReactNode | null>(null);
            return <LiveProvider><DetailProvider value={setDetail}><WorkflowEditor project={project} workflowId="wf-1" onStarted={() => {}} onClose={() => {}}/>
                <DetailPane detail={detail} onClose={() => setDetail(null)}/></DetailProvider></LiveProvider>;
        }
        render(<Host/>);
        fireEvent.click(await screen.findByRole('button', {name: 'AI로 수정'}));
        fireEvent.click(await screen.findByRole('button', {name: '캔버스에서 수정안 보기'}));
        expect(screen.getByLabelText('AI 수정안 미리보기')).toBeInTheDocument();
        expect(screen.getByLabelText('흐름 이름')).toBeDisabled();
        expect(fakeApi.saveDraft).not.toHaveBeenCalled();
        fireEvent.click(screen.getByRole('button', {name: '원래 초안 보기'}));
        expect(screen.getByLabelText('흐름 이름')).toBeEnabled();
        expect(screen.getByLabelText('업무 흐름 편집기')).toBeInTheDocument();
    });
    // Review finding 5: a failed save must stop the confirm.
    it('does not confirm a version when saving the edited draft fails', async () => {
        fakeApi.saveDraft.mockRejectedValue('다른 곳에서 먼저 수정되었습니다');
        renderEditor();
        const title = await screen.findByLabelText('흐름 이름');
        fireEvent.change(title, {target: {value: '바뀐 흐름'}});
        fireEvent.click(screen.getByRole('button', {name: '버전 확정'}));
        expect(await screen.findByRole('alert')).toHaveTextContent('다른 곳에서 먼저 수정되었습니다');
        expect(fakeApi.confirmVersion).not.toHaveBeenCalled();
        expect(screen.getByText(/저장 안 됨/)).toBeInTheDocument();
    });

    it('saves then confirms the saved revision', async () => {
        fakeApi.saveDraft.mockResolvedValue({...workflow, title: '바뀐 흐름', revision: 4});
        fakeApi.workflow.mockResolvedValueOnce(workflow).mockResolvedValue({...workflow, revision: 4});
        fakeApi.confirmVersion.mockResolvedValue({version: {id: 'wv-1', number: 1}, validation: {issues: [], canVersion: true, canRun: true}});
        renderEditor();
        fireEvent.change(await screen.findByLabelText('흐름 이름'), {target: {value: '바뀐 흐름'}});
        fireEvent.click(screen.getByRole('button', {name: '버전 확정'}));
        await waitFor(() => expect(fakeApi.confirmVersion).toHaveBeenCalledWith('prj-1', 'wf-1', 4));
        expect(fakeApi.saveDraft).toHaveBeenCalledWith('prj-1', 'wf-1', 3, expect.objectContaining({title: '바뀐 흐름'}));
    });
});
