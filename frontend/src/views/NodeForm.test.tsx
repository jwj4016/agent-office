import {fireEvent, render, screen} from '@testing-library/react';
import {useState} from 'react';
import {describe, expect, it, vi} from 'vitest';
import {fakeApi} from '../test/fakeApi';

vi.mock('../api', () => ({api: fakeApi, errorText: (e: unknown) => String(e)}));

import type {Assignment, WorkflowSpec} from '../types';
import {NodeForm} from './WorkflowEditor';

const people: Assignment[] = [
    {id: 'a-arch', projectId: 'p', roleId: 'r', actorKind: 'ai', actorId: '', displayName: '설계 AI', connectionId: 'c', model: '', overrides: {}},
    {id: 'a-be', projectId: 'p', roleId: 'r', actorKind: 'ai', actorId: '', displayName: '백엔드 AI', connectionId: 'c', model: '', overrides: {}},
    {id: 'a-me', projectId: 'p', roleId: 'r', actorKind: 'human', actorId: 'local-owner', displayName: '나', connectionId: '', model: '', overrides: {}},
];

const flow = (): WorkflowSpec => ({
    schemaVersion: 1, title: 't', nodes: [
        {id: 'scope', title: '범위', kind: 'task', assignmentId: 'a-arch', dependsOn: [], outputs: [{key: 'scope', type: 'json'}]},
        {id: 'route', title: '분기', kind: 'condition', dependsOn: ['scope']},
        {id: 'design', title: '설계 회의', kind: 'task', assignmentId: 'a-arch', dependsOn: ['scope'], outputs: [{key: 'contract', type: 'markdown'}]},
    ],
});

// Editor renders NodeForm against live state and reports every change.
function Editor({id, onSpec}: { id: string; onSpec: (s: WorkflowSpec) => void }) {
    const [spec, setSpec] = useState(flow());
    return <NodeForm spec={spec} id={id} people={people} readOnly={false}
                     onChange={(s) => { setSpec(s); onSpec(s); }} onRenamed={() => {}} onDeleted={() => {}} onClose={() => {}}/>;
}

const node = (s: WorkflowSpec, id: string) => s.nodes.find((n) => n.id === id)!;

describe('NodeForm', () => {
    it('builds and edits a condition', () => {
        let spec = flow();
        render(<Editor id="route" onSpec={(s) => (spec = s)}/>);
        fireEvent.click(screen.getByRole('button', {name: '분기 구조 만들기'}));
        expect(node(spec, 'route').routing?.source).toEqual({fromStep: 'scope', outputKey: 'scope', fieldPath: ''});
        const join = node(spec, 'route').routing!.joinStep;
        expect(node(spec, join).kind).toBe('join');

        fireEvent.change(screen.getByLabelText('필드 경로'), {target: {value: 'legal.needed'}});
        const value = screen.getByLabelText('비교 값');
        fireEvent.change(value, {target: {value: 'false'}});
        fireEvent.blur(value);
        expect(node(spec, 'route').routing).toMatchObject({source: {fieldPath: 'legal.needed'}, branches: [{operator: 'eq', value: false}]});

        fireEvent.change(screen.getByLabelText('비교 방식'), {target: {value: 'exists'}});
        expect(node(spec, 'route').routing!.branches[0]).toEqual({operator: 'exists', value: undefined, targetStep: node(spec, 'route').routing!.branches[0].targetStep});
        fireEvent.click(screen.getByRole('button', {name: '새 분기 업무와 규칙 추가'}));
        expect(node(spec, 'route').routing!.branches).toHaveLength(2);
    });

    it('turns a task into a meeting of other AI assignees', () => {
        let spec = flow();
        render(<Editor id="design" onSpec={(s) => (spec = s)}/>);
        fireEvent.click(screen.getByLabelText('회의(협의 작업)로 진행'));
        // The decider (설계 AI) and people are not offered as participants.
        expect(screen.queryByRole('checkbox', {name: '설계 AI'})).not.toBeInTheDocument();
        expect(screen.queryByRole('checkbox', {name: '나'})).not.toBeInTheDocument();
        fireEvent.click(screen.getByRole('checkbox', {name: '백엔드 AI'}));
        fireEvent.change(screen.getByLabelText('최대 라운드'), {target: {value: '2'}});
        expect(node(spec, 'design').meeting).toEqual({participants: ['a-be'], maxRounds: 2});
    });

    it('edits completion commands and limits', () => {
        let spec = flow();
        render(<Editor id="design" onSpec={(s) => (spec = s)}/>);
        fireEvent.click(screen.getByRole('button', {name: '검증 명령 추가'}));
        fireEvent.change(screen.getByLabelText('실행 파일'), {target: {value: 'go'}});
        const args = screen.getByLabelText('인수');
        fireEvent.change(args, {target: {value: 'test\n./...\n'}});
        fireEvent.blur(args);
        fireEvent.change(screen.getByLabelText('최대 수정 회차'), {target: {value: '1'}});
        expect(node(spec, 'design').completion).toEqual({commands: [{executable: 'go', args: ['test', './...']}]});
        expect(node(spec, 'design').limits).toEqual({maxRevisions: 1});
        fireEvent.click(screen.getAllByRole('button', {name: '삭제'}).at(-1)!);
        expect(node(spec, 'design').completion).toBeUndefined();
    });
});
