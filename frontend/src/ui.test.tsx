import {fireEvent, render, screen} from '@testing-library/react';
import {type ReactNode, useState} from 'react';
import {describe, expect, it} from 'vitest';
import {applyTheme, DetailPane, Field, Tabs} from './ui';

// Host re-creates the detail on every keystroke, like the flow editor.
function Host() {
    const [text, setText] = useState('');
    const [open, setOpen] = useState(false);
    const detail: ReactNode = <input aria-label="이름" value={text} onChange={(e) => setText(e.target.value)}/>;
    return (
        <>
            <button onClick={() => setOpen(true)}>열기</button>
            <DetailPane detail={open ? detail : null} onClose={() => setOpen(false)}/>
        </>
    );
}

describe('DetailPane', () => {
    it('recovers focus when a secondary detail replaces its focused trigger', () => {
        function ReplaceHost() {
            const [open, setOpen] = useState(false), [next, setNext] = useState(false);
            return <><button onClick={() => setOpen(true)}>상세 열기</button><DetailPane detail={open ? (next ? <p>활동 기록</p> : <button onClick={() => setNext(true)}>활동 열기</button>) : null} onClose={() => setOpen(false)}/></>;
        }
        render(<ReplaceHost/>);
        fireEvent.click(screen.getByRole('button', {name: '상세 열기'}));
        const trigger = screen.getByRole('button', {name: '활동 열기'});
        trigger.focus(); fireEvent.click(trigger);
        expect(screen.getByRole('complementary')).toHaveFocus();
    });
    it('takes focus when it opens, keeps it in a field while typing, and closes on Escape', () => {
        render(<Host/>);
        fireEvent.click(screen.getByRole('button', {name: '열기'}));
        const pane = screen.getByRole('complementary', {name: '상세'});
        expect(pane).toHaveFocus();
        const field = screen.getByLabelText('이름');
        field.focus();
        fireEvent.change(field, {target: {value: '근'}});
        fireEvent.change(field, {target: {value: '근거'}});
        expect(field).toHaveFocus();
        fireEvent.keyDown(field, {key: 'Escape'});
        expect(screen.queryByRole('complementary')).not.toBeInTheDocument();
    });
});

function TabsHost() {
    const [tab, setTab] = useState<'a' | 'b' | 'c'>('a');
    return (
        <>
            <Tabs label="보기" value={tab} onChange={setTab} items={[{id: 'a', label: '가'}, {id: 'b', label: '나'}, {id: 'c', label: '다'}]}/>
            <p>선택: {tab}</p>
        </>
    );
}

describe('Tabs', () => {
    it('keeps one tab in the Tab order and moves with arrows, Home and End', () => {
        render(<TabsHost/>);
        const [a, b, c] = screen.getAllByRole('tab');
        expect(a).toHaveAttribute('tabindex', '0');
        expect(b).toHaveAttribute('tabindex', '-1');
        a.focus();
        fireEvent.keyDown(a, {key: 'ArrowRight'});
        expect(b).toHaveFocus();
        expect(screen.getByText('선택: b')).toBeInTheDocument();
        fireEvent.keyDown(b, {key: 'End'});
        expect(c).toHaveFocus();
        fireEvent.keyDown(c, {key: 'ArrowRight'});
        expect(a).toHaveFocus();
        expect(a).toHaveAttribute('aria-selected', 'true');
    });
});

describe('Field', () => {
    it('labels the control and links the hint and error without adding them to the name', () => {
        render(<Field label="토큰 예산" hint="비우면 제한 없음" error="숫자를 입력하세요"><input/></Field>);
        const input = screen.getByLabelText('토큰 예산');
        expect(input).toHaveAccessibleDescription('비우면 제한 없음 숫자를 입력하세요');
        expect(input).toHaveAttribute('aria-invalid', 'true');
    });
});

describe('applyTheme', () => {
    it('sets the chosen theme and follows the OS for system', () => {
        applyTheme('dark');
        expect(document.documentElement.dataset.theme).toBe('dark');
        applyTheme('light');
        expect(document.documentElement.dataset.theme).toBe('light');
        applyTheme('system');
        expect(['light', 'dark']).toContain(document.documentElement.dataset.theme);
    });
});
