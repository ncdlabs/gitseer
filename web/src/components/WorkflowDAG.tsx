import { useMemo, memo } from "react";
import {
  Background,
  BackgroundVariant,
  Controls,
  Handle,
  MarkerType,
  Position,
  ReactFlow,
  type Edge,
  type Node,
  type NodeProps,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import type { WorkflowNode } from "../api/client";
import { useIsTerminalTheme } from "../hooks/useIsTerminalTheme";

type Props = {
  graph: WorkflowNode[];
  jobStatusByName?: Record<string, string>;
};

type JobNodeData = {
  label: string;
  status: string;
  unknownDeps: boolean;
};

const START_ID = "__lens_start__";
const COL_GAP = 200;
const ROW_GAP = 110;
const NODE_W = 160;

function resolveStatus(node: WorkflowNode, statusByName: Record<string, string>): string {
  return statusByName[node.name] || statusByName[node.job_key] || "unknown";
}

function statusTone(status: string): string {
  const s = status.toLowerCase();
  if (s === "failure" || s === "failed" || s === "fail" || s === "cancelled" || s === "canceled" || s === "timed_out") {
    return "danger";
  }
  if (s === "success" || s === "ok" || s === "completed" || s === "pass" || s === "skipped" || s === "neutral") {
    return "ok";
  }
  if (s === "running" || s === "in_progress") return "accent";
  if (s === "pending" || s === "queued" || s === "waiting" || s === "action_required") return "warn";
  return "muted";
}

function computeDepths(graph: WorkflowNode[]): Map<string, number> {
  const byKey = new Map(graph.map((n) => [n.job_key, n]));
  const depth = new Map<string, number>();
  function depthOf(key: string, stack: Set<string>): number {
    if (depth.has(key)) return depth.get(key)!;
    if (stack.has(key)) return 0;
    stack.add(key);
    const node = byKey.get(key);
    let d = 0;
    for (const n of node?.needs || []) {
      if (!byKey.has(n)) continue;
      d = Math.max(d, depthOf(n, stack) + 1);
    }
    stack.delete(key);
    depth.set(key, d);
    return d;
  }
  for (const n of graph) depthOf(n.job_key, new Set());
  return depth;
}

function asciiDAG(graph: WorkflowNode[], statusByName: Record<string, string>): string {
  const byKey = new Map(graph.map((n) => [n.job_key, n]));
  const depth = computeDepths(graph);
  const columns = new Map<number, WorkflowNode[]>();
  for (const n of graph) {
    const d = depth.get(n.job_key) || 0;
    const list = columns.get(d) || [];
    list.push(n);
    columns.set(d, list);
  }

  const maxDepth = Math.max(0, ...depth.values());
  const lines: string[] = ["[start]"];
  for (let d = 0; d <= maxDepth; d++) {
    lines.push("  |");
    lines.push("  v");
    const nodes = columns.get(d) || [];
    const row = nodes
      .map((n) => {
        const status = resolveStatus(n, statusByName);
        return `[${n.name}|${status}]`;
      })
      .join("  ");
    lines.push(row || "(empty)");
  }

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
    lines.push("// deps");
    lines.push(...edgeLines);
  }
  return lines.join("\n");
}

const JobNode = memo(function JobNode({ data }: NodeProps) {
  const d = data as JobNodeData;
  const tone = statusTone(d.status);
  return (
    <div className={`dag-node dag-node--${tone}`} data-status={d.status}>
      <Handle type="target" position={Position.Top} className="dag-node__handle" />
      <div className="dag-node__body">
        <span className="dag-node__name" title={d.label}>
          {d.label}
        </span>
        <span className={`badge ${d.status}`}>{d.status}</span>
      </div>
      {d.unknownDeps ? <span className="dag-node__hint muted">unknown deps</span> : null}
      <Handle type="source" position={Position.Bottom} className="dag-node__handle" />
    </div>
  );
});

const StartNode = memo(function StartNode() {
  return (
    <div className="dag-node dag-node--start">
      <div className="dag-node__body">
        <span className="dag-node__name">Start</span>
      </div>
      <Handle type="source" position={Position.Bottom} className="dag-node__handle" />
    </div>
  );
});

const nodeTypes = { job: JobNode, start: StartNode };

