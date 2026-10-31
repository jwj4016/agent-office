import {CalendarClock} from 'lucide-react';
import {useState} from 'react';
import {api} from '../api';
import {t} from '../i18n';
import {useLoad} from '../live';
import type {Department, DepartmentSchedule, Project, ScheduleView} from '../types';
import {Button, ErrorBox, Field, Icon, Loading, shortTime, useAction, useToast} from '../ui';

const weekdays = ['일', '월', '화', '수', '목', '금', '토'];

// The PC's time zone; a schedule keeps the one it was saved with.
export const pcTimezone = () => Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC';

export function scheduleLabel(s: Pick<DepartmentSchedule, 'frequency' | 'timeOfDay' | 'weekday'>) {
    return s.frequency === 'weekly' ? `매주 ${weekdays[s.weekday ?? 1]}요일 ${s.timeOfDay}` : `매일 ${s.timeOfDay}`;
}

// ScheduleSection sets a department's regular check in its judgment
// settings: the lead AI looks for new work at that time, the same way it
// does when a person asks.
export function ScheduleSection({project, department}: { project: Project; department: Department }) {
    const list = useLoad(() => api.schedules(project.id), [project.id], project.id);
    if (list.error) return <ErrorBox error={list.error}/>;
    if (!list.data) return <Loading/>;
    const current = list.data.find((s) => s.departmentId === department.id);
    return <ScheduleForm key={current?.updatedAt ?? 'new'} project={project} department={department} current={current} onSaved={list.reload}/>;
}

function ScheduleForm({project, department, current, onSaved}: {
    project: Project; department: Department; current?: ScheduleView; onSaved: () => void;
}) {
    const [form, setForm] = useState({
        enabled: current?.enabled ?? false, frequency: current?.frequency ?? 'daily' as DepartmentSchedule['frequency'],
        timeOfDay: current?.timeOfDay ?? '09:00', weekday: current?.weekday ?? 1, note: current?.note ?? '',
    });
    const action = useAction();
    const toast = useToast();
    const archived = project.status === 'archived';
    const zone = pcTimezone();
    const save = () => action.run(async () => {
        await api.saveSchedule({
            projectId: project.id, departmentId: department.id, enabled: form.enabled, frequency: form.frequency, timeOfDay: form.timeOfDay,
            weekday: form.frequency === 'weekly' ? form.weekday : null, timezone: zone, note: form.note,
        });
        toast(form.enabled ? `${department.name} 정기 점검: ${scheduleLabel(form)}` : `${department.name} 정기 점검을 껐습니다`);
        onSaved();
    });
    return (
        <form className="card stack" aria-label="정기 점검" onSubmit={(e) => { e.preventDefault(); save(); }}>
            <span className="row small"><Icon icon={CalendarClock} size={16}/><strong>정기 점검</strong></span>
            <p className="muted small" style={{margin: 0}}>정한 때마다 대표 AI가 새로 할 일이 있는지 살펴 업무를 제안합니다. 사람이 기다리지 않는 판단이라
                다른 판단과 같은 순서로 처리합니다. 앱이 꺼져 있어 놓친 점검은 가장 최근 한 번만 합니다.</p>
            <label className="check">
                <input type="checkbox" checked={form.enabled} disabled={archived} onChange={(e) => setForm({...form, enabled: e.target.checked})}/>정기 점검 사용
            </label>
            <span className="row">
                <select aria-label="점검 주기" value={form.frequency} disabled={archived}
                        onChange={(e) => setForm({...form, frequency: e.target.value as DepartmentSchedule['frequency']})}>
                    <option value="daily">매일</option>
                    <option value="weekly">매주</option>
                </select>
                {form.frequency === 'weekly' ? (
                    <select aria-label="요일" value={form.weekday} disabled={archived} onChange={(e) => setForm({...form, weekday: Number(e.target.value)})}>
                        {weekdays.map((w, i) => <option key={w} value={i}>{w}요일</option>)}
                    </select>
                ) : null}
                <input type="time" aria-label="점검 시각" value={form.timeOfDay} disabled={archived} required
                       onChange={(e) => setForm({...form, timeOfDay: e.target.value})}/>
            </span>
            <Field label="점검 관점" optional hint="예: 이번 주 공유된 자료에서 마케팅이 할 일을 찾는다. 판단 지시에 그대로 들어갑니다.">
                <textarea rows={2} value={form.note} disabled={archived} onChange={(e) => setForm({...form, note: e.target.value})}/></Field>
            <span className="muted xsmall">
                시간대: {zone}{current && current.timezone !== zone ? ` (저장된 시간대 ${current.timezone}, 저장하면 이 PC 시간대로 바뀝니다)` : ''}
                {current?.nextDueAt ? ` · 다음 점검 ${shortTime(current.nextDueAt)}` : ''}
                {current?.lastSlotAt ? ` · 지난 점검 ${shortTime(current.lastSlotAt)}` : ''}
            </span>
            <ErrorBox error={action.error}/>
            <div className="form-actions">
                <Button variant="primary" type="submit" busy={action.busy} disabled={archived || !form.timeOfDay}>{t.common.save}</Button>
            </div>
        </form>
    );
}
