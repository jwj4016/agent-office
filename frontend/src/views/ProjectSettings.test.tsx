import {fireEvent, render, screen, waitFor} from '@testing-library/react';
import {beforeEach, describe, expect, it, vi} from 'vitest';
import {fakeApi} from '../test/fakeApi';

vi.mock('../api', () => ({api: fakeApi, errorText: (e: unknown) => String(e)}));

import {LiveProvider} from '../live';
import {ToastProvider} from '../ui';
import type {Project} from '../types';
import {ProjectSettings} from './ProjectView';

const project: Project = {
    id: 'prj-1', organizationId: 'org-default', name: '빌라노트', goal: '', instructions: '', workspacePath: '', budget: {},
    mode: 'auto', status: 'active', createdAt: '', updatedAt: '',
};

const show = (p: Project) => render(<LiveProvider><ToastProvider><ProjectSettings project={p} onSaved={() => {}}/></ToastProvider></LiveProvider>);

beforeEach(() => vi.clearAllMocks());

describe('ProjectSettings budget', () => {
    it('saves a daily budget', async () => {
        show(project);
        fireEvent.change(await screen.findByLabelText('예산 기간'), {target: {value: 'day'}});
        fireEvent.change(screen.getByLabelText(/하루 비용 예산/), {target: {value: '5'}});
        fireEvent.click(screen.getByRole('button', {name: '저장'}));
        await waitFor(() => expect(fakeApi.updateProject).toHaveBeenCalledWith(expect.objectContaining({
            budget: {maxCostUsd: 5, period: 'day'},
        })));
    });

    it("shows today's usage for a daily budget", async () => {
        fakeApi.projectUsage.mockResolvedValueOnce({inputTokens: 10, outputTokens: 5, costUsd: 1.25, unknownCostAttempts: 0, attempts: 2,
            budget: {maxCostUsd: 5, period: 'day'}, since: '2026-11-02T15:00:00Z'});
        show({...project, budget: {maxCostUsd: 5, period: 'day'}});
        expect(await screen.findByText('오늘(0시부터)')).toBeInTheDocument();
    });
});
