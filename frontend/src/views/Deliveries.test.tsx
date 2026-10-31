import {fireEvent, render, screen, waitFor, within} from '@testing-library/react';
import type {ReactNode} from 'react';
import {beforeEach, describe, expect, it, vi} from 'vitest';
import {fakeApi} from '../test/fakeApi';

vi.mock('../api', () => ({api: fakeApi, errorText: (e: unknown) => String(e)}));

import {LiveProvider} from '../live';
import type {Delivery, Department, Project, WorkEventView} from '../types';
import {ToastProvider} from '../ui';
import {DeliveryInbox, WorkEvents} from './Deliveries';

const project: Project = {id: 'prj-1', organizationId: 'org-default', name: '캠페인', goal: '', instructions: '', workspacePath: '',
    budget: {}, mode: 'review', status: 'active', createdAt: '', updatedAt: ''};

const dept = (id: string, name: string): Department => ({
    id, projectId: 'prj-1', name, responsibility: '', leadAssignmentId: '', memberIds: [], templateId: '', automation: {}, limits: {},
    status: 'active', createdAt: '', updatedAt: '',
});
const departments = [dept('dev', '개발팀'), dept('fin', '재무팀')];

const delivery = (over: Partial<Delivery>): Delivery => ({
    id: 'd', projectId: 'prj-1', eventSequence: 1, eventKind: 'work.created', eventAt: '2026-10-23T01:00:00Z',
    event: {departmentId: 'mkt', title: '캠페인 기획안', categories: [], causeSequence: null, chainRootSequence: null, chainDepth: 0, automated: false},
    departmentId: 'dev', subscriptionId: 's', purpose: 'judge', status: 'pending', note: '', error: '', holdReason: '', createdAt: '', updatedAt: '',
    ...over,
});

const Shell = ({children}: { children: ReactNode }) => <LiveProvider><ToastProvider>{children}</ToastProvider></LiveProvider>;

beforeEach(() => {
    vi.clearAllMocks();
});

describe('department inbox', () => {
    it('shows what each department received and retries a failed delivery', async () => {
        fakeApi.deliveries.mockResolvedValue([
            delivery({id: 'd1'}),
            delivery({id: 'd2', departmentId: 'fin', status: 'failed', error: '대표 담당자가 없습니다'}),
        ]);
        render(<Shell><DeliveryInbox project={project} departments={departments}/></Shell>);
        const table = await screen.findByRole('table');
        expect(within(table).getAllByText('업무 등록됨')).toHaveLength(2);
        expect(within(table).getByText('대표 담당자가 없습니다')).toBeInTheDocument();
        expect(fakeApi.deliveries).toHaveBeenCalledWith('prj-1', {status: 'open'});
        expect(within(table).queryByRole('button', {name: '개발팀 전달 다시 시도'})).toBeNull(); // pending
        fireEvent.click(within(table).getByRole('button', {name: '재무팀 전달 다시 시도'}));
        await waitFor(() => expect(fakeApi.retryDelivery).toHaveBeenCalledWith('prj-1', 'd2'));
    });

    it('asks one department to look for work', async () => {
        fakeApi.deliveries.mockResolvedValue([]);
        render(<Shell><DeliveryInbox project={project} departments={departments}/></Shell>);
        const form = await screen.findByRole('form', {name: '부서 점검 요청'});
        const send = within(form).getByRole('button', {name: '점검 요청'});
        expect(send).toBeDisabled();
        fireEvent.change(within(form).getByLabelText('점검할 부서'), {target: {value: 'fin'}});
        fireEvent.change(within(form).getByLabelText('요청 내용'), {target: {value: '다음 달 비용'}});
        fireEvent.click(send);
        await waitFor(() => expect(fakeApi.requestDepartmentCheck).toHaveBeenCalledWith('prj-1', 'fin', '다음 달 비용'));
    });
});

describe('work events', () => {
    it('lists a work item’s events with the departments that got them', async () => {
        const events: WorkEventView[] = [
            {sequence: 1, kind: 'work.created', event: delivery({}).event, createdAt: '', deliveries: [delivery({id: 'a'}), delivery({id: 'b', departmentId: 'fin', status: 'done'})]},
            {sequence: 2, kind: 'work.completed', event: delivery({}).event, createdAt: '', deliveries: []},
        ];
        fakeApi.workEvents.mockResolvedValue(events);
        render(<Shell><WorkEvents project={project} workItemId="w1" departments={departments}/></Shell>);
        const list = await screen.findByRole('list', {name: '이 업무의 사건'});
        expect(within(list).getByText('업무 등록됨')).toBeInTheDocument();
        expect(within(list).getByText('재무팀')).toBeInTheDocument();
        expect(within(list).getByText('처리됨')).toBeInTheDocument();
        expect(within(list).getByText('받은 부서 없음')).toBeInTheDocument();
        expect(fakeApi.workEvents).toHaveBeenCalledWith('prj-1', 'w1');
    });
});
