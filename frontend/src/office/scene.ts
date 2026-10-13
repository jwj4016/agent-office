// Pixel office drawing. All coordinates are in scene pixels (320 wide);
// the canvas scales them by a whole number so pixels stay crisp.
import type {Desk, DeskState, Office} from './model';

export const SCENE_W = 320;
const COLS = 3;
const CELL_W = 72;
const CELL_H = 58;
const TOP = 18;
const ROOM_X = 234;
const ROOM_W = 80;

export type Rect = { x: number; y: number; w: number; h: number };
export type Layout = {
    height: number;
    // desks[i] is desk i's cell; seat is where its character sits.
    desks: { cell: Rect; seat: { x: number; y: number } }[];
    room: Rect;
    lounge: Rect;
    // meeting seats in order (decider first).
    roomSeats: { x: number; y: number }[];
};

export function layout(deskCount: number): Layout {
    const rows = Math.max(2, Math.ceil(deskCount / COLS));
    const height = Math.max(180, TOP + rows * CELL_H + 8);
    const desks = Array.from({length: deskCount}, (_, i) => {
        const x = 8 + (i % COLS) * CELL_W;
        const y = TOP + Math.floor(i / COLS) * CELL_H;
        return {cell: {x, y, w: CELL_W - 4, h: CELL_H - 4}, seat: {x: x + 30, y: y + 14}};
    });
    const room = {x: ROOM_X, y: TOP, w: ROOM_W, h: 84};
    const lounge = {x: ROOM_X, y: TOP + 92, w: ROOM_W, h: height - TOP - 100};
    // Table in the middle of the room; seats around it.
    const tx = room.x + 22, ty = room.y + 34;
    const roomSeats = [
        {x: tx - 12, y: ty - 2}, {x: tx + 40, y: ty - 2}, {x: tx + 6, y: ty - 18}, {x: tx + 22, y: ty - 18},
        {x: tx + 6, y: ty + 16}, {x: tx + 22, y: ty + 16},
    ];
    return {height, desks, room, lounge, roomSeats};
}

// seatOf says where an assignment's character is drawn: at the meeting
// table while a meeting it belongs to runs, otherwise at its own desk.
export function seats(office: Office, l: Layout): Map<string, { x: number; y: number; inMeeting: boolean }> {
    const out = new Map<string, { x: number; y: number; inMeeting: boolean }>();
    office.desks.forEach((d, i) => out.set(d.assignment.id, {...l.desks[i].seat, inMeeting: false}));
    const meeting = office.meetings[0]; // one table; other meetings stay listed in text
    if (meeting) {
        meeting.members.slice(0, l.roomSeats.length).forEach((id, i) => {
            if (out.has(id)) out.set(id, {...l.roomSeats[i], inMeeting: true});
        });
    }
    return out;
}

// hit returns the desk index under a scene point (a desk cell or the
// character's meeting seat), or -1.
export function hit(office: Office, l: Layout, x: number, y: number): number {
    const where = seats(office, l);
    for (let i = 0; i < office.desks.length; i++) {
        const s = where.get(office.desks[i].assignment.id)!;
        if (s.inMeeting && x >= s.x - 2 && x <= s.x + 10 && y >= s.y - 8 && y <= s.y + 14) return i;
    }
    for (let i = 0; i < l.desks.length; i++) {
        const c = l.desks[i].cell;
        if (x >= c.x && x <= c.x + c.w && y >= c.y && y <= c.y + c.h) return i;
    }
    return -1;
}

const pal = {
    floor: '#fbefd9', floor2: '#f6e6c8', wall: '#efdcc0', wallLine: '#e2cba8',
    wood: '#c99a6b', woodDark: '#a87a50', chair: '#8f7aa8',
    monitor: '#4b4458', screenOff: '#6d6680', screenOn: ['#bfe8d8', '#9fdcc4', '#d9cff5'],
    mint: '#bfe8d8', lavender: '#d9cff5', apricot: '#ffd3b0', ink: '#3b3247', muted: '#7c7189',
    rug: '#e9e1f7', table: '#d8b48a', plant: '#7cc4a0', pot: '#d98f5f', sofa: '#ffc79e',
    ok: '#3fa57a', warn: '#e8913a', bad: '#c0392b', focus: '#6b55d9', paper: '#ffffff',
};
const shirts = ['#9fd8c2', '#c3b4ef', '#ffbf92', '#a9cdf0', '#f5b3c8', '#e6d27a'];
const hairs = ['#5b3a29', '#2f2a35', '#8a4b2a', '#d4a85a', '#6b6375'];
const skins = ['#ffdcc0', '#f2c39b', '#d9a77c', '#b98260'];

function hash(s: string): number {
    let h = 2166136261;
    for (let i = 0; i < s.length; i++) h = Math.imul(h ^ s.charCodeAt(i), 16777619);
    return h >>> 0;
}

