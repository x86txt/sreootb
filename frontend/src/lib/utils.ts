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
  if (time < 1) return `${Math.round(time * 1000)}ms`;
  return `${time.toFixed(2)}s`;
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