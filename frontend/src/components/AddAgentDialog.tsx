"use client"

import { useState, useEffect } from "react"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Plus, AlertCircle, CheckCircle, Copy, Eye, EyeOff, Info, RefreshCw, Users, UserPlus } from "lucide-react"
import { useAuth } from "@/context/AuthContext"

interface AddAgentDialogProps {
  onAgentAdded?: () => void
}

// Generate a cryptographically secure API key
const generateSecureAPIKey = (): string => {
  const array = new Uint8Array(32);
  crypto.getRandomValues(array);
  return Array.from(array, byte => byte.toString(16).padStart(2, '0')).join('');
};

export function AddAgentDialog({ onAgentAdded }: AddAgentDialogProps) {
  const [open, setOpen] = useState(false)
  const [name, setName] = useState("")
  const [description, setDescription] = useState("")
  const [apiKey, setApiKey] = useState("")
  const [isSubmitting, setIsSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [success, setSuccess] = useState(false)
  const [bootstrapAPIKey, setBootstrapAPIKey] = useState<string>("")
  const [serverURL, setServerURL] = useState<string>("")
  const [agentPort, setAgentPort] = useState<string>("8081")
  const [showAPIKey, setShowAPIKey] = useState(false)
  const [showBootstrapKey, setShowBootstrapKey] = useState(false)
  const [copied, setCopied] = useState(false)
  const [registrationType, setRegistrationType] = useState<'manual' | 'auto'>('manual')
  const { isAuthenticated } = useAuth()

  // Generate a unique API key when dialog opens for manual registration
  useEffect(() => {
    if (open && isAuthenticated) {
      if (registrationType === 'manual') {
        // Generate new unique key for manual registration
        const newKey = generateSecureAPIKey();
        setApiKey(newKey);
      }
      fetchServerInfo();
    }
  }, [open, isAuthenticated, registrationType])

  const fetchServerInfo = async () => {
    try {
      const response = await fetch("/api/agents/api-key")
      if (response.ok) {
        const data = await response.json()
        setBootstrapAPIKey(data.api_key)
        setServerURL(data.server_url || 'https://your-server')
        setAgentPort(data.agent_port || '8081')
        
        // For auto-registration, use bootstrap key
        if (registrationType === 'auto') {
          setApiKey(data.api_key)
        }
      }
    } catch (err) {
      console.error("Failed to fetch server agent API key:", err)
    }
  }

  const copyToClipboard = async (text: string) => {
    try {
      await navigator.clipboard.writeText(text)
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    } catch (err) {
      console.error("Failed to copy to clipboard:", err)
    }
  }

  const generateNewKey = () => {
    const newKey = generateSecureAPIKey();
    setApiKey(newKey);
  }

  const handleRegistrationTypeChange = (type: 'manual' | 'auto') => {
    setRegistrationType(type)
    if (type === 'manual') {
      generateNewKey()
    } else {
      setApiKey(bootstrapAPIKey)
    }
  }

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setIsSubmitting(true)
    setError(null)

    try {
      const response = await fetch("/api/agents", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify({
          name: name.trim(),
          api_key: apiKey.trim(),
          description: description.trim() || null,
          registration_type: registrationType,
        }),
      })

      if (!response.ok) {
        const errorText = await response.text()
        throw new Error(errorText || "Failed to add agent")
      }

      setSuccess(true)
      setTimeout(() => {
        setOpen(false)
        resetForm()
        onAgentAdded?.()
      }, 1500)
    } catch (err) {
      setError(err instanceof Error ? err.message : "An error occurred")
    } finally {
      setIsSubmitting(false)
    }
  }

  const resetForm = () => {
    setName("")
    setDescription("")
    setApiKey("")
    setError(null)
    setSuccess(false)
    setShowAPIKey(false)
    setShowBootstrapKey(false)
    setCopied(false)
    setRegistrationType('manual')
  }

  const handleOpenChange = (newOpen: boolean) => {
    setOpen(newOpen)
    if (!newOpen) {
      resetForm()
    }
  }

  // Don't render if not authenticated
  if (!isAuthenticated) {
    return null
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger asChild>
        <Button>
          <Plus className="mr-2 h-4 w-4" />
          Add Agent
        </Button>
      </DialogTrigger>
      <DialogContent className="sm:max-w-[700px]">
        <DialogHeader>
          <DialogTitle>Add New Agent</DialogTitle>
          <DialogDescription>
            Choose between manual registration with a unique key or auto-registration with the bootstrap key.
          </DialogDescription>
        </DialogHeader>

        {success ? (
          <div className="flex items-center justify-center py-8">
            <div className="text-center">
              <CheckCircle className="mx-auto h-12 w-12 text-green-500" />
              <p className="mt-2 text-sm text-muted-foreground">Agent registered successfully!</p>
              {registrationType === 'manual' && (
                <p className="mt-1 text-xs text-muted-foreground">
                  Use the generated API key to connect your agent.
                </p>
              )}
            </div>
          </div>
        ) : (
          <form onSubmit={handleSubmit}>
            <div className="grid gap-4 py-4">
              {/* Registration Type Selection */}
              <div className="grid gap-2">
                <Label>Registration Type</Label>
                <div className="flex gap-2">
                  <Button
                    type="button"
                    variant={registrationType === 'manual' ? "default" : "outline"}
                    size="sm"
                    onClick={() => handleRegistrationTypeChange('manual')}
                    className="flex items-center gap-2"
                  >
                    <UserPlus className="h-4 w-4" />
                    Manual Registration
                  </Button>
                  <Button
                    type="button"
                    variant={registrationType === 'auto' ? "default" : "outline"}
                    size="sm"
                    onClick={() => handleRegistrationTypeChange('auto')}
                    className="flex items-center gap-2"
                  >
                    <Users className="h-4 w-4" />
                    Auto-Registration
                  </Button>
                </div>
                <p className="text-xs text-muted-foreground">
                  {registrationType === 'manual' 
                    ? "Generate a unique API key for this specific agent. Most secure option." 
                    : "Use the shared bootstrap key for agent auto-registration. Agents will upgrade to permanent keys automatically."
                  }
                </p>
              </div>

              {/* Agent API Key */}
              <div className="grid gap-2">
                <div className="flex items-center gap-2">
                  <Label htmlFor="apiKey">
                    {registrationType === 'manual' ? 'Unique Agent API Key' : 'Bootstrap API Key'}
                  </Label>
                  {registrationType === 'manual' && (
                    <Button
                      type="button"
                      variant="outline"
                      size="sm"
                      onClick={generateNewKey}
                      className="h-6 px-2"
                    >
                      <RefreshCw className="h-3 w-3" />
                    </Button>
                  )}
                </div>
                <div className="flex items-center space-x-2">
                  <Input
                    id="apiKey"
                    type={showAPIKey ? "text" : "password"}
                    value={apiKey}
                    readOnly
                    className="font-mono text-sm bg-muted"
                  />
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    onClick={() => setShowAPIKey(!showAPIKey)}
                  >
                    {showAPIKey ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
                  </Button>
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    onClick={() => copyToClipboard(apiKey)}
                  >
                    <Copy className="h-4 w-4" />
                  </Button>
                </div>
                {copied && (
                  <p className="text-xs text-green-600">✓ Copied to clipboard!</p>
                )}
              </div>

              {/* Agent Command */}
              <div className="grid gap-2">
                <Label>Agent Command</Label>
                <div className="flex items-start gap-2 p-3 bg-muted/50 rounded-md border">
                  <Info className="h-4 w-4 text-muted-foreground mt-0.5 flex-shrink-0" />
                  <div className="text-xs text-foreground">
                    <p className="font-medium mb-1">
                      {registrationType === 'manual' 
                        ? 'Start your agent with this unique key:' 
                        : 'Start your agent with the bootstrap key (will auto-upgrade):'
                      }
                    </p>
                    <code className="bg-muted px-2 py-1 rounded text-xs block font-mono break-all">
                      ./sreootb agent --agent-id "{name || 'your-agent-name'}" --api-key "{apiKey}" --server-url "{serverURL.replace(/:\d+$/, '')}:{agentPort}"
                    </code>
                    {registrationType === 'auto' && (
                      <p className="text-muted-foreground mt-2 text-xs">
                        💡 The agent will automatically negotiate a permanent key and restart with it.
                      </p>
                    )}
                  </div>
                </div>
              </div>

              {/* Bootstrap Key Reference (only show in auto mode) */}
              {registrationType === 'auto' && bootstrapAPIKey && (
                <div className="grid gap-2">
                  <Label>Bootstrap Key Reference</Label>
                  <div className="flex items-center space-x-2">
                    <Input
                      type={showBootstrapKey ? "text" : "password"}
                      value={bootstrapAPIKey}
                      readOnly
                      className="font-mono text-sm bg-muted/30"
                    />
                    <Button
                      type="button"
                      variant="outline"
                      size="sm"
                      onClick={() => setShowBootstrapKey(!showBootstrapKey)}
                    >
                      {showBootstrapKey ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
                    </Button>
                  </div>
                  <p className="text-xs text-muted-foreground">
                    This is the server's shared bootstrap key. Agents using this key will automatically upgrade to permanent keys.
                  </p>
                </div>
              )}

              <div className="grid gap-2">
                <Label htmlFor="name">Agent Name</Label>
                <Input
                  id="name"
                  placeholder="e.g., Production Monitor, EU West Agent"
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  required
                />
              </div>

              <div className="grid gap-2">
                <Label htmlFor="description">Description (Optional)</Label>
                <Input
                  id="description"
                  placeholder="Brief description of this agent's purpose"
                  value={description}
                  onChange={(e) => setDescription(e.target.value)}
                />
              </div>

              {error && (
                <div className="flex items-center gap-2 text-sm text-red-600">
                  <AlertCircle className="h-4 w-4" />
                  {error}
                </div>
              )}
            </div>

            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={() => setOpen(false)}
                disabled={isSubmitting}
              >
                Cancel
              </Button>
              <Button type="submit" disabled={isSubmitting || !name.trim() || !apiKey.trim()}>
                {isSubmitting ? "Registering..." : `Register ${registrationType === 'manual' ? 'Unique' : 'Bootstrap'} Agent`}
              </Button>
            </DialogFooter>
          </form>
        )}
      </DialogContent>
    </Dialog>
  )
} 