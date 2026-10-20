import {FolderOpen, X} from 'lucide-react';
import {api} from '../api';
import {Button, ErrorBox, useAction} from '../ui';

// FolderInput shows the chosen folder and picks one with the system
// folder dialog instead of typing a path.
export function FolderInput({id, value, onChange, disabled, title = '폴더 선택', ...rest}: {
    id?: string; value: string; onChange: (path: string) => void; disabled?: boolean; title?: string; 'aria-describedby'?: string;
}) {
    const action = useAction();
    const pick = () => action.run(async () => {
        const path = await api.chooseFolder(title, value);
        if (path) onChange(path);
    });
    return (
        <div className="stack tight">
            <div className="row nowrap">
                <input id={id} className="mono" readOnly value={value} placeholder="선택한 폴더 없음" style={{flex: 1}}
                       aria-describedby={rest['aria-describedby']} onDoubleClick={disabled ? undefined : pick}/>
                <Button icon={FolderOpen} disabled={disabled} busy={action.busy} onClick={pick}>폴더 선택…</Button>
                {value && !disabled ? <Button variant="ghost" icon={X} onClick={() => onChange('')}>비우기</Button> : null}
            </div>
            <ErrorBox error={action.error}/>
        </div>
    );
}
