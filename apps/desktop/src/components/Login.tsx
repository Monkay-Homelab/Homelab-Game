import { useState } from 'react';
import { useGameStore } from '../stores/gameStore';

export function Login() {
  const [isRegister, setIsRegister] = useState(false);
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [displayName, setDisplayName] = useState('');
  const { login, register, loading, error } = useGameStore();

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (isRegister) {
      register(email, password, displayName);
    } else {
      login(email, password);
    }
  };

  return (
    <div className="h-screen flex items-center justify-center" style={{ background: 'var(--bg-deep)' }}>
      <div className="w-96 panel p-8">
        <div className="text-center mb-8">
          <span className="font-mono text-xs px-2 py-1 rounded inline-block mb-3" style={{ background: 'var(--bg-card)', color: 'var(--accent-green)' }}>
            HLG v0.1
          </span>
          <h1 className="text-2xl font-semibold tracking-tight">Homelab the Game</h1>
          <p className="text-sm mt-1" style={{ color: 'var(--text-secondary)' }}>Build. Deploy. Colocate.</p>
        </div>

        <form onSubmit={handleSubmit} className="space-y-3">
          {isRegister && (
            <input
              type="text"
              placeholder="Display Name"
              value={displayName}
              onChange={e => setDisplayName(e.target.value)}
              className="w-full px-3 py-2.5 rounded text-sm outline-none transition-colors"
              style={{ background: 'var(--bg-card)', border: '1px solid var(--border)', color: 'var(--text-primary)' }}
            />
          )}
          <input
            type="email"
            placeholder="Email"
            value={email}
            onChange={e => setEmail(e.target.value)}
            className="w-full px-3 py-2.5 rounded text-sm outline-none transition-colors"
            style={{ background: 'var(--bg-card)', border: '1px solid var(--border)', color: 'var(--text-primary)' }}
          />
          <input
            type="password"
            placeholder="Password"
            value={password}
            onChange={e => setPassword(e.target.value)}
            className="w-full px-3 py-2.5 rounded text-sm outline-none transition-colors"
            style={{ background: 'var(--bg-card)', border: '1px solid var(--border)', color: 'var(--text-primary)' }}
          />
          {error && <p className="text-xs" style={{ color: 'var(--accent-red)' }}>{error}</p>}
          <button
            type="submit"
            disabled={loading}
            className="btn w-full py-2.5 text-sm font-semibold"
            style={{ background: 'rgba(34,197,94,0.1)', color: 'var(--accent-green)', border: '1px solid rgba(34,197,94,0.3)' }}
          >
            {loading ? '...' : isRegister ? 'Create Account' : 'Sign In'}
          </button>
        </form>

        <button
          onClick={() => setIsRegister(!isRegister)}
          className="w-full mt-4 text-xs py-2"
          style={{ color: 'var(--text-secondary)' }}
        >
          {isRegister ? 'Already have an account? Sign in' : "Need an account? Register"}
        </button>
      </div>
    </div>
  );
}
