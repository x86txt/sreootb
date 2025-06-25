import { type ClassValue, clsx } from "clsx";
import { twMerge } from "tailwind-merge";

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

export function formatDate(date: string | Date) {
  return new Intl.DateTimeFormat("en-US", {
    year: "numeric",
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  }).format(new Date(date));
}

export function formatResponseTime(time: number | null | undefined) {
  if (time === null || time === undefined) return "N/A";
  
  // Handle unit conversion: backend may send seconds (old monitor) or milliseconds (agent-based)
  // Values >= 1 are likely seconds, values < 1 could be either seconds or milliseconds
  // We'll convert based on magnitude - if < 1 and seems too small, treat as seconds
  let milliseconds: number;
  
  if (time >= 1) {
    // Definitely seconds (e.g., 1.234 seconds)
    milliseconds = time * 1000;
  } else if (time > 0.001) {
    // Likely seconds (e.g., 0.066 seconds = 66ms)
    milliseconds = time * 1000;
  } else {
    // Likely already in milliseconds or very fast response
    milliseconds = time;
  }
  
  if (milliseconds < 1000) {
    return `${Math.round(milliseconds)}ms`;
  }
  return `${(milliseconds / 1000).toFixed(2)}s`;
}

export function getStatusColor(status: string | null | undefined) {
  switch (status) {
    case "up":
      return "text-green-600 bg-green-50 border-green-200";
    case "down":
      return "text-red-600 bg-red-50 border-red-200";
    default:
      return "text-gray-600 bg-gray-50 border-gray-200";
  }
}

export function getStatusDot(status: string | null | undefined) {
  switch (status) {
    case "up":
      return "bg-green-500";
    case "down":
      return "bg-red-500";
    default:
      return "bg-gray-400";
  }
}

// Color utility functions for dynamic theming
export function applyAccentColor(hexColor: string) {
  const hsl = hexToHsl(hexColor);
  const hslString = `${hsl.h} ${Math.round(hsl.s * 100)}% ${Math.round(hsl.l * 100)}%`;
  
  // Apply to CSS custom properties
  document.documentElement.style.setProperty('--primary', hslString);
  document.documentElement.style.setProperty('--ring', hslString);
  document.documentElement.style.setProperty('--destructive', hslString);
  document.documentElement.style.setProperty('--accent', hslString);
  
  // For lighter/darker variants, adjust lightness
  const lighterHsl = { ...hsl, l: Math.min(hsl.l + 0.1, 0.95) };
  const darkerHsl = { ...hsl, l: Math.max(hsl.l - 0.1, 0.05) };
  
  const lighterHslString = `${lighterHsl.h} ${Math.round(lighterHsl.s * 100)}% ${Math.round(lighterHsl.l * 100)}%`;
  const darkerHslString = `${darkerHsl.h} ${Math.round(darkerHsl.s * 100)}% ${Math.round(darkerHsl.l * 100)}%`;
  
  // Apply variants
  document.documentElement.style.setProperty('--primary-foreground', hsl.l > 0.5 ? '0 0% 0%' : '0 0% 100%');
  document.documentElement.style.setProperty('--destructive-foreground', '0 0% 98%');
  document.documentElement.style.setProperty('--accent-foreground', hsl.l > 0.5 ? '0 0% 0%' : '0 0% 100%');
  
  // Convert hex to RGB for shadow effects
  const rgb = hexToRgb(hexColor);
  
  // Update custom shadows with the new accent color
  const cardShadowLight = `-3px -3px 8px 1px rgba(0,0,0,0.1), 3px 3px 8px 1px rgba(${rgb.r},${rgb.g},${rgb.b},0.12)`;
  const cardShadowDark = `-3px -3px 8px 1px rgba(0,0,0,0.3), 3px 3px 8px 1px rgba(${rgb.r},${rgb.g},${rgb.b},0.18)`;
  
  // Apply the updated shadows to the document
  const style = document.createElement('style');
  style.textContent = `
    .shadow-card {
      box-shadow: ${cardShadowLight} !important;
    }
    .shadow-card-dark {
      box-shadow: ${cardShadowDark} !important;
    }
  `;
  
  // Remove any existing accent color styles
  const existing = document.querySelector('#accent-color-styles');
  if (existing) {
    existing.remove();
  }
  
  style.id = 'accent-color-styles';
  document.head.appendChild(style);
}

export function hexToHsl(hex: string): { h: number; s: number; l: number } {
  // Remove # if present
  hex = hex.replace('#', '');
  
  // Convert hex to RGB
  const r = parseInt(hex.substring(0, 2), 16) / 255;
  const g = parseInt(hex.substring(2, 4), 16) / 255;
  const b = parseInt(hex.substring(4, 6), 16) / 255;
  
  // Convert RGB to HSL
  const max = Math.max(r, g, b);
  const min = Math.min(r, g, b);
  let h: number, s: number, l: number;
  
  l = (max + min) / 2;
  
  if (max === min) {
    h = s = 0; // achromatic
  } else {
    const d = max - min;
    s = l > 0.5 ? d / (2 - max - min) : d / (max + min);
    
    switch (max) {
      case r: h = (g - b) / d + (g < b ? 6 : 0); break;
      case g: h = (b - r) / d + 2; break;
      case b: h = (r - g) / d + 4; break;
      default: h = 0;
    }
    h /= 6;
  }
  
  return {
    h: Math.round(h * 360),
    s: Math.round(s * 100) / 100,
    l: Math.round(l * 100) / 100
  };
}

export function hexToRgb(hex: string): { r: number; g: number; b: number } {
  // Remove # if present
  hex = hex.replace('#', '');
  
  // Convert hex to RGB
  const r = parseInt(hex.substring(0, 2), 16);
  const g = parseInt(hex.substring(2, 4), 16);
  const b = parseInt(hex.substring(4, 6), 16);
  
  return { r, g, b };
} 