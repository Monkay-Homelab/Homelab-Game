import { useState } from 'react';
import { useGameStore } from '../stores/gameStore';

const TIER_JOBS: Record<string, string[]> = {
  coffee_table: ['Compiling a script...', 'Running apt update...', 'Pinging localhost...', 'Downloading ISO...'],
  closet_floor: ['Transcoding video...', 'Building Docker image...', 'Running backup...', 'Indexing media library...'],
  rack_12u: ['Deploying containers...', 'Running Ansible playbook...', 'Syncing NAS...', 'Processing logs...'],
  rack_24u: ['CI/CD pipeline running...', 'Swarm service scaling...', 'Mail queue processing...', 'Camera feed analyzing...'],
  rack_36u: ['K8s pod scheduling...', 'ELK ingesting logs...', 'DB cluster rebalancing...', 'DNS zone transfer...'],
  rack_48u: ['Training ML model...', 'CDN cache warming...', 'Federation sync...', 'Terraform applying...'],
};

export function ClickArea({ tier }: { tier: string }) {
  const runJob = useGameStore(s => s.runJob);
  const [lastJob, setLastJob] = useState('Ready');
  const [clickCount, setClickCount] = useState(0);
  const [isActive, setIsActive] = useState(false);

  const handleClick = () => {
    runJob();
    const jobs = TIER_JOBS[tier] || TIER_JOBS.coffee_table;
    setLastJob(jobs[Math.floor(Math.random() * jobs.length)]);
    setClickCount(c => c + 1);
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
        <span className="font-mono text-xs truncate" style={{ color: 'var(--text-secondary)' }}>{lastJob}</span>
        <span className="font-mono text-xs shrink-0 ml-2" style={{ color: 'var(--text-muted)' }}>#{clickCount}</span>
      </div>
    </div>
  );
}
