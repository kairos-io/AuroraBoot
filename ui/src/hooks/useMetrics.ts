import { useEffect, useState } from "react";
import { getLatestMetrics, getNodeMetrics, type NodeMetrics } from "@/api/metrics";

// Both hooks poll on an interval. A failed fetch keeps the last data, so a
// short network blip does not blank the gauges.

export function useNodeMetrics(
  id: string,
  intervalMs = 10000,
): { latest: NodeMetrics | null; samples: NodeMetrics[]; loading: boolean } {
  const [latest, setLatest] = useState<NodeMetrics | null>(null);
  const [samples, setSamples] = useState<NodeMetrics[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let cancelled = false;
    const load = () => {
      getNodeMetrics(id)
        .then((res) => {
          if (cancelled) return;
          setLatest(res.latest);
          setSamples(res.samples);
        })
        .catch(() => {
          /* keep the last data */
        })
        .finally(() => {
          if (!cancelled) setLoading(false);
        });
    };
    load();
    const timer = setInterval(load, intervalMs);
    return () => {
      cancelled = true;
      clearInterval(timer);
    };
  }, [id, intervalMs]);

  return { latest, samples, loading };
}

export function useLatestMetrics(
  intervalMs = 10000,
): { byNode: Record<string, NodeMetrics>; loading: boolean } {
  const [byNode, setByNode] = useState<Record<string, NodeMetrics>>({});
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let cancelled = false;
    const load = () => {
      getLatestMetrics()
        .then((res) => {
          if (!cancelled) setByNode(res);
        })
        .catch(() => {
          /* keep the last data */
        })
        .finally(() => {
          if (!cancelled) setLoading(false);
        });
    };
    load();
    const timer = setInterval(load, intervalMs);
    return () => {
      cancelled = true;
      clearInterval(timer);
    };
  }, [intervalMs]);

  return { byNode, loading };
}
