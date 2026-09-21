import { useMemo } from "react";
import {
  Background,
  Controls,
  MarkerType,
  MiniMap,
  ReactFlow,
  type Edge,
  type Node,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import type { WorkflowNode } from "../api/client";
import { useIsTerminalTheme } from "../hooks/useIsTerminalTheme";

type Props = {
  graph: WorkflowNode[];
  jobStatusByName?: Record<string, string>;
};

function asciiDAG(graph: WorkflowNode[], statusByName: Record<string, string>): string {
  const byKey = new Map(graph.map((n) => [n.job_key, n]));
  const depth = new Map<string, number>();

  function depthOf(key: string, stack: Set<string>): number {
    if (depth.has(key)) return depth.get(key)!;
    if (stack.has(key)) return 0;
    stack.add(key);
    const node = byKey.get(key);
    let d = 0;
    for (const n of node?.needs || []) {
      d = Math.max(d, depthOf(n, stack) + 1);
    }
    stack.delete(key);
    depth.set(key, d);
    return d;
  }
  for (const n of graph) depthOf(n.job_key, new Set());

  const columns = new Map<number, WorkflowNode[]>();
  for (const n of graph) {
    const d = depth.get(n.job_key) || 0;
    const list = columns.get(d) || [];
    list.push(n);
    columns.set(d, list);
  }

  const maxDepth = Math.max(0, ...depth.values());
  const lines: string[] = [];
  for (let d = 0; d <= maxDepth; d++) {
    const nodes = columns.get(d) || [];
    const row = nodes
      .map((n) => {
        const status = statusByName[n.name] || statusByName[n.job_key] || "?";
        return `[${n.name}|${status}]`;
      })
      .join("  ");
    lines.push(row || "(empty)");
    if (d < maxDepth) {
      lines.push("      |");
      lines.push("      v");
    }
  }

  // Also list edges explicitly for clarity.
  const edgeLines: string[] = [];
  for (const n of graph) {
    for (const need of n.needs || []) {
      if (!byKey.has(need)) continue;
      const src = byKey.get(need)!;
      edgeLines.push(`${src.name} --> ${n.name}`);
    }
  }
  if (edgeLines.length) {
    lines.push("");
    lines.push("// edges");
    lines.push(...edgeLines);
  }
  return lines.join("\n");
}

export function WorkflowDAG({ graph, jobStatusByName = {} }: Props) {
  const terminal = useIsTerminalTheme();
  const { nodes, edges } = useMemo(() => layoutDAG(graph, jobStatusByName), [graph, jobStatusByName]);
  if (!graph.length) {
    return <p className="muted">Graph unavailable (workflow YAML not fetched or Actions path unknown).</p>;
  }

  if (terminal) {
    return (
      <div className="dag-host dag-host--ascii" role="img" aria-label="Workflow dependency graph">
        <pre className="dag-ascii mono">{asciiDAG(graph, jobStatusByName)}</pre>
      </div>
    );
  }

  return (
    <div className="dag-host" role="img" aria-label="Workflow dependency graph">
      <ReactFlow nodes={nodes} edges={edges} fitView proOptions={{ hideAttribution: true }} nodesDraggable={false}>
        <Background gap={16} />
        <MiniMap pannable zoomable />
        <Controls showInteractive={false} />
      </ReactFlow>
      <details style={{ marginTop: "0.75rem" }}>
        <summary>Accessible list fallback</summary>
        <ul className="dag-fallback">
          {graph.map((node) => (
            <li key={node.job_key}>
              <strong>{node.name}</strong>
              {node.needs?.length ? <span className="muted"> needs [{node.needs.join(", ")}]</span> : <span className="muted"> (root)</span>}
            </li>
          ))}
        </ul>
      </details>
    </div>
  );
}

function layoutDAG(graph: WorkflowNode[], statusByName: Record<string, string>): { nodes: Node[]; edges: Edge[] } {
  const depth = new Map<string, number>();
  const byKey = new Map(graph.map((n) => [n.job_key, n]));
  function depthOf(key: string, stack: Set<string>): number {
    if (depth.has(key)) return depth.get(key)!;
    if (stack.has(key)) return 0;
    stack.add(key);
    const node = byKey.get(key);
    let d = 0;
    for (const n of node?.needs || []) {
      d = Math.max(d, depthOf(n, stack) + 1);
    }
    stack.delete(key);
    depth.set(key, d);
    return d;
  }
  for (const n of graph) depthOf(n.job_key, new Set());

  const columns = new Map<number, string[]>();
  for (const n of graph) {
    const d = depth.get(n.job_key) || 0;
    const list = columns.get(d) || [];
    list.push(n.job_key);
    columns.set(d, list);
  }

  const nodes: Node[] = [];
  for (const [d, keys] of columns) {
    keys.forEach((key, i) => {
      const n = byKey.get(key)!;
      const status = statusByName[n.name] || statusByName[n.job_key] || "unknown";
      nodes.push({
        id: key,
        position: { x: d * 220, y: i * 90 },
        data: { label: `${n.name}\n${status}` },
        style: {
          borderRadius: 8,
          border: "1px solid var(--line)",
          background: "var(--bg-elevated)",
          color: "var(--ink)",
          fontSize: 12,
          padding: 8,
          whiteSpace: "pre-line",
          minWidth: 140,
        },
      });
    });
  }

  const edges: Edge[] = [];
  for (const n of graph) {
    for (const need of n.needs || []) {
      if (!byKey.has(need)) continue;
      edges.push({
        id: `${need}->${n.job_key}`,
        source: need,
        target: n.job_key,
        markerEnd: { type: MarkerType.ArrowClosed },
        style: { stroke: "var(--muted)" },
      });
    }
  }
  return { nodes, edges };
}
