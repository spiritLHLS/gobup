export const createDefaultDashboardConfig = () => ({
  autoFileScan: true,
  enableFileWatcher: true,
  fileScanInterval: 60,
  fileScanMinAge: 12,
  fileScanMinSize: 1048576,
  fileScanMaxAge: 720,
  workPath: '',
  customScanPaths: '',
  autoDataRepair: false,
  enableOrphanScan: true,
  orphanScanInterval: 360,
  enableDanmakuProxy: false,
  danmakuProxyList: '',
  uploadSpeedLimitMbps: 0,
  uploadWhileRecording: false,
  publishWhileRecording: false,
  publishMode: 'local',
  publishAgentEndpoint: '',
  publishAgentToken: '',
  publishAgentTimeout: 30,
  agentPurpose: 'both',
  agentInstallerSource: 'controller',
  agentControllerBaseUrl: '',
  agentGitHubRepo: 'spiritlhls/gobup',
  agentCdnBaseUrl: '',
  fileCheckMode: 'local',
  danmakuBurnStyle: 'default',
  danmakuFontSize: 0,
  danmakuFontColor: '',
  danmakuScrollArea: 0.75,
  danmakuDisplayArea: 0.8
})

export const normalizeDashboardConfig = (currentConfig, data) => {
  const merged = { ...currentConfig, ...(data || {}) }
  if (!merged.danmakuBurnStyle) merged.danmakuBurnStyle = 'default'
  if (!merged.danmakuScrollArea || merged.danmakuScrollArea <= 0) merged.danmakuScrollArea = 0.75
  if (!merged.danmakuDisplayArea || merged.danmakuDisplayArea <= 0) merged.danmakuDisplayArea = 0.8
  if (merged.danmakuFontSize < 0) merged.danmakuFontSize = 0
  if (merged.uploadSpeedLimitMbps < 0) merged.uploadSpeedLimitMbps = 0
  if (!['local', 'remote'].includes(merged.publishMode)) merged.publishMode = 'local'
  if (!merged.publishAgentTimeout || merged.publishAgentTimeout < 3) merged.publishAgentTimeout = 30
  if (!['upload', 'filescan', 'both'].includes(merged.agentPurpose)) merged.agentPurpose = 'both'
  if (!['controller', 'github', 'cdn'].includes(merged.agentInstallerSource)) merged.agentInstallerSource = 'controller'
  if (!merged.agentGitHubRepo) merged.agentGitHubRepo = 'spiritlhls/gobup'
  if (!['local', 'remote'].includes(merged.fileCheckMode)) merged.fileCheckMode = 'local'
  return merged
}
