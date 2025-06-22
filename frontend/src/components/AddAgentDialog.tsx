"use client"

import { useState } from "react"
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
import { Plus, AlertCircle, CheckCircle } from "lucide-react"
import { useAuth } from "@/context/AuthContext"

interface AddAgentDialogProps {
  onAgentAdded?: () => void
}

export function AddAgentDialog({ onAgentAdded }: AddAgentDialogProps) {
  const [open, setOpen] = useState(false)
  const [name, setName] = useState("")
  const [apiKey, setApiKey] = useState("")
  const [description, setDescription] = useState("")
  const [isSubmitting, setIsSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [success, setSuccess] = useState(false)
  const { isAuthenticated } = useAuth()

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
        const errorData = await response.json()
        throw new Error(errorData.detail || "Failed to add agent")
      }

      setSuccess(true)
      setTimeout(() => {
        setOpen(false)
        setName("")
        setApiKey("")
        setDescription("")
        setSuccess(false)
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
    setApiKey("")
    setDescription("")
    setError(null)
    setSuccess(false)
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
      <DialogContent className="sm:max-w-[425px]">
        <DialogHeader>
          <DialogTitle>Add New Agent</DialogTitle>
          <DialogDescription>
            Register a new monitoring agent with the system. The agent must generate its API key using{" "}
            <code className="bg-muted px-1 py-0.5 rounded text-sm">./sreoob-agent -genkey</code>
          </DialogDescription>
        </DialogHeader>

        {success ? (
          <div className="flex items-center justify-center py-8">
            <div className="text-center">
              <CheckCircle className="mx-auto h-12 w-12 text-green-500" />
              <p className="mt-2 text-sm text-muted-foreground">Agent added successfully!</p>
            </div>
          </div>
        ) : (
          <form onSubmit={handleSubmit}>
            <div className="grid gap-4 py-4">
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
                <Label htmlFor="apiKey">API Key</Label>
                <Input
                  id="apiKey"
                  type="password"
                  placeholder="Paste the 64-character API key here"
                  value={apiKey}
                  onChange={(e) => setApiKey(e.target.value)}
                  required
                  minLength={64}
                />
                <p className="text-xs text-muted-foreground">
                  Generate using: <code className="bg-muted px-1 py-0.5 rounded">./sreoob-agent -genkey</code>
                </p>
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
                {isSubmitting ? "Adding..." : "Add Agent"}
              </Button>
            </DialogFooter>
          </form>
        )}
      </DialogContent>
    </Dialog>
  )
} 