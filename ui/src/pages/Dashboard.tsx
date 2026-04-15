import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { PageHeader } from "@/components/PageHeader";
import { GetStartedHero } from "@/components/GetStartedHero";
import { listNodes, type Node } from "@/api/nodes";
import { listGroups } from "@/api/groups";
import { listArtifacts, type Artifact } from "@/api/artifacts";
import { listDeployments } from "@/api/deployments";
import {
  Loader2,
  Plus,
  Download,
  Server,
  Package,
  AlertTriangle,
  Activity,
  CheckCircle2,
  XCircle,
  Rocket,
  ArrowRight,
} from "lucide-react";

function timeAgo(dateStr: string): string {
  const seconds = Math.floor((Date.now() - new Date(dateStr).getTime()) / 1000);
  if (seconds < 60) return `${seconds}s ago`;
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  return `${days}d ago`;
}

interface ActivityItem {
  id: string;
  type: "artifact" | "node";
  label: string;
  status: string;
  time: string;
  link: string;
}

function artifactStatusBadge(phase: string) {
  switch (phase.toLowerCase()) {
    case "ready":
      return <Badge className="bg-green-600 text-white border-0">Ready</Badge>;
    case "building":
      return <Badge className="bg-[#EE5007] text-white border-0">Building</Badge>;
    case "error":
    case "failed":
      return <Badge variant="destructive">Failed</Badge>;
    default:
      return <Badge variant="secondary">{phase}</Badge>;
  }
}

function nodeStatusBadge(phase: string) {
  if (phase === "Online") {
    return <Badge className="bg-green-600 text-white border-0">Online</Badge>;
  }
  return <Badge variant="destructive">Offline</Badge>;
}

