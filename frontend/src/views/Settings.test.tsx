import {fireEvent, render, screen, waitFor, within} from '@testing-library/react';
import {beforeEach, describe, expect, it, vi} from 'vitest';
import {fakeApi, state} from '../test/fakeApi';

vi.mock('../api', () => ({api: fakeApi, errorText: (e: unknown) => String(e)}));

import {LiveProvider} from '../live';
import {Settings} from './Settings';

beforeEach(() => {
    vi.clearAllMocks();
    state.settings = {};
    fakeApi.connections.mockResolvedValue([]);
});

describe('Settings', () => {
    it('changes how many AI steps run at once', async () => {
        render(<LiveProvider><Settings/></LiveProvider>);
        const [group] = await screen.findAllByRole('group', {name: '동시에 실행할 AI 업무 수'});
        await waitFor(() => expect(within(group).getByRole('button', {name: '2개'})).toHaveAttribute('aria-pressed', 'true'));

        fireEvent.click(within(group).getByRole('button', {name: '3개'}));
        await waitFor(() => expect(fakeApi.setSetting).toHaveBeenCalledWith('engine.maxActive', '3'));
        await waitFor(() => expect(within(group).getByRole('button', {name: '3개'})).toHaveAttribute('aria-pressed', 'true'));
        expect(within(group).getByRole('button', {name: '2개'})).toHaveAttribute('aria-pressed', 'false');
    });
});
