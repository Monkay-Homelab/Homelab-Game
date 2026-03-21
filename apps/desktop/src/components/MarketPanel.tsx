import { useState, useRef, useEffect, useCallback } from 'react';
import type { GameState, BitcoinPricePoint } from '../api';
import { useGameStore } from '../stores/gameStore';
import { useConfig } from '../hooks/useConfig';

function formatCurrency(n: number): string {
  if (n >= 1_000_000) return '$' + (n / 1_000_000).toFixed(1) + 'M';
  if (n >= 1_000) return '$' + n.toLocaleString();
  return '$' + n.toString();
}

function PriceChart({ history, minPrice, maxPrice }: { history: BitcoinPricePoint[]; minPrice: number; maxPrice: number }) {
  const canvasRef = useRef<HTMLCanvasElement>(null);

  const draw = useCallback(() => {
    const canvas = canvasRef.current;
    if (!canvas || history.length < 2) return;

    const ctx = canvas.getContext('2d');
    if (!ctx) return;

    const dpr = window.devicePixelRatio || 1;
    const rect = canvas.getBoundingClientRect();
    canvas.width = rect.width * dpr;
    canvas.height = rect.height * dpr;
    ctx.scale(dpr, dpr);

    const w = rect.width;
    const h = rect.height;
    const padding = { top: 20, right: 50, bottom: 20, left: 10 };
    const chartW = w - padding.left - padding.right;
    const chartH = h - padding.top - padding.bottom;

    // Compute price bounds from data with some margin
    const prices = history.map(p => p.price);
    const dataMin = Math.min(...prices);
    const dataMax = Math.max(...prices);
    const margin = Math.max((dataMax - dataMin) * 0.1, 100);
    const yMin = Math.max(minPrice, dataMin - margin);
    const yMax = Math.min(maxPrice, dataMax + margin);

    // Clear
    ctx.clearRect(0, 0, w, h);

    // Draw reference lines for min/max price bounds
    ctx.strokeStyle = 'rgba(148, 163, 184, 0.15)';
    ctx.lineWidth = 1;
    ctx.setLineDash([4, 4]);

    // Mean price line
    const meanPrice = (minPrice + maxPrice) / 2;
    if (meanPrice >= yMin && meanPrice <= yMax) {
      const meanY = padding.top + chartH * (1 - (meanPrice - yMin) / (yMax - yMin));
      ctx.beginPath();
      ctx.moveTo(padding.left, meanY);
      ctx.lineTo(padding.left + chartW, meanY);
      ctx.stroke();

      ctx.fillStyle = 'rgba(148, 163, 184, 0.4)';
      ctx.font = '10px monospace';
      ctx.textAlign = 'left';
      ctx.fillText(formatCurrency(meanPrice), padding.left + chartW + 4, meanY + 3);
    }

    ctx.setLineDash([]);

    // Draw price line
    const isUp = prices[prices.length - 1] >= prices[prices.length - 2];
    const lineColor = isUp ? 'rgba(34, 197, 94, 0.9)' : 'rgba(239, 68, 68, 0.9)';
    const fillColor = isUp ? 'rgba(34, 197, 94, 0.08)' : 'rgba(239, 68, 68, 0.08)';

    ctx.beginPath();
    for (let i = 0; i < history.length; i++) {
      const x = padding.left + (i / (history.length - 1)) * chartW;
      const y = padding.top + chartH * (1 - (history[i].price - yMin) / (yMax - yMin));
      if (i === 0) ctx.moveTo(x, y);
      else ctx.lineTo(x, y);
    }
    ctx.strokeStyle = lineColor;
    ctx.lineWidth = 1.5;
    ctx.stroke();

    // Fill under line
    const lastX = padding.left + chartW;
    const baseY = padding.top + chartH;
    ctx.lineTo(lastX, baseY);
    ctx.lineTo(padding.left, baseY);
    ctx.closePath();
    ctx.fillStyle = fillColor;
    ctx.fill();

    // Draw current price label on right
    const currentPrice = prices[prices.length - 1];
    const currentY = padding.top + chartH * (1 - (currentPrice - yMin) / (yMax - yMin));
    ctx.fillStyle = isUp ? 'rgba(34, 197, 94, 1)' : 'rgba(239, 68, 68, 1)';
    ctx.font = 'bold 11px monospace';
    ctx.textAlign = 'left';
    ctx.fillText(formatCurrency(currentPrice), padding.left + chartW + 4, currentY + 4);

    // Y-axis labels (top and bottom)
    ctx.fillStyle = 'rgba(148, 163, 184, 0.5)';
    ctx.font = '10px monospace';
    ctx.textAlign = 'left';
    ctx.fillText(formatCurrency(yMax), padding.left + chartW + 4, padding.top + 3);
    ctx.fillText(formatCurrency(yMin), padding.left + chartW + 4, padding.top + chartH + 3);
  }, [history, minPrice, maxPrice]);

  useEffect(() => {
    draw();
    const handleResize = () => draw();
    window.addEventListener('resize', handleResize);
    return () => window.removeEventListener('resize', handleResize);
  }, [draw]);

  if (history.length < 2) {
    return (
      <div className="flex-1 flex items-center justify-center">
        <span className="font-mono text-xs" style={{ color: 'var(--text-muted)' }}>Waiting for price data...</span>
      </div>
    );
  }

  return (
    <canvas
      ref={canvasRef}
      className="w-full flex-1"
      style={{ minHeight: 0 }}
    />
  );
}

