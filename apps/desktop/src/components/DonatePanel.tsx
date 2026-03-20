import { useState } from 'react';
import type { GameState } from '../api';
import { useGameStore } from '../stores/gameStore';

function formatNumber(n: number): string {
  if (n >= 1_000_000) return (n / 1_000_000).toFixed(1) + 'M';
  if (n >= 1_000) return (n / 1_000).toFixed(1) + 'K';
  return n.toString();
}

const PRESETS = [1000, 10000, 100000, 1000000, 10000000, 100000000];

export function DonatePanel({ state }: { state: GameState }) {
  const donateCU = useGameStore(s => s.donateCU);
  const [donating, setDonating] = useState(false);

  const handleDonate = async (amount: number) => {
    if (amount <= 0 || amount > state.compute_units) return;
    setDonating(true);
    await donateCU(amount);
    setDonating(false);
  };

  return (
    <div className="panel p-4">
      <div className="flex justify-between items-center mb-2">
        <span className="text-xs font-medium" style={{ color: 'var(--text-secondary)' }}>Global CU Store</span>
        <span className="font-mono text-xs" style={{ color: 'var(--accent-amber)' }}>{formatNumber(state.global_donated_cu || 0)} total</span>
      </div>

      <div className="font-mono text-xs mb-3" style={{ color: 'var(--text-muted)' }}>
        You've donated: {formatNumber(state.total_donated_cu || 0)} CU
      </div>

      <div className="grid grid-cols-3 gap-1.5">
        {PRESETS.map(amount => {
          const canAfford = state.compute_units >= amount;
          return (
            <button
              key={amount}
              onClick={() => handleDonate(amount)}
              disabled={!canAfford || donating}
              className="btn py-1.5 text-xs font-mono"
              style={{
                background: canAfford ? 'rgba(245,158,11,0.1)' : 'var(--bg-card)',
                color: canAfford ? 'var(--accent-amber)' : 'var(--text-muted)',
                border: `1px solid ${canAfford ? 'rgba(245,158,11,0.2)' : 'var(--border)'}`,
              }}
            >
              {formatNumber(amount)}
            </button>
          );
        })}
      </div>
    </div>
  );
}
