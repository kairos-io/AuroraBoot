import { useEffect, useState } from "react";
import { PageHeader } from "@/components/PageHeader";
import { StatusBadge } from "@/components/StatusBadge";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Wifi, Server, Rocket } from "lucide-react";
import { listDeployments, type Deployment } from "@/api/deployments";

export function Deployments() {
  const [deployments, setDeployments] = useState<Deployment[]>([]);
  const [loading, setLoading] = useState(true);
  const [search, setSearch] = useState("");

  function fetchDeployments() {
    listDeployments()
      .then(setDeployments)
      .catch(() => {})
      .finally(() => setLoading(false));
  }

  useEffect(() => {
    fetchDeployments();
  }, []);

  // Auto-poll every 5s when any deployment is active
  useEffect(() => {
    const hasActive = deployments.some(
      (d) => d.status.toLowerCase() === "active" || d.status.toLowerCase() === "running"
    );
    if (!hasActive) return;
    const interval = setInterval(fetchDeployments, 5000);
    return () => clearInterval(interval);
  }, [deployments]);

  function methodIcon(method: string) {
    if (method.toLowerCase() === "pxe" || method.toLowerCase() === "netboot") {
      return <Wifi className="h-4 w-4 text-[#EE5007]" />;
    }
    return <Server className="h-4 w-4 text-[#FF7442]" />;
  }

  const filtered = deployments.filter(
    (d) => !search || d.artifactId.toLowerCase().includes(search.toLowerCase())
  );

  return (
    <div>
      <PageHeader
        title="Deployments"
        description="Track artifact deployments to bare-metal nodes"
      />

      <div className="flex items-center gap-4 mb-4">
        <Input placeholder="Search by artifact ID..." value={search} onChange={e => setSearch(e.target.value)} className="max-w-sm" />
      </div>

      <Card>
        <CardContent className="p-0">
          {loading ? (
            <div className="flex items-center justify-center py-16 text-muted-foreground">
              Loading...
            </div>
          ) : filtered.length === 0 ? (
            <div className="flex flex-col items-center justify-center py-16 text-muted-foreground">
              {search ? (
                <>
                  <Server className="h-10 w-10 mb-3 opacity-40" />
                  <p>No matching deployments</p>
                  <p className="text-xs mt-1">Try adjusting your search query.</p>
                </>
              ) : (
                <>
                  <Rocket className="h-12 w-12 mb-3 text-muted-foreground/30" />
                  <div className="text-center">
                    <p className="font-medium text-foreground">No deployments yet</p>
                    <p className="text-sm mt-1">
                      Deploy an artifact to hardware from the artifact detail page.
                    </p>
                  </div>
                </>
              )}
            </div>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className="w-12">Method</TableHead>
                  <TableHead>Artifact</TableHead>
                  <TableHead>Target</TableHead>
                  <TableHead>Status</TableHead>
                  <TableHead>Progress</TableHead>
                  <TableHead>Started</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {filtered.map((d) => (
                  <TableRow key={d.id}>
                    <TableCell>{methodIcon(d.method)}</TableCell>
                    <TableCell className="font-mono text-xs">
                      {d.artifactId.slice(0, 12)}
                    </TableCell>
                    <TableCell className="font-mono text-xs">
                      {d.bmcTargetId ? d.bmcTargetId.slice(0, 12) : "-"}
                    </TableCell>
                    <TableCell>
                      <StatusBadge status={d.status} />
                    </TableCell>
                    <TableCell>
                      {d.progress > 0 ? (
                        <div className="flex items-center gap-2">
                          <div className="h-2 w-24 rounded-full bg-secondary overflow-hidden">
                            <div
                              className="h-full rounded-full bg-[#EE5007] transition-all"
                              style={{ width: `${Math.min(d.progress, 100)}%` }}
                            />
                          </div>
                          <span className="text-xs text-muted-foreground">
                            {d.progress}%
                          </span>
                        </div>
                      ) : (
                        <span className="text-xs text-muted-foreground">-</span>
                      )}
                    </TableCell>
                    <TableCell className="text-xs text-muted-foreground">
                      {d.startedAt
                        ? new Date(d.startedAt).toLocaleString()
                        : "-"}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
