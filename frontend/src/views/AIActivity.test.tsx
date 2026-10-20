import {act, fireEvent, render, screen, waitFor} from '@testing-library/react';
import {beforeEach, describe, expect, it, vi} from 'vitest';
import {fakeApi} from '../test/fakeApi';
import type {Project, StoredEvent} from '../types';

vi.mock('../api', () => ({api: fakeApi, errorText: (e: unknown) => String(e)}));
import {LiveProvider} from '../live';
import {ActivityFeed, AIActivityPanel} from './AIActivity';
import {WorkflowComparison} from './WorkflowComparison';

const project = {id: 'p', name: '게임', status: 'active'} as Project;
const ev = (n: number, text: string, stepId = 'plan', kind = 'provider.completed'): StoredEvent => ({
    sequence: n, id: `e${n}`, projectId: 'p', runId: 'r', stepAttemptId: `a-${stepId}`, stepId, assignmentId: 'ai', attempt: 1,
    kind, payload: {text}, createdAt: `2026-10-20T01:00:${String(n).padStart(2, '0')}Z`,
});
beforeEach(() => {
    vi.clearAllMocks();
    fakeApi.aiActivity.mockResolvedValue({events: [ev(1, '기획 완료'), ev(2, '다른 업무', 'dev')], hasMore: false});
    fakeApi.runMessages.mockResolvedValue([]);
    fakeApi.assignments.mockResolvedValue([{id: 'ai', displayName: '기획 AI'}]);
});

