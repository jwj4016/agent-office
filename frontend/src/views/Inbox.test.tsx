import {fireEvent, render, screen, waitFor, within} from '@testing-library/react';
import {beforeEach, describe, expect, it, vi} from 'vitest';
import {fakeApi, inboxItem, state} from '../test/fakeApi';

vi.mock('../api', () => ({api: fakeApi, errorText: (e: unknown) => String(e)}));

import {LiveProvider} from '../live';
import {Inbox} from './Inbox';

const renderInbox = () => render(<LiveProvider><Inbox onOpenRun={() => {}}/></LiveProvider>);

beforeEach(() => {
    state.inbox = [];
    vi.clearAllMocks();
});

describe('Inbox', () => {
    it('approves with the pinned generation', async () => {
        state.inbox = [inboxItem({generation: 2})];
        renderInbox();
        const card = await screen.findByRole('article', {name: '게임 기획 승인'});
        expect(within(card).getByText('게임')).toBeInTheDocument();
        fireEvent.click(within(card).getByRole('button', {name: '승인'}));
        await waitFor(() => expect(fakeApi.decideApproval).toHaveBeenCalledWith('prj-1', 'apr-1', 2, 'approved', '', []));
        expect(await within(card).findByText('처리했습니다.')).toBeInTheDocument();
    });

    it('requires a reason and a target to reject', async () => {
        state.inbox = [inboxItem({reworkTargets: [{id: 'plan', title: '기획'}, {id: 'design', title: '설계'}]})];
        renderInbox();
        const card = await screen.findByRole('article');
        fireEvent.click(within(card).getByRole('button', {name: '반려'}));
        const send = within(card).getByRole('button', {name: '반려하기'});
        expect(send).toBeDisabled();
        fireEvent.change(within(card).getByLabelText(/반려 이유/), {target: {value: '범위가 큽니다'}});
        expect(send).toBeDisabled(); // no target chosen yet
        fireEvent.click(within(card).getByLabelText('설계'));
        fireEvent.click(send);
        await waitFor(() => expect(fakeApi.decideApproval).toHaveBeenCalledWith('prj-1', 'apr-1', 1, 'rejected', '범위가 큽니다', ['design']));
    });

    it('shows why a task submission was refused and keeps the form', async () => {
        state.inbox = [inboxItem({kind: 'task', stepTitle: '자료 조사', approvalId: undefined, outputs: [{key: 'notes', type: 'markdown'}]})];
        fakeApi.submitHuman.mockResolvedValueOnce({outcome: {status: '', already: false}, problem: '필수 결과 "notes"가 없습니다'});
        renderInbox();
        const card = await screen.findByRole('article');
        fireEvent.click(within(card).getByRole('button', {name: '제출'}));
        expect(await within(card).findByRole('alert')).toHaveTextContent('필수 결과 "notes"가 없습니다');
        expect(within(card).getByRole('button', {name: '제출'})).toBeInTheDocument();
    });

    it('tells the person when an item was already handled', async () => {
        state.inbox = [inboxItem({kind: 'review', stepTitle: '코드 리뷰', approvalId: undefined})];
        fakeApi.submitReview.mockResolvedValueOnce({outcome: {status: 'succeeded', already: true}, problem: ''});
        renderInbox();
        const card = await screen.findByRole('article');
        fireEvent.click(within(card).getByRole('button', {name: '통과'}));
        expect(await within(card).findByText('이미 처리된 항목입니다.')).toBeInTheDocument();
    });

    it('says when there is nothing to do', async () => {
        renderInbox();
        expect(await screen.findByText('지금 처리할 일이 없습니다.')).toBeInTheDocument();
    });
});

describe('Inbox holds', () => {
    // Review finding 6: holding records a reason but keeps the decision open.
    it('keeps the approval controls after a hold', async () => {
        state.inbox = [inboxItem()];
        fakeApi.decideApproval.mockResolvedValueOnce({outcome: {status: 'waiting_approval', already: false, decision: 'held'}, problem: ''});
        renderInbox();
        const card = await screen.findByRole('article');
        fireEvent.click(within(card).getByRole('button', {name: '보류'}));
        fireEvent.change(within(card).getByLabelText('보류 이유'), {target: {value: '시장 조사 결과를 기다림'}});
        fireEvent.click(within(card).getByRole('button', {name: '보류 기록'}));
        expect(await within(card).findByText(/보류를 기록했습니다/)).toBeInTheDocument();
        expect(within(card).getByRole('button', {name: '승인'})).toBeEnabled();
        expect(within(card).getByRole('button', {name: '반려'})).toBeEnabled();
    });
});
