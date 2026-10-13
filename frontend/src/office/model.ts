// The office shows each assignment's real state (spec §11). Everything
// here is derived from stored run steps and inbox items; animation never
// decides a state, and a person's activity outside the app is not guessed.
import type {ArtifactView, Assignment, InboxItem, InboxKind, Role, RunDetail, RunStatus, StepStatus, StepView} from '../types';

export type DeskState = 'idle' | 'working' | 'meeting' | 'waiting' | 'checking' | 'done' | 'problem';

// DeskWork is one step this assignment owns or takes part in.
export type DeskWork = {
    runId: string; runTitle: string; versionNumber: number; runStatus: RunStatus;
    stepId: string; stepTitle: string; status: StepStatus; state: DeskState;
    error?: string; artifacts: ArtifactView[];
    // inbox lists the cards waiting for the person about this step.
    inbox: InboxKind[];
    canRetry: boolean;
    // participant: a meeting the assignment joins but does not decide.
    participant: boolean;
};

export type Desk = {
    assignment: Assignment; roleName: string; state: DeskState; label: string; summary: string; work: DeskWork[];
};

export type MeetingView = { runId: string; stepId: string; title: string; members: string[] };

export type Office = { desks: Desk[]; meetings: MeetingView[] };

// Higher wins when one assignment has several steps.
const priority: Record<DeskState, number> = {idle: 0, done: 1, checking: 2, working: 3, meeting: 4, waiting: 5, problem: 6};

const activeRun = (s: RunStatus) => s === 'running' || s === 'waiting' || s === 'paused' || s === 'failed' || s === 'interrupted';

export function stepState(s: StepView, inbox: InboxKind[]): DeskState | null {
    switch (s.status) {
        case 'failed':
        case 'interrupted':
            return 'problem';
        case 'waiting_human':
        case 'waiting_approval':
        case 'waiting_input':
            return 'waiting';
        case 'running':
            // A tool permission, question or budget hold blocks the AI on a person.
            if (inbox.length) return 'waiting';
            return s.participants?.length ? 'meeting' : 'working';
        case 'verifying':
            return 'checking';
        case 'succeeded':
            return 'done';
        default:
            return null;
    }
}

export function deskLabel(state: DeskState, human: boolean): string {
    switch (state) {
        case 'idle': return '대기 중';
        case 'working': return '작업 중';
        case 'meeting': return '회의 중';
        case 'waiting': return human ? '내 할 일 있음' : '사람 응답 대기';
        case 'checking': return '결과 확인 중';
        case 'done': return '완료';
        case 'problem': return '문제 발생';
    }
}

export function buildOffice(assignments: Assignment[], roles: Role[], runs: RunDetail[], inbox: InboxItem[]): Office {
    const roleName = new Map(roles.map((r) => [r.id, r.name]));
    const work = new Map<string, DeskWork[]>(assignments.map((a) => [a.id, []]));
    const meetings: MeetingView[] = [];
    const cards = (runId: string, stepId: string) =>
        inbox.filter((i) => i.runId === runId && i.stepId === stepId).map((i) => i.kind);

    for (const run of runs) {
        if (!activeRun(run.status)) continue;
        for (const s of run.steps) {
            if (!s.assignmentId) continue;
            const kinds = cards(run.id, s.id);
            const state = stepState(s, kinds);
            if (!state) continue;
            const base = {
                runId: run.id, runTitle: run.title, versionNumber: run.versionNumber, runStatus: run.status,
                stepId: s.id, stepTitle: s.title, status: s.status, error: s.error, artifacts: s.artifacts,
                canRetry: state === 'problem', // cancelled runs are not shown at all
            };
            work.get(s.assignmentId)?.push({...base, state, inbox: kinds, participant: false});
            if (state === 'meeting') {
                const members = [s.assignmentId, ...(s.participants ?? [])];
                meetings.push({runId: run.id, stepId: s.id, title: s.title, members});
                for (const p of s.participants ?? []) {
                    work.get(p)?.push({...base, state: 'meeting', inbox: [], canRetry: false, participant: true});
                }
            }
        }
    }

    const desks = assignments.map((a): Desk => {
        const list = (work.get(a.id) ?? []).sort((x, y) => priority[y.state] - priority[x.state]);
        const top = list[0];
        const state = top?.state ?? 'idle';
        return {
            assignment: a, roleName: roleName.get(a.roleId) ?? '', state, label: deskLabel(state, a.actorKind === 'human'),
            summary: top ? `${top.stepTitle} · ${top.runTitle}` : '', work: list,
        };
    });
    return {desks, meetings};
}

// describe is the text shown to screen readers and in the list view.
export function describe(d: Desk): string {
    const who = `${d.assignment.displayName} (${d.assignment.actorKind === 'ai' ? 'AI' : '사람'}${d.roleName ? ', ' + d.roleName : ''})`;
    return d.summary ? `${who}: ${d.label} — ${d.summary}` : `${who}: ${d.label}`;
}
