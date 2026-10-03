import {useEffect, useState} from 'react';
import {Ping} from '../wailsjs/go/main/App';
import {main} from '../wailsjs/go/models';
import {EventsOn} from '../wailsjs/runtime/runtime';

export const EVENT_PING = 'system:ping';

function App() {
    const [reply, setReply] = useState<main.PingResult | null>(null);
    const [event, setEvent] = useState<main.PingResult | null>(null);

    useEffect(() => EventsOn(EVENT_PING, (res: main.PingResult) => setEvent(res)), []);

    return (
        <main style={{padding: 24}}>
            <h1>Agent Office</h1>
            <button onClick={() => Ping('연결 확인').then(setReply)}>연결 확인</button>
            <p data-testid="reply">응답: {reply ? `#${reply.sequence} ${reply.message}` : '없음'}</p>
            <p data-testid="event">이벤트: {event ? `#${event.sequence} ${event.at}` : '없음'}</p>
        </main>
    );
}

export default App;
