import type { MainPage, SettingsPage } from '@/features/control/types'

export const SETTINGS_PAGES: SettingsPage[] = ['soul', 'telegram', 'setting', 'behavior', 'variables', 'user']

export function readRouteState(pathname: string): { activePage: MainPage; activeSettingsPage: SettingsPage } {
  if (pathname.startsWith('/logs')) {
    return { activePage: 'logs', activeSettingsPage: 'setting' }
  }
  if (pathname.startsWith('/agents')) {
    return { activePage: 'agents', activeSettingsPage: 'setting' }
  }
  if (pathname.startsWith('/settings/')) {
    const maybePage = pathname.slice('/settings/'.length).split('/')[0]
    const activeSettingsPage = SETTINGS_PAGES.includes(maybePage as SettingsPage)
      ? (maybePage as SettingsPage)
      : 'setting'
    return { activePage: 'settings', activeSettingsPage }
  }
  if (pathname === '/settings') {
    return { activePage: 'settings', activeSettingsPage: 'setting' }
  }
  return { activePage: 'dashboard', activeSettingsPage: 'setting' }
}

export function formatCurrentPageLabel(activePage: MainPage, activeSettingsPage: SettingsPage) {
  if (activePage === 'settings') {
    return `Settings / ${activeSettingsPage === 'soul'
      ? 'Soul'
      : activeSettingsPage === 'telegram'
        ? 'Telegram'
        : activeSettingsPage === 'behavior'
          ? 'Behavior'
          : activeSettingsPage === 'setting'
            ? 'Setting'
            : activeSettingsPage === 'variables'
              ? 'Global Variables'
              : 'User'}`
  }
  if (activePage === 'agents') {
    return 'Agents'
  }
  if (activePage === 'logs') {
    return 'Logs'
  }
  return 'Dashboard'
}
