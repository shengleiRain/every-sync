import React from 'react';

export const ProgressBar: React.FC<{ percent: number; tone?: 'blue' | 'green'; height?: number }> = ({
  percent,
  tone = 'blue',
  height = 4,
}) => {
  const clamped = Math.min(100, Math.max(0, Number.isFinite(percent) ? percent : 0));
  const color = tone === 'green' ? 'var(--accent-green)' : 'var(--accent-blue)';
  return (
    <div style={{ width: '100%', height, background: 'var(--border-default)', borderRadius: height, overflow: 'hidden' }}>
      <div style={{ width: `${clamped}%`, height: '100%', background: color, transition: 'width 0.2s ease' }} />
    </div>
  );
};
