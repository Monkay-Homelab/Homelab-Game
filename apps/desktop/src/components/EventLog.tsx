import { useGameStore, type GameEvent } from '../stores/gameStore';

const severityStyles: Record<string, { border: string; bg: string }> = {
  minor: { border: 'rgba(245,158,11,0.4)', bg: 'rgba(245,158,11,0.08)' },
  moderate: { border: 'rgba(249,115,22,0.4)', bg: 'rgba(249,115,22,0.08)' },
  major: { border: 'rgba(239,68,68,0.4)', bg: 'rgba(239,68,68,0.08)' },
};

export function EventLog() {
  const events = useGameStore(s => s.events);
  const dismissEvent = useGameStore(s => s.dismissEvent);

  if (events.length === 0) return null;

  return (
    <div className="fixed bottom-4 right-4 w-80 space-y-2 z-50">
      {events.map((evt, i) => (
        <EventCard key={`${evt.type}-${i}`} event={evt} onDismiss={() => dismissEvent(i)} />
      ))}
    </div>
  );
}

function EventCard({ event, onDismiss }: { event: GameEvent; onDismiss: () => void }) {
  const mitigated = event.description.includes('(Mitigated!)');
  const style = mitigated
    ? { border: 'rgba(34,197,94,0.4)', bg: 'rgba(34,197,94,0.08)' }
    : (severityStyles[event.severity] || severityStyles.minor);

  return (
    <div
      className="rounded-lg p-3 shadow-xl animate-slide-in cursor-pointer"
      onClick={onDismiss}
      style={{
        background: `var(--bg-panel)`,
        border: `1px solid ${style.border}`,
        backdropFilter: 'blur(8px)',
      }}
    >
      <div className="flex justify-between items-start gap-2">
        <div className="flex-1 min-w-0">
          <div className="font-medium text-sm truncate">{event.name}</div>
          <p className="text-xs mt-1 leading-relaxed" style={{ color: 'var(--text-secondary)' }}>{event.description}</p>
        </div>
        <span className="font-mono text-xs shrink-0 px-1.5 py-0.5 rounded" style={{ background: style.bg, color: mitigated ? 'var(--accent-green)' : 'var(--text-secondary)' }}>
          {mitigated ? 'OK' : event.severity.toUpperCase()}
        </span>
      </div>
    </div>
  );
}
