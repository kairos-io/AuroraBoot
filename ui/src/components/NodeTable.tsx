import { useNavigate } from "react-router";
import type { Node } from "@/api/nodes";
import { StatusBadge } from "@/components/StatusBadge";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Server, Plus } from "lucide-react";
import { timeAgo } from "@/lib/time";

interface NodeTableProps {
  nodes: Node[];
  emptyAction?: () => void;
}


export function NodeTable({ nodes, emptyAction }: NodeTableProps) {
  const navigate = useNavigate();

  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>Hostname</TableHead>
          <TableHead>Group</TableHead>
          <TableHead>Labels</TableHead>
          <TableHead>Phase</TableHead>
          <TableHead>OS Version</TableHead>
          <TableHead>IP</TableHead>
          <TableHead>Last Heartbeat</TableHead>
          <TableHead>Status</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {nodes.length === 0 ? (
          <TableRow>
            <TableCell colSpan={8} className="text-center py-12">
              <div className="flex flex-col items-center gap-3 py-16">
                <Server className="h-12 w-12 text-muted-foreground/30" />
                <div className="text-center">
                  <p className="font-medium">No nodes registered</p>
                  <p className="text-sm text-muted-foreground mt-1">
                    Import your first node to start managing your fleet.
                  </p>
                </div>
                {emptyAction && (
                  <Button className="mt-2" onClick={emptyAction}>
                    <Plus className="h-4 w-4 mr-2" /> Import First Node
                  </Button>
                )}
              </div>
            </TableCell>
          </TableRow>
        ) : (
          nodes.map((node) => (
            <TableRow
              key={node.id}
              className="cursor-pointer hover:bg-primary-soft"
              onClick={() => navigate(`/nodes/${node.id}`)}
            >
              <TableCell className="font-medium">{node.hostname}</TableCell>
              <TableCell>{node.group?.name || node.groupID || "-"}</TableCell>
              <TableCell>
                <div className="flex flex-wrap gap-1">
                  {Object.entries(node.labels || {}).map(([k, v]) => (
                    <Badge key={k} variant="secondary" className="text-xs">
                      {k}={v}
                    </Badge>
                  ))}
                </div>
              </TableCell>
              <TableCell>{node.phase}</TableCell>
              <TableCell className="text-xs">{node.agentVersion || "-"}</TableCell>
              <TableCell className="font-mono text-xs">{node.remoteIP || "-"}</TableCell>
              <TableCell className="text-xs">
                {timeAgo(node.lastHeartbeat)}
              </TableCell>
              <TableCell>
                <StatusBadge status={node.phase} />
              </TableCell>
            </TableRow>
          ))
        )}
      </TableBody>
    </Table>
  );
}
