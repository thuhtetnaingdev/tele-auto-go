import { Bot, Database, LayoutDashboard, Settings2, ShieldCheck, SlidersHorizontal, Terminal, UserCircle2, WandSparkles } from 'lucide-react'

import { Button } from '@/components/ui/button'
import type { MainPage, SettingsPage } from '@/features/control/types'

type NavigationMenuProps = {
  activePage: MainPage
  activeSettingsPage: SettingsPage
  onOpenPage: (page: MainPage) => void
  onOpenSettingsPage: (page: SettingsPage) => void
}

export function NavigationMenu({ activePage, activeSettingsPage, onOpenPage, onOpenSettingsPage }: NavigationMenuProps) {
  return (
    <>
      <p className="px-2 text-xs font-semibold uppercase tracking-wide text-muted-foreground">Navigation</p>
      <div className="space-y-2">
        <Button
          size="sm"
          variant={activePage === 'dashboard' ? 'default' : 'outline'}
          className="w-full justify-start"
          onClick={() => onOpenPage('dashboard')}
        >
          <LayoutDashboard className="size-4" /> Dashboard
        </Button>
        <Button
          size="sm"
          variant={activePage === 'logs' ? 'default' : 'outline'}
          className="w-full justify-start"
          onClick={() => onOpenPage('logs')}
        >
          <Terminal className="size-4" /> Logs
        </Button>
        <Button
          size="sm"
          variant={activePage === 'agents' ? 'default' : 'outline'}
          className="w-full justify-start"
          onClick={() => onOpenPage('agents')}
        >
          <Bot className="size-4" /> Agents
        </Button>
        <Button
          size="sm"
          variant={activePage === 'settings' ? 'default' : 'outline'}
          className="w-full justify-start"
          onClick={() => onOpenPage('settings')}
        >
          <Settings2 className="size-4" /> Setting
        </Button>
        {activePage === 'settings' ? (
          <div className="ml-3 space-y-1 border-l border-border/60 pl-3">
            <Button size="sm" variant={activeSettingsPage === 'soul' ? 'secondary' : 'ghost'} className="w-full justify-start" onClick={() => onOpenSettingsPage('soul')}>
              <WandSparkles className="size-4" /> Soul
            </Button>
            <Button size="sm" variant={activeSettingsPage === 'telegram' ? 'secondary' : 'ghost'} className="w-full justify-start" onClick={() => onOpenSettingsPage('telegram')}>
              <Bot className="size-4" /> Telegram
            </Button>
            <Button size="sm" variant={activeSettingsPage === 'setting' ? 'secondary' : 'ghost'} className="w-full justify-start" onClick={() => onOpenSettingsPage('setting')}>
              <SlidersHorizontal className="size-4" /> Setting
            </Button>
            <Button size="sm" variant={activeSettingsPage === 'behavior' ? 'secondary' : 'ghost'} className="w-full justify-start" onClick={() => onOpenSettingsPage('behavior')}>
              <ShieldCheck className="size-4" /> Behavior
            </Button>
            <Button size="sm" variant={activeSettingsPage === 'variables' ? 'secondary' : 'ghost'} className="w-full justify-start" onClick={() => onOpenSettingsPage('variables')}>
              <Database className="size-4" /> Global Variables
            </Button>
            <Button size="sm" variant={activeSettingsPage === 'user' ? 'secondary' : 'ghost'} className="w-full justify-start" onClick={() => onOpenSettingsPage('user')}>
              <UserCircle2 className="size-4" /> User
            </Button>
          </div>
        ) : null}
      </div>
    </>
  )
}
