import {fireEvent, render, screen, waitFor} from '@testing-library/react';
import {beforeEach, describe, expect, it, vi} from 'vitest';
import {fakeApi} from '../test/fakeApi';

vi.mock('../api', () => ({api: fakeApi, errorText: (e: unknown) => String(e)}));

import {LiveProvider} from '../live';
import type {AssistRecord, Project} from '../types';
import {FolderInput} from './FolderInput';
import {WorkflowAssist} from './WorkflowAssist';

const project: Project = {
    id: 'prj-1', organizationId: 'org-default', name: '게임', goal: '', instructions: '', workspacePath: '', budget: {},
    mode: 'review', status: 'active', createdAt: '', updatedAt: '',
};
const conn = (id: string, provider: string) => ({id, name: id, provider, usable: true});
const record = (over: Partial<AssistRecord>): AssistRecord => ({
    id: 'ast-1', projectId: 'prj-1', workflowId: 'wf-1', kind: 'edit', request: '리뷰 추가', baseRevision: 3, connectionId: 'claude',
    model: '', status: 'ready', result: {}, error: '', createdAt: '2026-10-18T06:00:00Z', updatedAt: '', ...over,
});

beforeEach(() => {
    vi.clearAllMocks();
    fakeApi.connections.mockResolvedValue([conn('claude', 'claude'), conn('api', 'claude_api')]);
    fakeApi.assists.mockResolvedValue([]);
});

const renderAssist = (props: Partial<{ dirty: boolean; saveFirst: () => Promise<unknown>; onApplied: () => void }> = {}) => render(
    <LiveProvider><WorkflowAssist project={project} workflowId="wf-1" dirty={props.dirty ?? false}
                                  saveFirst={props.saveFirst ?? (async () => {})} onApplied={props.onApplied ?? (() => {})}/></LiveProvider>);

const confirm = async (label: string) => {
    fireEvent.click(await screen.findByRole('button', {name: label}));
    fireEvent.click(screen.getByRole('button', {name: 'AI 사용량을 써서 요청'}));
};

