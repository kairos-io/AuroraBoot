import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";

interface StatusBadgeProps {
  status: string;
  className?: string;
}

const statusStyles: Record<string, string> = {
  online: "bg-green-500/15 text-green-700 border-green-500/25 dark:text-green-400",
  offline: "bg-red-500/15 text-red-700 border-red-500/25 dark:text-red-400",
  pending: "bg-yellow-500/15 text-yellow-700 border-yellow-500/25 dark:text-yellow-400",
  building: "bg-[#EE5007]/15 text-[#EE5007] border-[#EE5007]/25",
  running: "bg-[#FF7442]/15 text-[#FF7442] border-[#FF7442]/25",
  completed: "bg-green-500/15 text-green-700 border-green-500/25 dark:text-green-400",
  ready: "bg-green-500/15 text-green-700 border-green-500/25 dark:text-green-400",
  failed: "bg-red-500/15 text-red-700 border-red-500/25 dark:text-red-400",
  error: "bg-red-500/15 text-red-700 border-red-500/25 dark:text-red-400",
  active: "bg-green-500/15 text-green-700 border-green-500/25 dark:text-green-400",
  upgrading: "bg-[#FF7442]/15 text-[#FF7442] border-[#FF7442]/25",
};

export function StatusBadge({ status, className }: StatusBadgeProps) {
  const style = statusStyles[status.toLowerCase()] ?? "bg-secondary text-secondary-foreground";
  return (
    <Badge variant="outline" className={cn(style, className)}>
      {status}
    </Badge>
  );
}
