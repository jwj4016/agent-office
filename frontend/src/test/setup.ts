import '@testing-library/jest-dom/vitest';
import {cleanup, configure} from '@testing-library/react';
import {afterEach} from 'vitest';

afterEach(() => cleanup());

// CI machines run test files in parallel and can take over the default
// 1 second to show a screen; stay under vitest's 5-second test limit.
configure({asyncUtilTimeout: 3000});

// React Flow measures its container; jsdom has no ResizeObserver.
class ResizeObserverStub {
    observe() {}
    unobserve() {}
    disconnect() {}
}
globalThis.ResizeObserver ??= ResizeObserverStub as unknown as typeof ResizeObserver;

// jsdom has no top layer; simulate only the native dialog's open/close state.
HTMLDialogElement.prototype.showModal ??= function () { this.open = true; };
HTMLDialogElement.prototype.close ??= function () { this.open = false; };
