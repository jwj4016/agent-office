import {fireEvent, render, screen, waitFor, within} from '@testing-library/react';
import {type ReactNode, useState} from 'react';
import {beforeEach, describe, expect, it, vi} from 'vitest';
import {fakeApi} from '../test/fakeApi';

vi.mock('../api', () => ({api: fakeApi, errorText: (e: unknown) => String(e)}));

import {LiveProvider} from '../live';
import type {Department, DocumentVersion, ImpactCheck, LinkedDocument, Project, SharedDocument, WorkItem} from '../types';
import {DetailProvider, ToastProvider} from '../ui';
import {Documents, ShareResult, WorkDocuments} from './Documents';

const project: Project = {id: 'prj-1', organizationId: 'org-default', name: '캠페인', goal: '', instructions: '', workspacePath: 'C:/work',
    budget: {}, mode: 'review', status: 'active', createdAt: '', updatedAt: ''};
const dept = (id: string, name: string): Department => ({
    id, projectId: 'prj-1', name, responsibility: '', leadAssignmentId: '', memberIds: [], templateId: '', automation: {}, limits: {},
    status: 'active', createdAt: '', updatedAt: '',
});
const version = (over: Partial<DocumentVersion>): DocumentVersion => ({
    id: 'v1', documentId: 'doc-1', projectId: 'prj-1', number: 1, stage: 'draft', confirmedBy: '', confirmedAt: '', confirmationNote: '',
    sourceKind: 'artifact', sourceArtifactId: 'art-1', sourcePath: '', fileName: 'result.md', path: 'p', hash: 'h', size: 120, withdrawnAt: '',
    createdAt: '2026-10-23T01:00:00Z', ...over,
});
const doc = (over: Partial<SharedDocument> = {}): SharedDocument => ({
    id: 'doc-1', projectId: 'prj-1', title: '캠페인 기획안', categories: ['기획'], ownerDepartmentId: 'mkt', workItemId: 'w-plan', scope: 'service',
    scopeDepartments: [], status: 'active', createdAt: '', updatedAt: '2026-10-23T01:00:00Z', versions: [version({})], ...over,
});
const item: WorkItem = {
    id: 'w-dev', projectId: 'prj-1', kind: 'work', parentId: '', departmentId: 'dev', assigneeAssignmentId: '', title: '구현 검토', description: '',
    doneCriteria: '', priority: 'normal', status: 'in_progress', statusReason: '', categories: ['개발'], dueAt: '', createdAt: '', updatedAt: '', closedAt: '',
};

function Shell({children}: { children: ReactNode }) {
    const [detail, setDetail] = useState<ReactNode | null>(null);
    return (
        <LiveProvider><ToastProvider><DetailProvider value={setDetail}>
            {children}
            {detail ? <aside aria-label="상세">{detail}</aside> : null}
        </DetailProvider></ToastProvider></LiveProvider>
    );
}

beforeEach(() => {
    vi.clearAllMocks();
    fakeApi.departments.mockResolvedValue([dept('mkt', '마케팅팀'), dept('dev', '개발팀')]);
    fakeApi.categories.mockResolvedValue(['기획', '개발']);
    fakeApi.documents.mockResolvedValue([doc()]);
    fakeApi.workDocuments.mockResolvedValue([]);
    fakeApi.impactChecks.mockResolvedValue([]);
});

