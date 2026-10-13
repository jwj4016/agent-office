import {describe, expect, it} from 'vitest';
import type {Assignment} from '../types';
import type {Office} from './model';
import {hit, layout, SCENE_W, seats} from './scene';

const desk = (id: string) => ({
    assignment: {id, displayName: id, actorKind: 'ai'} as Assignment, roleName: '', state: 'idle' as const, label: '', summary: '', work: [],
});

describe('scene layout', () => {
    it('fits desks in three columns left of the meeting room and grows downward', () => {
        const l = layout(20);
        expect(l.desks).toHaveLength(20);
        for (const d of l.desks) expect(d.cell.x + d.cell.w).toBeLessThan(l.room.x);
        expect(l.room.x + l.room.w).toBeLessThanOrEqual(SCENE_W);
        expect(l.height).toBeGreaterThan(l.desks[19].cell.y + l.desks[19].cell.h);
        expect(layout(2).height).toBe(180);
    });

    it('moves meeting members to the table and finds them there', () => {
        const office: Office = {desks: ['a', 'b', 'c'].map(desk), meetings: [{runId: 'r', stepId: 's', title: '회의', members: ['c', 'a']}]};
        const l = layout(3);
        const where = seats(office, l);
        expect(where.get('c')).toMatchObject({...l.roomSeats[0], inMeeting: true});
        expect(where.get('b')).toMatchObject({inMeeting: false});
        const c = where.get('c')!;
        expect(hit(office, l, c.x + 3, c.y + 4)).toBe(2);
        const b = l.desks[1].cell;
        expect(hit(office, l, b.x + 5, b.y + 5)).toBe(1);
        expect(hit(office, l, l.lounge.x + 2, l.lounge.y + 2)).toBe(-1);
    });
});