describe('WorkflowAssist', () => {
    it('saves unsaved edits first, then asks for an edit proposal', async () => {
        const saveFirst = vi.fn(async () => {});
        fakeApi.startAssist.mockResolvedValue(record({status: 'working'}));
        renderAssist({dirty: true, saveFirst});
        fireEvent.change(await screen.findByLabelText(/어떻게 고칠까요/), {target: {value: '보안 검토를 넣어 줘'}});
        await waitFor(() => expect((screen.getByLabelText('AI 연결') as HTMLSelectElement).value).toBe('claude'));
        await confirm('수정안 요청');
        await waitFor(() => expect(fakeApi.startAssist).toHaveBeenCalledWith(
            {projectId: 'prj-1', workflowId: 'wf-1', kind: 'edit', request: '보안 검토를 넣어 줘', connectionId: 'claude', model: ''}));
        expect(saveFirst).toHaveBeenCalled();
    });

    it('offers only connections that can read the skill for diagrams', async () => {
        renderAssist();
        fireEvent.click(screen.getByText('AI 연결·모델'));
        await waitFor(() => expect(screen.getByRole('option', {name: 'api'})).toBeInTheDocument());
        fireEvent.click(screen.getByLabelText(/파이프라인 다이어그램 만들기/));
        await waitFor(() => expect(screen.queryByRole('option', {name: 'api'})).toBeNull());
    });

    it('shows what a proposal changes and applies it', async () => {
        const onApplied = vi.fn();
        fakeApi.assists.mockResolvedValue([record({result: {
            workflow: {schemaVersion: 1, title: '흐름', nodes: []}, notes: '리뷰를 넣었습니다', issues: null, canVersion: true, // Go sends null for no issues
            changes: {added: ['기획 리뷰'], removed: [], changed: ['기획 승인']},
        }})]);
        fakeApi.applyAssist.mockResolvedValue({});
        renderAssist({onApplied});
        expect(await screen.findByText('리뷰를 넣었습니다')).toBeInTheDocument();
        expect(screen.getByText('기획 리뷰')).toBeInTheDocument();
        expect(screen.getByText('검증 문제 없음')).toBeInTheDocument();
        fireEvent.click(screen.getByRole('button', {name: '초안에 적용'}));
        await waitFor(() => expect(onApplied).toHaveBeenCalled());
        expect(fakeApi.applyAssist).toHaveBeenCalledWith('ast-1');
    });

    it('previews a diagram and saves it as HTML', async () => {
        fakeApi.assists.mockResolvedValue([record({kind: 'diagram', request: '', result: {path: 'projects/prj-1/diagrams/ast-1.html', checked: true}})]);
        fakeApi.workflowDiagram.mockResolvedValue('<html><body><svg></svg></body></html>');
        fakeApi.saveWorkflowDiagram.mockResolvedValue('/tmp/x.html');
        renderAssist();
        const frame = await screen.findByTitle('다이어그램 파이프라인');
        expect(frame.getAttribute('sandbox')).toBe('');
        fireEvent.click(screen.getByRole('button', {name: 'HTML로 저장…'}));
        await waitFor(() => expect(fakeApi.saveWorkflowDiagram).toHaveBeenCalledWith('ast-1'));
    });

    it('refines a proposal without applying it', async () => {
        fakeApi.assists.mockResolvedValue([record({result: {
            workflow: {schemaVersion: 1, title: '흐름', nodes: []}, notes: '리뷰 추가', issues: [], canVersion: true,
            changes: {added: [], removed: [], changed: []},
        }})]);
        fakeApi.startAssist.mockResolvedValue(record({id: 'ast-2', status: 'working'}));
        renderAssist();
        fireEvent.click(await screen.findByRole('button', {name: '추가 수정 요청'}));
        const input = screen.getByLabelText(/어떻게 고칠까요/);
        expect(input).toHaveFocus();
        fireEvent.change(input, {target: {value: '승인은 유지해 줘'}});
        await confirm('추가 수정안 요청');
        await waitFor(() => expect(fakeApi.startAssist).toHaveBeenCalledWith(expect.objectContaining({parentId: 'ast-1', request: '승인은 유지해 줘'})));
        expect(fakeApi.applyAssist).not.toHaveBeenCalled();
    });

    it('protects unsaved edits when applying a proposal', async () => {
        fakeApi.assists.mockResolvedValue([record({result: {
            workflow: {schemaVersion: 1, title: '흐름', nodes: []}, notes: '', issues: [], canVersion: true,
            changes: {added: [], removed: [], changed: []},
        }})]);
        renderAssist({dirty: true});
        fireEvent.click(await screen.findByRole('button', {name: '초안에 적용'}));
        expect(await screen.findByText(/먼저 초안을 저장한 뒤/)).toBeInTheDocument();
        expect(fakeApi.applyAssist).not.toHaveBeenCalled();
    });

    it('shows why a job failed', async () => {
        fakeApi.assists.mockResolvedValue([record({status: 'failed', error: '흐름 수정 AI 실패: timeout'})]);
        renderAssist();
        expect(await screen.findByText('흐름 수정 AI 실패: timeout')).toBeInTheDocument();
    });
});

describe('FolderInput', () => {
    it('picks a folder with the system dialog and can clear it', async () => {
        const onChange = vi.fn();
        fakeApi.chooseFolder.mockResolvedValue('/Users/me/dev/villalens');
        const {rerender} = render(<FolderInput value="" onChange={onChange} title="작업 폴더 선택"/>);
        fireEvent.click(screen.getByRole('button', {name: '폴더 선택…'}));
        await waitFor(() => expect(onChange).toHaveBeenCalledWith('/Users/me/dev/villalens'));
        expect(fakeApi.chooseFolder).toHaveBeenCalledWith('작업 폴더 선택', '');
        rerender(<FolderInput value="/Users/me/dev/villalens" onChange={onChange}/>);
        fireEvent.click(screen.getByRole('button', {name: '비우기'}));
        expect(onChange).toHaveBeenLastCalledWith('');
    });

    it('keeps the folder when the dialog is cancelled', async () => {
        const onChange = vi.fn();
        fakeApi.chooseFolder.mockResolvedValue('');
        render(<FolderInput value="/old" onChange={onChange}/>);
        fireEvent.click(screen.getByRole('button', {name: '폴더 선택…'}));
        await waitFor(() => expect(fakeApi.chooseFolder).toHaveBeenCalled());
        expect(onChange).not.toHaveBeenCalled();
    });
});
