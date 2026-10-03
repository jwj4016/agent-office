import {fireEvent, render, screen, waitFor} from '@testing-library/react';
import {beforeEach, describe, expect, it, vi} from 'vitest';
import {fakeApi} from '../test/fakeApi';

vi.mock('../api', () => ({api: fakeApi, errorText: (e: unknown) => String(e)}));

import {LiveProvider} from '../live';
import type {Project, Workflow} from '../types';
import {WorkflowEditor} from './WorkflowEditor';

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
