import { useEffect, useState } from 'react';
import { api, type GroupInfo, type LeaderboardData } from '../api';
import { useConfig } from '../hooks/useConfig';

type GroupListItem = NonNullable<GroupInfo['group']>;

export function SocialPanel() {
  const config = useConfig();
  const LB_CATEGORIES = config.leaderboard.categories;
  const [group, setGroup] = useState<GroupInfo | null>(null);
  const [allGroups, setAllGroups] = useState<GroupListItem[]>([]);
  const [leaderboard, setLeaderboard] = useState<LeaderboardData | null>(null);
  const [lbCategory, setLbCategory] = useState('compute');
  const [groupName, setGroupName] = useState('');
  const [error, setError] = useState('');
  const [tab, setTab] = useState<'group' | 'leaderboard'>('group');

  const loadGroup = async () => {
    try {
      const data = await api.getMyGroup();
      setGroup(data);
      if (!data?.group) {
        // Load available groups if not in one
        const list = await api.listGroups();
        setAllGroups((list.groups || []).filter((g): g is GroupListItem => g !== null));
      }
    } catch { /* no group */ }
  };

  const loadLeaderboard = async (cat: string) => {
    try {
      const data = await api.getLeaderboard(cat);
      setLeaderboard(data);
    } catch { /* ignore */ }
  };

  useEffect(() => { loadGroup(); }, []);
  useEffect(() => { loadLeaderboard(lbCategory); }, [lbCategory]);

  const handleCreate = async () => {
    setError('');
    try {
      const data = await api.createGroup(groupName);
      setGroup(data);
      setGroupName('');
    } catch (e) {
      setError((e as Error).message);
    }
  };

  const handleLeave = async () => {
    try {
      await api.leaveGroup();
      setGroup(null);
    } catch (e) {
      setError((e as Error).message);
    }
  };

  return (
    <div className="h-full flex flex-col min-h-0">
      {/* Sub-tabs */}
      <div className="shrink-0 flex gap-1 mb-3">
        <button
          onClick={() => setTab('group')}
          className="px-4 py-2 text-sm font-medium rounded-t transition-all"
          style={{
            background: tab === 'group' ? 'var(--bg-panel)' : 'transparent',
            color: tab === 'group' ? '#22c55e' : 'var(--text-secondary)',
            borderBottom: tab === 'group' ? '2px solid #22c55e' : '2px solid transparent',
          }}
        >
          Group
        </button>
        <button
          onClick={() => setTab('leaderboard')}
          className="px-4 py-2 text-sm font-medium rounded-t transition-all"
          style={{
            background: tab === 'leaderboard' ? 'var(--bg-panel)' : 'transparent',
            color: tab === 'leaderboard' ? '#f59e0b' : 'var(--text-secondary)',
            borderBottom: tab === 'leaderboard' ? '2px solid #f59e0b' : '2px solid transparent',
          }}
        >
          Leaderboard
        </button>
      </div>

      {error && (
        <div className="shrink-0 mb-3 px-3 py-2 rounded text-xs" style={{ background: 'rgba(239,68,68,0.1)', color: 'var(--accent-red)', border: '1px solid rgba(239,68,68,0.2)' }}>
          {error}
        </div>
      )}

      {tab === 'group' && (
        <div className="flex-1 min-h-0 flex gap-4">
          {!group?.group ? (
            /* No group — show create + browse groups */
            <>
              {/* Create */}
              <div className="w-72 shrink-0 panel p-4 flex flex-col min-h-0">
                <h3 className="text-sm font-semibold mb-3" style={{ color: '#22c55e' }}>Create a Collective</h3>
                <p className="text-xs mb-4" style={{ color: 'var(--text-secondary)' }}>
                  {config.group.description}
                </p>
                <div className="flex gap-2">
                  <input
                    value={groupName}
                    onChange={e => setGroupName(e.target.value)}
                    placeholder="Group name"
                    className="flex-1 px-3 py-2 rounded text-sm outline-none"
                    style={{ background: 'var(--bg-card)', border: '1px solid var(--border)', color: 'var(--text-primary)' }}
                  />
                  <button onClick={handleCreate} className="btn px-4 py-2 text-sm" style={{ background: 'rgba(34,197,94,0.1)', color: '#22c55e', border: '1px solid rgba(34,197,94,0.25)' }}>
                    Create
                  </button>
                </div>
              </div>

              {/* Browse groups */}
              <div className="flex-1 panel p-4 flex flex-col min-h-0">
                <h3 className="text-sm font-semibold mb-3" style={{ color: '#22c55e' }}>Join a Collective</h3>
                <div className="space-y-2 overflow-y-auto min-h-0 flex-1">
                  {allGroups.length > 0 ? allGroups.map(g => (
                    <div key={g.id} className="panel-card p-3 flex items-center justify-between">
                      <div>
                        <div className="font-medium text-sm">{g.name}</div>
                        <div className="font-mono text-xs mt-0.5" style={{ color: 'var(--text-secondary)' }}>
                          Open
                        </div>
                      </div>
                      <button
                        onClick={async () => {
                          setError('');
                          try {
                            const data = await api.joinGroup(g.name);
                            setGroup(data);
                          } catch (e) {
                            setError((e as Error).message);
                          }
                        }}
                        className="btn px-3 py-1 text-xs"
                        style={{ background: 'rgba(59,130,246,0.1)', color: '#3b82f6', border: '1px solid rgba(59,130,246,0.25)' }}
                      >
                        Join
                      </button>
                    </div>
                  )) : (
                    <p className="font-mono text-xs" style={{ color: 'var(--text-muted)' }}>No groups yet — create the first one!</p>
                  )}
                </div>
              </div>
            </>
          ) : (
            /* In a group — show details */
            <>
              <div className="flex-1 panel p-4 flex flex-col min-h-0">
                <div className="flex justify-between items-center mb-3 shrink-0">
                  <h3 className="text-sm font-semibold" style={{ color: '#22c55e' }}>{group.group.name}</h3>
                  <span className="font-mono text-xs px-2 py-0.5 rounded" style={{ background: 'rgba(34,197,94,0.1)', color: '#22c55e' }}>
                    {group.my_role}
                  </span>
                </div>
                <div className="font-mono text-xs mb-3 shrink-0" style={{ color: 'var(--text-secondary)' }}>
                  Combined Compute Pool: {(group.compute_pool || 0).toLocaleString()} CU
                </div>
                <div className="space-y-2 overflow-y-auto min-h-0 flex-1">
                  {(group.members || []).map(m => (
                    <div key={m.user_id} className="panel-card p-3 flex justify-between items-center">
                      <div>
                        <div className="font-medium text-sm">{m.display_name}</div>
                        <div className="font-mono text-xs mt-0.5" style={{ color: 'var(--text-secondary)' }}>{m.role}</div>
                      </div>
                    </div>
                  ))}
                </div>
                <button
                  onClick={handleLeave}
                  className="btn mt-3 w-full py-2 text-sm shrink-0"
                  style={{ background: 'rgba(239,68,68,0.1)', color: 'var(--accent-red)', border: '1px solid rgba(239,68,68,0.2)' }}
                >
                  {group.my_role === 'founder' ? 'Disband Group' : 'Leave Group'}
                </button>
              </div>
            </>
          )}
        </div>
      )}

      {tab === 'leaderboard' && (
        <div className="flex-1 min-h-0 flex gap-4">
          {/* Category selector */}
          <div className="w-48 shrink-0 panel p-4 flex flex-col min-h-0">
            <h3 className="text-sm font-semibold mb-3 shrink-0" style={{ color: '#f59e0b' }}>Categories</h3>
            <div className="space-y-1">
              {LB_CATEGORIES.map(cat => (
                <button
                  key={cat.id}
                  onClick={() => setLbCategory(cat.id)}
                  className="btn w-full text-left px-3 py-2 text-sm rounded"
                  style={{
                    background: lbCategory === cat.id ? 'rgba(245,158,11,0.1)' : 'transparent',
                    color: lbCategory === cat.id ? '#f59e0b' : 'var(--text-secondary)',
                    border: lbCategory === cat.id ? '1px solid rgba(245,158,11,0.25)' : '1px solid transparent',
                  }}
                >
                  {cat.label}
                </button>
              ))}
            </div>
          </div>

          {/* Rankings */}
          <div className="flex-1 panel p-4 flex flex-col min-h-0">
            <h3 className="text-sm font-semibold mb-3 shrink-0" style={{ color: '#f59e0b' }}>
              Top Players — {LB_CATEGORIES.find(c => c.id === lbCategory)?.label}
            </h3>
            <div className="space-y-2 overflow-y-auto min-h-0 flex-1">
              {leaderboard?.entries?.length ? leaderboard.entries.map(e => (
                <div key={e.id} className="panel-card p-3 flex items-center justify-between">
                  <div className="flex items-center gap-3">
                    <span className="font-mono text-sm w-6 text-center" style={{
                      color: e.rank === 1 ? '#fbbf24' : e.rank === 2 ? '#94a3b8' : e.rank === 3 ? '#cd7f32' : 'var(--text-muted)',
                      fontWeight: e.rank <= 3 ? 700 : 400,
                    }}>
                      {e.rank}
                    </span>
                    <div className="font-medium text-sm">{e.username}</div>
                  </div>
                  <span className="font-mono text-sm" style={{ color: '#f59e0b' }}>{e.score.toLocaleString()}</span>
                </div>
              )) : (
                <p className="font-mono text-xs" style={{ color: 'var(--text-muted)' }}>No entries yet</p>
              )}
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