export function MarketPanel({ state }: { state: GameState }) {
  const config = useConfig();
  const btcConfig = config.bitcoin ?? { min_price: 1000, max_price: 50000, step_interval: 5, mean_price: 10000, buy_compute_cost_per_btc: 1000 };
  const buyBitcoin = useGameStore(s => s.buyBitcoin);
  const sellBitcoin = useGameStore(s => s.sellBitcoin);
  const storeError = useGameStore(s => s.error);

  const [buyAmount, setBuyAmount] = useState(1);
  const [sellAmount, setSellAmount] = useState(1);
  const [tradeError, setTradeError] = useState<string | null>(null);

  const price = state.bitcoin_price || 0;
  const balance = state.bitcoin_balance || 0;
  const history = state.bitcoin_price_history || [];

  // Determine price direction
  const prevPrice = history.length >= 2 ? history[history.length - 2].price : price;
  const priceDirection = price > prevPrice ? 'up' : price < prevPrice ? 'down' : 'flat';
  const directionIcon = priceDirection === 'up' ? '\u25B2' : priceDirection === 'down' ? '\u25BC' : '\u2014';
  const directionColor = priceDirection === 'up' ? 'var(--accent-green)' : priceDirection === 'down' ? 'var(--accent-red)' : 'var(--text-muted)';

  // Portfolio calculations
  const btcValue = balance * price;
  const totalPortfolio = state.money + btcValue;

  // Buy/sell constraints
  const buyCost = buyAmount * price;
  const cuCostPerBTC = btcConfig.buy_compute_cost_per_btc || 1000;
  const buyCUCost = buyAmount * cuCostPerBTC;
  const canBuy = buyAmount > 0 && state.money >= buyCost && state.compute_units >= buyCUCost;
  const sellProceeds = sellAmount * price;
  const canSell = sellAmount > 0 && balance >= sellAmount;

  // Max buy (limited by both money and CU)
  const maxByMoney = price > 0 ? Math.floor(state.money / price) : 0;
  const maxByCU = cuCostPerBTC > 0 ? Math.floor(state.compute_units / cuCostPerBTC) : 0;
  const maxBuyable = Math.min(maxByMoney, maxByCU);

  async function handleBuy(amount: number) {
    if (amount <= 0) return;
    setTradeError(null);
    try {
      await buyBitcoin(amount);
    } catch (e) {
      setTradeError((e as Error).message);
    }
  }

  async function handleSell(amount: number) {
    if (amount <= 0) return;
    setTradeError(null);
    try {
      await sellBitcoin(amount);
    } catch (e) {
      setTradeError((e as Error).message);
    }
  }

  const displayError = tradeError || storeError;

  return (
    <div className="h-full flex gap-4 min-h-0">
      {/* Left: Price + Chart */}
      <div className="flex-1 panel p-4 flex flex-col min-h-0">
        {/* Price Header */}
        <div className="flex items-center justify-between mb-3 shrink-0">
          <div className="flex items-center gap-3">
            <h3 className="text-sm font-semibold" style={{ color: 'var(--accent-amber)' }}>Bitcoin Market</h3>
            <span className="font-mono text-xs" style={{ color: 'var(--text-muted)' }}>
              {btcConfig.step_interval}s updates
            </span>
          </div>
        </div>

        {/* Current Price */}
        <div className="panel-card p-3 mb-3 shrink-0">
          <div className="flex items-center gap-2">
            <span className="font-mono text-2xl font-bold" style={{ color: 'var(--accent-amber)' }}>
              {formatCurrency(price)}
            </span>
            <span className="font-mono text-sm" style={{ color: directionColor }}>
              {directionIcon}
            </span>
          </div>
          <div className="font-mono text-xs mt-1" style={{ color: 'var(--text-muted)' }}>
            Range: {formatCurrency(btcConfig.min_price)} - {formatCurrency(btcConfig.max_price)}
          </div>
        </div>

        {/* Portfolio Summary */}
        <div className="grid grid-cols-3 gap-2 mb-3 shrink-0">
          <div className="panel-card p-2 text-center">
            <div className="font-mono text-xs" style={{ color: 'var(--text-muted)' }}>Your BTC</div>
            <div className="stat-value text-sm" style={{ color: 'var(--accent-amber)' }}>{balance}</div>
          </div>
          <div className="panel-card p-2 text-center">
            <div className="font-mono text-xs" style={{ color: 'var(--text-muted)' }}>BTC Value</div>
            <div className="stat-value text-sm" style={{ color: 'var(--accent-amber)' }}>{formatCurrency(btcValue)}</div>
          </div>
          <div className="panel-card p-2 text-center">
            <div className="font-mono text-xs" style={{ color: 'var(--text-muted)' }}>Portfolio</div>
            <div className="stat-value text-sm" style={{ color: 'var(--accent-green)' }}>{formatCurrency(totalPortfolio)}</div>
          </div>
        </div>

        {/* Price Chart */}
        <div className="flex-1 min-h-0 flex flex-col">
          <div className="font-mono text-xs mb-1 shrink-0" style={{ color: 'var(--text-muted)' }}>
            Price History ({history.length} points)
          </div>
          <PriceChart history={history} minPrice={btcConfig.min_price} maxPrice={btcConfig.max_price} />
        </div>
      </div>

      {/* Right: Buy/Sell Controls */}
      <div className="w-72 shrink-0 panel p-4 flex flex-col min-h-0">
        <h3 className="text-sm font-semibold mb-3 shrink-0" style={{ color: 'var(--accent-amber)' }}>Trade</h3>

        {/* Error Display */}
        {displayError && (
          <div
            className="mb-3 px-3 py-2 rounded text-xs font-mono shrink-0"
            style={{ background: 'rgba(239,68,68,0.1)', border: '1px solid rgba(239,68,68,0.3)', color: '#fca5a5' }}
          >
            {displayError}
          </div>
        )}

        {/* Buy Section */}
        <div className="mb-4 shrink-0">
          <div className="font-mono text-xs mb-2 uppercase tracking-wide" style={{ color: 'var(--accent-green)' }}>Buy</div>
          <div className="flex gap-2 mb-2">
            <input
              type="number"
              min={1}
              value={buyAmount}
              onChange={e => setBuyAmount(Math.max(1, parseInt(e.target.value) || 1))}
              className="flex-1 px-3 py-1.5 rounded font-mono text-sm"
              style={{
                background: 'var(--bg-card)',
                border: '1px solid var(--border)',
                color: 'var(--text-primary)',
                outline: 'none',
              }}
            />
            <span className="font-mono text-xs self-center" style={{ color: 'var(--text-muted)' }}>BTC</span>
          </div>
          <div className="font-mono text-xs mb-1" style={{ color: 'var(--text-secondary)' }}>
            Cost: {formatCurrency(buyCost)}
          </div>
          <div className="font-mono text-xs mb-2" style={{ color: state.compute_units >= buyCUCost ? 'var(--text-secondary)' : 'var(--accent-red)' }}>
            CU Cost: {buyCUCost.toLocaleString()}
          </div>
          <div className="flex gap-1 mb-2">
            <button
              onClick={() => setBuyAmount(1)}
              className="btn px-2 py-1 text-xs"
              style={{ background: 'rgba(34,197,94,0.1)', color: 'var(--accent-green)', border: '1px solid rgba(34,197,94,0.2)' }}
            >
              1
            </button>
            <button
              onClick={() => setBuyAmount(10)}
              className="btn px-2 py-1 text-xs"
              style={{ background: 'rgba(34,197,94,0.1)', color: 'var(--accent-green)', border: '1px solid rgba(34,197,94,0.2)' }}
            >
              10
            </button>
            <button
              onClick={() => setBuyAmount(Math.max(1, maxBuyable))}
              className="btn px-2 py-1 text-xs"
              style={{ background: 'rgba(34,197,94,0.1)', color: 'var(--accent-green)', border: '1px solid rgba(34,197,94,0.2)' }}
            >
              Max
            </button>
          </div>
          <button
            onClick={() => handleBuy(buyAmount)}
            disabled={!canBuy}
            className="btn w-full py-2 text-sm font-medium"
            style={{
              background: canBuy ? 'rgba(34,197,94,0.15)' : 'var(--bg-card)',
              color: canBuy ? 'var(--accent-green)' : 'var(--text-muted)',
              border: `1px solid ${canBuy ? 'rgba(34,197,94,0.3)' : 'var(--border)'}`,
            }}
          >
            Buy {buyAmount} BTC
          </button>
        </div>

        {/* Divider */}
        <div className="shrink-0 mb-4" style={{ borderTop: '1px solid var(--border)' }} />

        {/* Sell Section */}
        <div className="mb-4 shrink-0">
          <div className="font-mono text-xs mb-2 uppercase tracking-wide" style={{ color: 'var(--accent-red)' }}>Sell</div>
          <div className="flex gap-2 mb-2">
            <input
              type="number"
              min={1}
              value={sellAmount}
              onChange={e => setSellAmount(Math.max(1, parseInt(e.target.value) || 1))}
              className="flex-1 px-3 py-1.5 rounded font-mono text-sm"
              style={{
                background: 'var(--bg-card)',
                border: '1px solid var(--border)',
                color: 'var(--text-primary)',
                outline: 'none',
              }}
            />
            <span className="font-mono text-xs self-center" style={{ color: 'var(--text-muted)' }}>BTC</span>
          </div>
          <div className="font-mono text-xs mb-2" style={{ color: 'var(--text-secondary)' }}>
            Proceeds: {formatCurrency(sellProceeds)}
          </div>
          <div className="flex gap-1 mb-2">
            <button
              onClick={() => setSellAmount(1)}
              className="btn px-2 py-1 text-xs"
              style={{ background: 'rgba(239,68,68,0.1)', color: 'var(--accent-red)', border: '1px solid rgba(239,68,68,0.2)' }}
            >
              1
            </button>
            <button
              onClick={() => setSellAmount(10)}
              className="btn px-2 py-1 text-xs"
              style={{ background: 'rgba(239,68,68,0.1)', color: 'var(--accent-red)', border: '1px solid rgba(239,68,68,0.2)' }}
            >
              10
            </button>
            <button
              onClick={() => setSellAmount(Math.max(1, balance))}
              className="btn px-2 py-1 text-xs"
              style={{ background: 'rgba(239,68,68,0.1)', color: 'var(--accent-red)', border: '1px solid rgba(239,68,68,0.2)' }}
            >
              All
            </button>
          </div>
          <button
            onClick={() => handleSell(sellAmount)}
            disabled={!canSell}
            className="btn w-full py-2 text-sm font-medium"
            style={{
              background: canSell ? 'rgba(239,68,68,0.15)' : 'var(--bg-card)',
              color: canSell ? 'var(--accent-red)' : 'var(--text-muted)',
              border: `1px solid ${canSell ? 'rgba(239,68,68,0.3)' : 'var(--border)'}`,
            }}
          >
            Sell {sellAmount} BTC
          </button>
        </div>

        {/* Available funds */}
        <div className="mt-auto pt-3 shrink-0" style={{ borderTop: '1px solid var(--border)' }}>
          <div className="font-mono text-xs" style={{ color: 'var(--text-muted)' }}>
            Available: {formatCurrency(state.money)}
          </div>
          <div className="font-mono text-xs" style={{ color: 'var(--text-muted)' }}>
            Holdings: {balance} BTC
          </div>
        </div>
      </div>
    </div>
  );
}
