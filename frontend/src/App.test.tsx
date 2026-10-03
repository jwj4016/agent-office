import {fireEvent, render, screen, waitFor, within} from '@testing-library/react';
import {beforeEach, describe, expect, it, vi} from 'vitest';
import {fakeApi, inboxItem, state} from './test/fakeApi';

vi.mock('./api', () => ({api: fakeApi, errorText: (e: unknown) => String(e)}));

import App from './App';

beforeEach(() => {
    state.summaries = [];
    state.inbox = [];
    state.settings = {};
    vi.clearAllMocks();
});

describe('App shell', () => {
    it('creates a service from the empty dashboard and opens it', async () => {
        render(<App/>);
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
