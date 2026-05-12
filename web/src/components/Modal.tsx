import React, { useEffect } from 'react';
import { CloseIcon } from './Icons';

interface ModalProps {
  open: boolean;
  onClose: () => void;
  title: string;
  children: React.ReactNode;
  width?: number;
}

export const Modal: React.FC<ModalProps> = ({ open, onClose, title, children, width = 560 }) => {
  useEffect(() => {
    if (!open) return;
    const handler = (e: KeyboardEvent) => { if (e.key === 'Escape') onClose(); };
    document.addEventListener('keydown', handler);
    return () => document.removeEventListener('keydown', handler);
  }, [open, onClose]);

  if (!open) return null;

  return (
    <div
      style={{
        position: 'fixed',
        inset: 0,
        zIndex: 500,
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        background: 'var(--bg-overlay)',
      }}
      onClick={onClose}
    >
      <div
        className="card"
        style={{
          width: '100%',
          maxWidth: width,
          maxHeight: '85vh',
          overflowY: 'auto',
          margin: 'var(--space-4)',
          padding: 0,
        }}
        onClick={(e) => e.stopPropagation()}
      >
        <div
          style={{
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            padding: 'var(--space-4) var(--space-5)',
            borderBottom: '1px solid var(--border-muted)',
          }}
        >
          <h2 style={{ fontSize: 'var(--text-lg)', fontWeight: 600, margin: 0 }}>{title}</h2>
          <button
            onClick={onClose}
            style={{
              border: 'none',
              background: 'none',
              cursor: 'pointer',
              color: 'var(--text-secondary)',
              padding: 'var(--space-1)',
            }}
          >
            <CloseIcon size={18} />
          </button>
        </div>
        <div style={{ padding: 'var(--space-5)' }}>{children}</div>
      </div>
    </div>
  );
};