export type Frame = {
    // tick advances about 6 times a second; 0 when motion is reduced.
    tick: number;
    selected: number;
    // joy lists desks that just finished (a short hop).
    joy: Set<string>;
};

export function draw(ctx: CanvasRenderingContext2D, office: Office, l: Layout, f: Frame) {
    const r = (x: number, y: number, w: number, h: number, c: string) => { ctx.fillStyle = c; ctx.fillRect(x, y, w, h); };
    // Floor and wall.
    r(0, 0, SCENE_W, l.height, pal.floor);
    for (let y = TOP; y < l.height; y += 8) for (let x = (y / 8) % 2 ? 0 : 8; x < SCENE_W; x += 16) r(x, y, 8, 8, pal.floor2);
    r(0, 0, SCENE_W, TOP - 4, pal.wall);
    r(0, TOP - 5, SCENE_W, 1, pal.wallLine);
    for (const wx of [24, 96, 168]) { r(wx, 3, 26, 8, '#cfe9f5'); r(wx + 12, 3, 2, 8, pal.wallLine); }

    drawRoom(r, ctx, l, office);
    drawLounge(r, l);

    const where = seats(office, l);
    office.desks.forEach((d, i) => {
        const {cell} = l.desks[i];
        const s = where.get(d.assignment.id)!;
        drawDesk(r, ctx, d, cell, s.inMeeting, f);
        if (!s.inMeeting) drawPerson(r, d, s.x, s.y, f);
    });
    // People at the meeting table are drawn over the room.
    office.desks.forEach((d) => {
        const s = where.get(d.assignment.id)!;
        if (s.inMeeting) drawPerson(r, d, s.x, s.y, f);
    });
    if (office.meetings.length && f.tick % 6 < 4) {
        // Talking dots over the table.
        const t = l.roomSeats[0];
        [0, 4, 8].forEach((dx, i) => { if (f.tick % 6 >= i) r(t.x + 22 + dx, t.y - 26, 2, 2, pal.ink); });
    }
    if (f.selected >= 0 && f.selected < l.desks.length) {
        const c = l.desks[f.selected].cell;
        ctx.strokeStyle = pal.focus;
        ctx.lineWidth = 1;
        ctx.setLineDash([2, 2]);
        ctx.strokeRect(c.x + 0.5, c.y + 0.5, c.w - 1, c.h - 1);
        ctx.setLineDash([]);
    }
}

type R = (x: number, y: number, w: number, h: number, c: string) => void;

function drawRoom(r: R, ctx: CanvasRenderingContext2D, l: Layout, office: Office) {
    const {room} = l;
    r(room.x, room.y, room.w, room.h, pal.rug);
    r(room.x, room.y, room.w, 1, pal.wallLine);
    r(room.x, room.y + room.h - 1, room.w, 1, pal.wallLine);
    r(room.x, room.y, 1, room.h, pal.wallLine);
    r(room.x + room.w - 1, room.y, 1, room.h, pal.wallLine);
    const t = l.roomSeats[0];
    r(t.x + 10, t.y - 6, 36, 20, pal.table);
    r(t.x + 10, t.y + 14, 36, 2, pal.woodDark);
    label(ctx, office.meetings.length ? `회의 중 · ${office.meetings[0].title}` : '회의실', room.x + 3, room.y + room.h - 3, room.w - 6);
}

function drawLounge(r: R, l: Layout) {
    const {lounge} = l;
    if (lounge.h < 30) return;
    r(lounge.x + 6, lounge.y + 10, 40, 10, pal.sofa);
    r(lounge.x + 6, lounge.y + 6, 40, 5, '#f7b384');
    // Plants and a coffee machine.
    for (const px of [lounge.x + 54, lounge.x + 66]) {
        r(px, lounge.y + 4, 8, 8, pal.plant);
        r(px + 2, lounge.y + 1, 4, 4, pal.plant);
        r(px + 1, lounge.y + 12, 6, 6, pal.pot);
    }
    r(lounge.x + 8, lounge.y + 28, 10, 14, '#8d8199');
    r(lounge.x + 10, lounge.y + 31, 6, 3, pal.mint);
}

