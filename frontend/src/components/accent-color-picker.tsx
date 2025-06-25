'use client';

import * as React from "react";
import { Palette, Check } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { applyAccentColor } from "@/lib/utils";

const PRESET_COLORS = [
  { name: "Rose", value: "#E11D48", description: "Default rose" },
  { name: "Blue", value: "#3B82F6", description: "Professional blue" },
  { name: "Green", value: "#10B981", description: "Success green" },
  { name: "Purple", value: "#8B5CF6", description: "Creative purple" },
  { name: "Orange", value: "#F59E0B", description: "Energetic orange" },
  { name: "Yellow", value: "#EAB308", description: "Bright yellow" },
  { name: "Red", value: "#EF4444", description: "Alert red" },
  { name: "Indigo", value: "#6366F1", description: "Deep indigo" },
  { name: "Pink", value: "#EC4899", description: "Vibrant pink" },
  { name: "Teal", value: "#14B8A6", description: "Calming teal" },
  { name: "Cyan", value: "#06B6D4", description: "Bright cyan" },
];

export function AccentColorPicker() {
  const [currentColor, setCurrentColor] = React.useState("#E11D48");
  const [customColor, setCustomColor] = React.useState("");
  const [isOpen, setIsOpen] = React.useState(false);
  const [previewColor, setPreviewColor] = React.useState<string | null>(null);

  // Load saved color on mount
  React.useEffect(() => {
    if (typeof window !== 'undefined') {
      const saved = localStorage.getItem('sreootb-accent-color');
      if (saved) {
        setCurrentColor(saved);
        applyAccentColor(saved);
      }
    }
  }, []);

  // Apply preview color
  React.useEffect(() => {
    if (previewColor) {
      applyAccentColor(previewColor);
    }
  }, [previewColor]);

  // Reset to current color when preview ends
  const handleMouseLeave = () => {
    setPreviewColor(null);
    applyAccentColor(currentColor);
  };

  const handleColorSelect = (color: string) => {
    setCurrentColor(color);
    setCustomColor(color);
    applyAccentColor(color);
    
    // Save to localStorage
    if (typeof window !== 'undefined') {
      localStorage.setItem('sreootb-accent-color', color);
    }
    
    setIsOpen(false);
  };

  const handleCustomColorChange = (value: string) => {
    setCustomColor(value);
  };

  const handleCustomColorSubmit = () => {
    if (isValidColor(customColor)) {
      handleColorSelect(customColor);
    }
  };

  const handleCustomColorKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter') {
      handleCustomColorSubmit();
    }
  };

  const isValidColor = (color: string): boolean => {
    // Check for hex format
    if (/^#([A-Fa-f0-9]{6}|[A-Fa-f0-9]{3})$/.test(color)) {
      return true;
    }
    
    // Check for rgb/rgba format
    if (/^rgba?\(\s*(\d+)\s*,\s*(\d+)\s*,\s*(\d+)\s*(,\s*[0-9.]+)?\s*\)$/i.test(color)) {
      return true;
    }
    
    // Check for hsl/hsla format
    if (/^hsla?\(\s*(\d+)\s*,\s*(\d+%)\s*,\s*(\d+%)\s*(,\s*[0-9.]+)?\s*\)$/i.test(color)) {
      return true;
    }
    
    return false;
  };

  const convertColorToHex = (color: string): string => {
    // If already hex, return as is
    if (color.startsWith('#')) {
      return color;
    }
    
    // For other formats, create a temporary element to get computed color
    const tempDiv = document.createElement('div');
    tempDiv.style.color = color;
    document.body.appendChild(tempDiv);
    const computedColor = window.getComputedStyle(tempDiv).color;
    document.body.removeChild(tempDiv);
    
    // Convert rgb to hex
    const match = computedColor.match(/rgb\((\d+),\s*(\d+),\s*(\d+)\)/);
    if (match) {
      const r = parseInt(match[1]);
      const g = parseInt(match[2]);
      const b = parseInt(match[3]);
      return `#${((1 << 24) + (r << 16) + (g << 8) + b).toString(16).slice(1)}`;
    }
    
    return color;
  };

  return (
    <Popover open={isOpen} onOpenChange={setIsOpen}>
      <PopoverTrigger asChild>
        <Button
          variant="ghost"
          size="sm"
          className="h-8 w-8 px-0 relative"
          title="Change accent color"
        >
          <Palette className="h-[1.2rem] w-[1.2rem]" />
          <div 
            className="absolute -bottom-0.5 -right-0.5 w-3 h-3 rounded-full border-2 border-background"
            style={{ backgroundColor: currentColor }}
          />
          <span className="sr-only">Change accent color</span>
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-80 p-4" align="end">
        <div className="space-y-4">
          <div>
            <h4 className="font-medium text-sm">Accent Color</h4>
            <p className="text-xs text-muted-foreground">
              Choose a color theme for the interface
            </p>
          </div>
          
          {/* Preset Colors */}
          <div className="space-y-3">
            <Label className="text-xs font-medium">Preset Colors</Label>
            <div className="grid grid-cols-5 gap-2">
              {PRESET_COLORS.map((color) => (
                <button
                  key={color.value}
                  className="group relative w-12 h-12 rounded-md border-2 border-muted hover:border-foreground transition-colors"
                  style={{ backgroundColor: color.value }}
                  onClick={() => handleColorSelect(color.value)}
                  onMouseEnter={() => setPreviewColor(color.value)}
                  onMouseLeave={handleMouseLeave}
                  title={`${color.name} - ${color.description}`}
                >
                  {currentColor === color.value && (
                    <Check className="absolute inset-0 m-auto h-4 w-4 text-white drop-shadow-sm" />
                  )}
                </button>
              ))}
            </div>
          </div>

          {/* Custom Color Input */}
          <div className="space-y-2">
            <Label htmlFor="custom-color" className="text-xs font-medium">
              Custom Color
            </Label>
            <div className="flex space-x-2">
              <div className="relative flex-1">
                <Input
                  id="custom-color"
                  placeholder="#3B82F6, rgb(59, 130, 246), hsl(217, 91%, 60%)"
                  value={customColor}
                  onChange={(e) => handleCustomColorChange(e.target.value)}
                  onKeyDown={handleCustomColorKeyDown}
                  className="text-xs pr-10"
                />
                {customColor && isValidColor(customColor) && (
                  <div 
                    className="absolute right-2 top-1/2 -translate-y-1/2 w-4 h-4 rounded border border-muted"
                    style={{ backgroundColor: customColor }}
                    onMouseEnter={() => setPreviewColor(customColor)}
                    onMouseLeave={handleMouseLeave}
                  />
                )}
              </div>
              <Button
                size="sm"
                onClick={handleCustomColorSubmit}
                disabled={!isValidColor(customColor)}
                className="px-3"
              >
                Apply
              </Button>
            </div>
            <p className="text-xs text-muted-foreground">
              Supports hex (#3B82F6), RGB, and HSL formats
            </p>
          </div>

          {/* Current Color Display */}
          <div className="pt-2 border-t">
            <div className="flex items-center justify-between text-xs">
              <span className="text-muted-foreground">Current:</span>
              <div className="flex items-center space-x-2">
                <div 
                  className="w-4 h-4 rounded border border-muted"
                  style={{ backgroundColor: currentColor }}
                />
                <code className="text-xs bg-muted px-1 py-0.5 rounded">
                  {currentColor}
                </code>
              </div>
            </div>
          </div>
        </div>
      </PopoverContent>
    </Popover>
  );
} 