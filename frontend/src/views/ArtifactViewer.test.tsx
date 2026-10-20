import {render, screen} from '@testing-library/react';
import {describe, expect, it} from 'vitest';
import {CodeChangeView, DiagramView} from './ArtifactViewer';

describe('CodeChangeView', () => {
    it('lists changed files and the diff recorded from Git', () => {
        const content = JSON.stringify({
            baseCommit: 'aaaaaaaaaaaa', startCommit: 'aaaaaaaaaaaa', commit: 'bbbbbbbbbbbb', branch: 'agent-office/r1/integrate-g1-a1',
            changes: [{path: 'api/api.go', status: 'added'}, {path: 'web/web.go', status: 'modified'}],
            diffStat: {files: 2, insertions: 5, deletions: 1}, patch: '+package api', patchTruncated: false, summary: 'API 추가',
        });
        render(<CodeChangeView content={content}/>);
        expect(screen.getByText('api/api.go')).toBeInTheDocument();
        expect(screen.getByText('수정')).toBeInTheDocument();
        expect(screen.getByText('+package api')).toBeInTheDocument();
        expect(screen.getByText('요약: API 추가')).toBeInTheDocument();
        expect(screen.getByText(/git merge agent-office\/r1\/integrate-g1-a1/)).toBeInTheDocument();
    });

    it('falls back to raw JSON for results written without Git', () => {
        render(<CodeChangeView content={'{"changes": []}'}/>);
        expect(screen.getByText(/"changes"/)).toBeInTheDocument();
    });
});

describe('DiagramView', () => {
    it('previews the diagram in a frame with no permissions and keeps the source', () => {
        const html = '<html><body><svg role="img"><title>아키텍처</title></svg></body></html>';
        render(<DiagramView content={html} title="arch.arch"/>);
        const frame = screen.getByTitle('다이어그램 arch.arch');
        expect(frame.getAttribute('sandbox')).toBe('');
        expect(frame.getAttribute('srcdoc')).toBe(html);
        expect(screen.getByText('HTML 원본')).toBeInTheDocument();
    });
});
