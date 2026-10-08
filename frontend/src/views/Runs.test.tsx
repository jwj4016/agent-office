import {fireEvent, render, screen, waitFor} from '@testing-library/react';
import {beforeEach, describe, expect, it, vi} from 'vitest';
import {fakeApi} from '../test/fakeApi';

vi.mock('../api', () => ({api: fakeApi, errorText: (e: unknown) => String(e)}));

import {LiveProvider} from '../live';
import type {MessageView, Project, RunDetail} from '../types';
import {Conversation} from './Runs';

const project: Project = {
    id: 'prj-1', organizationId: 'org-default', name: '게임', goal: '', instructions: '', workspacePath: '', budget: {},
    mode: 'review', status: 'active', createdAt: '', updatedAt: '',
};
const run: RunDetail = {
    id: 'run-1', projectId: 'prj-1', versionId: 'wv-1', versionNumber: 1, title: '서비스 개발', status: 'waiting', paused: false,
    steps: [
        {id: 'plan', title: '기획', kind: 'task', status: 'succeeded', assignmentId: 'a1', attemptId: 'att-1', generation: 1, attempt: 1, round: 0, artifacts: []},
        {id: 'route', title: '분기', kind: 'condition', status: 'pending', assignmentId: '', attemptId: '', generation: 1, attempt: 0, round: 0, artifacts: []},
        {id: 'design', title: '설계', kind: 'task', status: 'pending', assignmentId: 'a2', attemptId: '', generation: 1, attempt: 0, round: 0, artifacts: []},
    ],
};
const handoff: MessageView = {
    id: 'msg-1', kind: 'handoff', sender: 'a1', senderName: '기획 AI', recipient: 'step:design', recipientName: '업무 설계',
    stepId: 'plan', body: '기획서를 넘깁니다', createdAt: '2026-10-08T00:00:00Z',
    refs: {artifacts: [{id: 'art-1', stepId: 'plan', outputKey: 'spec', version: 1, hash: 'h'}], from: 'plan'},
};

beforeEach(() => vi.clearAllMocks());

describe('Conversation', () => {
    it('shows handoffs with their source version and posts a note to a step', async () => {
        fakeApi.runMessages.mockResolvedValue([handoff]);
        render(<LiveProvider><Conversation project={project} run={run} finished={false}/></LiveProvider>);
        expect(await screen.findByText('기획서를 넘깁니다')).toBeInTheDocument();
        expect(screen.getByText(/plan\.spec v1/)).toBeInTheDocument();
        // Engine steps cannot receive notes.
        expect(screen.queryByRole('option', {name: '분기'})).not.toBeInTheDocument();
        const button = screen.getByRole('button', {name: '메모 남기기'});
        expect(button).toBeDisabled();
        fireEvent.change(screen.getByLabelText('메모 받을 업무'), {target: {value: 'design'}});
        fireEvent.change(screen.getByLabelText('메모 내용'), {target: {value: 'REST로 통일'}});
        fireEvent.click(button);
        await waitFor(() => expect(fakeApi.postNote).toHaveBeenCalledWith('prj-1', 'run-1', 'design', 'decision', 'REST로 통일'));
    });

    it('hides the note form for a finished run', async () => {
        fakeApi.runMessages.mockResolvedValue([]);
        render(<LiveProvider><Conversation project={project} run={{...run, status: 'succeeded'}} finished/></LiveProvider>);
        expect(await screen.findByText('아직 기록된 대화가 없습니다.')).toBeInTheDocument();
        expect(screen.queryByRole('button', {name: '메모 남기기'})).not.toBeInTheDocument();
    });
});
