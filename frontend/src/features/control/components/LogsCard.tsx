import { Terminal } from 'lucide-react'

import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import type { LogEntry } from '@/features/control/types'

type LogsCardProps = {
  logMode: 'all' | 'orchestrator'
  onLogModeChange: (mode: 'all' | 'orchestrator') => void
  filteredLogs: LogEntry[]
}

export function LogsCard({ logMode, onLogModeChange, filteredLogs }: LogsCardProps) {
  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Terminal className="size-4" /> Realtime Logs
        </CardTitle>
        <CardDescription>Streaming from backend SSE endpoint `/api/logs/stream`.</CardDescription>
        <div className="flex flex-col gap-2 sm:flex-row">
          <Button size="sm" className="w-full sm:w-auto" variant={logMode === 'all' ? 'default' : 'outline'} onClick={() => onLogModeChange('all')}>
            All
          </Button>
          <Button size="sm" className="w-full sm:w-auto" variant={logMode === 'orchestrator' ? 'default' : 'outline'} onClick={() => onLogModeChange('orchestrator')}>
            Execution Logs
          </Button>
        </div>
      </CardHeader>
      <CardContent>
        <div className="max-h-[380px] overflow-auto rounded-lg border border-border/60 bg-zinc-950 p-3 font-mono text-xs text-zinc-100">
          {filteredLogs.map((entry, index) => (
            <div key={`${entry.time}-${index}`} className="mb-1 break-all">
              <span className="text-zinc-400">[{entry.time}]</span>{' '}
              <span className={entry.level.includes('error') ? 'text-rose-300' : entry.level.includes('warn') ? 'text-amber-300' : 'text-emerald-300'}>
                {entry.level.toUpperCase()}
              </span>{' '}
              <span>{entry.message}</span>
              {entry.attrs && Object.keys(entry.attrs).length > 0 ? (
                <>
                  {' '}
                  <span className="text-zinc-300">{JSON.stringify(entry.attrs)}</span>
                </>
              ) : null}
            </div>
          ))}
          {filteredLogs.length === 0 ? <p className="text-zinc-400">No logs yet.</p> : null}
        </div>
      </CardContent>
    </Card>
  )
}