function drawDesk(r: R, ctx: CanvasRenderingContext2D, d: Desk, c: Rect, away: boolean, f: Frame) {
    const x = c.x, y = c.y;
    // Chair behind the person, desk and monitor in front.
    r(x + 28, y + 16, 12, 12, pal.chair);
    if (away) r(x + 29, y + 10, 10, 8, pal.chair);
    r(x + 6, y + 28, 56, 10, pal.wood);
    r(x + 6, y + 38, 56, 2, pal.woodDark);
    r(x + 8, y + 40, 3, 6, pal.woodDark);
    r(x + 57, y + 40, 3, 6, pal.woodDark);
    // Monitor at the side so the person stays visible.
    const on = d.state === 'working' || d.state === 'checking';
    r(x + 44, y + 17, 15, 11, pal.monitor);
    r(x + 45, y + 18, 13, 8, on ? pal.screenOn[f.tick % 3] : pal.screenOff);
    r(x + 50, y + 28, 3, 1, pal.monitor);
    // Plant on some desks for warmth.
    if (hash(d.assignment.id) % 3 === 0) { r(x + 10, y + 22, 5, 5, pal.plant); r(x + 10, y + 27, 5, 2, pal.pot); }
    // Progress strip while working (indeterminate: it shows activity, not a percentage).
    if (d.state === 'working') {
        r(x + 6, y + 47, 56, 2, '#e6dccb');
        const p = f.tick === 0 ? 20 : (f.tick * 4) % 48;
        r(x + 6 + p, y + 47, 8, 2, pal.ok);
    }
    // Finished work leaves a paper on the desk.
    const outputs = d.work.filter((w) => w.state === 'done').reduce((n, w) => n + w.artifacts.length, 0);
    if (outputs > 0) { r(x + 18, y + 30, 7, 5, pal.paper); r(x + 19, y + 31, 5, 1, pal.muted); r(x + 19, y + 33, 4, 1, pal.muted); }
    // Name and state under the desk.
    label(ctx, d.assignment.displayName, x + 2, y + 54, c.w - 4, d.assignment.actorKind === 'ai' ? pal.ink : '#8a4515');
}

function drawPerson(r: R, d: Desk, x: number, y: number, f: Frame) {
    const h = hash(d.assignment.id);
    const shirt = shirts[h % shirts.length], hair = hairs[(h >> 3) % hairs.length], skin = skins[(h >> 6) % skins.length];
    let dy = 0;
    if (f.joy.has(d.assignment.id) && f.tick > 0) dy = f.tick % 2 ? -2 : 0;
    else if (d.state === 'idle' && f.tick % 16 === 0 && f.tick > 0) dy = -1;
    y += dy;
    r(x + 1, y, 6, 2, hair);
    r(x + 1, y + 2, 6, 4, skin);
    r(x + 1, y + 2, 1, 2, hair);
    r(x + 2, y + 3, 1, 1, pal.ink);
    r(x + 5, y + 3, 1, 1, pal.ink);
    r(x, y + 6, 8, 6, shirt);
    if (d.state === 'working' && f.tick % 2) r(x - 1, y + 9, 2, 2, skin); else r(x - 1, y + 8, 2, 3, skin);
    r(x + 7, y + 8, 2, 3, skin);
    // AI or person badge on the chest.
    r(x + 5, y + 7, 2, 2, d.assignment.actorKind === 'ai' ? '#6b55d9' : '#e8913a');
    drawStatus(r, d.state, x, y - 9, f, f.joy.has(d.assignment.id));
}

function drawStatus(r: R, s: DeskState, x: number, y: number, f: Frame, joy: boolean) {
    switch (s) {
        case 'waiting': // a card: something is waiting for a person
            r(x + 1, y, 7, 7, pal.apricot); r(x + 1, y, 7, 1, pal.warn);
            r(x + 4, y + 2, 1, 2, pal.ink); r(x + 4, y + 5, 1, 1, pal.ink);
            break;
        case 'checking': // a checklist
            r(x + 1, y, 7, 8, pal.paper); r(x + 1, y, 7, 1, pal.muted);
            r(x + 2, y + 2, 1, 1, pal.ok); r(x + 4, y + 2, 3, 1, pal.muted);
            r(x + 2, y + 4, 1, 1, f.tick % 4 > 1 || f.tick === 0 ? pal.ok : pal.paper); r(x + 4, y + 4, 3, 1, pal.muted);
            r(x + 2, y + 6, 1, 1, pal.muted); r(x + 4, y + 6, 3, 1, pal.muted);
            break;
        case 'problem': // a clear red mark
            r(x + 1, y, 7, 7, pal.bad);
            for (let i = 0; i < 5; i++) { r(x + 2 + i, y + 1 + i, 1, 1, pal.paper); r(x + 6 - i, y + 1 + i, 1, 1, pal.paper); }
            break;
        case 'done':
            if (joy) { r(x + 4, y, 1, 5, '#f2c94c'); r(x + 2, y + 2, 5, 1, '#f2c94c'); }
            else { r(x + 2, y + 2, 1, 1, pal.ok); r(x + 3, y + 3, 1, 1, pal.ok); r(x + 4, y + 2, 1, 1, pal.ok); r(x + 5, y + 1, 1, 1, pal.ok); }
            break;
        default:
            break;
    }
}

function label(ctx: CanvasRenderingContext2D, text: string, x: number, y: number, max: number, color = pal.ink) {
    ctx.fillStyle = color;
    ctx.font = '5px -apple-system, "Apple SD Gothic Neo", "Malgun Gothic", "Noto Sans KR", sans-serif';
    ctx.textBaseline = 'alphabetic';
    let t = text;
    while (t.length > 1 && ctx.measureText(t).width > max) t = t.slice(0, -1);
    if (t !== text) t = t.slice(0, -1) + '…';
    ctx.fillText(t, x, y);
}
