import {fireEvent, render, screen, waitFor} from '@testing-library/react';
import {type ReactNode} from 'react';
import {beforeEach, describe, expect, it, vi} from 'vitest';
import {fakeApi} from '../test/fakeApi';

vi.mock('../api', () => ({api: fakeApi, errorText: (e: unknown) => String(e)}));

import {LiveProvider} from '../live';
import type {Department, Project, ScheduleView} from '../types';
import {DetailProvider, ToastProvider} from '../ui';
import {SubscriptionPanel} from './Judgments';
import {pcTimezone, scheduleLabel} from './Schedule';

const project: Project = {id: 'prj-1', organizationId: 'org-default', name: '캠페인', goal: '', instructions: '', workspacePath: '',
    budget: {}, mode: 'review', status: 'active', createdAt: '', updatedAt: ''};
const mkt: Department = {id: 'mkt', projectId: 'prj-1', name: '마케팅팀', responsibility: '', leadAssignmentId: 'a1', memberIds: ['a1'], templateId: '',
    automation: {}, limits: {}, status: 'active', createdAt: '', updatedAt: ''};

const Shell = ({children}: { children: ReactNode }) => (
    <LiveProvider><ToastProvider><DetailProvider value={() => {}}>{children}</DetailProvider></ToastProvider></LiveProvider>
);

beforeEach(() => {
    vi.clearAllMocks();
    fakeApi.categories.mockResolvedValue([]);
    fakeApi.subscriptions.mockResolvedValue([]);
    fakeApi.schedules.mockResolvedValue([]);
});

describe('regular checks', () => {
    it('names daily and weekly schedules', () => {
        expect(scheduleLabel({frequency: 'daily', timeOfDay: '09:00', weekday: null})).toBe('매일 09:00');
        expect(scheduleLabel({frequency: 'weekly', timeOfDay: '18:30', weekday: 5})).toBe('매주 금요일 18:30');
    });

    it('starts off for a department and saves a weekly check in the PC time zone', async () => {
        fakeApi.saveSchedule.mockResolvedValue({});
        render(<Shell><SubscriptionPanel project={project} department={mkt}/></Shell>);
        const form = await screen.findByRole('form', {name: '정기 점검'});
        const use = screen.getByLabelText('정기 점검 사용') as HTMLInputElement;
        expect(use.checked).toBe(false);
        fireEvent.click(use);
        fireEvent.change(screen.getByLabelText('점검 주기'), {target: {value: 'weekly'}});
        fireEvent.change(screen.getByLabelText('요일'), {target: {value: '3'}});
        fireEvent.change(screen.getByLabelText('점검 시각'), {target: {value: '10:30'}});
        fireEvent.change(screen.getByLabelText(/점검 관점/), {target: {value: '이번 주 자료를 본다'}});
        fireEvent.submit(form);
        await waitFor(() => expect(fakeApi.saveSchedule).toHaveBeenCalledWith({projectId: 'prj-1', departmentId: 'mkt', enabled: true,
            frequency: 'weekly', timeOfDay: '10:30', weekday: 3, timezone: pcTimezone(), note: '이번 주 자료를 본다'}));
    });

    it('shows when it runs next and offers earlier documents for review', async () => {
        const saved: ScheduleView = {id: 'sch-1', projectId: 'prj-1', departmentId: 'mkt', frequency: 'daily', timeOfDay: '09:00', weekday: null,
            timezone: 'Mars/Base', note: '', enabled: true, activeFrom: '', createdAt: '', updatedAt: 'u1', departmentName: '마케팅팀',
            lastSlotAt: '', lastSequence: 0, nextDueAt: '2026-10-27T00:00:00Z'};
        fakeApi.schedules.mockResolvedValue([saved]);
        render(<Shell><SubscriptionPanel project={project} department={mkt}/></Shell>);
        expect(await screen.findByText(/다음 점검/)).toBeInTheDocument();
        expect(screen.getByText(/저장된 시간대 Mars\/Base/)).toBeInTheDocument();
        expect((screen.getByLabelText('정기 점검 사용') as HTMLInputElement).checked).toBe(true);
        expect(screen.getByRole('button', {name: '기존 자료 검토 요청'})).toBeInTheDocument();
    });
});
