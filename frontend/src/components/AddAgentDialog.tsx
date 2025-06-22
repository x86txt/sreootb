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
import { Plus, AlertCircle, CheckCircle, Copy, Eye, EyeOff, Info } from "lucide-react"
import { useAuth } from "@/context/AuthContext"

interface AddAgentDialogProps {
  onAgentAdded?: () => void
}

export function AddAgentDialog({ onAgentAdded }: AddAgentDialogProps) {
  const [open, setOpen] = useState(false)
  const [name, setName] = useState("")
  const [description, setDescription] = useState("")
  const [apiKey, setApiKey] = useState("")
  const [isSubmitting, setIsSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [success, setSuccess] = useState(false)
  const [serverAPIKey, setServerAPIKey] = useState<string>("")
  const [serverURL, setServerURL] = useState<string>("")
  const [agentPort, setAgentPort] = useState<string>("8081")
  const [showAPIKey, setShowAPIKey] = useState(false)
  const [showServerKey, setShowServerKey] = useState(false)
  const [copied, setCopied] = useState(false)
  const [useServerKey, setUseServerKey] = useState(true)
  const { isAuthenticated } = useAuth()

  // Fetch server's agent API key when dialog opens
  useEffect(() => {
    if (open && isAuthenticated) {
      fetchServerAPIKey()
    }
  }, [open, isAuthenticated])

  const fetchServerAPIKey = async () => {
    try {
      const response = await fetch("/api/agents/api-key")
      if (response.ok) {
        const data = await response.json()
        setServerAPIKey(data.api_key)
        setServerURL(data.server_url || 'https://your-server')
        setAgentPort(data.agent_port || '8081')
        // Pre-fill the API key field with server key by default
        setApiKey(data.api_key)
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

  const handleUseServerKey = () => {
    setUseServerKey(true)
    setApiKey(serverAPIKey)
  }

  const handleUseCustomKey = () => {
    setUseServerKey(false)
    setApiKey("")
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
    setShowServerKey(false)
    setCopied(false)
    setUseServerKey(true)
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
      <DialogContent className="sm:max-w-[600px]">
        <DialogHeader>
          <DialogTitle>Add New Agent</DialogTitle>
          <DialogDescription>
            Register a new monitoring agent with the system. You can use the server's shared API key or provide a custom one.
          </DialogDescription>
        </DialogHeader>

        {success ? (
          <div className="flex items-center justify-center py-8">
            <div className="text-center">
              <CheckCircle className="mx-auto h-12 w-12 text-green-500" />
              <p className="mt-2 text-sm text-muted-foreground">Agent registered successfully!</p>
            </div>
          </div>
        ) : (
          <form onSubmit={handleSubmit}>
            <div className="grid gap-4 py-4">
              {/* Server API Key Reference */}
              <div className="grid gap-2">
                <Label>Server's Agent API Key</Label>
                <div className="flex items-center space-x-2">
                  <Input
                    type={showServerKey ? "text" : "password"}
                    value={serverAPIKey}
                    readOnly
                    className="font-mono text-sm bg-muted"
                  />
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    onClick={() => setShowServerKey(!showServerKey)}
                  >
                    {showServerKey ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
                  </Button>
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    onClick={() => copyToClipboard(serverAPIKey)}
                  >
                    <Copy className="h-4 w-4" />
                  </Button>
                </div>
                {copied && (
                  <p className="text-xs text-green-600">✓ Copied to clipboard!</p>
                )}
                <div className="flex items-start gap-2 p-3 bg-muted/50 rounded-md border">
                  <Info className="h-4 w-4 text-muted-foreground mt-0.5 flex-shrink-0" />
                  <div className="text-xs text-foreground">
                    <p className="font-medium mb-1">Agent Command:</p>
                    <code className="bg-muted px-2 py-1 rounded text-xs block font-mono">
                      ./sreootb agent --agent-id "your-agent-name" --api-key "{serverAPIKey}" --server-url "{serverURL.replace(/:\d+$/, '')}:{agentPort}"
                    </code>
                  </div>
                </div>
              </div>

              {/* API Key Selection */}
              <div className="grid gap-2">
                <Label>API Key Configuration</Label>
                <div className="flex gap-2">
                  <Button
                    type="button"
                    variant={useServerKey ? "default" : "outline"}
                    size="sm"
                    onClick={handleUseServerKey}
                  >
                    Use Server Key
                  </Button>
                  <Button
                    type="button"
                    variant={!useServerKey ? "default" : "outline"}
                    size="sm"
                    onClick={handleUseCustomKey}
                  >
                    Custom Key
                  </Button>
                </div>
              </div>

              {/* Agent API Key Input */}
              <div className="grid gap-2">
                <Label htmlFor="apiKey">Agent API Key</Label>
                <div className="flex items-center space-x-2">
                  <Input
                    id="apiKey"
                    type={showAPIKey ? "text" : "password"}
                    placeholder={useServerKey ? "Using server's API key" : "Enter custom API key (min 64 characters)"}
                    value={apiKey}
                    onChange={(e) => setApiKey(e.target.value)}
                    required
                    className="font-mono text-sm"
                    readOnly={useServerKey}
                  />
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    onClick={() => setShowAPIKey(!showAPIKey)}
                  >
                    {showAPIKey ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
                  </Button>
                </div>
                <p className="text-xs text-muted-foreground">
                  {useServerKey 
                    ? "Using the server's shared API key. All agents with this key can connect to the server."
                    : "Enter a custom API key (minimum 64 characters). This allows for agent-specific authentication."
                  }
                </p>
              </div>

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
                {isSubmitting ? "Registering..." : "Register Agent"}
              </Button>
            </DialogFooter>
          </form>
        )}
      </DialogContent>
    </Dialog>
  )
} 