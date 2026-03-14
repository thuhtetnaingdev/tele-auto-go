import { Save } from 'lucide-react'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import type { AgentDefinition } from '@/features/control/types'

type AgentsCardProps = {
  busy: boolean
  agents: AgentDefinition[]
  editingAgentID: string
  allowUsersInput: string
  agentForm: AgentDefinition
  onAgentFormChange: (next: AgentDefinition) => void
  onAllowUsersInputChange: (value: string) => void
  onSaveAgent: () => Promise<void>
  onResetAgentForm: () => void
  onStartEditAgent: (agent: AgentDefinition) => void
  onDeleteAgent: (id: string) => Promise<void>
}

export function AgentsCard({
  busy,
  agents,
  editingAgentID,
  allowUsersInput,
  agentForm,
  onAgentFormChange,
  onAllowUsersInputChange,
  onSaveAgent,
  onResetAgentForm,
  onStartEditAgent,
  onDeleteAgent,
}: AgentsCardProps) {
  return (
    <Card>
      <CardHeader>
        <CardTitle>Agents</CardTitle>
        <CardDescription>Create Markdown-based agents with frontmatter metadata.</CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
          <Input
            placeholder="id (e.g. price_agent)"
            value={agentForm.id}
            disabled={Boolean(editingAgentID)}
            onChange={(event) => onAgentFormChange({ ...agentForm, id: event.target.value })}
            className="h-9"
          />
          <Input
            placeholder="name"
            value={agentForm.name}
            onChange={(event) => onAgentFormChange({ ...agentForm, name: event.target.value })}
            className="h-9"
          />
          <Input
            placeholder="description"
            value={agentForm.description}
            onChange={(event) => onAgentFormChange({ ...agentForm, description: event.target.value })}
            className="h-9 sm:col-span-2"
          />
          <div className="space-y-1 sm:col-span-2">
            <Label>Visibility</Label>
            <select
              className="h-9 w-full rounded-md border border-input bg-background px-3 text-sm"
              value={agentForm.visibility || 'public'}
              onChange={(event) =>
                onAgentFormChange({
                  ...agentForm,
                  visibility: event.target.value === 'private' ? 'private' : 'public',
                })
              }
            >
              <option value="public">public</option>
              <option value="private">private</option>
            </select>
          </div>
          {agentForm.visibility === 'private' ? (
            <div className="space-y-1 sm:col-span-2">
              <Label>Allow Users</Label>
              <Textarea
                placeholder="User IDs or usernames (@name), comma or newline separated"
                value={allowUsersInput}
                onChange={(event) => onAllowUsersInputChange(event.target.value)}
                className="min-h-[76px] font-mono text-xs"
              />
            </div>
          ) : null}
          <Input
            placeholder="intents: price,order,info"
            value={agentForm.intents.join(',')}
            onChange={(event) =>
              onAgentFormChange({
                ...agentForm,
                intents: event.target.value.split(','),
              })
            }
            className="h-9 sm:col-span-2"
          />
          <Input
            placeholder="variables: API_TOKEN,BASE_URL"
            value={agentForm.variables.join(',')}
            onChange={(event) =>
              onAgentFormChange({
                ...agentForm,
                variables: event.target.value.split(','),
              })
            }
            className="h-9 sm:col-span-2"
          />
        </div>
        <Textarea
          placeholder="Agent markdown body instructions..."
          value={agentForm.body}
          onChange={(event) => onAgentFormChange({ ...agentForm, body: event.target.value })}
          className="min-h-[130px] font-mono text-xs sm:min-h-[160px]"
        />
        <div className="flex flex-col gap-2 sm:flex-row sm:flex-wrap">
          <Button size="sm" className="w-full sm:w-auto" onClick={() => void onSaveAgent()} disabled={busy}>
            <Save className="size-4" /> {editingAgentID ? 'Update Agent' : 'Create Agent'}
          </Button>
          <Button size="sm" variant="outline" className="w-full sm:w-auto" onClick={onResetAgentForm} disabled={busy}>
            Clear
          </Button>
        </div>
        <div className="max-h-52 overflow-auto rounded-lg border border-border/60 bg-background p-2 text-sm">
          {agents.map((agent) => (
            <div key={agent.id} className="flex flex-col gap-2 border-b border-border/40 py-1 last:border-b-0 sm:flex-row sm:items-center sm:justify-between">
              <div className="min-w-0">
                <p className="break-all font-medium">{agent.id}</p>
                <p className="break-words text-xs text-muted-foreground">{agent.description || agent.name}</p>
                <div className="mt-1 flex flex-wrap gap-1">
                  <Badge variant={(agent.visibility || 'public') === 'private' ? 'secondary' : 'outline'}>
                    {(agent.visibility || 'public') === 'private' ? 'Private' : 'Public'}
                  </Badge>
                  {(agent.visibility || 'public') === 'private' ? (
                    <Badge variant="outline">{(agent.allowUsers || []).length} allow users</Badge>
                  ) : null}
                </div>
              </div>
              <div className="flex w-full gap-1 sm:w-auto">
                <Button size="sm" variant="outline" className="flex-1 sm:flex-none" onClick={() => onStartEditAgent(agent)}>Edit</Button>
                <Button size="sm" variant="destructive" className="flex-1 sm:flex-none" onClick={() => void onDeleteAgent(agent.id)}>Delete</Button>
              </div>
            </div>
          ))}
          {agents.length === 0 ? <p className="text-xs text-muted-foreground">No agents yet.</p> : null}
        </div>
      </CardContent>
    </Card>
  )
}
