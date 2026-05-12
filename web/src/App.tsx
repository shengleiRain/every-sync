import React, { useState, useCallback } from 'react';
import { BrowserRouter, Routes, Route, Outlet } from 'react-router-dom';
import { Sidebar } from './components/Sidebar';
import { ToastContainer } from './components/Toast';
import { Dashboard } from './pages/Dashboard';
import { FileBrowser } from './pages/FileBrowser';
import { SyncPairs } from './pages/SyncPairs';
import { Providers } from './pages/Providers';
import { Conflicts } from './pages/Conflicts';
import { Versions } from './pages/Versions';
import { Logs } from './pages/Logs';
import { useWebSocket } from './hooks/useWebSocket';
import type { WSEvent } from './api/client';

const Layout: React.FC<{ conflictCount: number; wsConnected: boolean }> = ({ conflictCount, wsConnected }) => (
  <div
    style={{
      display: 'flex',
      width: '100%',
      height: '100vh',
      overflow: 'hidden',
    }}
  >
    <Sidebar conflictCount={conflictCount} wsConnected={wsConnected} />
    <main
      style={{
        flex: 1,
        overflow: 'auto',
        background: 'var(--bg-root)',
        minHeight: 0,
      }}
    >
      <Outlet />
    </main>
  </div>
);

const App: React.FC = () => {
  const [conflictCount, setConflictCount] = useState(0);

  const handleWSEvent = useCallback((event: WSEvent) => {
    if (event.type === 'conflict') {
      setConflictCount((c) => c + 1);
    }
  }, []);

  const { connected: wsConnected } = useWebSocket({ onEvent: handleWSEvent });

  return (
    <BrowserRouter>
      <ToastContainer />
      <Routes>
        <Route element={<Layout conflictCount={conflictCount} wsConnected={wsConnected} />}>
          <Route index element={<Dashboard />} />
          <Route path="/files" element={<FileBrowser />} />
          <Route path="/pairs" element={<SyncPairs />} />
          <Route path="/providers" element={<Providers />} />
          <Route path="/conflicts" element={<Conflicts />} />
          <Route path="/versions" element={<Versions />} />
          <Route path="/logs" element={<Logs />} />
        </Route>
      </Routes>
    </BrowserRouter>
  );
};

export default App;