function flowEdge(id: string, source: string, target: string): Edge {
  return {
    id,
    source,
    target,
    type: "smoothstep",
    markerEnd: { type: MarkerType.ArrowClosed, width: 16, height: 16, color: "var(--dag-edge)" },
    style: { stroke: "var(--dag-edge)", strokeWidth: 1.5 },
  };
}

function layoutFlowchart(
  graph: WorkflowNode[],
  statusByName: Record<string, string>,
): { nodes: Node[]; edges: Edge[] } {
  const byKey = new Map(graph.map((n) => [n.job_key, n]));
  const depth = computeDepths(graph);
  const roots = graph.filter((n) => !(n.needs || []).some((need) => byKey.has(need)));

  const rows = new Map<number, string[]>();
  for (const n of graph) {
    // +1 so Start occupies row 0
    const d = (depth.get(n.job_key) || 0) + 1;
    const list = rows.get(d) || [];
    list.push(n.job_key);
    rows.set(d, list);
  }
  for (const keys of rows.values()) {
    keys.sort((a, b) => {
      const na = byKey.get(a)?.name || a;
      const nb = byKey.get(b)?.name || b;
      return na.localeCompare(nb);
    });
  }

  const maxCols = Math.max(1, ...[...rows.values()].map((k) => k.length));
  const canvasW = Math.max(maxCols, 1) * COL_GAP;

  const nodes: Node[] = [
    {
      id: START_ID,
      type: "start",
      position: { x: canvasW / 2 - NODE_W / 2, y: 0 },
      data: {},
      sourcePosition: Position.Bottom,
      targetPosition: Position.Top,
    },
  ];

  for (const [row, keys] of rows) {
    const rowWidth = keys.length * COL_GAP;
    const originX = (canvasW - rowWidth) / 2 + COL_GAP / 2 - NODE_W / 2;
    keys.forEach((key, i) => {
      const n = byKey.get(key)!;
      const status = resolveStatus(n, statusByName);
      nodes.push({
        id: key,
        type: "job",
        position: { x: originX + i * COL_GAP, y: row * ROW_GAP },
        data: {
          label: n.name,
          status,
          unknownDeps: !!n.unknown_deps,
        } satisfies JobNodeData,
        sourcePosition: Position.Bottom,
        targetPosition: Position.Top,
      });
    });
  }

  const edges: Edge[] = [];
  for (const root of roots) {
    edges.push(flowEdge(`${START_ID}->${root.job_key}`, START_ID, root.job_key));
  }
  for (const n of graph) {
    for (const need of n.needs || []) {
      if (!byKey.has(need)) continue;
      edges.push(flowEdge(`${need}->${n.job_key}`, need, n.job_key));
    }
  }

  return { nodes, edges };
}

export function WorkflowDAG({ graph, jobStatusByName = {} }: Props) {
  const terminal = useIsTerminalTheme();
  const { nodes, edges } = useMemo(
    () => layoutFlowchart(graph, jobStatusByName),
    [graph, jobStatusByName],
  );

  if (!graph.length) {
    return <p className="muted">Graph unavailable — could not load workflow YAML for this run.</p>;
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
      <ReactFlow
        nodes={nodes}
        edges={edges}
        nodeTypes={nodeTypes}
        fitView
        fitViewOptions={{ padding: 0.22, maxZoom: 1.2 }}
        minZoom={0.35}
        maxZoom={1.6}
        proOptions={{ hideAttribution: true }}
        nodesDraggable={false}
        nodesConnectable={false}
        elementsSelectable={false}
        panOnScroll
        zoomOnScroll
        preventScrolling={false}
      >
        <Background variant={BackgroundVariant.Dots} gap={18} size={1} color="var(--dag-dot)" />
        <Controls showInteractive={false} className="dag-controls" />
      </ReactFlow>
      <details className="dag-fallback-wrap">
        <summary>Accessible list fallback</summary>
        <ul className="dag-fallback">
          {graph.map((node) => (
            <li key={node.job_key}>
              <strong>{node.name}</strong>
              {node.needs?.length ? (
                <span className="muted"> needs [{node.needs.join(", ")}]</span>
              ) : (
                <span className="muted"> (root)</span>
              )}
            </li>
          ))}
        </ul>
      </details>
    </div>
  );
}
