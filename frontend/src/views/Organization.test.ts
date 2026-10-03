import {describe, expect, it} from 'vitest';
import type {Role} from '../types';
import {roleTree} from './Organization';

const role = (id: string, name: string, parentRoleId = ''): Role => ({
    id, name, parentRoleId, organizationId: 'org-default', mission: '', instructions: '', outputDefaults: [], policy: {}, appearance: {}, updatedAt: '',
});

describe('roleTree', () => {
    it('orders parents before children with depth', () => {
        const tree = roleTree([role('be', '백엔드', 'dev'), role('cto', 'CTO'), role('dev', '개발자', 'cto'), role('legal', '법률')]);
        expect(tree.map((x) => `${x.depth}:${x.role.name}`)).toEqual(['0:법률', '0:CTO', '1:개발자', '2:백엔드']);
    });
    it('treats a missing parent as top level', () => {
        expect(roleTree([role('a', 'A', 'gone')])[0].depth).toBe(0);
    });
});
