import {fireEvent, render, screen, waitFor, within} from '@testing-library/react';
import {beforeEach, describe, expect, it, vi} from 'vitest';
import {fakeApi, inboxItem, state} from './test/fakeApi';

vi.mock('./api', () => ({api: fakeApi, errorText: (e: unknown) => String(e)}));

import App from './App';

beforeEach(() => {
    state.summaries = [];
    state.inbox = [];
    state.settings = {'ui.tutorialSeen': 'true'};
    vi.clearAllMocks();
});

describe('App shell', () => {
    it('automatically opens the tutorial once and can reopen it without losing work', async () => {
        state.settings = {};
        const {unmount} = render(<App/>);
        const tutorial = await screen.findByRole('dialog');
        expect(within(tutorial).getByRole('heading', {name: 'AI와 함께 일하는 사무실입니다'})).toBeInTheDocument();
        fireEvent.click(within(tutorial).getByRole('button', {name: '건너뛰기'}));
        await waitFor(() => expect(fakeApi.setSetting).toHaveBeenCalledWith('ui.tutorialSeen', 'true'));
        expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
        unmount();
        render(<App/>);
        await screen.findByRole('button', {name: '새 목표로 시작'});
        expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
        fireEvent.click(screen.getByRole('button', {name: '직접 만들기'}));
        fireEvent.change(screen.getByLabelText('서비스 이름'), {target: {value: '작성 중인 서비스'}});
        const open = screen.getByRole('button', {name: '튜토리얼 다시 보기'});
        open.focus(); fireEvent.click(open);
        fireEvent.click(within(await screen.findByRole('dialog')).getByRole('button', {name: '다음'}));
        fireEvent.click(screen.getByRole('button', {name: '튜토리얼 닫기'}));
        expect(screen.getByLabelText('서비스 이름')).toHaveValue('작성 중인 서비스');
        expect(open).toHaveFocus();
        fireEvent.click(open);
        expect(within(await screen.findByRole('dialog')).getByRole('heading', {name: 'AI와 함께 일하는 사무실입니다'})).toBeInTheDocument();
        expect(fakeApi.startDesign).not.toHaveBeenCalled();
        expect(fakeApi.startRun).not.toHaveBeenCalled();
    });

    it('keeps the app usable when persisting tutorial dismissal fails', async () => {
        state.settings = {};
        fakeApi.setSetting.mockRejectedValueOnce('저장 실패');
        render(<App/>);
        const tutorial = await screen.findByRole('dialog');
        fireEvent.click(within(tutorial).getByRole('button', {name: '건너뛰기'}));
        expect(await screen.findByText(/튜토리얼 확인 상태를 저장하지 못했습니다/)).toBeInTheDocument();
        expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
        expect(screen.getByRole('button', {name: '튜토리얼 다시 보기'})).toBeEnabled();
    });

    it('does not reopen a dismissed guide when the initial settings arrive late', async () => {
        let resolveSettings!: (settings: Record<string, string>) => void;
        fakeApi.settings.mockImplementationOnce(() => new Promise((resolve) => { resolveSettings = resolve; }));
        render(<App/>);
        await waitFor(() => expect(fakeApi.settings).toHaveBeenCalled());
        fireEvent.click(screen.getByRole('button', {name: '튜토리얼 다시 보기'}));
        fireEvent.click(screen.getByRole('button', {name: '튜토리얼 닫기'}));
        resolveSettings({'ui.theme': 'dark'});
        await waitFor(() => expect(document.documentElement.dataset.theme).toBe('dark'));
        expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    });

    it('waits for startup status and skips automatic guidance in performance runs', async () => {
        state.settings = {};
        let resolveStatus!: (status: Awaited<ReturnType<typeof fakeApi.systemStatus>> & {perf: boolean}) => void;
        fakeApi.systemStatus.mockImplementationOnce(() => new Promise((resolve) => { resolveStatus = resolve; }));
        render(<App/>);
        expect(fakeApi.settings).not.toHaveBeenCalled();
        resolveStatus({dataDir: '/data', schemaVersion: 2, secretsPersistent: true, version: 'test', error: '', perf: true});
        await waitFor(() => expect(fakeApi.settings).toHaveBeenCalled());
        expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
        fireEvent.click(screen.getByRole('button', {name: '튜토리얼 다시 보기'}));
        expect(await screen.findByRole('dialog')).toBeInTheDocument();
    });

    it('creates a service from the empty dashboard and opens it', async () => {
        render(<App/>);
        // An empty home offers the two ways to start; making one by hand opens the form.
        expect(await screen.findByRole('button', {name: '새 목표로 시작'})).toBeInTheDocument();
        fireEvent.click(screen.getByRole('button', {name: '직접 만들기'}));
        const form = await screen.findByRole('form', {name: '새 서비스'});
        fireEvent.change(within(form).getByLabelText('서비스 이름'), {target: {value: '게임 서비스'}});
        fireEvent.click(within(form).getByRole('button', {name: '서비스 만들기'}));
        await waitFor(() => expect(fakeApi.createProject).toHaveBeenCalledWith(expect.objectContaining({name: '게임 서비스', mode: 'review'})));
        // Opening a service remembers it for the next start.
        await waitFor(() => expect(fakeApi.setSetting).toHaveBeenCalledWith('ui.lastProjectId', 'prj-1'));
    });

    it('shows the inbox count across services', async () => {
        state.inbox = [inboxItem(), inboxItem({projectId: 'prj-2', projectName: '부동산', attemptId: 'att-2', approvalId: 'apr-2'})];
        render(<App/>);
        expect(await screen.findByLabelText('2건')).toBeInTheDocument();
    });
});