describe('AI activity', () => {
    it('filters a task, preserves reading focus and announces appended records', async () => {
        render(<LiveProvider><ActivityFeed projectId="p" runId="r" stepId="plan"/></LiveProvider>);
        expect(await screen.findByText('기획 완료')).toBeInTheDocument();
        expect(screen.queryByText('다른 업무')).not.toBeInTheDocument();
        const tab = screen.getByRole('tab', {name: '대화·결정'});
        tab.focus();
        fakeApi.aiActivity.mockResolvedValue({events: [ev(1, '기획 완료'), ev(3, '새 응답')], hasMore: false});
        const receive = (fakeApi.onEvents.mock.calls as unknown as Array<[(events: StoredEvent[]) => void]>)[0][0];
        await act(async () => { receive([{...ev(3, ''), kind: 'provider.completed'}]); });
        expect(await screen.findByText('새 응답')).toBeInTheDocument();
        expect(tab).toHaveFocus();
        fireEvent.click(screen.getByRole('button', {name: /새 기록 1개/}));
        expect(screen.queryByRole('button', {name: /새 기록/})).not.toBeInTheDocument();
        expect(fakeApi.aiActivity).toHaveBeenCalledWith('p', 'r', '');
    });

    it('shows public output in logs and loads older records with a cursor', async () => {
        fakeApi.aiActivity.mockResolvedValue({events: [ev(10, '진행 출력', 'plan', 'activity.output')], hasMore: true});
        render(<LiveProvider><ActivityFeed projectId="p" runId="r"/></LiveProvider>);
        fireEvent.click(screen.getByRole('tab', {name: '실행 로그'}));
        expect(await screen.findByText('진행 출력')).toBeInTheDocument();
        fakeApi.aiActivity.mockResolvedValue({events: [ev(1, '이전 응답')], hasMore: false});
        fireEvent.click(screen.getByRole('button', {name: '이전 기록 불러오기'}));
        expect(await screen.findByText('이전 응답')).toBeInTheDocument();
        expect(fakeApi.aiActivity).toHaveBeenLastCalledWith('p', 'r', '', 10);
    });

    it('includes incoming handoffs and next-attempt notes in the task conversation', async () => {
        fakeApi.runMessages.mockResolvedValue([{id: 'note', kind: 'decision', sender: 'local-owner', senderName: '나', recipient: 'step:plan',
            recipientName: '기획', stepId: '', body: '다음 시도 메모', refs: {note: true}, createdAt: '2026-10-20T00:00:00Z'},
            {id: 'handoff', kind: 'handoff', sender: 'ai', senderName: '개발 AI', recipient: 'step:plan', recipientName: '기획', stepId: 'dev',
                body: '입력 전달', refs: {}, createdAt: '2026-10-20T00:00:01Z'}]);
        render(<LiveProvider><ActivityFeed projectId="p" runId="r" stepId="plan"/></LiveProvider>);
        expect(await screen.findByText('다음 시도 메모')).toBeInTheDocument();
        expect(screen.getByText('입력 전달')).toBeInTheDocument();
    });

    it('deduplicates a provider completion without hiding the same reply from a later attempt', async () => {
        fakeApi.aiActivity.mockResolvedValue({events: [ev(1, '같은 응답', 'plan', 'provider.message'), ev(2, '같은 응답'),
            {...ev(3, '같은 응답'), stepAttemptId: 'retry-plan'}], hasMore: false});
        render(<LiveProvider><ActivityFeed projectId="p" runId="r"/></LiveProvider>);
        await waitFor(() => expect(screen.getAllByText('같은 응답')).toHaveLength(2));
    });

    it('keeps side-session transport responses in logs while showing attributed collaboration messages', async () => {
        fakeApi.aiActivity.mockResolvedValue({events: [ev(1, '보조 세션 JSON', 'plan', 'side.completed')], hasMore: false});
        fakeApi.runMessages.mockResolvedValue([{id: 'answer', kind: 'answer', sender: 'reviewer', senderName: '검토 AI', recipient: 'ai',
            recipientName: '기획 AI', stepId: 'plan', body: '검토 담당자의 답변', refs: {}, createdAt: '2026-10-20T00:00:00Z'}]);
        render(<LiveProvider><ActivityFeed projectId="p" runId="r"/></LiveProvider>);
        expect(await screen.findByText('검토 담당자의 답변')).toBeInTheDocument();
        expect(screen.queryByText('보조 세션 JSON')).not.toBeInTheDocument();
        fireEvent.click(screen.getByRole('tab', {name: '실행 로그'}));
        expect(await screen.findByText('보조 세션 JSON')).toBeInTheDocument();
    });

    it('shows context, routes questions to the inbox and posts notes only for the selected task', async () => {
        fakeApi.run.mockResolvedValue({id: 'r', title: '개발 흐름', versionNumber: 2, status: 'running', steps: [
            {id: 'plan', title: '기획', kind: 'task', status: 'running', assignmentId: 'ai'},
        ]});
        fakeApi.inbox.mockResolvedValue([{projectId: 'p', runId: 'r', stepId: 'plan', kind: 'question'}] as never);
        const onInbox = vi.fn();
        render(<LiveProvider><AIActivityPanel project={project} runId="r" stepId="plan" onInbox={onInbox}/></LiveProvider>);
        expect(await screen.findByText('개발 흐름 · v2')).toBeInTheDocument();
        expect(await screen.findByText('기획 · 기획 AI')).toBeInTheDocument();
        fireEvent.click(await screen.findByRole('button', {name: '내 할 일에서 처리'}));
        expect(onInbox).toHaveBeenCalled();
        fireEvent.change(screen.getByLabelText('다음 시도에 전달할 메모'), {target: {value: '모바일도 포함'}});
        fireEvent.click(screen.getByRole('button', {name: '메모 남기기'}));
        await waitFor(() => expect(fakeApi.postNote).toHaveBeenCalledWith('p', 'r', 'plan', 'decision', '모바일도 포함'));
    });

    it('compares dependencies and instructions before applying a draft', async () => {
        const before = {schemaVersion: 1, title: '흐름', nodes: [{id: 'plan', title: '기획', kind: 'task' as const, dependsOn: [], instructions: '원래 지시'}]};
        const after = {...before, nodes: [{...before.nodes[0], dependsOn: ['review'], instructions: '새 지시'}]};
        render(<LiveProvider><WorkflowComparison projectId="p" before={before} after={after}/></LiveProvider>);
        expect(screen.getByText('원래 지시')).toBeInTheDocument();
        expect(screen.getByText('새 지시')).toBeInTheDocument();
        expect(screen.getByText('review')).toBeInTheDocument();
    });
});
