// UI strings. Korean first; another language is a second object with the
// same shape.
import type {InboxKind, MessageKind, NodeKind, OutputType, RunStatus, StepStatus} from './types';

export const ko = {
    app: {name: 'Agent Office'},
    nav: {
        dashboard: '전체 서비스', inbox: '내 할 일', design: '목표 요청', organization: '회사 조직', settings: '설정',
        services: '서비스', newService: '새 서비스', archived: '보관된 서비스 보기',
    },
    project: {
        tabs: {overview: '개요', team: '담당자', workflows: '업무 흐름', runs: '실행 기록'},
        name: '서비스 이름', goal: '목표', instructions: '서비스 지침', mode: '실행 모드',
        modeReview: '검토 후 실행', modeAuto: '자동 구성·실행',
        create: '서비스 만들기', save: '저장', archive: '보관', restore: '보관 해제',
        archivedNote: '보관된 서비스입니다. 기록은 남아 있고 수정은 할 수 없습니다.',
    },
    common: {
        save: '저장', cancel: '취소', add: '추가', delete: '삭제', edit: '편집', close: '닫기',
        loading: '불러오는 중…', none: '없음', ai: 'AI', human: '사람', required: '필수', optional: '선택',
        refresh: '새로 고침', confirm: '확인',
    },
    runStatus: {
        running: '진행 중', waiting: '사람 대기', paused: '일시 정지', succeeded: '완료',
        failed: '막힘·실패', interrupted: '중단됨', cancelled: '취소됨',
    } satisfies Record<RunStatus, string>,
    stepStatus: {
        pending: '대기', running: '작업 중', verifying: '검증 중', waiting_human: '사람 작업 대기',
        waiting_approval: '승인 대기', waiting_input: '답변 대기', succeeded: '완료', skipped: '건너뜀',
        failed: '실패', interrupted: '중단됨', cancelled: '취소됨', superseded: '대체됨(재작업)',
    } satisfies Record<StepStatus, string>,
    nodeKind: {
        task: '작업', review: '리뷰', approval: '승인', condition: '조건 분기', join: '합류',
    } satisfies Record<NodeKind, string>,
    outputType: {
        markdown: '문서(Markdown)', json: 'JSON', file: '파일', code_change: '코드 변경', report: '보고(JSON)',
    } satisfies Record<OutputType, string>,
    messageKind: {
        question: '질문', answer: '답변', review_request: '검토 요청', proposal: '제안', decision: '결정',
        handoff: '전달', escalation: '사용자에게 넘김',
    } satisfies Record<MessageKind, string>,
    inboxKind: {
        task: '작업 제출', review: '리뷰', approval: '승인', tool_approval: '도구 사용 승인', question: 'AI 질문', budget: '예산 한도',
    } satisfies Record<InboxKind, string>,
};

export const t = ko;