describe('Department decisions as to-dos', () => {
    it('counts waiting collaboration on the home screen, the sidebar and the service', async () => {
        state.summaries = [{project: {id: 'prj-1', organizationId: 'org-default', name: '게임', goal: '', instructions: '', workspacePath: '',
            budget: {}, mode: 'auto', status: 'active', createdAt: '', updatedAt: ''}, activeRuns: 0, waitingRuns: 0, failedRuns: 0,
            pausedRuns: 0, inbox: 0, lastRun: null}];
        fakeApi.pendingDecisions.mockResolvedValue({questions: [], proposals: [], collabs: [{projectId: 'prj-1'}], notices: [], aiDecisions: [],
            autoReviews: [{projectId: 'prj-1'}], results: [], goals: {}});
        render(<App/>);
        expect(await screen.findByText('부서 간 협업과 결정 2건')).toBeInTheDocument();
        expect(screen.getByText('확인이 필요한 일이 2건 있습니다.')).toBeInTheDocument();
        expect(screen.getByLabelText('2건')).toBeInTheDocument(); // sidebar inbox badge
        expect(screen.getByText('대기 2')).toBeInTheDocument(); // the service's state
    });
});

describe('App start-up errors', () => {
    it('explains when another instance holds the data folder', async () => {
        fakeApi.systemStatus.mockResolvedValueOnce({dataDir: '/data', schemaVersion: 0, secretsPersistent: true, version: 'test',
            error: '다른 Agent Office가 이 데이터 폴더를 사용 중입니다'});
        render(<App/>);
        expect(await screen.findByRole('alert')).toHaveTextContent('사용 중');
        expect(screen.queryByRole('navigation')).not.toBeInTheDocument();
    });
});

describe('Dashboard first load', () => {
    it('does not open the new-service form when services exist', async () => {
        state.summaries = [{project: {id: 'prj-1', organizationId: 'org-default', name: '게임', goal: '', instructions: '', workspacePath: '',
            budget: {}, mode: 'review', status: 'active', createdAt: '', updatedAt: ''}, activeRuns: 0, waitingRuns: 0, failedRuns: 1,
            pausedRuns: 0, inbox: 0, lastRun: null}];
        render(<App/>);
        expect(await screen.findByText('막힘 1')).toBeInTheDocument(); // the dashboard card rendered
        expect(screen.queryByRole('form', {name: '새 서비스'})).not.toBeInTheDocument();
    });
});
