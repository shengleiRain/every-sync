import React, { useCallback, useState } from 'react';

export type ToastType = 'success' | 'error' | 'info';

interface Toast {
  id: number;
  message: string;
  type: ToastType;
}

let nextId = 0;
const listeners: Array<(toast: Toast) => void> = [];

export function showToast(message: string, type: ToastType = 'info') {
  const toast: Toast = { id: nextId++, message, type };
  listeners.forEach((fn) => fn(toast));
}

const typeStyles: Record<ToastType, React.CSSProperties> = {
  success: { borderLeft: '3px solid var(--accent-green)' },
  error: { borderLeft: '3px solid var(--accent-red)' },
  info: { borderLeft: '3px solid var(--accent-blue)' },
};

export const ToastContainer: React.FC = () => {
  const [toasts, setToasts] = useState<Toast[]>([]);

  const addToast = useCallback((toast: Toast) => {
    setToasts((prev) => [...prev, toast]);
    setTimeout(() => {
      setToasts((prev) => prev.filter((t) => t.id !== toast.id));
    }, 4000);
  }, []);

  React.useEffect(() => {
    listeners.push(addToast);
    return () => {
      const idx = listeners.indexOf(addToast);
      if (idx >= 0) listeners.splice(idx, 1);
    };
  }, [addToast]);

  if (toasts.length === 0) return null;

  return (
    <div
      style={{
        position: 'fixed',
        top: 'var(--space-4)',
        right: 'var(--space-4)',
        zIndex: 1000,
        display: 'flex',
        flexDirection: 'column',
        gap: 'var(--space-2)',
        pointerEvents: 'none',
      }}
    >
      {toasts.map((t) => (
        <div
          key={t.id}
          className="card"
          style={{
            ...typeStyles[t.type],
            padding: 'var(--space-3) var(--space-4)',
            fontSize: 'var(--text-sm)',
            boxShadow: 'var(--shadow-lg)',
            animation: 'slideIn 200ms ease',
            pointerEvents: 'auto',
            maxWidth: '380px',
          }}
        >
          {t.message}
        </div>
      ))}
    </div>
  );
};
