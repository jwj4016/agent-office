import {fireEvent, render, screen, within} from '@testing-library/react';
import {describe, expect, it, vi} from 'vitest';
import {Tutorial} from './Tutorial';

describe('Tutorial', () => {
    it('walks through all steps, moves focus, supports going back and completes', () => {
        const onClose = vi.fn();
        render(<Tutorial onClose={onClose}/>);
        const dialog = screen.getByRole('dialog');
        expect(within(dialog).getByRole('heading')).toHaveFocus();
        expect(within(dialog).getByRole('button', {name: '이전'})).toBeDisabled();
        fireEvent.click(within(dialog).getByRole('button', {name: '다음'}));
        expect(screen.getByRole('heading', {name: '먼저 AI 연결을 준비하세요'})).toHaveFocus();
        fireEvent.click(screen.getByRole('button', {name: '이전'}));
        expect(screen.getByRole('heading', {name: 'AI와 함께 일하는 사무실입니다'})).toHaveFocus();
        for (let i = 0; i < 4; i++) fireEvent.click(screen.getByRole('button', {name: '다음'}));
        expect(screen.getByRole('progressbar', {name: '튜토리얼 진행'})).toHaveAttribute('value', '5');
        expect(screen.getByText(/메모는.*다음 시도부터/)).toBeInTheDocument();
        fireEvent.click(screen.getByRole('button', {name: '시작하기'}));
        expect(onClose).toHaveBeenCalledOnce();
    });

    it('allows native Escape dismissal through the cancel event', () => {
        const onClose = vi.fn();
        render(<Tutorial onClose={onClose}/>);
        const event = new Event('cancel', {cancelable: true, bubbles: true});
        fireEvent(screen.getByRole('dialog'), event);
        expect(onClose).toHaveBeenCalledOnce();
        expect(event.defaultPrevented).toBe(true);
    });
});
