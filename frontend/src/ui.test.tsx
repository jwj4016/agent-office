import {fireEvent, render, screen} from '@testing-library/react';
import {type ReactNode, useState} from 'react';
import {describe, expect, it} from 'vitest';
import {DetailPane} from './ui';

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
