import {useEffect, useState} from 'react';
import {GetSettings, Ping, SetSetting, SystemStatus} from '../wailsjs/go/main/App';
import {main} from '../wailsjs/go/models';
import {EventsOn} from '../wailsjs/runtime/runtime';

export const EVENT_PING = 'system:ping';
const REDUCED_MOTION = 'ui.reducedMotion';

function App() {
    const [reply, setReply] = useState<main.PingResult | null>(null);
    const [event, setEvent] = useState<main.PingResult | null>(null);
    const [status, setStatus] = useState<main.SystemStatus | null>(null);
    const [reducedMotion, setReducedMotion] = useState(false);
    const [error, setError] = useState('');

    useEffect(() => EventsOn(EVENT_PING, (res: main.PingResult) => setEvent(res)), []);

    useEffect(() => {
        SystemStatus().then(setStatus);
        GetSettings()
            .then((s) => setReducedMotion(s[REDUCED_MOTION] === 'true'))
            .catch((e) => setError(String(e)));
    }, []);

    function toggleReducedMotion(next: boolean) {
        setReducedMotion(next);
        SetSetting(REDUCED_MOTION, String(next)).catch((e) => {
            setReducedMotion(!next);
            setError(String(e));
        });
    }

    return (
        <main style={{padding: 24}}>
            <h1>Agent Office</h1>
            <button onClick={() => Ping('연결 확인').then(setReply)}>연결 확인</button>
            <p data-testid="reply">응답: {reply ? `#${reply.sequence} ${reply.message}` : '없음'}</p>
            <p data-testid="event">이벤트: {event ? `#${event.sequence} ${event.at}` : '없음'}</p>

            <label>
                <input type="checkbox" checked={reducedMotion}
                       onChange={(e) => toggleReducedMotion(e.target.checked)}/>
                모션 줄이기
            </label>

            {status && (
                <p data-testid="status">
                    저장 위치: {status.dataDir} · 스키마 v{status.schemaVersion} ·
                    키 저장: {status.secretsPersistent ? 'OS 비밀 저장소' : '이번 세션 메모리만'}
                </p>
            )}
            {(error || status?.error) && <p role="alert">오류: {error || status?.error}</p>}
        </main>
    );
}

export default App;
