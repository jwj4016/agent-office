import {fireEvent, render, screen} from '@testing-library/react';
import {beforeEach, describe, expect, it, vi} from 'vitest';
import {fakeApi} from '../test/fakeApi';

vi.mock('../api', () => ({api: fakeApi, errorText: (e: unknown) => String(e)}));

import {ExportButton} from './ExportButton';

beforeEach(() => vi.clearAllMocks());

describe('ExportButton', () => {
    it('reports where the export went, what was masked and what was missing', async () => {
        fakeApi.exportProject.mockResolvedValue({path: '/Users/me/게임-내보내기.zip', projectName: '게임', tables: 14, rows: 120, artifacts: 9,
            missing: ['projects/p/artifacts/x.md'], masked: 2});
        render(<ExportButton projectId="prj-1"/>);
        fireEvent.click(screen.getByRole('button', {name: '내보내기 (키 제외)'}));
        const status = await screen.findByRole('status');
        expect(fakeApi.exportProject).toHaveBeenCalledWith('prj-1');
        expect(status).toHaveTextContent('/Users/me/게임-내보내기.zip');
        expect(status).toHaveTextContent('기록 120건 · 결과 파일 9개 · 키로 보이는 값 2곳을 가렸습니다');
        expect(status).toHaveTextContent('찾지 못한 결과 파일 1개');
    });

    it('shows nothing when the save dialog was cancelled', async () => {
        fakeApi.exportProject.mockResolvedValue({path: '', projectName: '', tables: 0, rows: 0, artifacts: 0, missing: [], masked: 0});
        render(<ExportButton projectId="prj-1"/>);
        fireEvent.click(screen.getByRole('button', {name: '내보내기 (키 제외)'}));
        await screen.findByRole('button', {name: '내보내기 (키 제외)'});
        expect(screen.queryByRole('status')).not.toBeInTheDocument();
    });
});
