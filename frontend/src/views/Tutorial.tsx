import {ArrowLeft, ArrowRight, Building2, Check, ChevronRight, Inbox, Plus, Settings, Workflow, X} from 'lucide-react';
import {useEffect, useId, useRef, useState} from 'react';
import {Button, Icon} from '../ui';

export const TUTORIAL_SEEN = 'ui.tutorialSeen';

const steps = [
    {
        title: 'AI와 함께 일하는 사무실입니다', icon: Building2, where: 'Agent Office',
        description: '목표를 정하면 AI와 사람이 역할을 나눠 업무를 진행합니다. 먼저 전체 사용 흐름을 살펴보세요.',
        items: ['서비스는 하나의 목표와 자료, 담당자를 묶는 공간입니다.', '업무 흐름은 누가 어떤 순서로 일하고 검토할지 정한 설계도입니다.', '실행을 시작하면 진행 상황과 결과가 기록됩니다.'],
    },
    {
        title: '먼저 AI 연결을 준비하세요', icon: Settings, where: '왼쪽 메뉴 → 설정 → AI 연결',
        description: '사용할 AI를 연결하면 목표 설계와 실제 업무를 맡길 수 있습니다.',
        items: ['Codex·Claude CLI 또는 지원하는 API 연결을 등록합니다.', '‘연결 확인 (무료)’으로 준비 상태를 확인합니다.', '‘실제 호출 시험’은 연결한 AI의 사용량을 써서 응답을 확인합니다.'],
    },
    {
        title: '새 목표로 업무를 설계하세요', icon: Plus, where: '왼쪽 메뉴 → 새 목표',
        description: '무엇을 이루고 싶은지 적으면 AI가 필요한 담당자와 업무 흐름을 제안합니다.',
        items: ['목표를 구체적으로 적고 사람이 직접 맡을 일을 지정합니다.', '설계에 사용할 AI 연결을 선택해 요청합니다.', '제안된 역할과 업무 흐름을 검토한 뒤 적용합니다. 처음에는 ‘검토 후 실행’ 방식으로 시작해 보세요.'],
    },
    {
        title: '업무 흐름을 확인하고 실행하세요', icon: Workflow, where: '서비스 → 업무 흐름',
        description: '각 업무의 담당자와 연결을 확인하고, 필요하면 AI에게 수정을 요청하세요.',
        items: ['업무를 눌러 지시·담당자·결과를 설정하거나 ‘AI로 수정’을 사용합니다.', 'AI 수정안은 변경 전후와 미리보기를 확인한 뒤 ‘초안에 적용’합니다.', '‘저장·검증’ → ‘버전 확정’ → ‘실행’ 순서로 시작합니다. 초안을 수정해도 이미 실행 중인 버전은 유지됩니다.'],
    },
    {
        title: '진행을 살피고 필요한 결정을 내려 주세요', icon: Inbox, where: '서비스 현황 · 내 할 일 · 실행',
        description: 'AI가 일하는 동안 진행 상황을 확인하고, 질문이나 승인이 도착하면 응답합니다.',
        items: ['‘현황’과 ‘사무실’에서 누가 어떤 일을 하는지 확인합니다.', '‘AI 활동’에서 대화·결정과 실행 로그를 보고, ‘내 할 일’에서 질문·승인·리뷰를 처리합니다.', '‘실행’에서 업무별 결과물을 엽니다. 활동 패널의 메모는 이미 작업 중인 시도에 즉시 전달되지 않고 다음 시도부터 반영됩니다.'],
    },
];

// A native modal keeps keyboard focus inside the guide and the current work
// inert. The guide itself never navigates, starts a run, or calls an AI.
export function Tutorial({onClose}: {onClose: () => void}) {
    const [step, setStep] = useState(0);
    const dialog = useRef<HTMLDialogElement>(null);
    const heading = useRef<HTMLHeadingElement>(null);
    const titleId = useId(), descriptionId = useId();
    useEffect(() => {
        const element = dialog.current!;
        const trigger = document.activeElement as HTMLElement | null;
        element.showModal();
        return () => { element.close(); if (trigger?.isConnected) trigger.focus(); };
    }, []);
    useEffect(() => { heading.current?.focus(); }, [step]);
    const current = steps[step];
    const last = step === steps.length - 1;
    return <dialog ref={dialog} className="tutorial" aria-labelledby={titleId} aria-describedby={descriptionId}
                   onCancel={(e) => { e.preventDefault(); onClose(); }}>
        <div className="stack loose">
            <div className="row between nowrap">
                <span className="muted small">Agent Office 시작하기 · {step + 1}/{steps.length}</span>
                <Button variant="ghost" size="sm" icon={X} iconOnly aria-label="튜토리얼 닫기" onClick={onClose}/>
            </div>
            <progress className="tutorial-progress" value={step + 1} max={steps.length} aria-label="튜토리얼 진행"/>
            <div className="stack">
                <div className="tutorial-icon"><Icon icon={current.icon} size={28}/></div>
                <p className="muted small" style={{margin: 0}}>{current.where}</p>
                <h2 id={titleId} ref={heading} tabIndex={-1}>{current.title}</h2>
                <p id={descriptionId} style={{margin: 0}}>{current.description}</p>
            </div>
            {step === 0 ? <div className="tutorial-path" aria-label="전체 사용 순서">
                {['목표', '담당자·업무 흐름', '실행·검토', '결과'].map((label, i) => <span className="row nowrap" key={label}>
                    {i > 0 ? <Icon icon={ChevronRight} size={14}/> : null}<span className="badge info">{label}</span>
                </span>)}
            </div> : null}
            <ol className="card tutorial-tasks stack">{current.items.map((item) => <li key={item}>{item}</li>)}</ol>
            <p className="muted small" style={{margin: 0}}>왼쪽 메뉴의 ‘튜토리얼’에서 언제든 다시 볼 수 있습니다.</p>
            <div className="row between tutorial-actions">
                <Button variant="ghost" onClick={onClose}>건너뛰기</Button>
                <div className="row">
                    <Button icon={ArrowLeft} disabled={step === 0} onClick={() => setStep(step - 1)}>이전</Button>
                    <Button variant="primary" icon={last ? Check : ArrowRight} onClick={() => last ? onClose() : setStep(step + 1)}>{last ? '시작하기' : '다음'}</Button>
                </div>
            </div>
        </div>
    </dialog>;
}
