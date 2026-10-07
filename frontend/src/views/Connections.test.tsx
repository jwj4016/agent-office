import {fireEvent, render, screen, waitFor, within} from '@testing-library/react';
import {beforeEach, describe, expect, it, vi} from 'vitest';
import {fakeApi} from '../test/fakeApi';

vi.mock('../api', () => ({api: fakeApi, errorText: (e: unknown) => String(e)}));

import {LiveProvider} from '../live';
import type {Connection} from '../types';
import {Connections} from './Connections';

const conn = (over: Partial<Connection> = {}): Connection => ({
    id: 'conn-1', name: '내 Claude', provider: 'claude', executablePath: '', secretRef: '', config: {}, verifiedCapabilities: {},
    usable: false, note: '실제 호출 시험 전이라 실행할 수 없습니다', hasKey: false, settings: {authMode: 'api_key'}, ...over,
});

const renderIt = () => render(<LiveProvider><Connections/></LiveProvider>);

beforeEach(() => vi.clearAllMocks());

describe('Connections', () => {
    it('stores a key without showing it again and asks before a real call', async () => {
        fakeApi.connections.mockResolvedValue([conn()]);
        renderIt();
        const card = await screen.findByRole('region', {name: '연결 내 Claude'}).catch(() => screen.findByLabelText('연결 내 Claude'));
        fireEvent.change(within(card).getByLabelText('API 키'), {target: {value: 'sk-ant-xyz'}});
        fireEvent.click(within(card).getByRole('button', {name: '키 저장'}));
        await waitFor(() => expect(fakeApi.setConnectionKey).toHaveBeenCalledWith('conn-1', 'sk-ant-xyz'));
        expect(within(card).getByLabelText('API 키')).toHaveValue('');

        fireEvent.click(within(card).getByRole('button', {name: '실제 호출 시험'}));
        expect(fakeApi.testConnectionCall).not.toHaveBeenCalled(); // needs a second, explicit click
        fireEvent.click(within(card).getByRole('button', {name: 'AI 사용량을 써서 시험'}));
        await waitFor(() => expect(fakeApi.testConnectionCall).toHaveBeenCalledWith('conn-1', ''));
        expect(await within(card).findByText(/실제 호출까지 확인됨/)).toBeInTheDocument();
    });

    it('warns when choosing the personal Claude login', async () => {
        fakeApi.connections.mockResolvedValue([]);
        renderIt();
        fireEvent.click(await screen.findByRole('button', {name: '연결 추가'}));
        const form = screen.getByRole('form', {name: '연결 편집'});
        expect(within(form).queryByRole('note')).not.toBeInTheDocument();
        fireEvent.click(within(form).getByLabelText(/이 PC의 Claude 로그인/));
        expect(within(form).getByRole('note')).toHaveTextContent('본인 PC에서 본인만');
        fireEvent.change(within(form).getByLabelText('이름'), {target: {value: '개인 Claude'}});
        fireEvent.click(within(form).getByRole('button', {name: '저장'}));
        await waitFor(() => expect(fakeApi.saveConnection).toHaveBeenCalledWith(expect.objectContaining({
            provider: 'claude', settings: expect.objectContaining({authMode: 'local_login'}),
        })));
    });
});
