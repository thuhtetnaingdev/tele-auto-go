import { ShieldCheck, UserCircle2 } from 'lucide-react'

import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import type { AdminSession } from '@/features/control/types'

type AdminLoginViewProps = {
  adminSession: AdminSession
  adminUsername: string
  adminPassword: string
  busy: boolean
  message: string
  onAdminUsernameChange: (value: string) => void
  onAdminPasswordChange: (value: string) => void
  onLogin: () => Promise<void>
}

export function AdminLoginView({
  adminSession,
  adminUsername,
  adminPassword,
  busy,
  message,
  onAdminUsernameChange,
  onAdminPasswordChange,
  onLogin,
}: AdminLoginViewProps) {
  return (
    <main className="min-h-screen bg-background px-4 py-10 text-foreground">
      <div className="mx-auto max-w-md space-y-4">
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <UserCircle2 className="size-5" /> Dashboard Login
            </CardTitle>
            <CardDescription>
              {adminSession.configured
                ? 'Sign in with admin username and password configured during installation.'
                : 'Admin login is not configured on server. Set ADMIN_* values and restart backend.'}
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="admin-username">Username</Label>
              <Input id="admin-username" value={adminUsername} onChange={(event) => onAdminUsernameChange(event.target.value)} />
            </div>
            <div className="space-y-2">
              <Label htmlFor="admin-password">Password</Label>
              <Input
                id="admin-password"
                type="password"
                value={adminPassword}
                onChange={(event) => onAdminPasswordChange(event.target.value)}
                onKeyDown={(event) => {
                  if (event.key === 'Enter') {
                    void onLogin()
                  }
                }}
              />
            </div>
            <Button onClick={() => void onLogin()} disabled={busy || !adminSession.configured} className="w-full">
              <ShieldCheck className="size-4" /> Sign In
            </Button>
            {message ? <p className="text-sm text-destructive">{message}</p> : null}
          </CardContent>
        </Card>
      </div>
    </main>
  )
}