describe('shared library', () => {
    it('lists documents and confirms a draft without changing it', async () => {
        fakeApi.document.mockResolvedValue(doc());
        fakeApi.confirmDocumentVersion.mockResolvedValue(version({stage: 'final', confirmedAt: 'x'}));
        render(<Shell><Documents project={project}/></Shell>);
        const table = await screen.findByRole('table');
        expect(within(table).getByText('마케팅팀')).toBeInTheDocument();
        expect(within(table).getByText('1판 초안')).toBeInTheDocument();
        fireEvent.click(within(table).getByRole('button', {name: '캠페인 기획안 자료 보기'}));
        const pane = await screen.findByRole('complementary', {name: '상세'});
        fireEvent.click(await within(pane).findByRole('button', {name: '1판 확정'}));
        await waitFor(() => expect(fakeApi.confirmDocumentVersion).toHaveBeenCalledWith('prj-1', 'v1', ''));
    });

    it('shares a work folder file with a limited scope', async () => {
        fakeApi.shareWorkspaceFile.mockResolvedValue(version({fileName: 'budget.md'}));
        render(<Shell><Documents project={project}/></Shell>);
        fireEvent.click(await screen.findByRole('button', {name: '작업 폴더 파일 공유'}));
        const form = screen.getByRole('form', {name: '작업 폴더 파일 공유'});
        fireEvent.change(within(form).getByLabelText(/파일 경로/), {target: {value: 'docs/budget.md'}});
        fireEvent.change(within(form).getByLabelText('자료 제목'), {target: {value: '예산 초안'}});
        fireEvent.click(within(form).getByLabelText('지정한 부서만'));
        fireEvent.click(within(form).getByRole('checkbox', {name: '개발팀'}));
        fireEvent.click(within(form).getByRole('button', {name: '공유'}));
        await waitFor(() => expect(fakeApi.shareWorkspaceFile).toHaveBeenCalledWith('prj-1', 'docs/budget.md', expect.objectContaining({
            title: '예산 초안', scope: 'departments', scopeDepartments: ['dev'],
        })));
    });
});

describe('sharing a run result', () => {
    it('adds a new version to a document this work already shared', async () => {
        fakeApi.documents.mockResolvedValue([doc({workItemId: 'w-dev'})]);
        fakeApi.shareArtifact.mockResolvedValue(version({number: 2}));
        const onDone = vi.fn();
        render(<Shell><ShareResult project={project} artifactId="art-9" item={item} departments={[]} categories={[]} onDone={onDone}/></Shell>);
        const form = await screen.findByRole('form', {name: '결과 공유'});
        fireEvent.change(await within(form).findByLabelText('올릴 곳'), {target: {value: 'doc-1'}});
        fireEvent.click(within(form).getByRole('button', {name: '공유'}));
        await waitFor(() => expect(fakeApi.shareArtifact).toHaveBeenCalledWith('prj-1', 'art-9', {documentId: 'doc-1'}));
        await waitFor(() => expect(onDone).toHaveBeenCalled());
    });
});

describe('work documents', () => {
    it('links a visible document, shows a newer version and closes an impact check', async () => {
        const linked: LinkedDocument = {version: version({}), document: doc({versions: [version({}), version({id: 'v2', number: 2})]}), newer: 2, linkedAt: ''};
        const check: ImpactCheck = {id: 'imp-1', projectId: 'prj-1', workItemId: 'w-dev', cause: 'revised', refId: 'v2', documentId: 'doc-1',
            summary: '‘캠페인 기획안’ 2판이 공유되었습니다', status: 'open', note: '', createdAt: ''};
        fakeApi.workDocuments.mockResolvedValue([linked]);
        fakeApi.impactChecks.mockResolvedValue([check]);
        fakeApi.documents.mockResolvedValue([doc({id: 'doc-2', title: '예산표', versions: [version({id: 'b1', documentId: 'doc-2'})]})]);
        fakeApi.document.mockResolvedValue(linked.document);
        render(<Shell><WorkDocuments project={project} item={item}/></Shell>);
        expect(await screen.findByText('2판 있음')).toBeInTheDocument();
        expect(fakeApi.documents).toHaveBeenCalledWith('prj-1', {visibleTo: 'dev'});
        fireEvent.click(screen.getByRole('button', {name: '2판으로 바꾸기'}));
        await waitFor(() => expect(fakeApi.linkDocument).toHaveBeenCalledWith('prj-1', 'w-dev', 'v2'));
        fireEvent.click(screen.getByRole('button', {name: '확인함'}));
        await waitFor(() => expect(fakeApi.closeImpactCheck).toHaveBeenCalledWith('prj-1', 'imp-1', 'resolved', ''));
        const form = screen.getByRole('form', {name: '자료 연결'});
        fireEvent.change(await within(form).findByLabelText('연결할 자료'), {target: {value: 'doc-2'}});
        fireEvent.click(within(form).getByRole('button', {name: '연결'}));
        await waitFor(() => expect(fakeApi.linkDocument).toHaveBeenCalledWith('prj-1', 'w-dev', 'b1'));
    });
});
