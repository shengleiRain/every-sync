import React, { createContext, useContext, useState, useCallback } from 'react';

export type Lang = 'en' | 'zh';

export function getPairModeLabelKey(mode: string): string {
  return mode === 'virtual' ? 'pairs.virtual' : 'pairs.normal';
}

export function getPairDirectionLabelKey(direction: string): string {
  switch (direction) {
    case 'up':
      return 'pairs.uploadOnly';
    case 'down':
      return 'pairs.downloadOnly';
    default:
      return 'pairs.bidirectional';
  }
}

export function getSyncStatusLabelKey(status: string): string {
  switch (status) {
    case 'synced':
      return 'status.synced';
    case 'syncing':
      return 'status.syncing';
    case 'virtual':
      return 'status.virtual';
    case 'conflict':
      return 'status.conflict';
    case 'excluded':
      return 'status.excluded';
    case 'error':
      return 'status.error';
    default:
      return 'status.pending';
  }
}

const translations: Record<Lang, Record<string, string>> = {
  en: {
    // Sidebar
    'sidebar.collapse': 'Collapse',
    'sidebar.expand': 'Expand sidebar',
    'sidebar.connected': 'Connected',
    'sidebar.disconnected': 'Disconnected',
    'sidebar.lightMode': 'Light Mode',
    'sidebar.darkMode': 'Dark Mode',
    'sidebar.switchLang': '中文',
    'sidebar.dashboard': 'Dashboard',
    'sidebar.files': 'File Browser',
    'sidebar.pairs': 'Sync Pairs',
    'sidebar.providers': 'Providers',
    'sidebar.conflicts': 'Conflicts',
    'sidebar.versions': 'Versions',
    'sidebar.recent': 'Recent Records',
    'sidebar.logs': 'Logs',

    // Common
    'common.loading': 'Loading...',
    'common.cancel': 'Cancel',
    'common.saving': 'Saving...',
    'common.save': 'Save',
    'common.edit': 'Edit',
    'common.delete': 'Delete',
    'common.create': 'Create',
    'common.search': 'Search',
    'common.name': 'Name',
    'common.enabled': 'Enabled',
    'common.disabled': 'Disabled',
    'common.status': 'Status',
    'common.actions': 'Actions',
    'common.error': 'Error',
    'common.retry': 'Retry',
    'common.notAvailable': 'Not available',

    // Time
    'time.never': 'Never',
    'time.justNow': 'Just now',
    'time.minutesAgo': '{n}m ago',
    'time.hoursAgo': '{n}h ago',
    'time.daysAgo': '{n}d ago',

    // Status
    'status.running': 'Running',
    'status.paused': 'Paused',
    'status.stopped': 'Stopped',
    'status.synced': 'Synced',
    'status.syncing': 'Syncing',
    'status.virtual': 'Virtual',
    'status.conflict': 'Conflict',
    'status.excluded': 'Excluded',
    'status.error': 'Error',
    'status.active': 'Active',
    'status.pending': 'Pending',
    'status.alert': 'Alert',
    'status.needsAttention': 'Needs attention',

    // Dashboard
    'dashboard.title': 'Dashboard',
    'dashboard.subtitle': 'Overview of your sync engine',
    'dashboard.syncAll': 'Sync All',
    'dashboard.refresh': 'Refresh',
    'dashboard.engineStatus': 'Engine Status',
    'dashboard.syncPairs': 'Sync Pairs',
    'dashboard.pendingTasks': 'Pending Tasks',
    'dashboard.workers': 'Workers',
    'dashboard.upload': 'Upload',
    'dashboard.download': 'Download',
    'dashboard.conflicts': 'Conflicts',
    'dashboard.virtualFiles': 'Virtual Files',
    'dashboard.activePairs': 'Active Sync Pairs',
    'dashboard.noPairs': 'No sync pairs configured. Create one to get started.',
    'dashboard.mode': 'Mode',
    'dashboard.lastSync': 'Last Sync',
    'dashboard.files': 'Files',
    'dashboard.sync': 'Sync',
    'dashboard.syncTriggered': 'Sync triggered for all pairs',
    'dashboard.syncFailed': 'Sync failed',
    'dashboard.loadFailed': 'Failed to load dashboard data',
    'dashboard.currentFile': 'Current file',
    'dashboard.bytesTransferred': 'Transferred',

    // File Browser
    'files.title': 'File Browser',
    'files.subtitle': 'Browse and manage synced files',
    'files.selectPair': 'Select sync pair...',
    'files.local': 'Local',
    'files.remote': 'Remote',
    'files.name': 'Name',
    'files.size': 'Size',
    'files.modified': 'Modified',
    'files.items': 'items',
    'files.selectToBrowse': 'Select a sync pair to browse files',
    'files.empty': 'This folder is empty',
    'files.materialize': 'Materialize',
    'files.viewVersions': 'View Versions',
    'files.resolveConflict': 'Resolve Conflict',
    'files.exclude': 'Exclude',
    'files.error': 'Error',
    'files.loadPairsFailed': 'Failed to load sync pairs',
    'files.materializeFailed': 'Materialize failed',
    'files.selectionFailed': 'Folder selection failed',

    // Sync Pairs
    'pairs.title': 'Sync Pairs',
    'pairs.subtitle': 'Configure and manage your sync pairs',
    'pairs.newPair': '+ New Pair',
    'pairs.noPairs': 'No sync pairs configured. Click "+ New Pair" to create one.',
    'pairs.editPair': 'Edit Sync Pair',
    'pairs.newPairTitle': 'New Sync Pair',
    'pairs.provider': 'Provider',
    'pairs.selectProvider': 'Select provider',
    'pairs.localPath': 'Local Path',
    'pairs.remotePath': 'Remote Path',
    'pairs.direction': 'Direction',
    'pairs.mode': 'Mode',
    'pairs.conflictStrategy': 'Conflict Strategy',
    'pairs.includePatterns': 'Include Patterns',
    'pairs.excludePatterns': 'Exclude Patterns',
    'pairs.saveChanges': 'Save Changes',
    'pairs.createPair': 'Create Pair',
    'pairs.disable': 'Disable',
    'pairs.enable': 'Enable',
    'pairs.bidirectional': 'Bidirectional',
    'pairs.uploadOnly': 'Upload Only',
    'pairs.downloadOnly': 'Download Only',
    'pairs.normal': 'Normal',
    'pairs.latestWins': 'Latest Wins',
    'pairs.localWins': 'Local Wins',
    'pairs.remoteWins': 'Remote Wins',
    'pairs.manual': 'Manual',
    'pairs.skip': 'Skip',
    'pairs.mirror': 'Mirror',
    'pairs.selective': 'Selective',
    'pairs.virtual': 'Virtual',
    'pairs.confirmDelete': 'Delete this sync pair?',
    'pairs.pairUpdated': 'Pair updated',
    'pairs.pairCreated': 'Pair created',
    'pairs.pairDeleted': 'Pair deleted',
    'pairs.pairDisabled': 'Pair disabled',
    'pairs.pairEnabled': 'Pair enabled',
    'pairs.syncTriggered': 'Sync triggered',
    'pairs.operationFailed': 'Operation failed',
    'pairs.loadFailed': 'Failed to load sync pairs',

    // Providers
    'providers.title': 'Providers',
    'providers.subtitle': 'Configure cloud storage providers',
    'providers.newProvider': '+ New Provider',
    'providers.noProviders': 'No providers configured. Click "+ New Provider" to add one.',
    'providers.configured': 'Configured',
    'providers.notConfigured': 'Not configured',
    'providers.editProvider': 'Edit Provider',
    'providers.newProviderTitle': 'New Provider',
    'providers.type': 'Type',
    'providers.params': 'Parameters (JSON)',
    'providers.saveChanges': 'Save Changes',
    'providers.createProvider': 'Create Provider',
    'providers.providerUpdated': 'Provider updated',
    'providers.providerCreated': 'Provider created',
    'providers.providerDeleted': 'Provider deleted',
    'providers.confirmDelete': 'Delete this provider?',
    'providers.loadFailed': 'Failed to load providers',
    'providers.invalidParams': 'Parameters must be valid JSON object values',
    'providers.testConnection': 'Test Connection',
    'providers.testing': 'Testing...',
    'providers.testSuccess': 'Connection successful!',
    'providers.testFailed': 'Connection failed',
    'providers.confirmCascadeDelete': 'This provider is used by: {pairs}. Delete provider and all dependent sync pairs?',
    'providers.providerAndPairsDeleted': 'Provider and dependent pairs deleted',
    'providers.deleteFailed': 'Delete failed',
    'providers.paramsHint.webdav': 'endpoint, username, password, prefix, timeout, auth_mode',
    'providers.paramsHint.local': 'root_path',

    // Conflicts
    'conflicts.title': 'Conflicts',
    'conflicts.subtitle': 'Resolve file synchronization conflicts',
    'conflicts.noConflicts': 'No conflicts detected. All files are in sync.',
    'conflicts.local': 'Local',
    'conflicts.remote': 'Remote',
    'conflicts.modified': 'Modified',
    'conflicts.size': 'Size',
    'conflicts.bytes': 'bytes',
    'conflicts.keepLocal': 'Keep Local',
    'conflicts.keepRemote': 'Keep Remote',
    'conflicts.latestWins': 'Latest Wins',
    'conflicts.skip': 'Skip',
    'conflicts.resolved': 'Conflict resolved',
    'conflicts.resolutionFailed': 'Resolution failed',
    'conflicts.loadFailed': 'Failed to load conflicts',

    // Versions
    'versions.title': 'Version History',
    'versions.subtitle': 'View and restore previous file versions',
    'versions.syncPair': 'Sync Pair',
    'versions.selectPair': 'Select pair',
    'versions.path': 'Path',
    'versions.search': 'Search',
    'versions.source': 'Source',
    'versions.size': 'Size',
    'versions.fileTime': 'File Time',
    'versions.recorded': 'Recorded',
    'versions.restore': 'Restore',
    'versions.noRecords': 'No version records found.',
    'versions.selectHint': 'Select a sync pair and path to view version history.',
    'versions.versionRestored': 'Version restored',
    'versions.restoreFailed': 'Restore failed',
    'versions.loadFailed': 'Failed to load versions',
    'versions.pairsLoadFailed': 'Failed to load sync pairs',

    // Logs
    'logs.title': 'Logs',
    'logs.subtitle': 'Sync engine activity log',
    'logs.filterPlaceholder': 'Filter logs...',
    'logs.allLevels': 'All levels',
    'logs.debug': 'Debug',
    'logs.info': 'Info',
    'logs.warning': 'Warning',
    'logs.error': 'Error',
    'logs.resume': 'Resume',
    'logs.pause': 'Pause',
    'logs.clear': 'Clear',
    'logs.noEntries': 'No log entries found.',
    'logs.total': 'Total',
    'logs.showing': 'Showing',
    'logs.paused': 'PAUSED',
    'logs.logsCleared': 'Logs cleared',
    'logs.loadFailed': 'Failed to load logs',
    'logs.connected': 'Live',
    'logs.disconnected': 'Disconnected',
    'logs.scrollToBottom': 'Scroll to bottom',

    // Progress
    'progress.processing': 'Processing...',
    'progress.noActive': 'No active transfer',
    'progress.current': 'Current',
    'progress.queue': 'Queue',
    'progress.transfer': 'Transfer',
    'progress.recent': 'Recent',
    'progress.noRecent': 'No recent files',
    'progress.expandQueue': 'Show queue',
    'progress.collapseQueue': 'Hide queue',
    'progress.noQueuedFiles': 'No queued files',
    'progress.viewRecent': 'View recent records',

    // Recent records
    'recent.title': 'Recent Records',
    'recent.subtitle': 'Recently synchronized files',
    'recent.file': 'File',
    'recent.pair': 'Sync Pair',
    'recent.time': 'Sync Time',
    'recent.status': 'Status',
    'recent.direction': 'Direction',
    'recent.size': 'Size',
    'recent.empty': 'No recent sync records.',
    'recent.loadFailed': 'Failed to load recent records',
  },

  zh: {
    // Sidebar
    'sidebar.collapse': '收起',
    'sidebar.expand': '展开侧边栏',
    'sidebar.connected': '已连接',
    'sidebar.disconnected': '已断开',
    'sidebar.lightMode': '浅色模式',
    'sidebar.darkMode': '深色模式',
    'sidebar.switchLang': 'EN',
    'sidebar.dashboard': '仪表盘',
    'sidebar.files': '文件浏览',
    'sidebar.pairs': '同步对',
    'sidebar.providers': '存储提供商',
    'sidebar.conflicts': '冲突',
    'sidebar.versions': '版本历史',
    'sidebar.recent': '最近记录',
    'sidebar.logs': '日志',

    // Common
    'common.loading': '加载中...',
    'common.cancel': '取消',
    'common.saving': '保存中...',
    'common.save': '保存',
    'common.edit': '编辑',
    'common.delete': '删除',
    'common.create': '创建',
    'common.search': '搜索',
    'common.name': '名称',
    'common.enabled': '已启用',
    'common.disabled': '已禁用',
    'common.status': '状态',
    'common.actions': '操作',
    'common.error': '错误',
    'common.retry': '重试',
    'common.notAvailable': '不可用',

    // Time
    'time.never': '从未',
    'time.justNow': '刚刚',
    'time.minutesAgo': '{n}分钟前',
    'time.hoursAgo': '{n}小时前',
    'time.daysAgo': '{n}天前',

    // Status
    'status.running': '运行中',
    'status.paused': '已暂停',
    'status.stopped': '已停止',
    'status.synced': '已同步',
    'status.syncing': '同步中',
    'status.virtual': '虚拟',
    'status.conflict': '冲突',
    'status.excluded': '已排除',
    'status.error': '错误',
    'status.active': '活动',
    'status.pending': '等待中',
    'status.alert': '警告',
    'status.needsAttention': '需要关注',

    // Dashboard
    'dashboard.title': '仪表盘',
    'dashboard.subtitle': '同步引擎概览',
    'dashboard.syncAll': '全部同步',
    'dashboard.refresh': '刷新',
    'dashboard.engineStatus': '引擎状态',
    'dashboard.syncPairs': '同步对',
    'dashboard.pendingTasks': '待处理任务',
    'dashboard.workers': '工作线程',
    'dashboard.upload': '上传',
    'dashboard.download': '下载',
    'dashboard.conflicts': '冲突',
    'dashboard.virtualFiles': '虚拟文件',
    'dashboard.activePairs': '活动同步对',
    'dashboard.noPairs': '暂无同步对配置，请先创建一个。',
    'dashboard.mode': '模式',
    'dashboard.lastSync': '上次同步',
    'dashboard.files': '文件',
    'dashboard.sync': '同步',
    'dashboard.syncTriggered': '已触发全部同步',
    'dashboard.syncFailed': '同步失败',
    'dashboard.loadFailed': '加载仪表盘数据失败',
    'dashboard.currentFile': '当前文件',
    'dashboard.bytesTransferred': '已传输',

    // File Browser
    'files.title': '文件浏览器',
    'files.subtitle': '浏览和管理同步文件',
    'files.selectPair': '选择同步对...',
    'files.local': '本地',
    'files.remote': '远程',
    'files.name': '名称',
    'files.size': '大小',
    'files.modified': '修改时间',
    'files.items': '项',
    'files.selectToBrowse': '选择一个同步对来浏览文件',
    'files.empty': '此文件夹为空',
    'files.materialize': '实体化',
    'files.viewVersions': '查看版本',
    'files.resolveConflict': '解决冲突',
    'files.exclude': '排除',
    'files.error': '错误',
    'files.loadPairsFailed': '加载同步对失败',
    'files.materializeFailed': '实体化失败',
    'files.selectionFailed': '文件夹选择失败',

    // Sync Pairs
    'pairs.title': '同步对',
    'pairs.subtitle': '配置和管理同步对',
    'pairs.newPair': '+ 新建同步对',
    'pairs.noPairs': '暂无同步对配置，点击"+ 新建同步对"创建。',
    'pairs.editPair': '编辑同步对',
    'pairs.newPairTitle': '新建同步对',
    'pairs.provider': '提供商',
    'pairs.selectProvider': '选择提供商',
    'pairs.localPath': '本地路径',
    'pairs.remotePath': '远程路径',
    'pairs.direction': '方向',
    'pairs.mode': '模式',
    'pairs.conflictStrategy': '冲突策略',
    'pairs.includePatterns': '包含规则',
    'pairs.excludePatterns': '排除规则',
    'pairs.saveChanges': '保存修改',
    'pairs.createPair': '创建同步对',
    'pairs.disable': '禁用',
    'pairs.enable': '启用',
    'pairs.bidirectional': '双向',
    'pairs.uploadOnly': '仅上传',
    'pairs.downloadOnly': '仅下载',
    'pairs.normal': '普通',
    'pairs.latestWins': '最新优先',
    'pairs.localWins': '本地优先',
    'pairs.remoteWins': '远程优先',
    'pairs.manual': '手动',
    'pairs.skip': '跳过',
    'pairs.mirror': '镜像',
    'pairs.selective': '选择性',
    'pairs.virtual': '虚拟',
    'pairs.confirmDelete': '确定删除此同步对？',
    'pairs.pairUpdated': '同步对已更新',
    'pairs.pairCreated': '同步对已创建',
    'pairs.pairDeleted': '同步对已删除',
    'pairs.pairDisabled': '同步对已禁用',
    'pairs.pairEnabled': '同步对已启用',
    'pairs.syncTriggered': '同步已触发',
    'pairs.operationFailed': '操作失败',
    'pairs.loadFailed': '加载同步对失败',

    // Providers
    'providers.title': '存储提供商',
    'providers.subtitle': '配置云存储提供商',
    'providers.newProvider': '+ 新建提供商',
    'providers.noProviders': '暂无提供商配置，点击"+ 新建提供商"添加。',
    'providers.configured': '已配置',
    'providers.notConfigured': '未配置',
    'providers.editProvider': '编辑提供商',
    'providers.newProviderTitle': '新建提供商',
    'providers.type': '类型',
    'providers.params': '参数 (JSON)',
    'providers.saveChanges': '保存修改',
    'providers.createProvider': '创建提供商',
    'providers.providerUpdated': '提供商已更新',
    'providers.providerCreated': '提供商已创建',
    'providers.providerDeleted': '提供商已删除',
    'providers.confirmDelete': '确定删除此提供商？',
    'providers.loadFailed': '加载提供商失败',
    'providers.invalidParams': '参数必须是有效的 JSON 对象',
    'providers.testConnection': '测试连接',
    'providers.testing': '测试中...',
    'providers.testSuccess': '连接成功！',
    'providers.testFailed': '连接失败',
    'providers.confirmCascadeDelete': '此存储源被以下同步对使用：{pairs}。确定删除存储源及所有关联的同步对？',
    'providers.providerAndPairsDeleted': '存储源及关联同步对已删除',
    'providers.deleteFailed': '删除失败',
    'providers.paramsHint.webdav': 'endpoint, username, password, prefix, timeout, auth_mode',
    'providers.paramsHint.local': 'root_path',

    // Conflicts
    'conflicts.title': '冲突',
    'conflicts.subtitle': '解决文件同步冲突',
    'conflicts.noConflicts': '未检测到冲突，所有文件已同步。',
    'conflicts.local': '本地',
    'conflicts.remote': '远程',
    'conflicts.modified': '修改时间',
    'conflicts.size': '大小',
    'conflicts.bytes': '字节',
    'conflicts.keepLocal': '保留本地',
    'conflicts.keepRemote': '保留远程',
    'conflicts.latestWins': '最新优先',
    'conflicts.skip': '跳过',
    'conflicts.resolved': '冲突已解决',
    'conflicts.resolutionFailed': '解决失败',
    'conflicts.loadFailed': '加载冲突失败',

    // Versions
    'versions.title': '版本历史',
    'versions.subtitle': '查看和恢复历史文件版本',
    'versions.syncPair': '同步对',
    'versions.selectPair': '选择同步对',
    'versions.path': '路径',
    'versions.search': '搜索',
    'versions.source': '来源',
    'versions.size': '大小',
    'versions.fileTime': '文件时间',
    'versions.recorded': '记录时间',
    'versions.restore': '恢复',
    'versions.noRecords': '未找到版本记录。',
    'versions.selectHint': '选择一个同步对和路径来查看版本历史。',
    'versions.versionRestored': '版本已恢复',
    'versions.restoreFailed': '恢复失败',
    'versions.loadFailed': '加载版本失败',
    'versions.pairsLoadFailed': '加载同步对失败',

    // Logs
    'logs.title': '日志',
    'logs.subtitle': '同步引擎活动日志',
    'logs.filterPlaceholder': '筛选日志...',
    'logs.allLevels': '所有级别',
    'logs.debug': '调试',
    'logs.info': '信息',
    'logs.warning': '警告',
    'logs.error': '错误',
    'logs.resume': '继续',
    'logs.pause': '暂停',
    'logs.clear': '清空',
    'logs.noEntries': '暂无日志条目。',
    'logs.total': '总计',
    'logs.showing': '显示',
    'logs.paused': '已暂停',
    'logs.logsCleared': '日志已清空',
    'logs.loadFailed': '加载日志失败',
    'logs.connected': '实时',
    'logs.disconnected': '已断开',
    'logs.scrollToBottom': '滚动到底部',

    // Progress
    'progress.processing': '处理中...',
    'progress.noActive': '没有正在传输的文件',
    'progress.current': '当前文件',
    'progress.queue': '队列',
    'progress.transfer': '传输',
    'progress.recent': '最近记录',
    'progress.noRecent': '暂无最近文件',
    'progress.expandQueue': '展开队列',
    'progress.collapseQueue': '收起队列',
    'progress.noQueuedFiles': '暂无队列文件',
    'progress.viewRecent': '查看最近记录',

    // Recent records
    'recent.title': '最近记录',
    'recent.subtitle': '近期同步过的文件',
    'recent.file': '文件名',
    'recent.pair': '同步对',
    'recent.time': '同步时间',
    'recent.status': '同步状态',
    'recent.direction': '同步方向',
    'recent.size': '大小',
    'recent.empty': '暂无最近同步记录。',
    'recent.loadFailed': '加载最近记录失败',
  },
};

interface I18nContextValue {
  lang: Lang;
  t: (key: string, params?: Record<string, string | number>) => string;
  toggleLang: () => void;
}

const I18nContext = createContext<I18nContextValue>({
  lang: 'en',
  t: (key: string) => key,
  toggleLang: () => {},
});

export const useI18n = () => useContext(I18nContext);

export const LanguageProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const [lang, setLang] = useState<Lang>(() => {
    if (typeof window === 'undefined') return 'en';
    return (localStorage.getItem('lang') as Lang) || 'en';
  });

  const toggleLang = useCallback(() => {
    setLang((prev) => {
      const next = prev === 'en' ? 'zh' : 'en';
      localStorage.setItem('lang', next);
      return next;
    });
  }, []);

  const t = useCallback(
    (key: string, params?: Record<string, string | number>): string => {
      let value = translations[lang]?.[key] ?? translations.en[key] ?? key;
      if (params) {
        Object.entries(params).forEach(([k, v]) => {
          value = value.replace(`{${k}}`, String(v));
        });
      }
      return value;
    },
    [lang],
  );

  return (
    <I18nContext.Provider value={{ lang, t, toggleLang }}>
      {children}
    </I18nContext.Provider>
  );
};
