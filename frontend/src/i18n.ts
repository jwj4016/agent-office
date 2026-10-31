// UI strings. Korean first; another language is a second object with the
// same shape.
import type {AutoStartStatus, CollabAction, CollabRequirement, CollabStatus, DeliveryPurpose, DeliveryStatus, InboxKind, MessageKind, NodeKind, OutputType, RunStatus, StepStatus, WorkPriority, WorkStatus} from './types';

export const ko = {
    app: {name: 'Agent Office'},
    nav: {
        dashboard: '홈', inbox: '내 할 일', design: '새 목표', organization: '회사 조직', settings: '설정',
        services: '서비스', newService: '새 서비스', archived: '보관된 서비스 보기',
    },
    project: {
        tabs: {status: '현황', work: '업무', docs: '자료', office: '사무실', runs: '실행', workflows: '업무 흐름', team: '부서·담당자', settings: '설정'},
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
        markdown: '문서(Markdown)', json: 'JSON', file: '파일', code_change: '코드 변경', report: '보고(JSON)', diagram: '다이어그램(HTML)',
    } satisfies Record<OutputType, string>,
    messageKind: {
        question: '질문', answer: '답변', review_request: '검토 요청', proposal: '제안', decision: '결정',
        handoff: '전달', escalation: '사용자에게 넘김',
    } satisfies Record<MessageKind, string>,
    workStatus: {
        received: '접수', ready: '준비', in_progress: '진행', waiting: '대기', blocked: '막힘', done: '완료', cancelled: '취소',
    } satisfies Record<WorkStatus, string>,
    workPriority: {
        urgent: '긴급', high: '높음', normal: '보통', low: '낮음',
    } satisfies Record<WorkPriority, string>,
    workEvent: {
        'work.created': '업무 등록됨', 'work.collab_requested': '협업 요청됨', 'work.completed': '업무 완료됨', 'work.reopened': '업무 재개됨',
        'document.shared': '자료 공유됨', 'document.revised': '자료 개정됨', 'document.confirmed': '자료 확정됨', 'document.withdrawn': '자료 철회됨',
        'department.check_requested': '부서 점검 요청됨',
    } as Record<string, string>,
    deliveryStatus: {
        pending: '대기', processing: '처리 중', waiting_answer: '답변 대기', done: '처리됨', skipped: '건너뜀', held: '보류', failed: '실패',
    } satisfies Record<DeliveryStatus, string>,
    deliveryPurpose: {
        judge: '판단', notify: '알림', collab_request: '협업 요청',
    } satisfies Record<DeliveryPurpose, string>,
    collabStatus: {
        requested: '수락 대기', question: '질문 중', accepted: '수락됨', declined: '거절됨', cancelled: '취소됨',
    } satisfies Record<CollabStatus, string>,
    collabRequirement: {
        reference: '참고용', required: '완료에 필요',
    } satisfies Record<CollabRequirement, string>,
    collabAction: {
        requested: '요청', asked: '질문', answered: '답변', accepted: '수락', declined: '거절', cancelled: '취소',
        required: '필수로 지정', reference: '필수 해제', replaced: '대체', reverted: 'AI 결정 되돌림',
    } satisfies Record<CollabAction, string>,
    autoStartStatus: {
        pending: '확인 대기', planning: 'AI가 계획 작성 중', approved: '시작 승인됨', started: '자동 실행 중', finished: '자동 실행 끝남',
        review: '검토 필요', cancelled: '자동 시작 안 함',
    } satisfies Record<AutoStartStatus, string>,
    inboxKind: {
        task: '작업 제출', review: '리뷰', approval: '승인', tool_approval: '도구 사용 승인', question: 'AI 질문', budget: '예산 한도',
    } satisfies Record<InboxKind, string>,
};

export const t = ko;
