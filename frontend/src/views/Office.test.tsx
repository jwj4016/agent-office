import {fireEvent, render, screen, waitFor, within} from '@testing-library/react';
import {type ReactNode, useState} from 'react';
import {afterEach, beforeEach, describe, expect, it, vi} from 'vitest';
import {fakeApi, inboxItem, state} from '../test/fakeApi';

vi.mock('../api', () => ({api: fakeApi, errorText: (e: unknown) => String(e)}));

import {LiveProvider} from '../live';
import type {Assignment, Project, RunDetail} from '../types';
import {DetailProvider} from '../ui';
import {Office} from './Office';

const project: Project = {id: 'prj-1', organizationId: 'org-default', name: '게임', goal: '', instructions: '', workspacePath: '',
    budget: {}, mode: 'review', status: 'active', createdAt: '', updatedAt: ''};

const asg = (id: string, name: string, kind: 'ai' | 'human' = 'ai'): Assignment => ({
    id, projectId: 'prj-1', roleId: 'role-dev', actorKind: kind, actorId: kind === 'human' ? 'local-owner' : '', displayName: name,
    connectionId: kind === 'ai' ? 'conn-1' : '', model: '', overrides: {},
});

const run: RunDetail = {
    id: 'run-1', projectId: 'prj-1', versionId: 'v1', versionNumber: 2, title: '서비스 개발', status: 'failed', paused: false,
    steps: [
        {id: 'plan', title: '기획', kind: 'task', status: 'succeeded', assignmentId: 'a-plan', attemptId: 'att-plan', generation: 1, attempt: 1, round: 0,
            artifacts: [{id: 'art-1', outputKey: 'plan', version: 1, type: 'markdown', path: '', hash: '', validity: 'valid'}]},
        {id: 'dev', title: '개발', kind: 'task', status: 'failed', assignmentId: 'a-dev', attemptId: 'att-dev', generation: 1, attempt: 1, round: 0,
            error: '검증 명령 실패', artifacts: []},
        {id: 'review', title: '코드 리뷰', kind: 'review', status: 'waiting_human', assignmentId: 'a-me', attemptId: 'att-r', generation: 1, attempt: 1, round: 0,
            artifacts: []},
    ],
};

// Shell renders the right-hand detail like the app does.
function Shell({children}: { children: ReactNode }) {
    const [detail, setDetail] = useState<ReactNode | null>(null);
    return (
        <LiveProvider>
            <DetailProvider value={setDetail}>
                {children}
                {detail ? <aside aria-label="상세">{detail}</aside> : null}
            </DetailProvider>
        </LiveProvider>
    );
}

const onRoute = vi.fn();
const renderIt = () => render(<Shell><Office project={project} onRoute={onRoute}/></Shell>);

const drawn: string[] = [];
beforeEach(() => {
    vi.clearAllMocks();
    state.settings = {};
    state.inbox = [inboxItem({kind: 'review', runId: 'run-1', stepId: 'review', approvalId: undefined})];
    drawn.length = 0;
    fakeApi.assignments.mockResolvedValue([asg('a-plan', '기획 AI'), asg('a-dev', '개발 AI'), asg('a-me', '나', 'human')]);
    fakeApi.roles.mockResolvedValue([{id: 'role-dev', organizationId: 'o', parentRoleId: '', name: '개발', mission: '', instructions: '',
        outputDefaults: {}, policy: {}, appearance: {}, updatedAt: ''}]);
    fakeApi.runs.mockResolvedValue([run]);
    fakeApi.connections.mockResolvedValue([{id: 'conn-1', name: '내 Codex', provider: 'codex', usable: true}]);
    // A recording 2D context: drawing runs for real, nothing is painted.
    const ctx = new Proxy({measureText: (s: string) => ({width: s.length * 3}), fillText: (s: string) => drawn.push(s)} as Record<string, unknown>, {
        get: (target, key: string) => (key in target ? target[key] : () => {}),
        set: () => true,
    });
    vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(ctx as never);
});
afterEach(() => vi.restoreAllMocks());

describe('Office', () => {
    it('draws every desk and announces the selected person in words', async () => {
        renderIt();
        const canvas = await screen.findByRole('application');
        await waitFor(() => expect(drawn).toEqual(expect.arrayContaining(['기획 AI', '개발 AI', '나', '회의실'])));
        expect(screen.getByText('기획 AI (AI, 개발): 완료 — 기획 · 서비스 개발')).toBeInTheDocument();
        fireEvent.keyDown(canvas, {key: 'ArrowRight'});
        expect(screen.getByText('개발 AI (AI, 개발): 문제 발생 — 개발 · 서비스 개발')).toBeInTheDocument();
        fireEvent.keyDown(canvas, {key: 'End'});
        expect(screen.getByText('나 (사람, 개발): 내 할 일 있음 — 코드 리뷰 · 서비스 개발')).toBeInTheDocument();
    });

    it('opens the detail with Enter and offers the real recovery action', async () => {
        renderIt();
        const canvas = await screen.findByRole('application');
        await screen.findByText(/기획 AI \(AI/);
        fireEvent.keyDown(canvas, {key: 'ArrowRight'});
        fireEvent.keyDown(canvas, {key: 'Enter'});
        const detail = await screen.findByRole('complementary', {name: '상세'});
        await within(detail).findByText('검증 명령 실패');
        expect(within(detail).getByText(/내 Codex/)).toBeInTheDocument();
        fireEvent.click(within(detail).getByRole('button', {name: '다시 시도'}));
        await waitFor(() => expect(fakeApi.retryStep).toHaveBeenCalledWith('prj-1', 'run-1', 'dev'));
        fireEvent.click(within(detail).getByRole('button', {name: '실행 보기'}));
        expect(onRoute).toHaveBeenCalledWith({view: 'project', projectId: 'prj-1', tab: 'runs', runId: 'run-1'});
    });

    it('offers a plain list with the same details, and remembers the choice', async () => {
        renderIt();
        fireEvent.click(await screen.findByRole('button', {name: '목록'}));
        const table = await screen.findByRole('table', {name: '담당자 상태'});
        expect(within(table).getByText('내 할 일 있음')).toBeInTheDocument();
        expect(within(table).getByText('문제 발생')).toBeInTheDocument();
        expect(fakeApi.setSetting).toHaveBeenCalledWith('ui.officeView', 'list');
        fireEvent.click(within(table).getByRole('button', {name: '나 상세'}));
        const detail = await screen.findByRole('complementary', {name: '상세'});
        expect(await within(detail).findByText('기다리는 일: 리뷰')).toBeInTheDocument();
        fireEvent.click(within(detail).getByRole('button', {name: '내 할 일에서 처리'}));
        expect(onRoute).toHaveBeenCalledWith({view: 'inbox'});
    });

    it('starts in the list when that was chosen before', async () => {
        state.settings = {'ui.officeView': 'list'};
        renderIt();
        expect(await screen.findByRole('table', {name: '담당자 상태'})).toBeInTheDocument();
        expect(screen.queryByRole('application')).not.toBeInTheDocument();
    });

    it('explains an empty office', async () => {
        fakeApi.assignments.mockResolvedValue([]);
        renderIt();
        expect(await screen.findByText(/아직 담당자가 없습니다/)).toBeInTheDocument();
    });
});
