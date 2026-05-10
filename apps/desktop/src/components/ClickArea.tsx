import { useState } from 'react';
import { useGameStore } from '../stores/gameStore';
import { useConfig, getTier } from '../hooks/useConfig';

export function ClickArea({ tier }: { tier: string }) {
  const config = useConfig();
  const runJob = useGameStore((s) => s.runJob);
  const [lastJob, setLastJob] = useState('Ready');
  const [clickCount, setClickCount] = useState(0);
  const [isActive, setIsActive] = useState(false);

  const handleClick = () => {
    runJob();
    const tierCfg = getTier(config, tier);
    const jobs = tierCfg?.jobs || ['Running job...'];
    setLastJob(jobs[Math.floor(Math.random() * jobs.length)]);
    setClickCount((c) => c + 1);
    setIsActive(true);
    setTimeout(() => setIsActive(false), 100);
  };

  return (
    <div className="panel p-4 flex flex-col items-center">
      <button
        onClick={handleClick}
        className="w-full py-6 rounded-lg font-semibold text-lg transition-all"
        style={{
          background: isActive ? 'rgba(34,197,94,0.3)' : 'rgba(34,197,94,0.1)',
          color: 'var(--accent-green)',
          border: `1px solid ${isActive ? 'var(--accent-green)' : 'rgba(34,197,94,0.3)'}`,
          boxShadow: isActive ? 'var(--glow-green)' : 'none',
          transform: isActive ? 'scale(0.98)' : 'scale(1)',
        }}
      >
        Run Job
      </button>
      <div className="mt-3 w-full flex justify-between items-center">
        <span className="font-mono text-xs truncate" style={{ color: 'var(--text-secondary)' }}>
          {lastJob}
        </span>
        <span className="font-mono text-xs shrink-0 ml-2" style={{ color: 'var(--text-muted)' }}>
          #{clickCount}
        </span>
      </div>
    </div>
  );
}