export function Dashboard() {
  const navigate = useNavigate();
  const [nodes, setNodes] = useState<Node[]>([]);
  const [artifacts, setArtifacts] = useState<Artifact[]>([]);
  const [groupCount, setGroupCount] = useState(0);
  const [activeDeployments, setActiveDeployments] = useState(0);
  // We must not render anything until BOTH nodes and artifacts have been
  // fetched at least once. Otherwise the initial empty arrays cause a
  // split-second zero-state wizard flash right before the real dashboard
  // paints, which looks glitchy on every page load.
  const [loaded, setLoaded] = useState(false);

  useEffect(() => {
    let nodesDone = false;
    let artifactsDone = false;
    const markLoaded = () => {
      if (nodesDone && artifactsDone) setLoaded(true);
    };
    listNodes()
      .then((v) => {
        setNodes(v);
      })
      .catch(() => {})
      .finally(() => {
        nodesDone = true;
        markLoaded();
      });
    listArtifacts()
      .then((v) => {
        setArtifacts(v);
      })
      .catch(() => {})
      .finally(() => {
        artifactsDone = true;
        markLoaded();
      });
    // Non-gating background loads — wizard decision doesn't depend on them.
    listGroups()
      .then((groups) => setGroupCount(groups.length))
      .catch(() => {});
    listDeployments()
      .then((deployments) => {
        setActiveDeployments(
          deployments.filter(
            (d) =>
              d.status.toLowerCase() === "active" ||
              d.status.toLowerCase() === "running"
          ).length
        );
      })
      .catch(() => {});
  }, []);

  const onlineNodes = nodes.filter((n) => n.phase === "Online").length;
  const offlineNodes = nodes.filter((n) => n.phase !== "Online");
  const activeBuilds = artifacts.filter((a) => a.phase === "building");

  // First-run and partial-run states. We only show the full welcome wizard
  // when the instance is completely empty — no nodes, no artifacts, no
  // history at all. Once the user has built something OR registered a node,
  // the normal dashboard takes over (but a partial-state banner nudges them
  // to the next step if only half the journey is done).
  const isZeroState = nodes.length === 0 && artifacts.length === 0;
  const hasArtifactsButNoNodes = nodes.length === 0 && artifacts.length > 0;
  const readyArtifact = artifacts.find((a) => a.phase === "Ready");

  // Build recent activity feed
  const activityItems: ActivityItem[] = [
    ...artifacts.slice(0, 20).map((a) => ({
      id: `artifact-${a.id}`,
      type: "artifact" as const,
      label: a.name || a.baseImage || "Untitled artifact",
      status: a.phase,
      time: a.createdAt,
      link: `/artifacts/${a.id}`,
    })),
    ...nodes.slice(0, 20).map((n) => ({
      id: `node-${n.id}`,
      type: "node" as const,
      label: n.hostname || n.machineID,
      status: n.phase,
      time: n.createdAt,
      link: `/nodes/${n.id}`,
    })),
  ]
    .sort((a, b) => new Date(b.time).getTime() - new Date(a.time).getTime())
    .slice(0, 10);

  // Hold the whole page until we actually know what the state is — prevents
  // the zero-state wizard from flashing on every dashboard paint.
  if (!loaded) {
    return (
      <div className="flex items-center justify-center py-24 text-muted-foreground">
        <Loader2 className="h-5 w-5 animate-spin" />
      </div>
    );
  }

  // Zero state takes over the entire Dashboard body — no page header,
  // no status strip, no stat counters. Just the wizard.
  if (isZeroState) {
    return <GetStartedHero />;
  }

  return (
    <div>
      <PageHeader title="Overview" description="Your Kairos node fleet at a glance" />

      {/* Partial-state banner: closes the loop for users who've built an
          artifact but haven't deployed it or imported any existing nodes
          yet. Only rendered when there's at least one artifact and zero
          nodes. */}
      {hasArtifactsButNoNodes && (
        <div className="mb-8 rounded-xl border border-[#EE5007]/30 bg-[#EE5007]/5 p-5 animate-fade-up">
          <div className="flex items-start gap-4">
            <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-full bg-[#EE5007]/15 text-[#EE5007]">
              <Rocket className="h-5 w-5" />
            </div>
            <div className="flex-1 min-w-0">
              <h3 className="font-semibold text-sm">
                {artifacts.length === 1
                  ? "You've built your first artifact."
                  : `You've built ${artifacts.length} artifacts.`}{" "}
                Ready to put {artifacts.length === 1 ? "it" : "one"} on a machine?
              </h3>
              <p className="text-sm text-muted-foreground mt-1">
                Flash the ISO to a USB stick, serve it over netboot, or point a
                Redfish BMC at the image — nodes auto-register on first boot.
              </p>
              <div className="mt-4 flex flex-wrap gap-2">
                {readyArtifact ? (
                  <Button
                    size="sm"
                    className="bg-[#EE5007] hover:bg-[#FF7442] text-white"
                    onClick={() => navigate(`/artifacts/${readyArtifact.id}`)}
                  >
                    Deploy "{readyArtifact.name || readyArtifact.baseImage}"
                    <ArrowRight className="h-4 w-4 ml-2" />
                  </Button>
                ) : (
                  <Button
                    size="sm"
                    className="bg-[#EE5007] hover:bg-[#FF7442] text-white"
                    onClick={() => navigate("/artifacts")}
                  >
                    View artifacts
                    <ArrowRight className="h-4 w-4 ml-2" />
                  </Button>
                )}
                <Button size="sm" variant="outline" onClick={() => navigate("/import")}>
                  <Download className="h-4 w-4 mr-2" />
                  Or import existing nodes
                </Button>
              </div>
            </div>
          </div>
        </div>
      )}

      {/* Status strip */}
      <div className="flex flex-wrap items-center gap-6 text-sm mb-8">
        <span className="flex items-center gap-2">
          <span className="h-2 w-2 rounded-full bg-green-500" />
          <span className="font-medium">{onlineNodes}</span>
          <span className="text-muted-foreground">online</span>
        </span>
        <span className="flex items-center gap-2">
          <span className="h-2 w-2 rounded-full bg-red-500" />
          <span className="font-medium">{offlineNodes.length}</span>
          <span className="text-muted-foreground">offline</span>
        </span>
        <span className="text-muted-foreground">
          {groupCount} groups
        </span>
        {activeBuilds.length > 0 && (
          <span className="flex items-center gap-2 text-[#EE5007]">
            <Loader2 className="h-3 w-3 animate-spin" />
            {activeBuilds.length} building
          </span>
        )}
        {activeDeployments > 0 && (
          <span className="flex items-center gap-2 text-[#FF7442]">
            <Activity className="h-3 w-3" />
            {activeDeployments} deploying
          </span>
        )}
      </div>

      {/* Quick actions */}
      <div className="flex flex-wrap gap-3 mb-8">
        <Button
          size="sm"
          className="bg-[#EE5007] hover:bg-[#FF7442] text-white"
          onClick={() => navigate("/artifacts/new")}
        >
          <Plus className="h-4 w-4 mr-2" /> Build Artifact
        </Button>
        <Button
          size="sm"
          variant="outline"
          onClick={() => navigate("/import")}
        >
          <Download className="h-4 w-4 mr-2" /> Import Node
        </Button>
        <Button
          size="sm"
          variant="outline"
          onClick={() => navigate("/nodes")}
        >
          <Server className="h-4 w-4 mr-2" /> View Nodes
        </Button>
      </div>

      {/* Two-column layout */}
      <div className="grid gap-6 lg:grid-cols-5">
        {/* Left column: Recent Activity */}
        <div className="lg:col-span-3">
          <h2 className="text-lg font-semibold mb-4">Recent Activity</h2>
          {activityItems.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              No recent activity yet. Build an artifact or import a node to get started.
            </p>
          ) : (
            <div className="space-y-1">
              {activityItems.map((item) => (
                <button
                  key={item.id}
                  className="w-full flex items-center gap-3 rounded-lg px-3 py-2.5 text-left text-sm hover:bg-muted/50 transition-colors"
                  onClick={() => navigate(item.link)}
                >
                  {item.type === "artifact" ? (
                    <Package className="h-4 w-4 shrink-0 text-[#EE5007]" />
                  ) : (
                    <Server className="h-4 w-4 shrink-0 text-[#03153A] dark:text-slate-300" />
                  )}
                  <span className="truncate flex-1">
                    {item.type === "artifact" ? "Artifact built" : "Node registered"}:{" "}
                    <span className="font-medium">{item.label}</span>
                  </span>
                  {item.type === "artifact"
                    ? artifactStatusBadge(item.status)
                    : nodeStatusBadge(item.status)}
                  <span className="text-xs text-muted-foreground whitespace-nowrap">
                    {timeAgo(item.time)}
                  </span>
                </button>
              ))}
            </div>
          )}
        </div>

        {/* Right column: Active Builds + Offline Nodes */}
        <div className="lg:col-span-2 space-y-6">
          {/* Active Builds */}
          {activeBuilds.length > 0 ? (
            <Card>
              <CardHeader className="pb-3">
                <CardTitle className="text-sm font-semibold flex items-center gap-2">
                  <Loader2 className="h-4 w-4 animate-spin text-[#EE5007]" />
                  Active Builds
                </CardTitle>
              </CardHeader>
              <CardContent className="space-y-3">
                {activeBuilds.map((build) => (
                  <button
                    key={build.id}
                    className="w-full flex items-center gap-3 text-left text-sm hover:bg-muted/50 rounded-md px-2 py-1.5 transition-colors"
                    onClick={() => navigate(`/artifacts/${build.id}`)}
                  >
                    <Package className="h-4 w-4 shrink-0 text-[#EE5007]" />
                    <div className="flex-1 min-w-0">
                      <p className="font-medium truncate">
                        {build.name || build.baseImage || "Untitled"}
                      </p>
                      <p className="text-xs text-muted-foreground">
                        Started {timeAgo(build.createdAt)}
                      </p>
                    </div>
                    <Loader2 className="h-3 w-3 animate-spin text-[#EE5007]" />
                  </button>
                ))}
              </CardContent>
            </Card>
          ) : (
            <div className="text-sm text-muted-foreground flex items-center gap-2 px-1">
              <CheckCircle2 className="h-4 w-4" />
              No active builds
            </div>
          )}

          {/* Offline Nodes */}
          {offlineNodes.length > 0 && (
            <Card className="border-red-200 dark:border-red-900">
              <CardHeader className="pb-3">
                <CardTitle className="text-sm font-semibold flex items-center gap-2 text-red-600 dark:text-red-400">
                  <AlertTriangle className="h-4 w-4" />
                  Offline Nodes
                </CardTitle>
              </CardHeader>
              <CardContent className="space-y-2">
                {offlineNodes.map((node) => (
                  <button
                    key={node.id}
                    className="w-full flex items-center justify-between text-left text-sm hover:bg-muted/50 rounded-md px-2 py-1.5 transition-colors"
                    onClick={() => navigate(`/nodes/${node.id}`)}
                  >
                    <span className="flex items-center gap-2">
                      <XCircle className="h-3.5 w-3.5 text-red-500" />
                      <span className="font-medium">
                        {node.hostname || node.machineID}
                      </span>
                    </span>
                    <span className="text-xs text-muted-foreground">
                      last seen{" "}
                      {node.lastHeartbeat ? timeAgo(node.lastHeartbeat) : "never"}
                    </span>
                  </button>
                ))}
              </CardContent>
            </Card>
          )}
        </div>
      </div>
    </div>
  );
}
